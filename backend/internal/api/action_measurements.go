package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

func registerActionMeasurementRoutes(mux *http.ServeMux, store InventoryStore) {
	backend, ok := store.(interface {
		ListTrackedActionsPage(context.Context, string, int, int) ([]repository.TrackedAction, int, error)
		ListActionMeasurementsPage(context.Context, string, int, int) ([]repository.ActionMeasurement, int, error)
		RecordActionMeasurement(context.Context, string, string, string, string, string, string, bool, string) (*repository.ActionMeasurement, error)
	})
	if !ok {
		return
	}
	mux.HandleFunc("GET /api/v1/environments/{id}/actions/export", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		format := r.URL.Query().Get("format")
		if !uuidPattern.MatchString(env) || (format != "csv" && format != "jsonl") {
			writeError(w, 400, CodeValidation, "Ambiente ou formato de exportação inválido.")
			return
		}
		first, total, err := backend.ListTrackedActionsPage(r.Context(), env, 50, 0)
		if err != nil {
			writeError(w, 500, CodeInternal, "Não foi possível iniciar a exportação.")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="acoes-%s.%s"`, env, format))
		if format == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		}
		csvWriter := csv.NewWriter(w)
		jsonWriter := json.NewEncoder(w)
		exportedActions, exportedMeasurements := 0, 0
		if format == "csv" {
			_ = csvWriter.Write([]string{"finding_id", "title", "status", "owner", "recurrences", "metric", "before", "after", "comparable", "comparison_note", "hypothesis", "window_note", "recorded_at"})
		}
		for offset, page := 0, first; offset < total && r.Context().Err() == nil; {
			if len(page) == 0 {
				return
			}
			pageSize := len(page)
			for _, item := range page {
				exportedActions++
				item.Measurements = nil // measurements are streamed as separate records
				if format == "jsonl" {
					if err := jsonWriter.Encode(map[string]any{"record_type": "action", "action": item}); err != nil {
						return
					}
				}
				measurementCount := 0
				for mOffset := 0; r.Context().Err() == nil; mOffset += 100 {
					measurements, count, err := backend.ListActionMeasurementsPage(r.Context(), item.FindingID, 100, mOffset)
					if err != nil {
						return
					}
					measurementCount = count
					for _, measurement := range measurements {
						exportedMeasurements++
						if format == "jsonl" {
							if err := jsonWriter.Encode(map[string]any{"record_type": "measurement", "measurement": measurement}); err != nil {
								return
							}
						} else if err := csvWriter.Write(actionCSVRow(item, &measurement)); err != nil {
							return
						}
					}
					if mOffset+len(measurements) >= count || len(measurements) == 0 {
						break
					}
				}
				if format == "csv" && measurementCount == 0 {
					if err := csvWriter.Write(actionCSVRow(item, nil)); err != nil {
						return
					}
				}
			}
			offset += pageSize
			if offset >= total {
				break
			}
			page, _, err = backend.ListTrackedActionsPage(r.Context(), env, 50, offset)
			if err != nil {
				return
			}
		}
		if r.Context().Err() != nil {
			return
		}
		if format == "jsonl" {
			_ = jsonWriter.Encode(map[string]any{"record_type": "complete", "actions": exportedActions, "measurements": exportedMeasurements})
		} else {
			footer := make([]string, 13)
			footer[0], footer[4], footer[5] = "# export_complete", fmt.Sprint(exportedActions), fmt.Sprint(exportedMeasurements)
			_ = csvWriter.Write(footer)
			csvWriter.Flush()
		}
	})
	mux.HandleFunc("GET /api/v1/environments/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		if !uuidPattern.MatchString(env) {
			writeError(w, 400, CodeValidation, "Ambiente inválido.")
			return
		}
		limit, offset, valid := parseListPage(r, 50)
		if !valid {
			writeError(w, 400, CodeValidation, "Paginação inválida.")
			return
		}
		items, total, err := backend.ListTrackedActionsPage(r.Context(), env, limit, offset)
		if err != nil {
			writeError(w, 500, CodeInternal, "Falha ao listar ações.")
			return
		}
		writePage(w, items, limit, offset, total)
	})
	mux.HandleFunc("GET /api/v1/findings/{id}/action/measurements", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeError(w, 400, CodeValidation, "Achado inválido.")
			return
		}
		limit, offset, valid := parseListPage(r, 50)
		if !valid {
			writeError(w, 400, CodeValidation, "Paginação inválida.")
			return
		}
		items, total, err := backend.ListActionMeasurementsPage(r.Context(), id, limit, offset)
		if err != nil {
			writeError(w, 500, CodeInternal, "Falha ao listar medições.")
			return
		}
		writePage(w, items, limit, offset, total)
	})
	mux.HandleFunc("POST /api/v1/findings/{id}/action/measurements", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			Before             string `json:"before_run_id"`
			After              string `json:"after_run_id"`
			Metric             string `json:"metric"`
			Hypothesis         string `json:"hypothesis"`
			Window             string `json:"window_note"`
			WorkloadComparable bool   `json:"workload_comparable"`
		}
		if !uuidPattern.MatchString(id) || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !uuidPattern.MatchString(body.Before) || !uuidPattern.MatchString(body.After) {
			writeError(w, 400, CodeValidation, "Execuções inválidas.")
			return
		}
		body.Hypothesis, body.Window = strings.TrimSpace(body.Hypothesis), strings.TrimSpace(body.Window)
		if (body.Metric != "table_size_bytes" && body.Metric != "finding_observed" && body.Metric != "query_mean_latency_us" && body.Metric != "query_reads_per_1000_calls") || len(body.Hypothesis) < 8 || len(body.Hypothesis) > 1000 || len(body.Window) < 4 || len(body.Window) > 500 {
			writeError(w, 400, CodeValidation, "Informe métrica, hipótese e janela de observação.")
			return
		}
		actor := "local"
		if user := requestIdentity(r); user != nil {
			actor = user.Username
		}
		item, err := backend.RecordActionMeasurement(r.Context(), id, body.Before, body.After, body.Metric, body.Hypothesis, body.Window, body.WorkloadComparable, actor)
		if errors.Is(err, repository.ErrMeasurementRun) || errors.Is(err, repository.ErrActionNotFound) {
			writeError(w, 422, CodeValidation, "Execuções ou achado incompatíveis.")
			return
		}
		if err != nil {
			writeError(w, 500, CodeInternal, "Falha ao registrar medição.")
			return
		}
		writeJSON(w, 201, item)
	})
}

func actionCSVRow(item repository.TrackedAction, measurement *repository.ActionMeasurement) []string {
	row := []string{safeCSVCell(item.FindingID), safeCSVCell(item.Title), safeCSVCell(item.Status), safeCSVCell(item.Owner), fmt.Sprint(item.Recurrences), "", "", "", "", "", "", "", ""}
	if measurement != nil {
		row[5] = safeCSVCell(measurement.Metric)
		if measurement.BeforeValue != nil {
			row[6] = fmt.Sprint(*measurement.BeforeValue)
		}
		if measurement.AfterValue != nil {
			row[7] = fmt.Sprint(*measurement.AfterValue)
		}
		row[8] = fmt.Sprint(measurement.Comparable)
		row[9] = safeCSVCell(measurement.ComparisonNote)
		row[10] = safeCSVCell(measurement.Hypothesis)
		row[11] = safeCSVCell(measurement.WindowNote)
		row[12] = measurement.RecordedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return row
}

func safeCSVCell(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}
