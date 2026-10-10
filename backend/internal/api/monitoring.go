package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type monitoringStore interface {
	CreateAuditAnnotation(context.Context, string, string, string, string, string, time.Time) (*repository.AuditAnnotation, error)
	ListAuditAnnotations(context.Context, string) ([]repository.AuditAnnotation, error)
	ListRegressionAlerts(context.Context, string) ([]repository.RegressionAlert, error)
	AcknowledgeRegressionAlert(context.Context, string, string, string, string) error
}

func registerMonitoringRoutes(mux *http.ServeMux, store InventoryStore) {
	backend, ok := store.(monitoringStore)
	if !ok {
		return
	}
	base := "/api/v1/environments/{id}"
	mux.HandleFunc("GET "+base+"/annotations", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		if !uuidPattern.MatchString(env) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente inválido.")
			return
		}
		items, err := backend.ListAuditAnnotations(r.Context(), env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao listar anotações.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("POST "+base+"/annotations", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		var body struct {
			AuditRunID string `json:"audit_run_id"`
			Kind       string `json:"kind"`
			Note       string `json:"note"`
			OccurredAt string `json:"occurred_at"`
		}
		if !uuidPattern.MatchString(env) || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil ||
			(body.AuditRunID != "" && !uuidPattern.MatchString(body.AuditRunID)) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Anotação inválida.")
			return
		}
		body.Note = strings.TrimSpace(body.Note)
		if (body.Kind != "deployment" && body.Kind != "maintenance" && body.Kind != "incident" && body.Kind != "note") || len(body.Note) == 0 || len(body.Note) > 2000 {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe tipo e nota de até 2000 caracteres.")
			return
		}
		at, err := time.Parse(time.RFC3339, body.OccurredAt)
		if err != nil || at.After(time.Now().Add(5*time.Minute)) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Data inválida.")
			return
		}
		actor := "local"
		if user := requestIdentity(r); user != nil {
			actor = user.Username
		}
		item, err := backend.CreateAuditAnnotation(r.Context(), env, body.AuditRunID, body.Kind, body.Note, actor, at)
		if errors.Is(err, repository.ErrMonitoringScope) {
			writeError(w, http.StatusNotFound, CodeNotFound, "Ambiente ou execução não encontrado.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao salvar anotação.")
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
	mux.HandleFunc("GET "+base+"/regressions", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		if !uuidPattern.MatchString(env) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente inválido.")
			return
		}
		items, err := backend.ListRegressionAlerts(r.Context(), env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao listar regressões.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("POST "+base+"/regressions/{alert}/acknowledge", func(w http.ResponseWriter, r *http.Request) {
		env, id := r.PathValue("id"), r.PathValue("alert")
		var body struct {
			Reason string `json:"reason"`
		}
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(id) || json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body) != nil || len(strings.TrimSpace(body.Reason)) == 0 || len(body.Reason) > 1000 {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe motivo para reconhecer a regressão.")
			return
		}
		actor := "local"
		if user := requestIdentity(r); user != nil {
			actor = user.Username
		}
		if err := backend.AcknowledgeRegressionAlert(r.Context(), env, id, actor, strings.TrimSpace(body.Reason)); err != nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Alerta indisponível.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "acknowledged"})
	})
}
