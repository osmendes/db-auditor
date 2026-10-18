package repository

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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

// EvaluateRegressions returns, per category ("findings", "structure"), either
// RegressionRecorded or the reason the pair was silenced.
func (s *Store) EvaluateRegressions(ctx context.Context, env, run string) (map[string]string, error) {
	silence := func(reason string) map[string]string {
		return map[string]string{"findings": reason, "structure": reason}
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
 (SELECT max(stats_reset) FROM table_snapshot WHERE audit_run_id=$5::uuid)`, env, priorStarted, started, prior, run).Scan(&pair.IncompleteBetween, &pair.PriorStatsReset, &pair.CurrentStatsReset); err != nil {
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
	if ok, reason := PairComparable(pair, maxGap, true); !ok {
		out["findings"] = reason
	} else {
		var baseFindings, priorFindings, nowFindings int
		err = s.pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM finding_event WHERE audit_run_id=$1::uuid AND event_type='observed' AND severity IN ('high','critical')),
 (SELECT count(*) FROM finding_event WHERE audit_run_id=$2::uuid AND event_type='observed' AND severity IN ('high','critical')),
 (SELECT count(*) FROM finding_event WHERE audit_run_id=$3::uuid AND event_type='observed' AND severity IN ('high','critical'))`, baseline, prior, run).Scan(&baseFindings, &priorFindings, &nowFindings)
		if err != nil {
			return nil, err
		}
		out["findings"], err = s.recordRegression(ctx, env, baseline, run, "findings", PersistentRegression(baseFindings, priorFindings, nowFindings), nowFindings,
			map[string]any{"baseline": baseFindings, "previous": priorFindings, "current": nowFindings, "metric": "high_or_critical_findings", "coverage": "complete",
				"threshold": "previous > baseline AND current > baseline", "window_hours": started.Sub(priorStarted).Hours()})
		if err != nil {
			return nil, err
		}
	}
	var previousChanges, currentChanges int
	err = s.pool.QueryRow(ctx, `SELECT
 (SELECT COALESCE(sum(removed_tables+changed_tables),0) FROM baseline_comparison WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND status='complete'),
 (SELECT COALESCE(sum(removed_tables+changed_tables),0) FROM baseline_comparison WHERE environment_id=$1::uuid AND audit_run_id=$3::uuid AND status='complete')`, env, prior, run).Scan(&previousChanges, &currentChanges)
	if err != nil {
		return nil, err
	}
	// A table created after the baseline without a primary key is a
	// structural regression too (#104, QA scenario 4); it counts with the
	// removed and changed tables under the same persistence rule.
	previousNoPK, err := s.addedTablesWithoutPK(ctx, baseline, prior)
	if err != nil {
		return nil, err
	}
	currentNoPK, err := s.addedTablesWithoutPK(ctx, baseline, run)
	if err != nil {
		return nil, err
	}
	previousChanges += len(previousNoPK)
	currentChanges += len(currentNoPK)
	out["structure"], err = s.recordRegression(ctx, env, baseline, run, "structure", PersistentRegression(0, previousChanges, currentChanges), currentChanges,
		map[string]any{"previous": previousChanges, "current": currentChanges, "metric": "removed_or_changed_tables_or_new_tables_without_pk", "coverage": "complete",
			"added_tables_without_pk": boundedKeys(currentNoPK), "added_tables_without_pk_total": len(currentNoPK),
			"threshold": "previous > 0 AND current > 0", "window_hours": started.Sub(priorStarted).Hours()})
	return out, err
}

// maxStructureKeys bounds the table identities stored in alert evidence.
const maxStructureKeys = 1000

func boundedKeys(keys []string) []string {
	if len(keys) > maxStructureKeys {
		return keys[:maxStructureKeys]
	}
	return keys
}

// addedTablesWithoutPKSQL lists, as "db.schema.table", tables present in the
// run but not in the baseline that have no primary key. It follows
// integrity.missing_primary_key: partitions, foreign tables and TimescaleDB
// internal chunks are excluded.
const addedTablesWithoutPKSQL = `SELECT c.database_name||'.'||c.schema_name||'.'||c.table_name
FROM table_snapshot c
WHERE c.audit_run_id=$2::uuid AND NOT c.has_primary_key AND NOT c.is_partition AND c.relkind IN ('r','p')
 AND c.schema_name NOT LIKE '\_timescaledb\_%' ESCAPE '\'
 AND EXISTS (SELECT 1 FROM table_snapshot d WHERE d.audit_run_id=$1::uuid AND d.database_name=c.database_name)
 AND NOT EXISTS (SELECT 1 FROM table_snapshot b WHERE b.audit_run_id=$1::uuid AND b.database_name=c.database_name AND b.schema_name=c.schema_name AND b.table_name=c.table_name)
ORDER BY 1`

func (s *Store) addedTablesWithoutPK(ctx context.Context, baseline, run string) ([]string, error) {
	rows, err := s.pool.Query(ctx, addedTablesWithoutPKSQL, baseline, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) recordRegression(ctx context.Context, env, baseline, run, category string, persistent bool, current int, evidence map[string]any) (string, error) {
	if !persistent {
		return RegressionNotPersistent, nil
	}
	var last *int
	err := s.pool.QueryRow(ctx, `SELECT (evidence->>'current')::int FROM regression_alert WHERE environment_id=$1::uuid AND baseline_run_id=$2::uuid AND category=$3 AND audit_run_id<>$4::uuid
ORDER BY created_at DESC,id DESC LIMIT 1`, env, baseline, category, run).Scan(&last)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if !NewerThanLastAlert(last, current) {
		return RegressionAlreadyAlerted, nil
	}
	payload, _ := json.Marshal(evidence)
	if _, err = s.pool.Exec(ctx, `INSERT INTO regression_alert(environment_id,baseline_run_id,audit_run_id,category,evidence)
VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::jsonb) ON CONFLICT DO NOTHING`, env, baseline, run, category, payload); err != nil {
		return "", err
	}
	return RegressionRecorded, nil
}
