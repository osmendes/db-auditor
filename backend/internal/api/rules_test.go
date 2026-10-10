package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
)

type rulesStoreStub struct{ *stubStore }

func (*rulesStoreStub) EffectiveRules(_ context.Context, env, schema string) ([]analyzer.EffectiveRule, error) {
	return analyzer.EffectiveCatalog(analyzer.SnapshotFacts{EnvironmentID: env}, schema), nil
}

func (*rulesStoreStub) SetRulePolicy(_ context.Context, p analyzer.RulePolicy) error {
	if p.RuleID == "missing.rule" {
		return errUnknown("unknown rule_id")
	}
	if _, ok := p.Parameters["not_a_parameter"]; ok {
		return errUnknown("unknown rule parameter not_a_parameter")
	}
	return nil
}

type errUnknown string

func (e errUnknown) Error() string { return string(e) }

func TestRulesEndpointReturnsVersionedCatalog(t *testing.T) {
	h := NewHandler(&rulesStoreStub{stubStore: &stubStore{}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/environments/11111111-1111-1111-1111-111111111111/rules?schema=public", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Items  []analyzer.EffectiveRule `json:"items"`
		Schema string                   `json:"schema"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Schema != "public" || len(body.Items) != len(analyzer.Catalog()) {
		t.Fatalf("invalid rules response: schema=%s items=%d", body.Schema, len(body.Items))
	}
	var storage string
	for _, item := range body.Items {
		if item.ID == "storage.large_table" {
			storage = item.Version
		}
	}
	if storage != "1.1.0" {
		t.Fatalf("storage rule version %s", storage)
	}
}

func TestPutRuleRejectsUnknownParameter(t *testing.T) {
	h := NewHandler(&rulesStoreStub{stubStore: &stubStore{}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/environments/11111111-1111-1111-1111-111111111111/rules/model.wide_table", strings.NewReader(`{"enabled":true,"schema":"public","parameters":{"not_a_parameter":1}}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestPutRuleAcceptsKnownToggle(t *testing.T) {
	h := NewHandler(&rulesStoreStub{stubStore: &stubStore{}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/environments/11111111-1111-1111-1111-111111111111/rules/model.wide_table", strings.NewReader(`{"enabled":false,"schema":"public","parameters":{}}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "não são reescritos") {
		t.Fatalf("body %s", w.Body.String())
	}
}
