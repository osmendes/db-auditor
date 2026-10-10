package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/config"
	"github.com/mayconmendes-qc/db-auditor/internal/quality"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type qualityStore interface {
	SaveQualityScan(context.Context, string, string, quality.Request, quality.Result) (*repository.QualityScan, error)
	ListQualityScans(context.Context, string) ([]repository.QualityScan, error)
	UpdateQualityIssue(context.Context, string, string, string, string, string, string, string) (*repository.QualityScan, error)
}

func registerQualityRoutes(mux *http.ServeMux, store InventoryStore, targets map[string]string) {
	backend, ok := store.(qualityStore)
	if !ok {
		return
	}
	base := "/api/v1/environments/{id}"
	mux.HandleFunc("GET "+base+"/quality-scans", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		if !uuidPattern.MatchString(env) {
			writeError(w, 400, CodeValidation, "Ambiente inválido.")
			return
		}
		items, err := backend.ListQualityScans(r.Context(), env)
		if err != nil {
			writeError(w, 500, CodeInternal, "Falha ao listar diagnósticos.")
			return
		}
		writeJSON(w, 200, map[string]any{"items": items, "enabled": os.Getenv("AUDITOR_DATA_QUALITY_ENABLED") == "1"})
	})
	mux.HandleFunc("POST "+base+"/quality-scans", func(w http.ResponseWriter, r *http.Request) {
		env := r.PathValue("id")
		if !uuidPattern.MatchString(env) {
			writeError(w, 400, CodeValidation, "Ambiente inválido.")
			return
		}
		if os.Getenv("AUDITOR_DATA_QUALITY_ENABLED") != "1" {
			writeError(w, 403, CodeUnavailable, "Diagnóstico de dados desativado. Solicite habilitação operacional.")
			return
		}
		var request quality.Request
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&request); err != nil || request.Validate() != nil {
			writeError(w, 400, CodeValidation, "Informe tabela, limite até 1000 e verificações válidas.")
			return
		}
		configuredTargets := targets
		if configuredTargets == nil {
			configuredTargets = config.LoadTargetDSNs()
		}
		dsn := config.DSNForEnvironment(configuredTargets, env)
		if dsn == "" {
			writeError(w, 503, CodeUnavailable, "Conexão de leitura do ambiente não configurada.")
			return
		}
		result, err := quality.Run(r.Context(), dsn, request)
		if err != nil {
			writeError(w, 422, CodeValidation, "Diagnóstico não concluído: "+err.Error())
			return
		}
		actor := "local"
		if user := requestIdentity(r); user != nil {
			actor = user.Username
		}
		item, err := backend.SaveQualityScan(r.Context(), env, actor, request, result)
		if err != nil {
			writeError(w, 500, CodeInternal, "Falha ao guardar contagens do diagnóstico.")
			return
		}
		writeJSON(w, 201, item)
	})
	mux.HandleFunc("PATCH "+base+"/quality-issues/{issue}", func(w http.ResponseWriter, r *http.Request) {
		env, id := r.PathValue("id"), r.PathValue("issue")
		var body struct {
			Status        string `json:"status"`
			Owner         string `json:"owner"`
			Justification string `json:"justification"`
			Result        string `json:"result"`
		}
		if !uuidPattern.MatchString(env) || !uuidPattern.MatchString(id) || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
			writeError(w, 400, CodeValidation, "Ação inválida.")
			return
		}
		body.Owner, body.Justification, body.Result = strings.TrimSpace(body.Owner), strings.TrimSpace(body.Justification), strings.TrimSpace(body.Result)
		valid := body.Status == "suggested" || body.Status == "in_review" || body.Status == "planned" || body.Status == "executed_externally" || body.Status == "validated" || body.Status == "discarded"
		if !valid || len(body.Owner) > 128 || len(body.Justification) > 2000 || len(body.Result) > 2000 || (body.Status == "planned" && body.Owner == "") || (body.Status == "discarded" && body.Justification == "") || ((body.Status == "executed_externally" || body.Status == "validated") && body.Result == "") {
			writeError(w, 400, CodeValidation, "Informe responsável, justificativa e resultado conforme a etapa.")
			return
		}
		actor := "local"
		if user := requestIdentity(r); user != nil {
			actor = user.Username
		}
		item, err := backend.UpdateQualityIssue(r.Context(), env, id, actor, body.Status, body.Owner, body.Justification, body.Result)
		if errors.Is(err, repository.ErrQualityNotFound) {
			writeError(w, 404, CodeNotFound, "Ação não encontrada.")
			return
		}
		if errors.Is(err, repository.ErrQualityEvidenceRequired) {
			writeError(w, 409, CodeConflict, "Para validar, repita o mesmo diagnóstico em uma nova coleta com limite e amostra comparáveis.")
			return
		}
		if err != nil {
			writeError(w, 500, CodeInternal, "Falha ao salvar ação.")
			return
		}
		writeJSON(w, 200, item)
	})
}
