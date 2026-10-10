package api

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type exportStore struct{ stubStore }

func (exportStore) ListTrackedActionsPage(_ context.Context, _ string, limit, offset int) ([]repository.TrackedAction, int, error) {
	items := []repository.TrackedAction{}
	for i := offset; i < offset+limit && i < 120; i++ {
		items = append(items, repository.TrackedAction{FindingID: strconv.Itoa(i), Title: "=1+1", Status: "planned"})
	}
	return items, 120, nil
}

func (exportStore) ListActionMeasurementsPage(_ context.Context, findingID string, limit, offset int) ([]repository.ActionMeasurement, int, error) {
	if findingID != "0" {
		return []repository.ActionMeasurement{}, 0, nil
	}
	items := []repository.ActionMeasurement{}
	for i := offset; i < offset+limit && i < 105; i++ {
		items = append(items, repository.ActionMeasurement{FindingID: findingID, Metric: "finding_observed"})
	}
	return items, 105, nil
}

func (exportStore) RecordActionMeasurement(context.Context, string, string, string, string, string, string, bool, string) (*repository.ActionMeasurement, error) {
	return nil, nil
}

func TestActionExportStreamsAllPagesAndEscapesSpreadsheetFormula(t *testing.T) {
	mux := http.NewServeMux()
	registerActionMeasurementRoutes(mux, &exportStore{})
	env := "00000000-0000-0000-0000-000000000001"
	for _, format := range []string{"csv", "jsonl"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/environments/"+env+"/actions/export?format="+format, nil))
		if response.Code != 200 || !strings.Contains(response.Header().Get("Content-Disposition"), "attachment") {
			t.Fatalf("%s export: %d %s", format, response.Code, response.Body.String())
		}
		if format == "jsonl" {
			if got := strings.Count(response.Body.String(), `"record_type":"action"`); got != 120 {
				t.Fatalf("exported %d actions, want 120", got)
			}
			if got := strings.Count(response.Body.String(), `"record_type":"measurement"`); got != 105 {
				t.Fatalf("exported %d measurements, want 105", got)
			}
			if !strings.Contains(response.Body.String(), `"record_type":"complete"`) {
				t.Fatal("missing export completion marker")
			}
			continue
		}
		rows, err := csv.NewReader(strings.NewReader(response.Body.String())).ReadAll()
		if err != nil || len(rows) != 226 || rows[1][1] != "'=1+1" || rows[len(rows)-1][0] != "# export_complete" {
			t.Fatalf("CSV rows=%d err=%v first=%v", len(rows), err, rows[1])
		}
	}
}
