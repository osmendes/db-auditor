package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// PolicyFacts is a policy job (retention, compression, refresh, etc.).
type PolicyFacts struct {
	DatabaseName     string     `json:"database_name"`
	JobID            int64      `json:"job_id"`
	PolicyType       string     `json:"policy_type"`
	ProcSchema       string     `json:"proc_schema"`
	ProcName         string     `json:"proc_name"`
	HypertableSchema *string    `json:"hypertable_schema,omitempty"`
	HypertableName   *string    `json:"hypertable_name,omitempty"`
	ScheduleInterval string     `json:"schedule_interval"`
	Scheduled        bool       `json:"scheduled"`
	ConfigJSON       *string    `json:"config_json,omitempty"`
	NextStart        *time.Time `json:"next_start,omitempty"`
	Owner            string     `json:"owner_name"`
}

// CollectPolicies lists retention/compression/refresh/reorder/columnstore policies.
// Returns an empty slice when timescaledb is not installed in this database.
func CollectPolicies(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]PolicyFacts, error) {
	ok, err := ensureExtension(ctx, conn, "policy collector")
	if err != nil {
		return nil, err
	}
	if !ok {
		return []PolicyFacts{}, nil
	}

	rows, err := conn.Query(ctx, policiesSQL)
	if err != nil {
		return nil, fmt.Errorf("policy collector: %w", err)
	}
	defer rows.Close()

	out := make([]PolicyFacts, 0)
	for rows.Next() {
		var f PolicyFacts
		var htSchema, htName, configJSON *string
		var nextStart *time.Time
		if err := rows.Scan(
			&f.DatabaseName,
			&f.JobID,
			&f.PolicyType,
			&f.ProcSchema,
			&f.ProcName,
			&htSchema,
			&htName,
			&f.ScheduleInterval,
			&f.Scheduled,
			&configJSON,
			&nextStart,
			&f.Owner,
		); err != nil {
			return nil, fmt.Errorf("policy collector scan: %w", err)
		}
		f.HypertableSchema = htSchema
		f.HypertableName = htName
		f.ConfigJSON = configJSON
		f.NextStart = nextStart
		if htSchema != nil && *htSchema != "" && !scope.AllowsSchema(*htSchema) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("policy collector rows: %w", err)
	}
	return out, nil
}
