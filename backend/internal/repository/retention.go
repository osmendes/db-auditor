package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Retention governance (BA-03/10/16). The store keeps every snapshot and job
// trail until the owner approves a policy. A destructive step additionally
// needs a recent verified backup and a successful restore drill recorded as
// evidence. Snapshot archival itself is a backup (see docs/ops.md); the API
// never deletes snapshots.

const (
	EvidenceBackupVerified = "backup_verified"
	EvidenceRestoreDrill   = "restore_drill"
	// MaxBackupEvidenceAge and MaxRestoreDrillAge bound how old the evidence
	// may be before a destructive cleanup is refused.
	MaxBackupEvidenceAge = 7 * 24 * time.Hour
	MaxRestoreDrillAge   = 30 * 24 * time.Hour
	MinReportJobDays     = 30
	MaxRetentionDays     = 3650
)

var (
	ErrRetentionInvalid = errors.New("invalid retention policy")
	ErrRetentionBlocked = errors.New("retention cleanup blocked")
	sha256Pattern       = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type RetentionPolicy struct {
	SnapshotRetentionDays  int        `json:"snapshot_retention_days"`
	ReportJobRetentionDays int        `json:"report_job_retention_days"`
	OwnerName              string     `json:"owner_name"`
	EncryptionAtRest       string     `json:"encryption_at_rest"`
	ArchiveNote            string     `json:"archive_note"`
	ApprovedBy             string     `json:"approved_by"`
	ApprovedAt             *time.Time `json:"approved_at,omitempty"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type RetentionPolicyInput struct {
	SnapshotRetentionDays  int    `json:"snapshot_retention_days"`
	ReportJobRetentionDays int    `json:"report_job_retention_days"`
	OwnerName              string `json:"owner_name"`
	EncryptionAtRest       string `json:"encryption_at_rest"`
	ArchiveNote            string `json:"archive_note"`
	Approve                bool   `json:"approve"`
}

type RetentionEvidence struct {
	ID               string          `json:"id"`
	Kind             string          `json:"kind"`
	EnvironmentLabel string          `json:"environment_label"`
	ArtifactSHA256   string          `json:"artifact_sha256"`
	Succeeded        bool            `json:"succeeded"`
	RTOSeconds       *int            `json:"rto_seconds,omitempty"`
	Integrity        json.RawMessage `json:"integrity"`
	Note             string          `json:"note"`
	RecordedBy       string          `json:"recorded_by"`
	PerformedAt      time.Time       `json:"performed_at"`
	RecordedAt       time.Time       `json:"recorded_at"`
}

// RetentionPlan is a dry run: counts only, nothing is removed.
type RetentionPlan struct {
	Policy                RetentionPolicy    `json:"policy"`
	SnapshotRunsEligible  int64              `json:"snapshot_runs_eligible"`
	ReportJobsEligible    int64              `json:"report_jobs_eligible"`
	ExpiredArtifacts      int64              `json:"expired_artifacts"`
	LastBackupVerified    *RetentionEvidence `json:"last_backup_verified,omitempty"`
	LastRestoreDrill      *RetentionEvidence `json:"last_restore_drill,omitempty"`
	Blockers              []string           `json:"blockers"`
	CleanupAllowed        bool               `json:"cleanup_allowed"`
	SnapshotDeletionNotes string             `json:"snapshot_deletion_notes"`
}

func ValidateRetentionInput(in RetentionPolicyInput) error {
	if in.SnapshotRetentionDays < 0 || in.SnapshotRetentionDays > MaxRetentionDays || in.ReportJobRetentionDays < MinReportJobDays || in.ReportJobRetentionDays > MaxRetentionDays {
		return fmt.Errorf("%w: snapshot 0..%d dias (0 = manter tudo), jobs %d..%d dias", ErrRetentionInvalid, MaxRetentionDays, MinReportJobDays, MaxRetentionDays)
	}
	owner := strings.TrimSpace(in.OwnerName)
	if owner == "" || len(owner) > 128 || len(in.EncryptionAtRest) > 500 || len(in.ArchiveNote) > 2000 || strings.TrimSpace(in.EncryptionAtRest) == "" {
		return fmt.Errorf("%w: informe dono (até 128) e política de criptografia em repouso (até 500)", ErrRetentionInvalid)
	}
	return nil
}

// retentionDB is what the retention queries need; both the pool and a
// transaction satisfy it.
type retentionDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (s *Store) GetRetentionPolicy(ctx context.Context) (RetentionPolicy, error) {
	return getRetentionPolicy(ctx, s.pool, false)
}

// getRetentionPolicy reads the single policy row; lock holds it FOR UPDATE so
// no edit or approval can land until the transaction ends.
func getRetentionPolicy(ctx context.Context, db retentionDB, lock bool) (RetentionPolicy, error) {
	query := `SELECT snapshot_retention_days,report_job_retention_days,owner_name,encryption_at_rest,archive_note,approved_by,approved_at,updated_at FROM retention_policy WHERE id`
	if lock {
		query += ` FOR UPDATE`
	}
	var p RetentionPolicy
	err := db.QueryRow(ctx, query).
		Scan(&p.SnapshotRetentionDays, &p.ReportJobRetentionDays, &p.OwnerName, &p.EncryptionAtRest, &p.ArchiveNote, &p.ApprovedBy, &p.ApprovedAt, &p.UpdatedAt)
	return p, err
}

// eligibleReportJobsSQL is the single definition of a removable job trail,
// shared by the dry-run count and the DELETE so they can never diverge: a
// finished job (never running or queued), older than the approved period
// (bound to $1), whose own expiry passed and that has no stored artifact.
const eligibleReportJobsSQL = `SELECT j.id FROM report_job j
 WHERE j.status IN ('success','failed','cancelled')
   AND j.created_at<now()-make_interval(days=>$1)
   AND j.expires_at<=now()
   AND NOT EXISTS (SELECT 1 FROM report_artifact a WHERE a.report_job_id=j.id)`

// UpdateRetentionPolicy stores an approved policy. A request without
// Approve writes nothing: the policy is only ever changed together with the
// responsible's approval, so the stored values are always the approved ones.
func (s *Store) UpdateRetentionPolicy(ctx context.Context, in RetentionPolicyInput, actor string) (RetentionPolicy, error) {
	if err := ValidateRetentionInput(in); err != nil {
		return RetentionPolicy{}, err
	}
	if !in.Approve || strings.TrimSpace(actor) == "" {
		return RetentionPolicy{}, fmt.Errorf("%w: a política só é gravada com approve:true do responsável", ErrRetentionInvalid)
	}
	var p RetentionPolicy
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO retention_policy(id) VALUES(true) ON CONFLICT DO NOTHING`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE retention_policy SET snapshot_retention_days=$1,report_job_retention_days=$2,owner_name=$3,encryption_at_rest=$4,archive_note=$5,
approved_by=$6,approved_at=now(),updated_at=now() WHERE id`,
			in.SnapshotRetentionDays, in.ReportJobRetentionDays, strings.TrimSpace(in.OwnerName), strings.TrimSpace(in.EncryptionAtRest), strings.TrimSpace(in.ArchiveNote), actor); err != nil {
			return err
		}
		var err error
		p, err = getRetentionPolicy(ctx, tx, false)
		return err
	})
	return p, err
}

func (s *Store) RecordRetentionEvidence(ctx context.Context, e RetentionEvidence) (RetentionEvidence, error) {
	if e.Kind != EvidenceBackupVerified && e.Kind != EvidenceRestoreDrill {
		return e, fmt.Errorf("%w: kind", ErrRetentionInvalid)
	}
	label := strings.TrimSpace(e.EnvironmentLabel)
	if label == "" || len(label) > 128 || !sha256Pattern.MatchString(e.ArtifactSHA256) || len(e.Note) > 2000 || e.PerformedAt.IsZero() || e.PerformedAt.After(time.Now().Add(5*time.Minute)) {
		return e, fmt.Errorf("%w: ambiente, sha256 do artefato e data da execução são obrigatórios", ErrRetentionInvalid)
	}
	if e.RTOSeconds != nil && *e.RTOSeconds < 0 {
		return e, fmt.Errorf("%w: rto_seconds", ErrRetentionInvalid)
	}
	integrity := e.Integrity
	if len(integrity) == 0 {
		integrity = json.RawMessage(`{}`)
	}
	var probe map[string]any
	if len(integrity) > 16<<10 || json.Unmarshal(integrity, &probe) != nil {
		return e, fmt.Errorf("%w: integrity deve ser um objeto JSON de até 16 KiB", ErrRetentionInvalid)
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO retention_evidence(kind,environment_label,artifact_sha256,succeeded,rto_seconds,integrity,note,recorded_by,performed_at)
VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9) RETURNING id::text,recorded_at`, e.Kind, label, e.ArtifactSHA256, e.Succeeded, e.RTOSeconds, []byte(integrity), strings.TrimSpace(e.Note), e.RecordedBy, e.PerformedAt).Scan(&e.ID, &e.RecordedAt)
	e.EnvironmentLabel, e.Integrity = label, integrity
	return e, err
}

func latestEvidence(ctx context.Context, db retentionDB, kind string) (*RetentionEvidence, error) {
	rows, err := db.Query(ctx, `SELECT id::text,kind,environment_label,artifact_sha256,succeeded,rto_seconds,integrity,note,recorded_by,performed_at,recorded_at
FROM retention_evidence WHERE kind=$1 AND succeeded ORDER BY performed_at DESC,recorded_at DESC LIMIT 1`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var e RetentionEvidence
	var integrity []byte
	if err = rows.Scan(&e.ID, &e.Kind, &e.EnvironmentLabel, &e.ArtifactSHA256, &e.Succeeded, &e.RTOSeconds, &integrity, &e.Note, &e.RecordedBy, &e.PerformedAt, &e.RecordedAt); err != nil {
		return nil, err
	}
	e.Integrity = integrity
	return &e, rows.Err()
}

// CleanupBlockers lists every reason a destructive cleanup must be refused.
func CleanupBlockers(p RetentionPolicy, backup, drill *RetentionEvidence, now time.Time) []string {
	blockers := []string{}
	if p.ApprovedAt == nil || strings.TrimSpace(p.ApprovedBy) == "" {
		blockers = append(blockers, "política sem aprovação registrada do responsável")
	} else if p.ApprovedAt.Before(p.UpdatedAt.Add(-time.Second)) {
		blockers = append(blockers, "política alterada depois da aprovação")
	}
	if backup == nil {
		blockers = append(blockers, "sem backup verificado registrado")
	} else if now.Sub(backup.PerformedAt) > MaxBackupEvidenceAge {
		blockers = append(blockers, fmt.Sprintf("backup verificado com mais de %d dias", int(MaxBackupEvidenceAge.Hours()/24)))
	}
	if drill == nil {
		blockers = append(blockers, "sem restauração de teste em ambiente isolado registrada")
	} else if now.Sub(drill.PerformedAt) > MaxRestoreDrillAge {
		blockers = append(blockers, fmt.Sprintf("restauração de teste com mais de %d dias", int(MaxRestoreDrillAge.Hours()/24)))
	}
	return blockers
}

func (s *Store) PlanRetention(ctx context.Context) (RetentionPlan, error) {
	return planRetention(ctx, s.pool, false)
}

// planRetention computes the dry run on db. With lock, the policy row is held
// FOR UPDATE for the rest of the transaction.
func planRetention(ctx context.Context, db retentionDB, lock bool) (RetentionPlan, error) {
	policy, err := getRetentionPolicy(ctx, db, lock)
	if err != nil {
		return RetentionPlan{}, err
	}
	plan := RetentionPlan{Policy: policy, SnapshotDeletionNotes: "Snapshots não são removidos pela API: arquive com backup verificado e siga o runbook de docs/ops.md antes de qualquer exclusão manual aprovada."}
	if err = db.QueryRow(ctx, `SELECT
 CASE WHEN $2>0 THEN (SELECT count(*) FROM audit_run WHERE started_at<now()-make_interval(days=>$2)) ELSE 0 END,
 (SELECT count(*) FROM (`+eligibleReportJobsSQL+`) eligible),
 (SELECT count(*) FROM report_artifact a JOIN report_job j ON j.id=a.report_job_id WHERE j.expires_at<=now())`, policy.ReportJobRetentionDays, policy.SnapshotRetentionDays).
		Scan(&plan.SnapshotRunsEligible, &plan.ReportJobsEligible, &plan.ExpiredArtifacts); err != nil {
		return plan, err
	}
	if plan.LastBackupVerified, err = latestEvidence(ctx, db, EvidenceBackupVerified); err != nil {
		return plan, err
	}
	if plan.LastRestoreDrill, err = latestEvidence(ctx, db, EvidenceRestoreDrill); err != nil {
		return plan, err
	}
	plan.Blockers = CleanupBlockers(policy, plan.LastBackupVerified, plan.LastRestoreDrill, time.Now())
	plan.CleanupAllowed = len(plan.Blockers) == 0
	return plan, nil
}

// RetentionApplyResult is what a cleanup removed and the approved policy it
// ran under, for the audit log.
type RetentionApplyResult struct {
	ReportJobsRemoved int64           `json:"report_jobs_removed"`
	Policy            RetentionPolicy `json:"policy"`
}

// ApplyReportJobRetention removes job trails older than the approved period
// whose artifact already expired. Plan, checks and DELETE run in one
// transaction with the policy row locked: an edit or revocation cannot slip in
// between, and the rows deleted must be exactly the count the caller confirmed
// from the dry run, otherwise everything rolls back.
func (s *Store) ApplyReportJobRetention(ctx context.Context, expected int64) (RetentionApplyResult, error) {
	var result RetentionApplyResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		plan, err := planRetention(ctx, tx, true)
		if err != nil {
			return err
		}
		if !plan.CleanupAllowed {
			return fmt.Errorf("%w: %s", ErrRetentionBlocked, strings.Join(plan.Blockers, "; "))
		}
		if expected != plan.ReportJobsEligible {
			return fmt.Errorf("%w: a contagem confirmada (%d) difere do plano atual (%d); gere o plano de novo", ErrRetentionBlocked, expected, plan.ReportJobsEligible)
		}
		rows, err := tx.Query(ctx, `DELETE FROM report_job WHERE id IN (`+eligibleReportJobsSQL+`) RETURNING id`, plan.Policy.ReportJobRetentionDays)
		if err != nil {
			return err
		}
		var removed int64
		for rows.Next() {
			removed++
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		if removed != expected {
			return fmt.Errorf("%w: a limpeza removeria %d trilhas, não as %d confirmadas; nada foi removido", ErrRetentionBlocked, removed, expected)
		}
		result = RetentionApplyResult{ReportJobsRemoved: removed, Policy: plan.Policy}
		return nil
	})
	if err != nil {
		return RetentionApplyResult{}, err
	}
	return result, nil
}
