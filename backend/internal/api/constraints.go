package api

import (
	"net/http"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

func listConstraints(store InventoryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, CodeEnvironmentRequired, "O identificador do ambiente é obrigatório.")
			return
		}
		q := parseInventoryQuery(r)
		items, total, err := store.ListConstraintSnapshots(r.Context(), repository.InventoryFilter{
			EnvironmentID: id,
			AuditRunID:    q.AuditRunID,
			Database:      q.Database,
			Schema:        q.Schema,
			Table:         q.Table,
			Q:             q.Q,
			Limit:         q.Limit,
			Offset:        q.Offset,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível listar as constraints.")
			return
		}
		if items == nil {
			items = []repository.ConstraintSnapshotRow{}
		}
		writePage(w, items, q.Limit, q.Offset, total)
	}
}
