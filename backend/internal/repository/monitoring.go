package repository

import (
	"context"
	"encoding/json"
	"errors"
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
// analyzed runs. Missing/partial collections can never produce an alert.
func (s *Store) EvaluateRegressionAlerts(ctx context.Context, env, run string) error {
	current, err := s.GetSnapshotCompleteness(ctx, env, run)
	if err != nil {
		return err
	}
	if current == nil || current.Completeness != "complete" {
		return nil
	}
	var started time.Time
	var analysis string
	err = s.pool.QueryRow(ctx, `SELECT r.started_at,COALESCE(a.status,'') FROM audit_run r LEFT JOIN analysis_run a ON a.audit_run_id=r.id
WHERE r.environment_id=$1::uuid AND r.id=$2::uuid AND r.status='success'`, env, run).Scan(&started, &analysis)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if analysis != "success" {
		return nil
	}
	var prior string
	err = s.pool.QueryRow(ctx, `SELECT id::text FROM audit_run WHERE environment_id=$1::uuid AND status='success' AND started_at<$2 ORDER BY started_at DESC,id DESC LIMIT 1`, env, started).Scan(&prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	previous, err := s.GetSnapshotCompleteness(ctx, env, prior)
	if err != nil {
		return err
	}
	if previous == nil || !AllowRegressionAlert(current.Completeness, previous.Completeness) {
		return nil
	}
	var previousAnalysis string
	if err = s.pool.QueryRow(ctx, `SELECT COALESCE((SELECT status FROM analysis_run WHERE audit_run_id=$1::uuid),'')`, prior).Scan(&previousAnalysis); err != nil {
		return err
	}
	if previousAnalysis != "success" {
		return nil
	}
	var baseline string
	err = s.pool.QueryRow(ctx, `SELECT audit_run_id::text FROM audit_baseline WHERE environment_id=$1::uuid AND database_name='' AND schema_name='' AND table_name=''`, env).Scan(&baseline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if baseline == run || baseline == prior {
		return nil
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
		return err
	}
	if !compatible {
		return nil
	}
	baseCoverage, err := s.GetSnapshotCompleteness(ctx, env, baseline)
	if err != nil {
		return err
	}
	if baseCoverage == nil || baseCoverage.Completeness != "complete" {
		return nil
	}
	var baseFindings, priorFindings, nowFindings int
	err = s.pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM finding_event WHERE audit_run_id=$1::uuid AND event_type='observed' AND severity IN ('high','critical')),
 (SELECT count(*) FROM finding_event WHERE audit_run_id=$2::uuid AND event_type='observed' AND severity IN ('high','critical')),
 (SELECT count(*) FROM finding_event WHERE audit_run_id=$3::uuid AND event_type='observed' AND severity IN ('high','critical'))`, baseline, prior, run).Scan(&baseFindings, &priorFindings, &nowFindings)
	if err != nil {
		return err
	}
	if priorFindings > baseFindings && nowFindings > baseFindings {
		evidence, _ := json.Marshal(map[string]any{"baseline": baseFindings, "previous": priorFindings, "current": nowFindings, "metric": "high_or_critical_findings", "coverage": "complete"})
		if _, err = s.pool.Exec(ctx, `INSERT INTO regression_alert(environment_id,baseline_run_id,audit_run_id,category,evidence)
VALUES($1::uuid,$2::uuid,$3::uuid,'findings',$4::jsonb) ON CONFLICT DO NOTHING`, env, baseline, run, evidence); err != nil {
			return err
		}
	}
	var previousChanges, currentChanges int
	err = s.pool.QueryRow(ctx, `SELECT
 (SELECT COALESCE(sum(removed_tables+changed_tables),0) FROM baseline_comparison WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND status='complete'),
 (SELECT COALESCE(sum(removed_tables+changed_tables),0) FROM baseline_comparison WHERE environment_id=$1::uuid AND audit_run_id=$3::uuid AND status='complete')`, env, prior, run).Scan(&previousChanges, &currentChanges)
	if err != nil {
		return err
	}
	if previousChanges > 0 && currentChanges > 0 {
		evidence, _ := json.Marshal(map[string]any{"previous": previousChanges, "current": currentChanges, "metric": "removed_or_changed_tables", "coverage": "complete"})
		_, err = s.pool.Exec(ctx, `INSERT INTO regression_alert(environment_id,baseline_run_id,audit_run_id,category,evidence)
VALUES($1::uuid,$2::uuid,$3::uuid,'structure',$4::jsonb) ON CONFLICT DO NOTHING`, env, baseline, run, evidence)
	}
	return err
}
