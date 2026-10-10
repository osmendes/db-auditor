package timescale

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// ContinuousAggregateFacts is one continuous aggregate row.
type ContinuousAggregateFacts struct {
	DatabaseName              string `json:"database_name"`
	SchemaName                string `json:"schema_name"`
	ViewName                  string `json:"view_name"`
	Owner                     string `json:"owner_name"`
	MaterializationSchema     string `json:"materialization_schema"`
	MaterializationHypertable string `json:"materialization_hypertable"`
	MaterializedOnly          bool   `json:"materialized_only"`
	CompressionEnabled        bool   `json:"compression_enabled"`
	Finalized                 *bool  `json:"finalized,omitempty"`
	SourceHypertableSchema    string `json:"source_hypertable_schema,omitempty"`
	SourceHypertableName      string `json:"source_hypertable_name,omitempty"`
	BucketInterval            string `json:"bucket_interval,omitempty"`
	ViewDefinition            string `json:"view_definition"`
	LagInterval               string `json:"lag_interval,omitempty"`
}

// CollectContinuousAggregates lists CAGGs and applies schema scope on view schema.
// Returns an empty slice when timescaledb is not installed in this database.
func CollectContinuousAggregates(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]ContinuousAggregateFacts, error) {
	ok, err := ensureExtension(ctx, conn, "cagg collector")
	if err != nil {
		return nil, err
	}
	if !ok {
		return []ContinuousAggregateFacts{}, nil
	}

	rows, err := conn.Query(ctx, continuousAggregatesSQL)
	if err != nil {
		return nil, fmt.Errorf("cagg collector: %w", err)
	}
	defer rows.Close()

	out := make([]ContinuousAggregateFacts, 0)
	for rows.Next() {
		var f ContinuousAggregateFacts
		var finalized *bool
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.ViewName,
			&f.Owner,
			&f.MaterializationSchema,
			&f.MaterializationHypertable,
			&f.MaterializedOnly,
			&f.CompressionEnabled,
			&finalized,
			&f.SourceHypertableSchema,
			&f.SourceHypertableName,
			&f.BucketInterval,
			&f.ViewDefinition,
		); err != nil {
			return nil, fmt.Errorf("cagg collector scan: %w", err)
		}
		f.Finalized = finalized
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cagg collector rows: %w", err)
	}
	attachCAGGLag(ctx, conn, out)
	return out, nil
}

func attachCAGGLag(ctx context.Context, conn *pgx.Conn, items []ContinuousAggregateFacts) {
	if len(items) == 0 {
		return
	}
	queries := []string{
		`SELECT ca.view_schema, ca.view_name, COALESCE((now() - _timescaledb_functions.to_timestamp(_timescaledb_functions.cagg_watermark(format('%I.%I', ca.view_schema, ca.view_name)::regclass)))::text, '') FROM timescaledb_information.continuous_aggregates ca`,
		`SELECT ca.view_schema, ca.view_name, COALESCE((now() - _timescaledb_internal.to_timestamp(_timescaledb_internal.cagg_watermark(format('%I.%I', ca.view_schema, ca.view_name)::regclass)))::text, '') FROM timescaledb_information.continuous_aggregates ca`,
	}
	for _, query := range queries {
		rows, err := conn.Query(ctx, query)
		if err != nil {
			continue
		}
		lag := map[string]string{}
		for rows.Next() {
			var schema, name, value string
			if err = rows.Scan(&schema, &name, &value); err != nil {
				rows.Close()
				lag = nil
				break
			}
			lag[schema+"."+name] = value
		}
		if rows.Err() != nil || lag == nil {
			rows.Close()
			continue
		}
		rows.Close()
		for i := range items {
			items[i].LagInterval = lag[items[i].SchemaName+"."+items[i].ViewName]
		}
		return
	}
}
