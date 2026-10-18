package audit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/osmendes/db-auditor/internal/collectors/postgres"
	"github.com/osmendes/db-auditor/internal/collectors/timescale"
	"github.com/osmendes/db-auditor/internal/config"
)

// InventoryWriter persists collector facts into snapshot tables.
// Implemented by repository.Store.
type InventoryWriter interface {
	SaveDiscovery(ctx context.Context, environmentID, auditRunID pgtype.UUID, databases []postgres.DatabaseFacts, schemas []postgres.SchemaFacts) error
	SaveObjectInventory(ctx context.Context, environmentID, auditRunID pgtype.UUID, tables []postgres.TableFacts, columns []postgres.ColumnFacts, indexes []postgres.IndexFacts) error
	SaveExtendedObjectInventory(ctx context.Context, environmentID, auditRunID pgtype.UUID, constraints []postgres.ConstraintFacts, views []postgres.ViewFacts, functions []postgres.FunctionFacts, extensions []postgres.ExtensionFacts) error
	SaveTimescaleCoreInventory(ctx context.Context, environmentID, auditRunID pgtype.UUID, result timescale.InventoryResult) error
	SaveTimescalePolicyInventory(ctx context.Context, environmentID, auditRunID pgtype.UUID, result timescale.PolicyInventoryResult) error
	SaveOperationalInventory(ctx context.Context, environmentID, auditRunID pgtype.UUID, columnStats []postgres.ColumnStatFacts, workload []postgres.WorkloadFacts) error
}

// LiveRegistryOptions configures collectors that hit audited databases.
type LiveRegistryOptions struct {
	Targets map[string]string
	Scope   config.Scope
	Policy  config.CollectionPolicy
	Writer  InventoryWriter
}

func parseRunUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return id, fmt.Errorf("uuid inválido %q: %w", s, err)
	}
	return id, nil
}

func runIDs(ctx context.Context) (envID, runID pgtype.UUID, err error) {
	meta, ok := RunMetaFromContext(ctx)
	if !ok || meta.EnvironmentID == "" || meta.AuditRunID == "" {
		return envID, runID, fmt.Errorf("contexto da execução sem environment_id/audit_run_id")
	}
	envID, err = parseRunUUID(meta.EnvironmentID)
	if err != nil {
		return envID, runID, err
	}
	runID, err = parseRunUUID(meta.AuditRunID)
	if err != nil {
		return envID, runID, err
	}
	return envID, runID, nil
}

func dsnFromContext(ctx context.Context, targets map[string]string) (string, error) {
	meta, ok := RunMetaFromContext(ctx)
	if !ok || meta.EnvironmentID == "" {
		return "", fmt.Errorf("contexto da execução sem environment_id")
	}
	dsn := config.DSNForEnvironment(targets, meta.EnvironmentID)
	if dsn == "" {
		return "", fmt.Errorf(
			"credenciais do ambiente não configuradas. Defina %s no .env (somente leitura) e reinicie a API",
			config.TargetSlotHint(meta.EnvironmentID),
		)
	}
	return dsn, nil
}

// finishMulti returns rows and either nil, *PartialWarning, or a hard error from listing DBs.
// Per-database failures never abort the audit run: they become warnings on the collector.
func finishMulti(rows int64, partial []postgres.PartialError, hard error) (int64, error) {
	if hard != nil {
		return 0, hard
	}
	if len(partial) == 0 {
		return rows, nil
	}
	msg := postgres.FormatPartialErrors(partial, 5)
	failures := make([]CoverageFailure, 0, len(partial))
	for _, item := range partial {
		failures = append(failures, CoverageFailure{Database: item.Database, Error: item.Message})
	}
	return rows, &PartialWarning{
		Rows:     rows,
		Warning:  fmt.Sprintf("%d database(s) com falha parcial: %s", len(partial), msg),
		Failures: failures,
	}
}

// NewLiveRegistry registers collectors that connect using AUDITOR_TARGET_N_* credentials
// and optionally persist inventory snapshots when Writer is set.
// Object collectors iterate every connectable database, continuing on per-DB errors.
func NewLiveRegistry(opts LiveRegistryOptions) *Registry {
	r := NewRegistry()
	scope := opts.Scope
	targets := opts.Targets
	if targets == nil {
		targets = config.LoadTargetDSNs()
	}
	writer := opts.Writer

	connectRoot := func(ctx context.Context) (*pgx.Conn, error) {
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return nil, err
		}
		conn, err := postgres.ConnectReadOnly(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("conectar ao ambiente auditado: %w", err)
		}
		return conn, nil
	}

	must := func(name string, fn CollectorFunc) {
		_ = r.Register(CollectorSpec{
			Name:     name,
			Version:  "1.0.0",
			Profiles: liveCollectorProfiles(name),
			Run:      fn,
		})
	}

	registerPostgresCollectors(must, connectRoot, writer, scope, targets)
	registerTimescaleCollectors(must, writer, scope, targets)
	registerStatsCollectors(must, writer, scope, targets, opts.Policy)
	must("postgres.last_insertion", func(ctx context.Context) (int64, error) {
		saver, ok := writer.(interface {
			InsertionMappings(context.Context, string, string) ([]postgres.InsertionMapping, error)
			SaveInsertionProbes(context.Context, pgtype.UUID, pgtype.UUID, []postgres.InsertionProbe) error
		})
		if !ok {
			return 0, nil
		}
		dsn, err := dsnFromContext(ctx, targets)
		if err != nil {
			return 0, err
		}
		envID, runID, err := runIDs(ctx)
		if err != nil {
			return 0, err
		}
		meta, _ := RunMetaFromContext(ctx)
		var total int64
		partial, hard := postgres.ForEachUserDatabase(ctx, dsn, scope, func(cctx context.Context, conn *pgx.Conn, database string) error {
			mappings, err := saver.InsertionMappings(cctx, meta.EnvironmentID, database)
			if err != nil || len(mappings) == 0 {
				return err
			}
			selected := mappings[:0]
			for _, mapping := range mappings {
				if scope.AllowsSchema(mapping.Schema) {
					selected = append(selected, mapping)
				}
			}
			if len(selected) == 0 {
				return nil
			}
			probes, err := postgres.CollectInsertionEvidence(cctx, conn, selected)
			if err != nil {
				return err
			}
			if err := saver.SaveInsertionProbes(cctx, envID, runID, probes); err != nil {
				return err
			}
			total += int64(len(probes))
			return nil
		})
		return finishMulti(total, partial, hard)
	})

	return r
}

func liveCollectorProfiles(name string) map[string]struct{} {
	if name == "postgres.last_insertion" {
		return map[string]struct{}{ProfileManual: {}}
	}
	profiles := map[string]struct{}{ProfileManual: {}, ProfileMonthly: {}}
	weekly := map[string]struct{}{
		"postgres.server": {}, "postgres.databases": {}, "postgres.schemas": {},
		"postgres.tables": {}, "postgres.columns": {}, "postgres.indexes": {},
		"postgres.constraints": {}, "postgres.views": {}, "postgres.functions": {},
		"postgres.extensions": {}, "timescale.version": {}, "timescale.hypertables": {},
		"timescale.dimensions": {}, "timescale.chunks": {}, "timescale.continuous_aggregates": {},
		"timescale.jobs": {}, "timescale.policies": {}, "timescale.chunk_dead_tuples": {},
		"postgres.column_stats": {}, "postgres.workload": {}, "postgres.replication": {},
	}
	daily := map[string]struct{}{
		"postgres.server": {}, "postgres.databases": {}, "postgres.schemas": {},
		"postgres.tables": {}, "postgres.indexes": {}, "timescale.version": {},
		"timescale.hypertables": {}, "timescale.jobs": {}, "timescale.policies": {},
		"postgres.workload": {}, "postgres.replication": {}, "timescale.chunk_dead_tuples": {},
	}
	if _, ok := weekly[name]; ok {
		profiles[ProfileWeekly] = struct{}{}
	}
	if _, ok := daily[name]; ok {
		profiles[ProfileDaily] = struct{}{}
	}
	if name == "postgres.server" || name == "postgres.databases" {
		profiles[ProfileFast] = struct{}{}
	}
	return profiles
}
