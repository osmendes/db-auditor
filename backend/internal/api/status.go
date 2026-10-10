package api

import (
	"context"
	"net/http"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/buildinfo"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

// StatusResponse is the operational snapshot for the Status page.
type StatusResponse struct {
	Service      string                       `json:"service"`
	Version      string                       `json:"version"`
	TimeUTC      time.Time                    `json:"time_utc"`
	API          string                       `json:"api_status"`
	Database     string                       `json:"database_status"`
	Environments int                          `json:"environments_count"`
	RecentRuns   []StatusRun                  `json:"recent_runs"`
	OpenFindings int                          `json:"open_findings"`
	FailedRuns   int                          `json:"failed_runs_recent"`
	Notes        []string                     `json:"notes,omitempty"`
	Storage      *repository.StorageFootprint `json:"storage,omitempty"`
}

// StatusRun is a compact audit run summary.
type StatusRun struct {
	ID            string     `json:"id"`
	EnvironmentID string     `json:"environment_id"`
	Profile       string     `json:"profile"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

func registerStatusRoutes(mux *http.ServeMux, store InventoryStore) {
	mux.HandleFunc("GET /api/v1/status", getStatus(store))
}

func getStatus(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		res := StatusResponse{
			Service: "timescale-auditor",
			Version: buildinfo.String(),
			TimeUTC: time.Now().UTC(),
			API:     "ok",
		}

		if err := store.Ping(ctx); err != nil {
			res.Database = "unavailable"
			res.Notes = append(res.Notes, "snapshot store ping failed")
		} else {
			res.Database = "ready"
			if stats, ok := store.(interface {
				GetStorageFootprint(context.Context) (*repository.StorageFootprint, error)
			}); ok {
				footprint, err := stats.GetStorageFootprint(ctx)
				if err != nil {
					res.Notes = append(res.Notes, "control store size unavailable")
				} else {
					res.Storage = footprint
				}
			}
		}

		envs, err := store.ListEnvironmentsAPI(ctx)
		if err != nil {
			res.Notes = append(res.Notes, "list environments failed")
		} else {
			res.Environments = len(envs)
		}

		runs, err := store.ListAuditRuns(ctx, "", "", "", 10)
		if err != nil {
			res.Notes = append(res.Notes, "list audit runs failed")
		} else {
			res.RecentRuns = make([]StatusRun, 0, len(runs))
			for _, run := range runs {
				sr := StatusRun{
					ID:            run.ID,
					EnvironmentID: run.EnvironmentID,
					Profile:       run.Profile,
					Status:        run.Status,
					StartedAt:     run.StartedAt,
				}
				if run.FinishedAt != nil {
					sr.FinishedAt = run.FinishedAt
				}
				res.RecentRuns = append(res.RecentRuns, sr)
				if run.Status == "FAILED" {
					res.FailedRuns++
				}
			}
		}

		findings, err := store.ListFindings(ctx, "", "", "", "open", 500)
		if err != nil {
			res.Notes = append(res.Notes, "list findings failed")
		} else {
			res.OpenFindings = len(findings)
		}

		writeJSON(w, http.StatusOK, res)
	}
}
