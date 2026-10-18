package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/osmendes/db-auditor/internal/report"
	"github.com/osmendes/db-auditor/internal/repository"
)

type pdfReportStore interface {
	CreateReportJob(context.Context, repository.ReportRequest) (*repository.ReportJob, error)
	GetReportJob(context.Context, string, string) (*repository.ReportJob, error)
	ListReportJobs(context.Context, string) ([]repository.ReportJob, error)
	CancelReportJob(context.Context, string, string) error
	RetryReportJob(context.Context, string, string) (*repository.ReportJob, error)
	GetReportArtifact(context.Context, string, string) (*repository.ReportArtifact, error)
	LatestArchitectureDecisions(context.Context, string, string) ([]report.ArchitectureDecision, error)
	ArchitectureDecisionHistory(context.Context, string, string) ([]report.ArchitectureDecision, error)
	RecordArchitectureDecision(context.Context, string, string, string, string, string, string) (report.ArchitectureDecision, error)
}

// Report endpoints are fail-closed until an operator sets a private server-side
// token. The frontend supplies it per session; it is never embedded in a build.
func authorizeReport(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if user := requestIdentity(r); user != nil {
		if roleAtLeast(user.Role, "auditor") {
			return true
		}
		writeError(w, http.StatusForbidden, CodeUnavailable, "Permissão de auditor necessária.")
		return false
	}
	expected := os.Getenv("AUDITOR_REPORT_API_TOKEN")
	if len(expected) < 32 {
		writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Esta operação exige AUDITOR_REPORT_API_TOKEN no servidor.")
		return false
	}
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		writeError(w, http.StatusUnauthorized, CodeUnavailable, "Token de operação ausente ou inválido.")
		return false
	}
	return true
}

func registerPDFReportRoutes(mux *http.ServeMux, store InventoryStore) {
	base := "/api/v1/environments/{id}/reports"
	mux.HandleFunc("GET "+base+"/architecture-decisions", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		if os.Getenv("AUDITOR_ARCHITECTURE_REPORT_ENABLED") != "true" {
			writeError(w, http.StatusNotFound, CodeNotFound, "Recurso indisponível.")
			return
		}
		database := r.URL.Query().Get("database")
		if database == "" || len(database) > 128 {
			writeError(w, http.StatusBadRequest, CodeValidation, "Database obrigatório.")
			return
		}
		var decisions []report.ArchitectureDecision
		var err error
		if r.URL.Query().Get("history") == "true" {
			decisions, err = backend.ArchitectureDecisionHistory(r.Context(), env, database)
		} else {
			decisions, err = backend.LatestArchitectureDecisions(r.Context(), env, database)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao consultar decisões.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": decisions})
	})
	mux.HandleFunc("POST "+base+"/architecture-decisions", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		if os.Getenv("AUDITOR_ARCHITECTURE_REPORT_ENABLED") != "true" {
			writeError(w, http.StatusNotFound, CodeNotFound, "Recurso indisponível.")
			return
		}
		user := requestIdentity(r)
		if user == nil || !roleAtLeast(user.Role, "auditor") {
			writeError(w, http.StatusForbidden, CodeUnavailable, "Conta de auditor ou operador necessária.")
			return
		}
		var body struct {
			Database  string `json:"database"`
			ObjectRef string `json:"object_ref"`
			Subject   string `json:"subject"`
			Value     string `json:"value"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Decisão inválida.")
			return
		}
		decision, err := backend.RecordArchitectureDecision(r.Context(), env, body.Database, body.ObjectRef, body.Subject, body.Value, user.Username)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Decisão inválida.")
			return
		}
		writeJSON(w, http.StatusCreated, decision)
	})
	mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		var body struct {
			AuditRunID     string                   `json:"audit_run_id"`
			ReportType     string                   `json:"report_type"`
			Format         string                   `json:"format"`
			Filters        repository.ReportFilters `json:"filters"`
			IdempotencyKey string                   `json:"idempotency_key"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body) != nil || !uuidPattern.MatchString(body.AuditRunID) || !validReportRequest(body.ReportType, body.Filters) || body.ReportType == "architecture" && body.Format != "markdown" || body.ReportType != "architecture" && body.Format != "" && body.Format != "pdf" || len(body.IdempotencyKey) > 128 {
			writeError(w, http.StatusBadRequest, CodeValidation, "Pedido de relatorio invalido.")
			return
		}
		requestedBy := "report-token"
		if user := requestIdentity(r); user != nil {
			requestedBy = user.Username
		}
		job, err := backend.CreateReportJob(r.Context(), repository.ReportRequest{EnvironmentID: env, AuditRunID: body.AuditRunID, Type: body.ReportType, Format: body.Format, Filters: body.Filters, RequestedBy: requestedBy, IdempotencyKey: body.IdempotencyKey})
		if errors.Is(err, repository.ErrReportIneligible) {
			writeError(w, http.StatusConflict, CodeValidation, "Execucao indisponivel para o ambiente.")
			return
		}
		if errors.Is(err, repository.ErrReportConflict) {
			writeError(w, http.StatusConflict, CodeValidation, "Chave idempotente ja pertence a outro pedido.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao solicitar relatorio.")
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	})
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		jobs, err := backend.ListReportJobs(r.Context(), env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao listar relatorios.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": jobs})
	})
	mux.HandleFunc("GET "+base+"/{job}", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		id := r.PathValue("job")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Relatorio invalido.")
			return
		}
		job, err := backend.GetReportJob(r.Context(), env, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao consultar relatorio.")
			return
		}
		if job == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Relatorio nao encontrado.")
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("POST "+base+"/{job}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		id := r.PathValue("job")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Relatorio invalido.")
			return
		}
		if err := backend.CancelReportJob(r.Context(), env, id); err != nil {
			writeError(w, http.StatusConflict, CodeValidation, "O relatorio nao pode ser cancelado.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	})
	mux.HandleFunc("POST "+base+"/{job}/retry", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		id := r.PathValue("job")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Relatorio invalido.")
			return
		}
		job, err := backend.RetryReportJob(r.Context(), env, id)
		if err != nil {
			writeError(w, http.StatusConflict, CodeValidation, "O relatorio nao pode ser reprocessado.")
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	})
	mux.HandleFunc("GET "+base+"/{job}/download", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeReport(w, r) {
			return
		}
		backend, env, ok := reportScope(w, r, store)
		if !ok {
			return
		}
		id := r.PathValue("job")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Relatorio invalido.")
			return
		}
		artifact, err := backend.GetReportArtifact(r.Context(), env, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao baixar relatorio.")
			return
		}
		if artifact == nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Artefato indisponivel ou expirado.")
			return
		}
		w.Header().Set("Content-Type", artifact.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.Filename))
		w.Header().Set("Content-Length", fmt.Sprint(len(artifact.Content)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("ETag", `"`+artifact.SHA256+`"`)
		_, _ = w.Write(artifact.Content) //nolint:gosec // G705: this is a PDF download with a fixed MIME type and nosniff.
	})
}

func reportScope(w http.ResponseWriter, r *http.Request, store InventoryStore) (pdfReportStore, string, bool) {
	env := r.PathValue("id")
	if !uuidPattern.MatchString(env) {
		writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente invalido.")
		return nil, "", false
	}
	backend, ok := store.(pdfReportStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Relatorios indisponiveis.")
		return nil, "", false
	}
	return backend, env, true
}

func validReportRequest(kind string, f repository.ReportFilters) bool {
	if kind != "executive" && kind != "technical" && kind != "table" && kind != "architecture" {
		return false
	}
	if kind == "architecture" && (os.Getenv("AUDITOR_ARCHITECTURE_REPORT_ENABLED") != "true" || f.Database == "" || f.Table != "" || f.Severity != "") {
		return false
	}
	if f.Schema != "" && f.Database == "" || f.Table != "" && f.Schema == "" || kind == "table" && f.Table == "" {
		return false
	}
	if f.Severity != "" && f.Severity != "critical" && f.Severity != "high" && f.Severity != "medium" && f.Severity != "low" && f.Severity != "info" {
		return false
	}
	if f.Redaction != "" && f.Redaction != "none" && f.Redaction != "identifiers" && f.Redaction != "strict" {
		return false
	}
	for _, value := range []string{f.Database, f.Schema, f.Table} {
		if len(value) > 128 || strings.ContainsAny(value, "\x00\n\r") {
			return false
		}
	}
	return true
}
