package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrMonitoringScope = errors.New("monitoring scope does not belong to environment")

type AuditAnnotation struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environment_id"`
	AuditRunID    *string   `json:"audit_run_id,omitempty"`
	Kind          string    `json:"kind"`
	Note          string    `json:"note"`
	Actor         string    `json:"actor"`
	OccurredAt    time.Time `json:"occurred_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type RegressionAlert struct {
	ID            string          `json:"id"`
	EnvironmentID string          `json:"environment_id"`
	BaselineRunID string          `json:"baseline_run_id"`
	AuditRunID    string          `json:"audit_run_id"`
	Category      string          `json:"category"`
	Evidence      json.RawMessage `json:"evidence"`
	Status        string          `json:"status"`
	Reason        string          `json:"reason"`
	CreatedAt     time.Time       `json:"created_at"`
}

func (s *Store) CreateAuditAnnotation(ctx context.Context, env, run, kind, note, actor string, at time.Time) (*AuditAnnotation, error) {
	var valid bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_environment e WHERE e.id=$1::uuid
 AND ($2='' OR EXISTS(SELECT 1 FROM audit_run r WHERE r.id=$2::uuid AND r.environment_id=e.id)))`, env, run).Scan(&valid)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrMonitoringScope
	}
	var item AuditAnnotation
	err = s.pool.QueryRow(ctx, `INSERT INTO audit_annotation(environment_id,audit_run_id,kind,note,actor,occurred_at)
VALUES($1::uuid,NULLIF($2,'')::uuid,$3,$4,$5,$6)
RETURNING id::text,environment_id::text,audit_run_id::text,kind,note,actor,occurred_at,created_at`,
		env, run, kind, note, actor, at).Scan(&item.ID, &item.EnvironmentID, &item.AuditRunID, &item.Kind, &item.Note, &item.Actor, &item.OccurredAt, &item.CreatedAt)
	return &item, err
}

func (s *Store) ListAuditAnnotations(ctx context.Context, env string) ([]AuditAnnotation, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text,environment_id::text,audit_run_id::text,kind,note,actor,occurred_at,created_at
FROM audit_annotation WHERE environment_id=$1::uuid ORDER BY occurred_at DESC,id DESC LIMIT 100`, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AuditAnnotation{}
	for rows.Next() {
		var item AuditAnnotation
		if err = rows.Scan(&item.ID, &item.EnvironmentID, &item.AuditRunID, &item.Kind, &item.Note, &item.Actor, &item.OccurredAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListRegressionAlerts(ctx context.Context, env string) ([]RegressionAlert, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text,environment_id::text,baseline_run_id::text,audit_run_id::text,category,evidence,status,reason,created_at
FROM regression_alert WHERE environment_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 100`, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RegressionAlert{}
	for rows.Next() {
		var item RegressionAlert
		if err = rows.Scan(&item.ID, &item.EnvironmentID, &item.BaselineRunID, &item.AuditRunID, &item.Category, &item.Evidence, &item.Status, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) AcknowledgeRegressionAlert(ctx context.Context, env, id, actor, reason string) error {
	cmd, err := s.pool.Exec(ctx, `UPDATE regression_alert SET status='acknowledged',reason=$4,acknowledged_by=$3,acknowledged_at=now()
WHERE environment_id=$1::uuid AND id=$2::uuid AND status='open'`, env, id, actor, reason)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrMonitoringScope
	}
	return nil
}

// EvaluateRegressionAlerts records only persistent regressions from complete,
// analyzed runs. Missing/partial collections, a temporal gap, an incomplete run
// in between, or a counter reset can never produce an alert. See
// docs/rules.md "Baseline e regressões" for the threshold and deduplication.
func (s *Store) EvaluateRegressionAlerts(ctx context.Context, env, run string) error {
	outcomes, err := s.EvaluateRegressions(ctx, env, run)
	for category, outcome := range outcomes {
		if outcome != RegressionRecorded {
			slog.Debug("regression silenced", "environment_id", env, "audit_run_id", run, "category", category, "reason", outcome)
		}
	}
	return err
}

// EvaluateRegressions returns, per category ("findings", "structure",
// "workload"), either RegressionRecorded or the reason the pair was silenced.
func (s *Store) EvaluateRegressions(ctx context.Context, env, run string) (map[string]string, error) {
	silence := func(reason string) map[string]string {
		return map[string]string{"findings": reason, "structure": reason, "workload": reason}
	}
	current, err := s.GetSnapshotCompleteness(ctx, env, run)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Completeness != "complete" {
		return silence(RegressionPartial), nil
	}
	var started time.Time
	var analysis string
	err = s.pool.QueryRow(ctx, `SELECT r.started_at,COALESCE(a.status,'') FROM audit_run r LEFT JOIN analysis_run a ON a.audit_run_id=r.id
WHERE r.environment_id=$1::uuid AND r.id=$2::uuid AND r.status='success'`, env, run).Scan(&started, &analysis)
	if errors.Is(err, pgx.ErrNoRows) {
		return silence(RegressionPartial), nil
	}
	if err != nil {
		return nil, err
	}
	if analysis != "success" {
		return silence(RegressionPartial), nil
	}
	var prior string
	var priorStarted time.Time
	err = s.pool.QueryRow(ctx, `SELECT id::text,started_at FROM audit_run WHERE environment_id=$1::uuid AND status='success' AND started_at<$2 ORDER BY started_at DESC,id DESC LIMIT 1`, env, started).Scan(&prior, &priorStarted)
	if errors.Is(err, pgx.ErrNoRows) {
		return silence(RegressionNoPrevious), nil
	}
	if err != nil {
		return nil, err
	}
	previous, err := s.GetSnapshotCompleteness(ctx, env, prior)
	if err != nil {
		return nil, err
	}
	if previous == nil || !AllowRegressionAlert(current.Completeness, previous.Completeness) {
		return silence(RegressionPartial), nil
	}
	var previousAnalysis string
	if err = s.pool.QueryRow(ctx, `SELECT COALESCE((SELECT status FROM analysis_run WHERE audit_run_id=$1::uuid),'')`, prior).Scan(&previousAnalysis); err != nil {
		return nil, err
	}
	if previousAnalysis != "success" {
		return silence(RegressionPartial), nil
	}
	pair := RegressionPair{PriorStarted: priorStarted, CurrentStarted: started}
	if err = s.pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM audit_run WHERE environment_id=$1::uuid AND started_at>$2 AND started_at<$3 AND status IN ('partial_success','failed','cancelled')),
 (SELECT max(stats_reset) FROM table_snapshot WHERE audit_run_id=$4::uuid),
 (SELECT max(stats_reset) FROM table_snapshot WHERE audit_run_id=$5::uuid),
 (SELECT max(stats_reset) FROM workload_snapshot WHERE audit_run_id=$4::uuid),
 (SELECT max(stats_reset) FROM workload_snapshot WHERE audit_run_id=$5::uuid)`, env, priorStarted, started, prior, run).Scan(&pair.IncompleteBetween, &pair.PriorStatsReset, &pair.CurrentStatsReset, &pair.PriorWorkloadReset, &pair.CurrentWorkloadReset); err != nil {
		return nil, err
	}
	maxGap := RegressionMaxGap()
	if ok, reason := PairComparable(pair, maxGap, false); !ok {
		return silence(reason), nil
	}
	var baseline string
	err = s.pool.QueryRow(ctx, `SELECT audit_run_id::text FROM audit_baseline WHERE environment_id=$1::uuid AND database_name='' AND schema_name='' AND table_name=''`, env).Scan(&baseline)
	if errors.Is(err, pgx.ErrNoRows) {
		return silence(RegressionNoBaseline), nil
	}
	if err != nil {
		return nil, err
	}
	if baseline == run || baseline == prior {
		return silence(RegressionNoBaseline), nil
	}
	// A reference chosen after the current run, or a change in collection or
	// rule versions, cannot establish a persistent regression.
	var compatible bool
	if err = s.pool.QueryRow(ctx, `SELECT count(*)=3 AND count(DISTINCT r.profile)=1
 AND count(DISTINCT r.collector_version)=1
 AND count(DISTINCT COALESCE(NULLIF(a.rule_manifest_hash,''),a.analyzer_version,''))=1
 AND bool_and(a.status='success')
 AND max(r.started_at)=(SELECT started_at FROM audit_run WHERE id=$3::uuid)
 FROM audit_run r JOIN analysis_run a ON a.audit_run_id=r.id
 WHERE r.id IN ($1::uuid,$2::uuid,$3::uuid)`, baseline, prior, run).Scan(&compatible); err != nil {
		return nil, err
	}
	if !compatible {
		return silence(RegressionIncompatible), nil
	}
	baseCoverage, err := s.GetSnapshotCompleteness(ctx, env, baseline)
	if err != nil {
		return nil, err
	}
	if baseCoverage == nil || baseCoverage.Completeness != "complete" {
		return silence(RegressionPartial), nil
	}
	out := map[string]string{}
	countersOK, countersReason := PairComparable(pair, maxGap, true)
	if !countersOK {
		out["findings"] = countersReason
		out["workload"] = countersReason
	} else {
		base, err := s.regressionMetric(ctx, "findings", baseline, baseline)
		if err != nil {
			return nil, err
		}
		previousMetric, err := s.regressionMetric(ctx, "findings", baseline, prior)
		if err != nil {
			return nil, err
		}
		currentMetric, err := s.regressionMetric(ctx, "findings", baseline, run)
		if err != nil {
			return nil, err
		}
		out["findings"], err = s.recordRegression(ctx, env, baseline, run, "findings", base.count,
			PersistentRegression(base.count, previousMetric.count, currentMetric.count), currentMetric,
			map[string]any{"baseline": base.count, "previous": previousMetric.count, "current": currentMetric.count, "metric": "high_or_critical_findings", "coverage": "complete",
				"threshold": "previous > baseline AND current > baseline", "window_hours": started.Sub(priorStarted).Hours()})
		if err != nil {
			return nil, err
		}
		// Slow queries and heavy workload patterns (performance.slow_query,
		// performance.workload_*) are low severity, so the findings metric
		// never saw them (#104). The same query must stay new against the
		// baseline in both runs.
		previousWorkload, err := s.regressionMetric(ctx, "workload", baseline, prior)
		if err != nil {
			return nil, err
		}
		currentWorkload, err := s.regressionMetric(ctx, "workload", baseline, run)
		if err != nil {
			return nil, err
		}
		persistent := regressionMetric{keys: PersistentKeys(previousWorkload.keys, currentWorkload.keys)}
		persistent.count = len(persistent.keys)
		out["workload"], err = s.recordRegression(ctx, env, baseline, run, "workload", 0, persistent.count > 0, persistent,
			map[string]any{"previous": len(previousWorkload.keys), "current": persistent.count, "metric": "new_slow_or_heavy_queries", "coverage": "complete",
				"threshold": "same query new against baseline in previous AND current", "window_hours": started.Sub(priorStarted).Hours()})
		if err != nil {
			return nil, err
		}
	}
	previousStructure, err := s.regressionMetric(ctx, "structure", baseline, prior)
	if err != nil {
		return nil, err
	}
	currentStructure, err := s.regressionMetric(ctx, "structure", baseline, run)
	if err != nil {
		return nil, err
	}
	persistent := regressionMetric{keys: PersistentKeys(previousStructure.keys, currentStructure.keys)}
	persistent.count = len(persistent.keys)
	// A table created after the baseline without a primary key is a
	// structural regression too (#104, QA scenario 4); it follows the same
	// identity-based persistence rule as removed tables and indexes.
	addedNoPK := []string{}
	for _, key := range persistent.keys {
		if table, ok := strings.CutPrefix(key, "table_added_without_pk:"); ok {
			addedNoPK = append(addedNoPK, table)
		}
	}
	addedNoPKTotal := len(addedNoPK)
	if len(addedNoPK) > maxRegressionKeys {
		addedNoPK = addedNoPK[:maxRegressionKeys]
	}
	out["structure"], err = s.recordRegression(ctx, env, baseline, run, "structure", 0, persistent.count > 0, persistent,
		map[string]any{"previous": previousStructure.count, "current": persistent.count, "metric": "removed_or_changed_tables_removed_indexes_or_new_tables_without_pk", "coverage": "complete",
			"removed_tables": currentStructure.removedTables, "changed_tables": currentStructure.changedTables, "removed_indexes": currentStructure.removedIndexes,
			"added_tables_without_pk": addedNoPK, "added_tables_without_pk_total": addedNoPKTotal,
			"threshold": "same table or index regressed in previous AND current", "window_hours": started.Sub(priorStarted).Hours()})
	return out, err
}

// regressionMetric is one category measured on a run against the baseline.
// keys identify what regressed: a finding dedup_key, a table or an index.
type regressionMetric struct {
	count                                        int
	keys                                         []string
	removedTables, changedTables, removedIndexes int
	addedTablesWithoutPK                         int
}

// maxRegressionKeys bounds the identity stored with an alert.
const maxRegressionKeys = 1000

const regressionFindingKeysSQL = `SELECT DISTINCT f.dedup_key FROM finding_event e JOIN finding f ON f.id=e.finding_id
 WHERE e.audit_run_id=$2::uuid AND e.event_type='observed' AND %s
 AND NOT EXISTS (SELECT 1 FROM finding_event b WHERE b.finding_id=e.finding_id AND b.audit_run_id=$1::uuid AND b.event_type='observed' AND %s)
 ORDER BY 1`

func (s *Store) regressionMetric(ctx context.Context, category, baseline, run string) (regressionMetric, error) {
	var m regressionMetric
	var query string
	switch category {
	case "findings":
		const high = "e.severity IN ('high','critical')"
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM finding_event e WHERE e.audit_run_id=$1::uuid AND e.event_type='observed' AND `+high, run).Scan(&m.count); err != nil {
			return m, err
		}
		query = fmt.Sprintf(regressionFindingKeysSQL, high, strings.ReplaceAll(high, "e.", "b."))
	case "workload":
		query = fmt.Sprintf(regressionFindingKeysSQL, "(f.finding_type='performance.slow_query' OR f.finding_type LIKE 'performance.workload\\_%')", "true")
	case "structure":
		query = `WITH base AS (SELECT database_name,schema_name,table_name,relkind,has_primary_key,column_count FROM table_snapshot WHERE audit_run_id=$1::uuid),
cur AS (SELECT database_name,schema_name,table_name,relkind,has_primary_key,column_count,is_partition FROM table_snapshot WHERE audit_run_id=$2::uuid),
base_idx AS (SELECT database_name,schema_name,index_name FROM index_snapshot WHERE audit_run_id=$1::uuid),
cur_idx AS (SELECT database_name,schema_name,index_name FROM index_snapshot WHERE audit_run_id=$2::uuid)
SELECT 'table_removed:'||b.database_name||'.'||b.schema_name||'.'||b.table_name FROM base b LEFT JOIN cur c USING(database_name,schema_name,table_name) WHERE c.table_name IS NULL
UNION ALL
SELECT 'table_changed:'||b.database_name||'.'||b.schema_name||'.'||b.table_name FROM base b JOIN cur c USING(database_name,schema_name,table_name)
 WHERE (b.relkind,b.has_primary_key,b.column_count) IS DISTINCT FROM (c.relkind,c.has_primary_key,c.column_count)
UNION ALL
SELECT 'index_removed:'||b.database_name||'.'||b.schema_name||'.'||b.index_name FROM base_idx b LEFT JOIN cur_idx c USING(database_name,schema_name,index_name)
 WHERE c.index_name IS NULL AND EXISTS (SELECT 1 FROM cur WHERE cur.database_name=b.database_name)
UNION ALL
SELECT 'table_added_without_pk:'||c.database_name||'.'||c.schema_name||'.'||c.table_name FROM cur c LEFT JOIN base b USING(database_name,schema_name,table_name)
 WHERE b.table_name IS NULL AND NOT c.has_primary_key AND NOT c.is_partition AND c.relkind IN ('r','p')
 AND c.schema_name NOT LIKE '\_timescaledb\_%' ESCAPE '\'
 AND EXISTS (SELECT 1 FROM base WHERE base.database_name=c.database_name)
ORDER BY 1`
	default:
		return m, fmt.Errorf("unknown regression category %q", category)
	}
	rows, err := s.pool.Query(ctx, query, baseline, run)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return m, err
		}
		switch {
		case strings.HasPrefix(key, "table_removed:"):
			m.removedTables++
		case strings.HasPrefix(key, "table_changed:"):
			m.changedTables++
		case strings.HasPrefix(key, "index_removed:"):
			m.removedIndexes++
		case strings.HasPrefix(key, "table_added_without_pk:"):
			m.addedTablesWithoutPK++
		}
		m.keys = append(m.keys, key)
	}
	if category != "findings" {
		m.count = len(m.keys)
	}
	return m, rows.Err()
}

// recordRegression stores a persistent regression unless the latest alert for
// the same baseline and category already covers it (RegressionIsNew). The
// alert keeps the identity (keys) it was raised for.
func (s *Store) recordRegression(ctx context.Context, env, baseline, run, category string, baseCount int, persistent bool, metric regressionMetric, evidence map[string]any) (string, error) {
	if !persistent {
		return RegressionNotPersistent, nil
	}
	var last *LastRegressionAlert
	var lastRun string
	var current *int
	var rawKeys []byte
	err := s.pool.QueryRow(ctx, `SELECT audit_run_id::text,(evidence->>'current')::int,evidence->'keys' FROM regression_alert WHERE environment_id=$1::uuid AND baseline_run_id=$2::uuid AND category=$3 AND audit_run_id<>$4::uuid
ORDER BY created_at DESC,id DESC LIMIT 1`, env, baseline, category, run).Scan(&lastRun, &current, &rawKeys)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err == nil {
		last = &LastRegressionAlert{}
		if current != nil {
			last.Current = *current
		}
		if len(rawKeys) > 0 && string(rawKeys) != "null" {
			last.HasKeys = json.Unmarshal(rawKeys, &last.Keys) == nil
		}
	}
	recovered := false
	if last != nil {
		if recovered, err = s.regressionRecovered(ctx, env, baseline, lastRun, run, category, baseCount, last); err != nil {
			return "", err
		}
	}
	if !RegressionIsNew(last, metric.keys, metric.count, recovered) {
		return RegressionAlreadyAlerted, nil
	}
	keys := metric.keys
	if keys == nil {
		keys = []string{}
	}
	evidence["keys_total"] = len(keys)
	if len(keys) > maxRegressionKeys {
		keys = keys[:maxRegressionKeys]
	}
	evidence["keys"] = keys
	if recovered {
		evidence["previous_alert_recovered"] = true
	}
	payload, _ := json.Marshal(evidence)
	if _, err = s.pool.Exec(ctx, `INSERT INTO regression_alert(environment_id,baseline_run_id,audit_run_id,category,evidence)
VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::jsonb) ON CONFLICT DO NOTHING`, env, baseline, run, category, payload); err != nil {
		return "", err
	}
	return RegressionRecorded, nil
}

// regressionRecovered reports whether a complete, analyzed run after the last
// alert and before the current run showed that regression gone: the findings
// count back at the baseline, or none of the alerted tables, indexes or
// queries still regressed. A new episode then alerts even with the same
// identity or a lower count (#127).
func (s *Store) regressionRecovered(ctx context.Context, env, baseline, lastRun, run, category string, baseCount int, last *LastRegressionAlert) (bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text FROM audit_run r JOIN analysis_run a ON a.audit_run_id=r.id AND a.status='success'
WHERE r.environment_id=$1::uuid AND r.status='success'
 AND r.started_at>(SELECT started_at FROM audit_run WHERE id=$2::uuid) AND r.started_at<(SELECT started_at FROM audit_run WHERE id=$3::uuid)
 AND EXISTS (SELECT 1 FROM database_snapshot d WHERE d.audit_run_id=r.id)
 AND NOT EXISTS (SELECT 1 FROM audit_run_coverage c WHERE c.audit_run_id=r.id AND c.status IN ('failed','skipped','attempted'))
ORDER BY r.started_at`, env, lastRun, run)
	if err != nil {
		return false, err
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		candidates = append(candidates, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		m, err := s.regressionMetric(ctx, category, baseline, candidate)
		if err != nil {
			return false, err
		}
		switch {
		case category == "findings" && m.count <= baseCount:
			return true, nil
		case category != "findings" && last.HasKeys && !Overlaps(last.Keys, m.keys):
			return true, nil
		case category != "findings" && !last.HasKeys && m.count == 0:
			return true, nil
		}
	}
	return false, nil
}
