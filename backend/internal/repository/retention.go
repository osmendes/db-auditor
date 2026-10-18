package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
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

func (s *Store) GetRetentionPolicy(ctx context.Context) (RetentionPolicy, error) {
	var p RetentionPolicy
	err := s.pool.QueryRow(ctx, `SELECT snapshot_retention_days,report_job_retention_days,owner_name,encryption_at_rest,archive_note,approved_by,approved_at,updated_at FROM retention_policy WHERE id`).
		Scan(&p.SnapshotRetentionDays, &p.ReportJobRetentionDays, &p.OwnerName, &p.EncryptionAtRest, &p.ArchiveNote, &p.ApprovedBy, &p.ApprovedAt, &p.UpdatedAt)
	return p, err
}

// UpdateRetentionPolicy changes the policy. Any change clears the previous
// approval unless the same request approves it, so an edited policy is never
// treated as approved.
func (s *Store) UpdateRetentionPolicy(ctx context.Context, in RetentionPolicyInput, actor string) (RetentionPolicy, error) {
	if err := ValidateRetentionInput(in); err != nil {
		return RetentionPolicy{}, err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO retention_policy(id) VALUES(true) ON CONFLICT DO NOTHING`)
	if err != nil {
		return RetentionPolicy{}, err
	}
	_, err = s.pool.Exec(ctx, `UPDATE retention_policy SET snapshot_retention_days=$1,report_job_retention_days=$2,owner_name=$3,encryption_at_rest=$4,archive_note=$5,
approved_by=CASE WHEN $6 THEN $7 ELSE '' END,approved_at=CASE WHEN $6 THEN now() ELSE NULL END,updated_at=now() WHERE id`,
		in.SnapshotRetentionDays, in.ReportJobRetentionDays, strings.TrimSpace(in.OwnerName), strings.TrimSpace(in.EncryptionAtRest), strings.TrimSpace(in.ArchiveNote), in.Approve, actor)
	if err != nil {
		return RetentionPolicy{}, err
	}
	return s.GetRetentionPolicy(ctx)
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

func (s *Store) latestEvidence(ctx context.Context, kind string) (*RetentionEvidence, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text,kind,environment_label,artifact_sha256,succeeded,rto_seconds,integrity,note,recorded_by,performed_at,recorded_at
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
	policy, err := s.GetRetentionPolicy(ctx)
	if err != nil {
		return RetentionPlan{}, err
	}
	plan := RetentionPlan{Policy: policy, SnapshotDeletionNotes: "Snapshots não são removidos pela API: arquive com backup verificado e siga o runbook de docs/ops.md antes de qualquer exclusão manual aprovada."}
	if err = s.pool.QueryRow(ctx, `SELECT
 CASE WHEN $1>0 THEN (SELECT count(*) FROM audit_run WHERE started_at<now()-make_interval(days=>$1)) ELSE 0 END,
 (SELECT count(*) FROM report_job j WHERE j.created_at<now()-make_interval(days=>$2) AND j.expires_at<=now() AND j.status IN ('success','failed','cancelled') AND NOT EXISTS (SELECT 1 FROM report_artifact a WHERE a.report_job_id=j.id)),
 (SELECT count(*) FROM report_artifact a JOIN report_job j ON j.id=a.report_job_id WHERE j.expires_at<=now())`, policy.SnapshotRetentionDays, policy.ReportJobRetentionDays).
		Scan(&plan.SnapshotRunsEligible, &plan.ReportJobsEligible, &plan.ExpiredArtifacts); err != nil {
		return plan, err
	}
	if plan.LastBackupVerified, err = s.latestEvidence(ctx, EvidenceBackupVerified); err != nil {
		return plan, err
	}
	if plan.LastRestoreDrill, err = s.latestEvidence(ctx, EvidenceRestoreDrill); err != nil {
		return plan, err
	}
	plan.Blockers = CleanupBlockers(policy, plan.LastBackupVerified, plan.LastRestoreDrill, time.Now())
	plan.CleanupAllowed = len(plan.Blockers) == 0
	return plan, nil
}

// ApplyReportJobRetention removes job trails older than the approved period
// whose artifact already expired. It refuses unless approval, a recent
// verified backup and a restore drill are on record, and only when the caller
// confirms the exact count from the dry run.
func (s *Store) ApplyReportJobRetention(ctx context.Context, expected int64) (int64, error) {
	plan, err := s.PlanRetention(ctx)
	if err != nil {
		return 0, err
	}
	if !plan.CleanupAllowed {
		return 0, fmt.Errorf("%w: %s", ErrRetentionBlocked, strings.Join(plan.Blockers, "; "))
	}
	if expected != plan.ReportJobsEligible {
		return 0, fmt.Errorf("%w: a contagem confirmada (%d) difere do plano atual (%d); gere o plano de novo", ErrRetentionBlocked, expected, plan.ReportJobsEligible)
	}
	cmd, err := s.pool.Exec(ctx, `DELETE FROM report_job j WHERE j.created_at<now()-make_interval(days=>$1) AND j.expires_at<=now() AND j.status IN ('success','failed','cancelled') AND NOT EXISTS (SELECT 1 FROM report_artifact a WHERE a.report_job_id=j.id)`, plan.Policy.ReportJobRetentionDays)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}
