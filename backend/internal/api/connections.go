package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// ConnectionStatus is the probe result for one audited environment.
type ConnectionStatus struct {
	EnvironmentID   string `json:"environment_id"`
	EnvironmentName string `json:"environment_name"`
	DSNConfigured   bool   `json:"dsn_configured"`
	Reachable       bool   `json:"reachable"`
	LatencyMS       int64  `json:"latency_ms,omitempty"`
	ServerVersion   string `json:"server_version,omitempty"`
	Error           string `json:"error,omitempty"`
}

func registerConnectionRoutes(mux *http.ServeMux, store InventoryStore, targets map[string]string) {
	mux.HandleFunc("GET /api/v1/environments/connection-status", listConnectionStatus(store, targets))
}

func listConnectionStatus(store InventoryStore, targets map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		envs, err := store.ListEnvironmentsAPI(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar os ambientes.")
			return
		}
		if targets == nil {
			targets = config.LoadTargetDSNs()
		}
		out := make([]ConnectionStatus, 0, len(envs))
		for _, env := range envs {
			item := ConnectionStatus{
				EnvironmentID:   env.ID,
				EnvironmentName: env.Name,
			}
			dsn := config.DSNForEnvironment(targets, env.ID)
			if dsn == "" {
				item.DSNConfigured = false
				item.Error = "Credenciais não configuradas: " + config.TargetSlotHint(env.ID)
				out = append(out, item)
				continue
			}
			item.DSNConfigured = true
			start := time.Now()
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			conn, err := pgx.Connect(ctx, dsn)
			if err != nil {
				cancel()
				item.Reachable = false
				item.Error = config.SanitizeDSN(err.Error())
				out = append(out, item)
				continue
			}
			var version string
			_ = conn.QueryRow(ctx, "SELECT current_setting('server_version')").Scan(&version)
			_ = conn.Close(ctx)
			cancel()
			item.Reachable = true
			item.LatencyMS = time.Since(start).Milliseconds()
			item.ServerVersion = version
			out = append(out, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}
}
