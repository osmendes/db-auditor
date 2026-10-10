package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrivateJSONReturns304(t *testing.T) {
	body := []byte(`{"items":[]}`)
	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rules", nil)
	writePrivateJSON(first, req, http.StatusOK, body)
	if first.Code != http.StatusOK || first.Header().Get("ETag") == "" || first.Header().Get("Cache-Control") != "private, max-age=60" {
		t.Fatalf("first response: %d %v", first.Code, first.Header())
	}
	second := httptest.NewRecorder()
	req.Header.Set("If-None-Match", first.Header().Get("ETag"))
	writePrivateJSON(second, req, http.StatusOK, body)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("expected 304, got %d %s", second.Code, second.Body.String())
	}
}
