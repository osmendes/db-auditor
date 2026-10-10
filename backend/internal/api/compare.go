package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/comparison"
)

type compareBody struct {
	SourceRunID string                  `json:"source_run_id"`
	TargetRunID string                  `json:"target_run_id"`
	Source      []comparison.ObjectItem `json:"source"`
	Target      []comparison.ObjectItem `json:"target"`
	Statuses    []string                `json:"statuses"`
	ObjectType  string                  `json:"object_type"`
}

func registerCompareRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/compare", compareHandler())
}

func compareHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body compareBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "Corpo da requisição inválido.")
			return
		}
		if len(body.Source) == 0 && len(body.Target) == 0 {
			writeError(w, http.StatusBadRequest, CodeValidation, "É necessário informar objetos de origem ou destino.")
			return
		}
		res := comparison.CompareSets(body.Source, body.Target)
		res.SourceRunID = body.SourceRunID
		res.TargetRunID = body.TargetRunID
		objects := res.Objects
		if len(body.Statuses) > 0 {
			objects = comparison.FilterByStatus(objects, body.Statuses)
		}
		if strings.TrimSpace(body.ObjectType) != "" {
			objects = comparison.FilterByObjectType(objects, body.ObjectType)
		}
		res.Objects = objects
		res.Summary = comparison.Summary{}
		for _, o := range objects {
			res.Summary.Total++
			switch o.Status {
			case comparison.StatusMatch:
				res.Summary.Match++
			case comparison.StatusDrift:
				res.Summary.Drift++
			case comparison.StatusOnlySource:
				res.Summary.OnlySource++
			case comparison.StatusOnlyTarget:
				res.Summary.OnlyTarget++
			default:
				res.Summary.Unknown++
			}
		}
		writeJSON(w, http.StatusOK, res)
	}
}
