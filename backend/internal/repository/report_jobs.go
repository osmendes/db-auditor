package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrReportIneligible = errors.New("report run is not completed in the requested environment")
var ErrReportConflict = errors.New("report job cannot transition from its current state")

type ReportFilters struct {
	Database  string `json:"database,omitempty"`
	Schema    string `json:"schema,omitempty"`
	Table     string `json:"table,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Redaction string `json:"redaction,omitempty"`
}
type ReportRequest struct {
	EnvironmentID  string        `json:"environment_id"`
	AuditRunID     string        `json:"audit_run_id"`
	Type           string        `json:"report_type"`
	Filters        ReportFilters `json:"filters"`
	RequestedBy    string        `json:"requested_by"`
	IdempotencyKey string        `json:"idempotency_key"`
}
type ReportJob struct {
	ID            string        `json:"id"`
	EnvironmentID string        `json:"environment_id"`
	AuditRunID    string        `json:"audit_run_id"`
	Type          string        `json:"report_type"`
	Filters       ReportFilters `json:"filters"`
	RequestedBy   string        `json:"requested_by"`
	RuleVersion   string        `json:"rule_version"`
	Status        string        `json:"status"`
	Attempts      int           `json:"attempts"`
	Error         *string       `json:"error,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	FinishedAt    *time.Time    `json:"finished_at,omitempty"`
	ExpiresAt     time.Time     `json:"expires_at"`
	SHA256        *string       `json:"sha256,omitempty"`
	SizeBytes     *int64        `json:"size_bytes,omitempty"`
}
type ReportArtifact struct {
	Content     []byte
	SHA256      string
	Filename    string
	ContentType string
}

const reportJobSelect = `SELECT j.id::text,j.environment_id::text,j.audit_run_id::text,j.report_type,j.filters,j.requested_by,j.rule_version,j.status,j.attempts,j.error,j.created_at,j.started_at,j.finished_at,j.expires_at,a.sha256,a.size_bytes FROM report_job j LEFT JOIN report_artifact a ON a.report_job_id=j.id`

func scanReportJob(row scannable) (*ReportJob, error) {
	var j ReportJob
	var raw []byte
	err := row.Scan(&j.ID, &j.EnvironmentID, &j.AuditRunID, &j.Type, &raw, &j.RequestedBy, &j.RuleVersion, &j.Status, &j.Attempts, &j.Error, &j.CreatedAt, &j.StartedAt, &j.FinishedAt, &j.ExpiresAt, &j.SHA256, &j.SizeBytes)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &j.Filters); err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) CreateReportJob(ctx context.Context, req ReportRequest) (*ReportJob, error) {
	var ruleVersion string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(NULLIF(a.rule_manifest_hash,''),a.analyzer_version) FROM audit_run r JOIN analysis_run a ON a.audit_run_id=r.id AND a.status='success' WHERE r.id=$1::uuid AND r.environment_id=$2::uuid AND r.status IN ('success','partial_success')`, req.AuditRunID, req.EnvironmentID).Scan(&ruleVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReportIneligible
	}
	if err != nil {
		return nil, err
	}
	if req.RequestedBy == "" {
		req.RequestedBy = "local"
	}
	if req.IdempotencyKey == "" {
		payload, _ := json.Marshal([]any{req.EnvironmentID, req.AuditRunID, req.Type, req.Filters, ruleVersion})
		digest := sha256.Sum256(payload)
		req.IdempotencyKey = hex.EncodeToString(digest[:])
	}
	filters, err := json.Marshal(req.Filters)
	if err != nil {
		return nil, err
	}
	for rotation := 0; rotation < 128; rotation++ {
		var id string
		err = s.pool.QueryRow(ctx, `INSERT INTO report_job(environment_id,audit_run_id,report_type,filters,requested_by,rule_version,idempotency_key) VALUES($1::uuid,$2::uuid,$3,$4::jsonb,$5,$6,$7)
ON CONFLICT(environment_id,idempotency_key) DO UPDATE SET idempotency_key=report_job.idempotency_key
RETURNING id::text`, req.EnvironmentID, req.AuditRunID, req.Type, filters, req.RequestedBy, ruleVersion, req.IdempotencyKey).Scan(&id)
		if err != nil {
			return nil, err
		}
		job, err := s.GetReportJob(ctx, req.EnvironmentID, id)
		if err != nil {
			return nil, err
		}
		if job == nil || job.AuditRunID != req.AuditRunID || job.Type != req.Type || job.Filters != req.Filters || job.RuleVersion != ruleVersion {
			return nil, ErrReportConflict
		}
		if job.ExpiresAt.After(time.Now()) {
			return job, nil
		}
		// Rotate the idempotency key to create a fresh job, preserving the
		// expired job's immutable provenance and its former attempts/result.
		digest := sha256.Sum256([]byte(req.IdempotencyKey + ":" + job.ID))
		req.IdempotencyKey = hex.EncodeToString(digest[:])
	}
	return nil, fmt.Errorf("too many expired report generations")
}

func (s *Store) GetReportJob(ctx context.Context, environmentID, id string) (*ReportJob, error) {
	j, err := scanReportJob(s.pool.QueryRow(ctx, reportJobSelect+` WHERE j.environment_id=$1::uuid AND j.id=$2::uuid`, environmentID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return j, err
}

func (s *Store) ListReportJobs(ctx context.Context, environmentID string) ([]ReportJob, error) {
	rows, err := s.pool.Query(ctx, reportJobSelect+` WHERE j.environment_id=$1::uuid ORDER BY j.created_at DESC LIMIT 100`, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReportJob{}
	for rows.Next() {
		j, err := scanReportJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

func (s *Store) ClaimReportJob(ctx context.Context) (*ReportJob, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM report_job WHERE status='queued' AND attempts<3 AND expires_at>now() ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE report_job SET status='running',attempts=attempts+1,started_at=now(),error=NULL WHERE id=$1::uuid`, id); err != nil {
		return nil, err
	}
	j, err := scanReportJob(tx.QueryRow(ctx, reportJobSelect+` WHERE j.id=$1::uuid`, id))
	if err != nil {
		return nil, err
	}
	return j, tx.Commit(ctx)
}

func (s *Store) FinishReportJob(ctx context.Context, id string, artifact ReportArtifact) error {
	if len(artifact.Content) == 0 || len(artifact.Content) > 16<<20 || artifact.ContentType != "application/pdf" {
		return fmt.Errorf("invalid PDF artifact")
	}
	digest := sha256.Sum256(artifact.Content)
	hash := hex.EncodeToString(digest[:])
	if hash != artifact.SHA256 {
		return fmt.Errorf("PDF hash mismatch")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cmd, err := tx.Exec(ctx, `UPDATE report_job SET status='success',finished_at=now() WHERE id=$1::uuid AND status='running'`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() != 1 {
		return ErrReportConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO report_artifact(report_job_id,content,sha256,size_bytes,filename,content_type) VALUES($1::uuid,$2,$3,$4,$5,$6)
ON CONFLICT(report_job_id) DO UPDATE SET content=EXCLUDED.content,sha256=EXCLUDED.sha256,size_bytes=EXCLUDED.size_bytes,filename=EXCLUDED.filename,created_at=now()`, id, artifact.Content, hash, len(artifact.Content), artifact.Filename, artifact.ContentType); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FailReportJob(ctx context.Context, id string, problem error) error {
	msg := problem.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.pool.Exec(ctx, `UPDATE report_job SET status='failed',error=$2,finished_at=now() WHERE id=$1::uuid AND status='running'`, id, msg)
	return err
}

func (s *Store) CancelReportJob(ctx context.Context, environmentID, id string) error {
	cmd, err := s.pool.Exec(ctx, `UPDATE report_job SET status='cancelled',finished_at=now() WHERE id=$1::uuid AND environment_id=$2::uuid AND status IN ('queued','running')`, id, environmentID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() != 1 {
		return ErrReportConflict
	}
	return nil
}

func (s *Store) RetryReportJob(ctx context.Context, environmentID, id string) (*ReportJob, error) {
	cmd, err := s.pool.Exec(ctx, `UPDATE report_job SET status='queued',error=NULL,started_at=NULL,finished_at=NULL WHERE id=$1::uuid AND environment_id=$2::uuid AND status='failed' AND attempts<3 AND expires_at>now()`, id, environmentID)
	if err != nil {
		return nil, err
	}
	if cmd.RowsAffected() != 1 {
		return nil, ErrReportConflict
	}
	return s.GetReportJob(ctx, environmentID, id)
}

func (s *Store) GetReportArtifact(ctx context.Context, environmentID, id string) (*ReportArtifact, error) {
	var a ReportArtifact
	err := s.pool.QueryRow(ctx, `SELECT a.content,a.sha256,a.filename,a.content_type FROM report_artifact a JOIN report_job j ON j.id=a.report_job_id WHERE j.id=$1::uuid AND j.environment_id=$2::uuid AND j.status='success' AND j.expires_at>now()`, id, environmentID).Scan(&a.Content, &a.SHA256, &a.Filename, &a.ContentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

const deleteExpiredArtifactsSQL = `DELETE FROM report_artifact WHERE report_job_id IN (SELECT id FROM report_job WHERE expires_at<=now())`

const requeueInterruptedReportsSQL = `UPDATE report_job SET status=CASE WHEN attempts<3 THEN 'queued' ELSE 'failed' END,error=CASE WHEN attempts<3 THEN NULL ELSE 'worker interrupted after three attempts' END,started_at=NULL WHERE status='running' AND started_at<now()-interval '3 minutes'`

func (s *Store) CleanupExpiredReports(ctx context.Context) (int64, error) {
	cmd, err := s.pool.Exec(ctx, deleteExpiredArtifactsSQL)
	if err != nil {
		return 0, err
	}
	// Jobs are the provenance trail. Artifact expiry never removes this trail;
	// archival of jobs requires an explicit, backed-up retention policy.
	return cmd.RowsAffected(), nil
}

func (s *Store) RequeueInterruptedReports(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, requeueInterruptedReportsSQL)
	return err
}
