package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mayconmendes-qc/db-auditor/internal/fingerprint"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

// MappingStore exposes object mapping persistence.
type MappingStore interface {
	ListObjectMappings(ctx context.Context, sourceEnv, targetEnv, status string) ([]repository.ObjectMapping, error)
	CreateObjectMapping(ctx context.Context, p repository.CreateObjectMappingParams) (*repository.ObjectMapping, error)
	UpdateObjectMappingStatus(ctx context.Context, id, status string, notes string) (*repository.ObjectMapping, error)
}

type createMappingBody struct {
	SourceEnvironmentID  string  `json:"source_environment_id"`
	TargetEnvironmentID  string  `json:"target_environment_id"`
	SourceDatabase       string  `json:"source_database"`
	SourceSchema         string  `json:"source_schema"`
	SourceObjectType     string  `json:"source_object_type"`
	SourceObjectName     string  `json:"source_object_name"`
	TargetDatabase       string  `json:"target_database"`
	TargetSchema         string  `json:"target_schema"`
	TargetObjectType     string  `json:"target_object_type"`
	TargetObjectName     string  `json:"target_object_name"`
	RelationType         string  `json:"relation_type"`
	Confidence           float64 `json:"confidence"`
	Status               string  `json:"status"`
	SourceFingerprint    string  `json:"source_fingerprint"`
	TargetFingerprint    string  `json:"target_fingerprint"`
	FingerprintAlgorithm string  `json:"fingerprint_algorithm"`
	Notes                string  `json:"notes"`
}

type updateMappingBody struct {
	Status string `json:"status"`
	Notes  string `json:"notes"`
}

type suggestBody struct {
	Source          []fingerprint.ObjectRef `json:"source"`
	Target          []fingerprint.ObjectRef `json:"target"`
	TargetDefaultDB string                  `json:"target_default_db"`
}

func registerMappingRoutes(mux *http.ServeMux, store MappingStore) {
	mux.HandleFunc("GET /api/v1/mappings", listMappings(store))
	mux.HandleFunc("POST /api/v1/mappings", createMapping(store))
	mux.HandleFunc("PATCH /api/v1/mappings/{id}", patchMapping(store))
	mux.HandleFunc("POST /api/v1/mappings/suggest", suggestMappings())
}

func listMappings(store MappingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		items, err := store.ListObjectMappings(r.Context(), q.Get("source_environment_id"), q.Get("target_environment_id"), q.Get("status"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os mapeamentos.")
			return
		}
		if items == nil {
			items = []repository.ObjectMapping{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func createMapping(store MappingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createMappingBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		if body.SourceEnvironmentID == "" || body.TargetEnvironmentID == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "Os identificadores de ambiente de origem e destino são obrigatórios.")
			return
		}
		m, err := store.CreateObjectMapping(r.Context(), repository.CreateObjectMappingParams{
			SourceEnvironmentID: body.SourceEnvironmentID, TargetEnvironmentID: body.TargetEnvironmentID,
			SourceDatabase: body.SourceDatabase, SourceSchema: body.SourceSchema,
			SourceObjectType: body.SourceObjectType, SourceObjectName: body.SourceObjectName,
			TargetDatabase: body.TargetDatabase, TargetSchema: body.TargetSchema,
			TargetObjectType: body.TargetObjectType, TargetObjectName: body.TargetObjectName,
			RelationType: body.RelationType, Confidence: body.Confidence, Status: body.Status,
			SourceFingerprint: body.SourceFingerprint, TargetFingerprint: body.TargetFingerprint,
			FingerprintAlgorithm: body.FingerprintAlgorithm, Notes: body.Notes,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível criar o mapeamento.")
			return
		}
		writeJSON(w, http.StatusCreated, m)
	}
}

func patchMapping(store MappingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body updateMappingBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		if body.Status == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "O campo status é obrigatório.")
			return
		}
		m, err := store.UpdateObjectMappingStatus(r.Context(), id, body.Status, body.Notes)
		if err != nil {
			writeError(w, http.StatusNotFound, CodeNotFound, "Mapeamento não encontrado.")
			return
		}
		writeJSON(w, http.StatusOK, m)
	}
}

func suggestMappings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body suggestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		items := fingerprint.SuggestMappings(body.Source, body.Target, body.TargetDefaultDB)
		if items == nil {
			items = []fingerprint.MappingCandidate{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}
