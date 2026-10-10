package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

const (
	assessmentEnv = "11111111-1111-4111-8111-111111111111"
	assessmentRun = "22222222-2222-4222-8222-222222222222"
)

type assessmentStub struct {
	*stubStore
	item  *repository.TableAssessment
	graph *repository.RelationshipGraph
}

func (s *assessmentStub) GetTableAssessment(_ context.Context, env, run, database, schema, table string) (*repository.TableAssessment, error) {
	if env != assessmentEnv || run != assessmentRun || database != "db" || schema != "public" || table != "orders" {
		return nil, nil
	}
	return s.item, nil
}
func (s *assessmentStub) TableRelationshipGraph(_ context.Context, _, _, _, _, _ string, _ int) (*repository.RelationshipGraph, error) {
	return s.graph, nil
}
func (s *assessmentStub) ListTableFindings(context.Context, string, string, string, string, string, int, int) ([]repository.Finding, int, error) {
	return []repository.Finding{}, 0, nil
}
func (s *assessmentStub) ListTableGrants(context.Context, string, string, string, string, string, int, int) ([]repository.GrantSnapshotRow, int, error) {
	return []repository.GrantSnapshotRow{}, 0, nil
}
func (s *assessmentStub) ListTableDependencies(context.Context, string, string, string, string, string, int, int) ([]repository.DependencySnapshotRow, int, error) {
	return []repository.DependencySnapshotRow{}, 0, nil
}
func (s *assessmentStub) ListTableTriggers(context.Context, string, string, string, string, string, int, int) ([]repository.TriggerSnapshotRow, int, error) {
	return []repository.TriggerSnapshotRow{}, 0, nil
}
func (s *assessmentStub) ListTableRLSPolicies(context.Context, string, string, string, string, string, int, int) ([]repository.RLSPolicySnapshotRow, int, error) {
	return []repository.RLSPolicySnapshotRow{}, 0, nil
}

func assessmentURL(table string) string {
	return "/api/v1/environments/" + assessmentEnv + "/runs/" + assessmentRun + "/databases/db/schemas/public/tables/" + table
}

func TestTableAssessmentContractAndPartialRun(t *testing.T) {
	score, confidence := 80.0, 0.75
	store := &assessmentStub{stubStore: &stubStore{}, item: &repository.TableAssessment{
		Version: 1,
		Run:     repository.AssessmentRun{ID: assessmentRun, Status: "partial_success", Partial: true, StartedAt: time.Now()},
		Table:   repository.AssessmentTable{TableSnapshotRow: repository.TableSnapshotRow{DatabaseName: "db", SchemaName: "public", TableName: "orders", AuditRunID: assessmentRun}},
		Summary: repository.AssessmentSummary{Score: &score, ScoreStatus: "available", ScoreVersion: "structural-v1", ScoreConfidence: &confidence,
			ScoreFactors: []repository.AssessmentScoreFactor{{Code: "missing_primary_key", Description: "Tabela sem chave primária", Penalty: 20}}},
		Links: map[string]string{"columns": "/api/v1/environments/" + assessmentEnv + "/columns?audit_run_id=" + assessmentRun},
	}}
	w := httptest.NewRecorder()
	NewHandler(store).ServeHTTP(w, httptest.NewRequest(http.MethodGet, assessmentURL("orders")+"/assessment", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var got repository.TableAssessment
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || !got.Run.Partial || got.Table.AuditRunID != assessmentRun || !strings.Contains(got.Links["columns"], assessmentRun) ||
		got.Summary.Score == nil || *got.Summary.Score != 80 || got.Summary.ScoreVersion != "structural-v1" || len(got.Summary.ScoreFactors) != 1 {
		t.Fatalf("bad run-scoped response: %#v", got)
	}
}

func TestTableAssessmentAbsentAndInvalidScope(t *testing.T) {
	h := NewHandler(&assessmentStub{stubStore: &stubStore{}})
	for _, tc := range []struct {
		path string
		want int
	}{
		{assessmentURL("missing") + "/assessment", http.StatusNotFound},
		{assessmentURL("orders") + "/graph", http.StatusNotFound},
		{"/api/v1/environments/bad/runs/" + assessmentRun + "/databases/db/schemas/public/tables/orders/assessment", http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.want)
		}
	}
}

func TestTableGraphBounded(t *testing.T) {
	store := &assessmentStub{stubStore: &stubStore{}, item: &repository.TableAssessment{}, graph: &repository.RelationshipGraph{Nodes: []repository.RelationshipNode{{Database: "db", Schema: "public", Table: "orders"}}}}
	h := NewHandler(store)
	for _, limit := range []string{"0", "101", "n"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, assessmentURL("orders")+"/graph?limit="+limit, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("limit=%s status=%d", limit, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, assessmentURL("orders")+"/graph?limit=10", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "orders") {
		t.Fatalf("graph status=%d body=%s", w.Code, w.Body)
	}
}
