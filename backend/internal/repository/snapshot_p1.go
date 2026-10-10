package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
)

func (s *Store) enrichP1Facts(ctx context.Context, environmentID, auditRunID string, f *analyzer.SnapshotFacts) error {
	f.Vacuum = capVacuum(f.Vacuum)
	markCAGGSignals(f.CAGGs)
	if err := s.loadAuditorRoles(ctx, environmentID, auditRunID, f); err != nil {
		return err
	}
	if err := s.loadAccountRoles(ctx, environmentID, auditRunID, f); err != nil {
		return err
	}
	if err := s.loadCompressionRatios(ctx, environmentID, auditRunID, f); err != nil {
		return err
	}
	if err := s.loadReadsOutsideTime(ctx, environmentID, auditRunID, f); err != nil {
		return err
	}
	side, err := s.loadServerSide(ctx, environmentID, "", auditRunID)
	if err != nil {
		return err
	}
	f.ServerVersion = side.ServerVersion
	f.TimescaleVersion = side.TimescaleVersion
	f.Extensions = side.Extensions
	f.Settings = side.Settings
	peers, err := s.peerRuns(ctx, environmentID)
	if err != nil {
		return err
	}
	for _, peer := range peers {
		item, err := s.loadServerSide(ctx, peer.environmentID, peer.name, peer.auditRunID)
		if err != nil {
			return err
		}
		f.Peers = append(f.Peers, item)
	}
	return nil
}

func capVacuum(in []analyzer.VacuumFact) []analyzer.VacuumFact {
	const capN = 2000
	out := make([]analyzer.VacuumFact, 0, len(in))
	for _, item := range in {
		if item.Schema == "_timescaledb_internal" {
			continue
		}
		out = append(out, item)
	}
	if len(out) <= capN {
		return out
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NDeadTup > out[j].NDeadTup })
	return out[:capN]
}

func markCAGGSignals(items []analyzer.CAGGFact) {
	names := map[string]struct{}{}
	for _, item := range items {
		names[item.Schema+"."+item.ViewName] = struct{}{}
	}
	for i := range items {
		items[i].Realtime = !items[i].MaterializedOnly
		for other := range names {
			if other == items[i].Schema+"."+items[i].ViewName {
				continue
			}
			if strings.Contains(items[i].ViewDefinition, other) {
				items[i].Hierarchical = true
			}
		}
	}
}

func (s *Store) loadAuditorRoles(ctx context.Context, environmentID, auditRunID string, f *analyzer.SnapshotFacts) error {
	rows, err := s.pool.Query(ctx, `SELECT database_name, role_name, is_superuser, can_create_db, can_create_role, replication, can_write, check_failed
FROM auditor_privilege_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid`, environmentID, auditRunID)
	if err != nil {
		return fmt.Errorf("auditor privilege snapshot: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role analyzer.RoleFact
		if err := rows.Scan(&role.Database, &role.RoleName, &role.Superuser, &role.CreateDB, &role.CreateRole, &role.Replication, &role.CanWrite, &role.PrivilegeCheckFailed); err != nil {
			return err
		}
		role.Current = true
		f.Roles = append(f.Roles, role)
	}
	return rows.Err()
}

func (s *Store) loadCompressionRatios(ctx context.Context, environmentID, auditRunID string, f *analyzer.SnapshotFacts) error {
	rows, err := s.pool.Query(ctx, `SELECT database_name, schema_name, hypertable_name,
CASE WHEN sum(after_compression_bytes) > 0 THEN sum(before_compression_bytes)::float8 / sum(after_compression_bytes)::float8 ELSE 0 END
FROM chunk_snapshot
WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND before_compression_bytes > 0 AND after_compression_bytes > 0
GROUP BY 1, 2, 3`, environmentID, auditRunID)
	if err != nil {
		return fmt.Errorf("compression ratio: %w", err)
	}
	defer rows.Close()
	ratios := map[string]float64{}
	for rows.Next() {
		var db, schema, name string
		var ratio float64
		if err := rows.Scan(&db, &schema, &name, &ratio); err != nil {
			return err
		}
		ratios[db+"."+schema+"."+name] = ratio
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range f.Policies {
		key := fmt.Sprintf("%s.%s.%s", f.Policies[i].Database, f.Policies[i].HypertableSchema, f.Policies[i].HypertableName)
		if ratio := ratios[key]; ratio > 0 {
			f.Policies[i].CompressionRatio = ratio
		}
	}
	return nil
}

func (s *Store) loadReadsOutsideTime(ctx context.Context, environmentID, auditRunID string, f *analyzer.SnapshotFacts) error {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT h.database_name, h.schema_name, h.hypertable_name
FROM hypertable_snapshot h
JOIN dimension_snapshot d ON d.audit_run_id = h.audit_run_id AND d.database_name = h.database_name
  AND d.schema_name = h.schema_name AND d.hypertable_name = h.hypertable_name AND COALESCE(d.column_name, '') <> ''
WHERE h.environment_id=$1::uuid AND h.audit_run_id=$2::uuid
AND EXISTS (
  SELECT 1 FROM workload_snapshot w
  WHERE w.audit_run_id = h.audit_run_id AND w.database_name = h.database_name AND w.calls > 0
  AND ((h.schema_name || '.' || h.hypertable_name) = ANY (w.referenced_objects) OR w.query_fingerprint ILIKE '%' || h.hypertable_name || '%')
  AND w.query_fingerprint NOT ILIKE '%' || d.column_name || '%'
)`, environmentID, auditRunID)
	if err != nil {
		return fmt.Errorf("reads outside time: %w", err)
	}
	defer rows.Close()
	outside := map[string]bool{}
	for rows.Next() {
		var db, schema, name string
		if err := rows.Scan(&db, &schema, &name); err != nil {
			return err
		}
		outside[db+"."+schema+"."+name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range f.Hypertables {
		key := fmt.Sprintf("%s.%s.%s", f.Hypertables[i].Database, f.Hypertables[i].Schema, f.Hypertables[i].Name)
		if outside[key] {
			f.Hypertables[i].ReadsOutsideTime = true
		}
	}
	return nil
}

type peerRun struct {
	environmentID string
	name          string
	auditRunID    string
}

func (s *Store) peerRuns(ctx context.Context, environmentID string) ([]peerRun, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.id::text, e.name, r.id::text
FROM audit_environment e
JOIN LATERAL (
  SELECT id FROM audit_run
  WHERE environment_id = e.id AND status IN ('success', 'partial_success')
  ORDER BY COALESCE(finished_at, started_at) DESC
  LIMIT 1
) r ON true
WHERE e.id <> $1::uuid AND e.active`, environmentID)
	if err != nil {
		return nil, fmt.Errorf("peer snapshots: %w", err)
	}
	defer rows.Close()
	var out []peerRun
	for rows.Next() {
		var item peerRun
		if err := rows.Scan(&item.environmentID, &item.name, &item.auditRunID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// LoadServerSide reads the latest successful snapshot of one environment. It does not open a target session.
func (s *Store) LoadServerSide(ctx context.Context, environmentID string) (analyzer.ServerSide, error) {
	var name, runID string
	err := s.pool.QueryRow(ctx, `SELECT e.name, r.id::text
FROM audit_environment e
JOIN LATERAL (
  SELECT id FROM audit_run
  WHERE environment_id = e.id AND status IN ('success', 'partial_success')
  ORDER BY COALESCE(finished_at, started_at) DESC
  LIMIT 1
) r ON true
WHERE e.id = $1::uuid`, environmentID).Scan(&name, &runID)
	if err != nil {
		return analyzer.ServerSide{}, fmt.Errorf("snapshot do ambiente: %w", err)
	}
	return s.loadServerSide(ctx, environmentID, name, runID)
}

func (s *Store) loadServerSide(ctx context.Context, environmentID, name, auditRunID string) (analyzer.ServerSide, error) {
	side := analyzer.ServerSide{EnvironmentID: environmentID, Name: name, AuditRunID: auditRunID, Settings: map[string]string{}}
	rows, err := s.pool.Query(ctx, `SELECT setting_name, max(setting_value) FROM server_setting_snapshot WHERE audit_run_id=$1::uuid GROUP BY setting_name`, auditRunID)
	if err != nil {
		return side, fmt.Errorf("server settings: %w", err)
	}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			rows.Close()
			return side, err
		}
		if key == "server_version" {
			side.ServerVersion = value
			continue
		}
		side.Settings[key] = value
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return side, err
	}
	rows.Close()

	versionRows, err := s.pool.Query(ctx, `SELECT DISTINCT extension_version FROM timescale_version_snapshot WHERE audit_run_id=$1::uuid AND COALESCE(extension_version,'') <> ''`, auditRunID)
	if err != nil {
		return side, err
	}
	var versions []string
	for versionRows.Next() {
		var version string
		if err = versionRows.Scan(&version); err != nil {
			versionRows.Close()
			return side, err
		}
		versions = append(versions, version)
	}
	versionRows.Close()
	sort.Strings(versions)
	side.TimescaleVersion = strings.Join(versions, ",")

	extRows, err := s.pool.Query(ctx, `SELECT database_name, extension_name, COALESCE(extension_version,'') FROM extension_snapshot WHERE audit_run_id=$1::uuid`, auditRunID)
	if err != nil {
		return side, err
	}
	for extRows.Next() {
		var item analyzer.ExtensionFact
		if err = extRows.Scan(&item.Database, &item.Name, &item.Version); err != nil {
			extRows.Close()
			return side, err
		}
		side.Extensions = append(side.Extensions, item)
	}
	extRows.Close()

	htRows, err := s.pool.Query(ctx, `SELECT h.database_name, h.schema_name, h.hypertable_name,
COALESCE((SELECT d.time_interval FROM dimension_snapshot d
  WHERE d.audit_run_id=h.audit_run_id AND d.database_name=h.database_name AND d.schema_name=h.schema_name AND d.hypertable_name=h.hypertable_name AND COALESCE(d.time_interval,'')<>''
  ORDER BY d.dimension_number LIMIT 1), '')
FROM hypertable_snapshot h WHERE h.audit_run_id=$1::uuid`, auditRunID)
	if err != nil {
		return side, err
	}
	for htRows.Next() {
		var item analyzer.HypertableFact
		if err = htRows.Scan(&item.Database, &item.Schema, &item.Name, &item.ChunkInterval); err != nil {
			htRows.Close()
			return side, err
		}
		side.Hypertables = append(side.Hypertables, item)
	}
	htRows.Close()

	polRows, err := s.pool.Query(ctx, `SELECT database_name, policy_type, COALESCE(hypertable_schema,''), COALESCE(hypertable_name,''), COALESCE(config_json,'') FROM policy_snapshot WHERE audit_run_id=$1::uuid`, auditRunID)
	if err != nil {
		return side, err
	}
	for polRows.Next() {
		var item analyzer.PolicyFact
		if err = polRows.Scan(&item.Database, &item.PolicyType, &item.HypertableSchema, &item.HypertableName, &item.Config); err != nil {
			polRows.Close()
			return side, err
		}
		side.Policies = append(side.Policies, item)
	}
	polRows.Close()

	caggRows, err := s.pool.Query(ctx, `SELECT c.database_name, c.schema_name, c.view_name, COALESCE(c.lag_interval,''), c.materialized_only, c.view_definition,
EXISTS(SELECT 1 FROM policy_snapshot p WHERE p.audit_run_id=c.audit_run_id AND p.database_name=c.database_name AND p.policy_type='refresh'
  AND ((p.hypertable_schema=c.schema_name AND p.hypertable_name=c.view_name) OR (p.hypertable_schema=c.materialization_schema AND p.hypertable_name=c.materialization_hypertable)))
FROM continuous_aggregate_snapshot c WHERE c.audit_run_id=$1::uuid`, auditRunID)
	if err != nil {
		return side, err
	}
	for caggRows.Next() {
		var item analyzer.CAGGFact
		if err = caggRows.Scan(&item.Database, &item.Schema, &item.ViewName, &item.Lag, &item.MaterializedOnly, &item.ViewDefinition, &item.HasRefreshPolicy); err != nil {
			caggRows.Close()
			return side, err
		}
		side.CAGGs = append(side.CAGGs, item)
	}
	caggRows.Close()
	markCAGGSignals(side.CAGGs)

	critRows, err := s.pool.Query(ctx, `SELECT f.dedup_key, e.title, e.severity
FROM finding_event e JOIN finding f ON f.id = e.finding_id
WHERE e.audit_run_id=$1::uuid AND e.event_type='observed' AND e.severity IN ('high','critical')
AND NOT EXISTS (
  SELECT 1 FROM finding_event prev
  WHERE prev.finding_id = e.finding_id AND prev.event_type='observed'
    AND prev.audit_run_id = (
      SELECT id FROM audit_run
      WHERE environment_id=$2::uuid AND status IN ('success','partial_success') AND id <> $1::uuid
      ORDER BY COALESCE(finished_at, started_at) DESC
      LIMIT 1
    )
)`, auditRunID, environmentID)
	if err != nil {
		return side, err
	}
	defer critRows.Close()
	for critRows.Next() {
		var item analyzer.CriticalFact
		if err = critRows.Scan(&item.DedupKey, &item.Title, &item.Severity); err != nil {
			return side, err
		}
		side.Critical = append(side.Critical, item)
	}
	return side, critRows.Err()
}
