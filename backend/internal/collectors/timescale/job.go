package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// JobFacts is one timescaledb_information.jobs row.
type JobFacts struct {
	DatabaseName         string     `json:"database_name"`
	JobID                int64      `json:"job_id"`
	ApplicationName      string     `json:"application_name"`
	ScheduleInterval     string     `json:"schedule_interval"`
	MaxRuntime           string     `json:"max_runtime"`
	MaxRetries           int        `json:"max_retries"`
	RetryPeriod          string     `json:"retry_period"`
	ProcSchema           string     `json:"proc_schema"`
	ProcName             string     `json:"proc_name"`
	Owner                string     `json:"owner_name"`
	Scheduled            bool       `json:"scheduled"`
	FixedSchedule        bool       `json:"fixed_schedule"`
	ConfigJSON           *string    `json:"config_json,omitempty"`
	NextStart            *time.Time `json:"next_start,omitempty"`
	InitialStart         *time.Time `json:"initial_start,omitempty"`
	HypertableSchema     *string    `json:"hypertable_schema,omitempty"`
	HypertableName       *string    `json:"hypertable_name,omitempty"`
	CheckSchema          *string    `json:"check_schema,omitempty"`
	CheckName            *string    `json:"check_name,omitempty"`
	LastRunStatus        string     `json:"last_run_status,omitempty"`
	TotalFailures        int64      `json:"total_failures"`
	LastRunDuration      string     `json:"last_run_duration,omitempty"`
	MaxBackgroundWorkers int        `json:"max_background_workers,omitempty"`
}

// CollectJobs lists background jobs. Schema scope filters jobs tied to a hypertable schema when present.
// Returns an empty slice when timescaledb is not installed in this database.
func CollectJobs(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]JobFacts, error) {
	ok, err := ensureExtension(ctx, conn, "jobs collector")
	if err != nil {
		return nil, err
	}
	if !ok {
		return []JobFacts{}, nil
	}
	_, _ = conn.Exec(ctx, "SET statement_timeout = '120s'")

	rows, err := conn.Query(ctx, jobsSQL)
	if err != nil {
		return nil, fmt.Errorf("jobs collector: %w", err)
	}
	defer rows.Close()

	out := make([]JobFacts, 0)
	for rows.Next() {
		var f JobFacts
		var configJSON, htSchema, htName, checkSchema, checkName *string
		var nextStart, initialStart *time.Time
		if err := rows.Scan(
			&f.DatabaseName,
			&f.JobID,
			&f.ApplicationName,
			&f.ScheduleInterval,
			&f.MaxRuntime,
			&f.MaxRetries,
			&f.RetryPeriod,
			&f.ProcSchema,
			&f.ProcName,
			&f.Owner,
			&f.Scheduled,
			&f.FixedSchedule,
			&configJSON,
			&nextStart,
			&initialStart,
			&htSchema,
			&htName,
			&checkSchema,
			&checkName,
			&f.LastRunStatus,
			&f.TotalFailures,
			&f.LastRunDuration,
			&f.MaxBackgroundWorkers,
		); err != nil {
			return nil, fmt.Errorf("jobs collector scan: %w", err)
		}
		f.ConfigJSON = configJSON
		f.NextStart = nextStart
		f.InitialStart = initialStart
		f.HypertableSchema = htSchema
		f.HypertableName = htName
		f.CheckSchema = checkSchema
		f.CheckName = checkName
		if htSchema != nil && *htSchema != "" && !scope.AllowsSchema(*htSchema) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("jobs collector rows: %w", err)
	}
	return out, nil
}
