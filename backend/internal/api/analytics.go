package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

// DashboardKPIs is the executive summary for the main dashboard.
type DashboardKPIs struct {
	GeneratedAtUTC       time.Time `json:"generated_at_utc"`
	Environments         int       `json:"environments"`
	OpenFindings         int       `json:"open_findings"`
	CriticalFindings     int       `json:"critical_findings"`
	HighFindings         int       `json:"high_findings"`
	FailedRunsRecent     int       `json:"failed_runs_recent"`
	SuccessfulRunsRecent int       `json:"successful_runs_recent"`
	TotalStorageBytes    int64     `json:"total_storage_bytes"`
	Hypertables          int       `json:"hypertables"`
	JobsScheduled        int       `json:"jobs_scheduled"`
	Policies             int       `json:"policies"`
	Databases            *int      `json:"databases"`
	Schemas              *int      `json:"schemas"`
	Tables               *int      `json:"tables"`
	InventoryStatus      string    `json:"inventory_status"`
	Notes                []string  `json:"notes,omitempty"`
}

// StorageSeriesPoint is one point in storage evolution.
type StorageSeriesPoint struct {
	Label      string `json:"label"`
	SizeBytes  int64  `json:"size_bytes"`
	ObjectKind string `json:"object_kind"`
}

// StorageGrowthResponse groups storage by environment/database.
type StorageGrowthResponse struct {
	GeneratedAtUTC time.Time                  `json:"generated_at_utc"`
	ByEnvironment  []StorageSeriesPoint       `json:"by_environment"`
	ByDatabase     []StorageSeriesPoint       `json:"by_database"`
	TopConsumers   []StorageSeriesPoint       `json:"top_consumers"`
	Series         []repository.RunTrendPoint `json:"series"`
	Notes          []string                   `json:"notes,omitempty"`
}

// FindingTrendBucket aggregates findings by severity or status.
type FindingTrendBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// FindingsTrendResponse is the findings summary for the dashboard.
type FindingsTrendResponse struct {
	GeneratedAtUTC time.Time                  `json:"generated_at_utc"`
	BySeverity     []FindingTrendBucket       `json:"by_severity"`
	ByStatus       []FindingTrendBucket       `json:"by_status"`
	ByType         []FindingTrendBucket       `json:"by_type"`
	Total          int                        `json:"total"`
	Series         []repository.RunTrendPoint `json:"series"`
}

// JobHealthItem summarizes job/policy health for an environment.
type JobHealthItem struct {
	EnvironmentID   string `json:"environment_id"`
	EnvironmentName string `json:"environment_name,omitempty"`
	JobsTotal       int    `json:"jobs_total"`
	JobsScheduled   int    `json:"jobs_scheduled"`
	PoliciesTotal   int    `json:"policies_total"`
}

// JobHealthResponse lists job/policy health.
type JobHealthResponse struct {
	GeneratedAtUTC time.Time       `json:"generated_at_utc"`
	Items          []JobHealthItem `json:"items"`
}

// InventoryReportResponse is a compact inventory report.
type InventoryReportResponse struct {
	GeneratedAtUTC time.Time `json:"generated_at_utc"`
	Environments   int       `json:"environments"`
	Databases      int       `json:"databases"`
	Schemas        int       `json:"schemas"`
	Tables         int       `json:"tables"`
	Hypertables    int       `json:"hypertables"`
	Indexes        int       `json:"indexes"`
	Notes          []string  `json:"notes,omitempty"`
}

// FindingsReportResponse is a findings export-oriented summary.
type FindingsReportResponse struct {
	GeneratedAtUTC time.Time            `json:"generated_at_utc"`
	Total          int                  `json:"total"`
	BySeverity     []FindingTrendBucket `json:"by_severity"`
	ByStatus       []FindingTrendBucket `json:"by_status"`
	Items          []repository.Finding `json:"items"`
}

func registerAnalyticsRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/analytics/kpis", getKPIs(store))
	mux.HandleFunc("GET /api/v1/analytics/storage", getStorageGrowth(store))
	mux.HandleFunc("GET /api/v1/analytics/findings-trends", getFindingsTrends(store))
	mux.HandleFunc("GET /api/v1/analytics/job-health", getJobHealth(store))
	mux.HandleFunc("GET /api/v1/reports/inventory", getInventoryReport(store))
	mux.HandleFunc("GET /api/v1/reports/findings", getFindingsReport(store))
}

func envFilter(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("environment_id"))
}

func getKPIs(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		envID := envFilter(r)
		res := DashboardKPIs{GeneratedAtUTC: time.Now().UTC(), InventoryStatus: "empty"}

		envs, err := store.ListEnvironmentsAPI(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os ambientes.")
			return
		} else if envID != "" {
			found := false
			for _, environment := range envs {
				if environment.ID == envID {
					found = true
					break
				}
			}
			if !found {
				writeError(w, http.StatusNotFound, CodeNotFound, "Ambiente não encontrado.")
				return
			}
			res.Environments = 1
		} else {
			res.Environments = len(envs)
		}

		findings, err := store.CountFindings(ctx, envID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível contar os achados.")
			return
		} else {
			res.OpenFindings = findings.Open
			res.CriticalFindings = findings.Critical
			res.HighFindings = findings.High
		}

		successful, failed, err := store.CountRecentRuns(ctx, envID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível contar as execuções recentes.")
			return
		} else {
			res.SuccessfulRunsRecent = successful
			res.FailedRunsRecent = failed
		}

		for _, e := range envs {
			if envID != "" && e.ID != envID {
				continue
			}
			var counts *repository.InventoryCounts
			var capabilities repository.CapabilityAggregate
			if counter, ok := store.(interface {
				CountLatestEnvironmentSnapshot(context.Context, string) (*repository.LatestEnvironmentSnapshot, error)
			}); ok {
				snapshot, snapshotErr := counter.CountLatestEnvironmentSnapshot(ctx, e.ID)
				if snapshotErr != nil {
					writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível resumir o inventário do ambiente.")
					return
				}
				if snapshot != nil {
					counts = &snapshot.Inventory
					capabilities = snapshot.Capabilities
				}
			} else {
				if counter, ok := store.(interface {
					CountLatestInventory(context.Context, string) (*repository.InventoryCounts, error)
				}); ok {
					var countErr error
					counts, countErr = counter.CountLatestInventory(ctx, e.ID)
					if countErr != nil {
						res.Notes = append(res.Notes, "Não foi possível contar o inventário do ambiente "+e.ID)
					}
				}
				var capabilityErr error
				capabilities, capabilityErr = store.CountLatestCapabilities(ctx, e.ID)
				if capabilityErr != nil {
					writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível resumir as capacidades do ambiente.")
					return
				}
			}
			if counts == nil {
				res.InventoryStatus = "partial"
			} else {
				if res.Databases == nil {
					res.Databases, res.Schemas, res.Tables = new(int), new(int), new(int)
				}
				*res.Databases += counts.Databases
				*res.Schemas += counts.Schemas
				*res.Tables += counts.Tables
				if counts.Status != "complete" {
					res.InventoryStatus = "partial"
				} else if res.InventoryStatus == "empty" {
					res.InventoryStatus = "complete"
				}
			}
			res.TotalStorageBytes += capabilities.TotalStorageBytes
			res.Hypertables += capabilities.Hypertables
			res.JobsScheduled += capabilities.JobsScheduled
			res.Policies += capabilities.Policies
		}

		writeJSON(w, http.StatusOK, res)
	}
}

func getStorageGrowth(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		envID := envFilter(r)
		from, to, granularity, err := analyticsWindow(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, err.Error())
			return
		}
		res := StorageGrowthResponse{
			GeneratedAtUTC: time.Now().UTC(),
			ByEnvironment:  []StorageSeriesPoint{},
			ByDatabase:     []StorageSeriesPoint{},
			TopConsumers:   []StorageSeriesPoint{},
			Series:         []repository.RunTrendPoint{},
		}
		res.Series, err = store.ListStorageTrend(ctx, envID, from, to, granularity)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível consultar o histórico de armazenamento.")
			return
		}

		envs, err := store.ListEnvironmentsAPI(ctx)
		if err != nil {
			res.Notes = append(res.Notes, "Não foi possível listar os ambientes.")
			writeJSON(w, http.StatusOK, res)
			return
		}

		for _, e := range envs {
			if envID != "" && e.ID != envID {
				continue
			}
			var envTotal int64
			dbs, err := store.ListDatabaseSnapshots(ctx, e.ID)
			if err != nil {
				continue
			}
			for _, d := range dbs {
				envTotal += d.SizeBytes
				res.ByDatabase = append(res.ByDatabase, StorageSeriesPoint{
					Label:      e.Name + "/" + d.DatabaseName,
					SizeBytes:  d.SizeBytes,
					ObjectKind: "database",
				})
			}
			res.ByEnvironment = append(res.ByEnvironment, StorageSeriesPoint{
				Label:      e.Name,
				SizeBytes:  envTotal,
				ObjectKind: "environment",
			})

			tables, err := store.ListTopTableConsumers(ctx, e.ID, 15)
			if err == nil {
				for _, t := range tables {
					res.TopConsumers = append(res.TopConsumers, StorageSeriesPoint{
						Label:      e.Name + "/" + t.DatabaseName + "." + t.SchemaName + "." + t.TableName,
						SizeBytes:  t.SizeBytes,
						ObjectKind: "table",
					})
				}
			}
		}

		sort.Slice(res.TopConsumers, func(i, j int) bool {
			return res.TopConsumers[i].SizeBytes > res.TopConsumers[j].SizeBytes
		})
		if len(res.TopConsumers) > 15 {
			res.TopConsumers = res.TopConsumers[:15]
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func getFindingsTrends(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		envID := envFilter(r)
		from, to, granularity, err := analyticsWindow(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, err.Error())
			return
		}
		res := FindingsTrendResponse{
			GeneratedAtUTC: time.Now().UTC(),
			BySeverity:     []FindingTrendBucket{},
			ByStatus:       []FindingTrendBucket{},
			ByType:         []FindingTrendBucket{},
			Series:         []repository.RunTrendPoint{},
		}
		counts, err := store.CountFindings(ctx, envID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível contar os achados.")
			return
		}
		res.Series, err = store.ListFindingTrend(ctx, envID, from, to, granularity)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível consultar o histórico de achados.")
			return
		}
		res.Total = counts.Total
		res.BySeverity = bucketsFromMap(counts.BySeverity)
		res.ByStatus = bucketsFromMap(counts.ByStatus)
		res.ByType = bucketsFromMap(counts.ByType)
		writeJSON(w, http.StatusOK, res)
	}
}

func analyticsWindow(r *http.Request) (time.Time, time.Time, string, error) {
	now := time.Now().UTC()
	from, to := now.AddDate(0, 0, -90), now
	query := r.URL.Query()
	for _, field := range []struct {
		key string
		dst *time.Time
	}{{"from", &from}, {"to", &to}} {
		if raw := query.Get(field.key); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return time.Time{}, time.Time{}, "", fmt.Errorf("%s deve usar RFC3339", field.key)
			}
			*field.dst = parsed.UTC()
		}
	}
	granularity := query.Get("granularity")
	if granularity == "" {
		granularity = "day"
	}
	if granularity != "hour" && granularity != "day" && granularity != "week" && granularity != "month" {
		return time.Time{}, time.Time{}, "", fmt.Errorf("granularidade inválida")
	}
	if !from.Before(to) || to.Sub(from) > 366*24*time.Hour || (granularity == "hour" && to.Sub(from) > 7*24*time.Hour) {
		return time.Time{}, time.Time{}, "", fmt.Errorf("intervalo inválido ou maior que o limite")
	}
	return from, to, granularity, nil
}

func bucketsFromMap(m map[string]int) []FindingTrendBucket {
	out := make([]FindingTrendBucket, 0, len(m))
	for k, v := range m {
		out = append(out, FindingTrendBucket{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Key < out[j].Key
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func getJobHealth(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		envID := envFilter(r)
		res := JobHealthResponse{
			GeneratedAtUTC: time.Now().UTC(),
			Items:          []JobHealthItem{},
		}
		envs, err := store.ListEnvironmentsAPI(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list environments failed"})
			return
		}
		for _, e := range envs {
			if envID != "" && e.ID != envID {
				continue
			}
			item := JobHealthItem{EnvironmentID: e.ID, EnvironmentName: e.Name}
			jobs, err := store.ListJobSnapshots(ctx, e.ID)
			if err == nil {
				item.JobsTotal = len(jobs)
				for _, j := range jobs {
					if j.Scheduled {
						item.JobsScheduled++
					}
				}
			}
			pols, err := store.ListPolicySnapshots(ctx, e.ID)
			if err == nil {
				item.PoliciesTotal = len(pols)
			}
			res.Items = append(res.Items, item)
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func getInventoryReport(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		envID := envFilter(r)
		res := InventoryReportResponse{GeneratedAtUTC: time.Now().UTC()}
		envs, err := store.ListEnvironmentsAPI(ctx)
		if err != nil {
			res.Notes = append(res.Notes, "Não foi possível listar os ambientes.")
			writeJSON(w, http.StatusOK, res)
			return
		}
		for _, e := range envs {
			if envID != "" && e.ID != envID {
				continue
			}
			res.Environments++
			if dbs, err := store.ListDatabaseSnapshots(ctx, e.ID); err == nil {
				res.Databases += len(dbs)
			}
			if schemas, err := store.ListSchemaSnapshots(ctx, e.ID); err == nil {
				res.Schemas += len(schemas)
			}
			if _, total, err := store.ListTableSnapshots(ctx, repository.InventoryFilter{EnvironmentID: e.ID, Limit: 1}); err == nil {
				res.Tables += total
			}
			if hts, err := store.ListHypertableSnapshots(ctx, e.ID); err == nil {
				res.Hypertables += len(hts)
			}
			if _, total, err := store.ListIndexSnapshots(ctx, repository.InventoryFilter{EnvironmentID: e.ID, Limit: 1}); err == nil {
				res.Indexes += total
			}
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func getFindingsReport(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		envID := envFilter(r)
		items, err := store.ListFindings(ctx, envID, "", "", "", 500)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list findings failed"})
			return
		}
		if items == nil {
			items = []repository.Finding{}
		}
		sev := map[string]int{}
		st := map[string]int{}
		for _, f := range items {
			sev[strings.ToLower(f.Severity)]++
			st[strings.ToLower(f.Status)]++
		}
		writeJSON(w, http.StatusOK, FindingsReportResponse{
			GeneratedAtUTC: time.Now().UTC(),
			Total:          len(items),
			BySeverity:     bucketsFromMap(sev),
			ByStatus:       bucketsFromMap(st),
			Items:          items,
		})
	}
}
