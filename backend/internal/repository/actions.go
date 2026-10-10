package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/actions"
	"github.com/mayconmendes-qc/db-auditor/internal/guidance"
)

var ErrActionNotFound = errors.New("finding action source not found")
var ErrActionEvidenceRequired = errors.New("a comparable post-change measurement is required")

type FindingAction struct {
	FindingID     string          `json:"finding_id"`
	EnvironmentID string          `json:"environment_id"`
	SourceRunID   *string         `json:"source_run_id,omitempty"`
	RuleID        string          `json:"rule_id"`
	Target        string          `json:"target"`
	Severity      string          `json:"severity"`
	Confidence    float64         `json:"confidence"`
	Evidence      json.RawMessage `json:"evidence"`
	Coverage      string          `json:"coverage"`
	Suggestion    string          `json:"suggestion"`
	Plan          actions.Plan    `json:"plan"`
	Status        string          `json:"status"`
	Owner         string          `json:"owner"`
	Justification string          `json:"justification"`
	Result        string          `json:"result"`
	UpdatedBy     string          `json:"updated_by,omitempty"`
	UpdatedAt     *time.Time      `json:"updated_at,omitempty"`
}

type ActionProgress struct {
	Status        string `json:"status"`
	Owner         string `json:"owner"`
	Justification string `json:"justification"`
	Result        string `json:"result"`
}

type ActionEvent struct {
	Status        string    `json:"status"`
	Owner         string    `json:"owner"`
	Justification string    `json:"justification"`
	Result        string    `json:"result"`
	Actor         string    `json:"actor"`
	RecordedAt    time.Time `json:"recorded_at"`
}

func (s *Store) ListFindingActionEvents(ctx context.Context, id string) ([]ActionEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT status,owner_name,justification,result_note,actor,recorded_at
FROM finding_action_event WHERE finding_id=$1::uuid ORDER BY recorded_at DESC,id DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ActionEvent{}
	for rows.Next() {
		var item ActionEvent
		if err = rows.Scan(&item.Status, &item.Owner, &item.Justification, &item.Result, &item.Actor, &item.RecordedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetFindingAction(ctx context.Context, id string) (*FindingAction, error) {
	f, err := s.GetFinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrActionNotFound
	}
	if err != nil {
		return nil, err
	}
	guide := guidance.For(f.FindingType)
	item := &FindingAction{FindingID: f.ID, EnvironmentID: f.EnvironmentID, SourceRunID: f.AuditRunID,
		RuleID: f.FindingType, Target: f.DatabaseName + "." + f.SchemaName + "." + f.ObjectName,
		Severity: f.Severity, Confidence: f.Confidence, Evidence: f.Evidence,
		Suggestion: guide.Next, Plan: actions.For(f.FindingType), Status: "suggested", Coverage: "unknown"}
	if f.AuditRunID != nil {
		err = s.pool.QueryRow(ctx, `SELECT CASE WHEN r.status='success' AND NOT EXISTS (
 SELECT 1 FROM audit_run_coverage c WHERE c.audit_run_id=r.id AND c.status IN ('failed','skipped','attempted')
) THEN 'complete' ELSE 'partial' END FROM audit_run r WHERE r.id=$1::uuid`, *f.AuditRunID).Scan(&item.Coverage)
		if err != nil {
			return nil, err
		}
	}
	var at time.Time
	err = s.pool.QueryRow(ctx, `SELECT status,owner_name,justification,result_note,updated_by,updated_at
FROM finding_action WHERE finding_id=$1::uuid`, id).Scan(&item.Status, &item.Owner, &item.Justification, &item.Result, &item.UpdatedBy, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, nil
	}
	if err != nil {
		return nil, err
	}
	item.UpdatedAt = &at
	return item, nil
}

func (s *Store) UpdateFindingAction(ctx context.Context, id, actor string, progress ActionProgress) (*FindingAction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finding WHERE id=$1::uuid)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrActionNotFound
	}
	if progress.Status == "validated" {
		var verified bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finding_action_measurement m
JOIN finding f ON f.id=m.finding_id JOIN audit_run after_run ON after_run.id=m.after_run_id
WHERE m.finding_id=$1::uuid AND m.comparable AND m.workload_comparable
AND m.before_value IS NOT NULL AND m.after_value IS NOT NULL
AND after_run.status='success' AND after_run.environment_id=f.environment_id
AND (f.audit_run_id IS NULL OR after_run.started_at > (SELECT started_at FROM audit_run WHERE id=f.audit_run_id)))`, id).Scan(&verified); err != nil {
			return nil, err
		}
		if !verified {
			return nil, ErrActionEvidenceRequired
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO finding_action(finding_id,status,owner_name,justification,result_note,updated_by)
VALUES($1::uuid,$2,$3,$4,$5,$6)
ON CONFLICT(finding_id) DO UPDATE SET status=EXCLUDED.status,owner_name=EXCLUDED.owner_name,
 justification=EXCLUDED.justification,result_note=EXCLUDED.result_note,updated_by=EXCLUDED.updated_by,updated_at=now()`,
		id, progress.Status, progress.Owner, progress.Justification, progress.Result, actor)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO finding_action_event(finding_id,status,owner_name,justification,result_note,actor)
VALUES($1::uuid,$2,$3,$4,$5,$6)`, id, progress.Status, progress.Owner, progress.Justification, progress.Result, actor)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetFindingAction(ctx, id)
}
