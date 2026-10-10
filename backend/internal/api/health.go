package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/osmendes/db-auditor/internal/buildinfo"
	"github.com/osmendes/db-auditor/internal/observability"
	"github.com/osmendes/db-auditor/internal/repository"
)

type readinessChecker interface {
	Ping(context.Context) error
}

type InventoryStore interface {
	readinessChecker
	ListEnvironmentsAPI(ctx context.Context) ([]repository.Environment, error)
	ListDatabaseSnapshots(ctx context.Context, environmentID string) ([]repository.DatabaseSnapshot, error)
	ListSchemaSnapshots(ctx context.Context, environmentID string) ([]repository.SchemaSnapshot, error)
	ListHypertableSnapshots(ctx context.Context, environmentID string) ([]repository.HypertableSnapshotRow, error)
	ListHypertableSnapshotsPage(ctx context.Context, f repository.InventoryFilter) ([]repository.HypertableSnapshotRow, int, error)
	ListDimensionSnapshots(ctx context.Context, environmentID string) ([]repository.DimensionSnapshotRow, error)
	ListChunkSnapshots(ctx context.Context, environmentID string) ([]repository.ChunkSnapshotRow, error)
	ListCAGGSnapshots(ctx context.Context, environmentID string) ([]repository.CAGGSnapshotRow, error)
	ListCAGGSnapshotsPage(ctx context.Context, f repository.InventoryFilter) ([]repository.CAGGSnapshotRow, int, error)
	ListJobSnapshots(ctx context.Context, environmentID string) ([]repository.JobSnapshotRow, error)
	ListPolicySnapshots(ctx context.Context, environmentID string) ([]repository.PolicySnapshotRow, error)
	ListTableSnapshots(ctx context.Context, f repository.InventoryFilter) ([]repository.TableSnapshotRow, int, error)
	ListColumnSnapshots(ctx context.Context, f repository.InventoryFilter) ([]repository.ColumnSnapshotRow, int, error)
	ListIndexSnapshots(ctx context.Context, f repository.InventoryFilter) ([]repository.IndexSnapshotRow, int, error)
	ListViewSnapshots(ctx context.Context, f repository.InventoryFilter) ([]repository.ViewSnapshotRow, int, error)
	ListFunctionSnapshots(ctx context.Context, f repository.InventoryFilter) ([]repository.FunctionSnapshotRow, int, error)
	ListConstraintSnapshots(ctx context.Context, f repository.InventoryFilter) ([]repository.ConstraintSnapshotRow, int, error)
	ListAuditRuns(ctx context.Context, environmentID, profile, status string, limit int) ([]repository.AuditRunRow, error)
	GetAuditRun(ctx context.Context, id string) (*repository.AuditRunRow, error)
	ListCollectorRuns(ctx context.Context, auditRunID string) ([]repository.CollectorRunRow, error)
	ListAuditRunCoverage(ctx context.Context, auditRunID string) ([]repository.AuditRunCoverage, error)
	GetAnalysisRun(ctx context.Context, auditRunID string) (*repository.AnalysisRun, error)
	GetSnapshotCompleteness(ctx context.Context, environmentID, auditRunID string) (*repository.SnapshotCompleteness, error)
	ListObjectMappings(ctx context.Context, sourceEnv, targetEnv, status string) ([]repository.ObjectMapping, error)
	CreateObjectMapping(ctx context.Context, p repository.CreateObjectMappingParams) (*repository.ObjectMapping, error)
	UpdateObjectMappingStatus(ctx context.Context, id, status string, notes string) (*repository.ObjectMapping, error)
	ListFindings(ctx context.Context, environmentID, findingType, severity, status string, limit int) ([]repository.Finding, error)
	CountFindings(ctx context.Context, environmentID string) (repository.FindingAggregate, error)
	CountRecentRuns(ctx context.Context, environmentID string) (successful, failed int, err error)
	CountLatestCapabilities(ctx context.Context, environmentID string) (repository.CapabilityAggregate, error)
	ListStorageTrend(ctx context.Context, environmentID string, from, to time.Time, granularity string) ([]repository.RunTrendPoint, error)
	ListFindingTrend(ctx context.Context, environmentID string, from, to time.Time, granularity string) ([]repository.RunTrendPoint, error)
	ListTopTableConsumers(ctx context.Context, environmentID string, limit int) ([]repository.StorageConsumer, error)
	GetFinding(ctx context.Context, id string) (*repository.Finding, error)
	UpdateFindingStatus(ctx context.Context, id, status, notes string) (*repository.Finding, error)
	UpsertFinding(ctx context.Context, p repository.UpsertFindingParams) (*repository.Finding, error)
}

// HandlerOptions wires optional run trigger support and target DSNs.
type HandlerOptions struct {
	Runner   ManualRunner
	Analysis AnalysisRunner
	Targets  map[string]string
	Auth     AuthStore
}

func NewHandler(store InventoryStore) http.Handler {
	return NewHandlerWithOptions(store, HandlerOptions{})
}

func NewHandlerWithOptions(store InventoryStore, opts HandlerOptions) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /ready", ready(store))
	mux.HandleFunc("GET /api/v1/environments", listEnvironments(store))
	mux.HandleFunc("POST /api/v1/environments", createEnvironmentLabel(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/databases", listDatabases(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/schemas", listSchemas(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/hypertables", listHypertables(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/dimensions", listDimensions(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/chunks", listChunks(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/continuous-aggregates", listCAGGs(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/jobs", listJobs(store))
	mux.HandleFunc("GET /api/v1/environments/{id}/policies", listPolicies(store))
	registerInventoryRoutes(mux, store)
	registerViewDetailRoutes(mux, store)
	registerIndexDetailRoutes(mux, store)
	registerFunctionDetailRoutes(mux, store)
	registerHypertableDetailRoutes(mux, store)
	registerCAGGDetailRoutes(mux, store)
	registerAssessmentRoutes(mux, store)
	registerBaselineRoutes(mux, store)
	registerMonitoringRoutes(mux, store)
	registerCapabilityRoutes(mux, store)
	registerQualityRoutes(mux, store, opts.Targets)
	registerActionMeasurementRoutes(mux, store)
	registerScopeScoreRoutes(mux, store)
	registerPDFReportRoutes(mux, store)
	registerHistoryRoutes(mux, store)
	registerRuleRoutes(mux, store)
	registerRunRoutes(mux, store, opts.Runner, opts.Analysis)
	registerMappingRoutes(mux, store)
	registerCompareRoutes(mux)
	registerServerCompare(mux, store)
	registerFindingRoutes(mux, store, opts.Analysis)
	registerStatusRoutes(mux, store)
	registerAnalyticsRoutes(mux, store)
	registerConnectionRoutes(mux, store, opts.Targets)
	if opts.Auth != nil {
		registerAuthRoutes(mux, opts.Auth)
		return observability.Middleware(protectPublic(authMiddleware(mux, opts.Auth)))
	}
	return observability.Middleware(protectPublic(mux))
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": buildinfo.String()})
}

func ready(store readinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	}
}

func listEnvironments(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.ListEnvironmentsAPI(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os ambientes.")
			return
		}
		if items == nil {
			items = []repository.Environment{}
		}
		if user := requestIdentity(r); user != nil && user.Role != "operator" {
			visible := make([]repository.Environment, 0, len(items))
			for _, item := range items {
				if hasEnvironment(user, item.ID) {
					visible = append(visible, item)
				}
			}
			items = visible
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

type environmentCreator interface {
	CreateEnvironmentLabel(ctx context.Context, name, envType, discoveryMode string, active bool) (pgtype.UUID, string, error)
}

func createEnvironmentLabel(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := requestIdentity(r)
		if user == nil || user.Role != "operator" {
			writeError(w, http.StatusForbidden, CodeUnavailable, "Somente um operador cria o rótulo do ambiente.")
			return
		}
		creator, ok := store.(environmentCreator)
		if !ok {
			writeError(w, http.StatusNotImplemented, CodeInternal, "Este servidor não cria ambientes.")
			return
		}
		var body struct {
			Name          string `json:"name"`
			Type          string `json:"type"`
			DiscoveryMode string `json:"discovery_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Informe o nome do ambiente. A conexão fica no segredo do servidor.")
			return
		}
		if body.Type == "" {
			body.Type = "self_hosted"
		}
		if body.DiscoveryMode == "" {
			body.DiscoveryMode = "single_database"
		}
		id, name, err := creator.CreateEnvironmentLabel(r.Context(), strings.TrimSpace(body.Name), body.Type, body.DiscoveryMode, true)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível criar o ambiente.")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": id.String(), "name": name, "dsn": nil,
		})
	}
}

func listDatabases(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListDatabaseSnapshots(ctx, id)
		if items == nil {
			items = []repository.DatabaseSnapshot{}
		}
		return items, err
	}, "Não foi possível listar os databases.")
}

func listSchemas(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListSchemaSnapshots(ctx, id)
		if items == nil {
			items = []repository.SchemaSnapshot{}
		}
		return items, err
	}, "Não foi possível listar os schemas.")
}

func listHypertables(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListHypertableSnapshots(ctx, id)
		if items == nil {
			items = []repository.HypertableSnapshotRow{}
		}
		return items, err
	}, "Não foi possível listar as hypertables.")
}

func listDimensions(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListDimensionSnapshots(ctx, id)
		if items == nil {
			items = []repository.DimensionSnapshotRow{}
		}
		return items, err
	}, "Não foi possível listar as dimensions.")
}

func listChunks(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListChunkSnapshots(ctx, id)
		if items == nil {
			items = []repository.ChunkSnapshotRow{}
		}
		return items, err
	}, "Não foi possível listar os chunks.")
}

func listCAGGs(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListCAGGSnapshots(ctx, id)
		if items == nil {
			items = []repository.CAGGSnapshotRow{}
		}
		return items, err
	}, "Não foi possível listar continuous aggregates.")
}

func listJobs(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListJobSnapshots(ctx, id)
		if items == nil {
			items = []repository.JobSnapshotRow{}
		}
		return items, err
	}, "Não foi possível listar as policies.")
}

func listPolicies(store InventoryStore) http.HandlerFunc {
	return envItems(store, func(ctx context.Context, id string) (any, error) {
		items, err := store.ListPolicySnapshots(ctx, id)
		if items == nil {
			items = []repository.PolicySnapshotRow{}
		}
		return items, err
	}, "Não foi possível listar as policies.")
}

func envItems(
	_ InventoryStore,
	load func(ctx context.Context, id string) (any, error),
	errMsg string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "Identificador do ambiente é obrigatório.")
			return
		}
		items, err := load(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, errMsg)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
