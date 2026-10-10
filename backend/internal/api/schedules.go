package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mayconmendes-qc/db-auditor/internal/scheduler"
)

type scheduleAPI interface {
	ListEntries() []scheduler.Entry
	UpsertSchedule(ctx context.Context, environmentID, profile string, enabled bool) (*scheduler.Entry, error)
}

func registerScheduleRoutes(mux *http.ServeMux, runner ManualRunner) {
	api, ok := runner.(scheduleAPI)
	if !ok || api == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/environments/{id}/schedules", listSchedules(api))
	mux.HandleFunc("PUT /api/v1/environments/{id}/schedules", putSchedule(api))
}

func listSchedules(api scheduleAPI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente inválido.")
			return
		}
		items := make([]scheduler.Entry, 0)
		for _, entry := range api.ListEntries() {
			if entry.EnvironmentID == id {
				items = append(items, entry)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func putSchedule(api scheduleAPI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeError(w, http.StatusBadRequest, CodeValidation, "Ambiente inválido.")
			return
		}
		var body struct {
			Profile string `json:"profile"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		if _, ok := scheduler.ProfileInterval[body.Profile]; !ok {
			writeError(w, http.StatusBadRequest, CodeValidation, "Perfil de agenda inválido.")
			return
		}
		entry, err := api.UpsertSchedule(r.Context(), id, body.Profile, body.Enabled)
		if err != nil {
			writeError(w, http.StatusConflict, CodeConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, entry)
	}
}
