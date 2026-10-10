package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
)

type AnalysisRun struct {
	ID               string     `json:"id"`
	AuditRunID       string     `json:"audit_run_id"`
	EnvironmentID    string     `json:"environment_id"`
	Status           string     `json:"status"`
	AnalyzerVersion  string     `json:"analyzer_version"`
	RuleManifestHash string     `json:"rule_manifest_hash"`
	FindingsProduced int        `json:"findings_produced"`
	FindingsSaved    int        `json:"findings_saved"`
	Error            *string    `json:"error,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

func (s *Store) StartAnalysisRun(ctx context.Context, environmentID, auditRunID, version, ruleManifestHash string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO analysis_run (audit_run_id, environment_id, status, analyzer_version, rule_manifest_hash)
VALUES ($1::uuid, $2::uuid, 'running', $3, $4)
ON CONFLICT (audit_run_id) DO UPDATE SET
  status = 'running', analyzer_version = EXCLUDED.analyzer_version, rule_manifest_hash = EXCLUDED.rule_manifest_hash,
  findings_produced = 0, findings_saved = 0, error = NULL,
  started_at = now(), finished_at = NULL
`, auditRunID, environmentID, version, ruleManifestHash)
	return err
}

func (s *Store) FinishAnalysisRun(ctx context.Context, auditRunID, status string, produced, saved int, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
UPDATE analysis_run SET status=$2, findings_produced=$3, findings_saved=$4,
  error=NULLIF($5,''), finished_at=now()
WHERE audit_run_id=$1::uuid
`, auditRunID, status, produced, saved, errMsg)
	return err
}

func (s *Store) GetAnalysisRun(ctx context.Context, auditRunID string) (*AnalysisRun, error) {
	var r AnalysisRun
	err := s.pool.QueryRow(ctx, `
SELECT id::text, audit_run_id::text, environment_id::text, status, analyzer_version, rule_manifest_hash,
  findings_produced, findings_saved, error, started_at, finished_at
FROM analysis_run WHERE audit_run_id=$1::uuid
`, auditRunID).Scan(&r.ID, &r.AuditRunID, &r.EnvironmentID, &r.Status, &r.AnalyzerVersion, &r.RuleManifestHash,
		&r.FindingsProduced, &r.FindingsSaved, &r.Error, &r.StartedAt, &r.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) SaveAnalysisFindings(ctx context.Context, findings []analyzer.Finding) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for i, f := range findings {
		refs, _ := json.Marshal(f.References)
		params, _ := json.Marshal(f.RuleParameters)
		previousStatus := ""
		err := tx.QueryRow(ctx, `SELECT status FROM finding WHERE environment_id=$1::uuid AND dedup_key=$2 AND rule_version=$3`, f.EnvironmentID, f.DedupKey, f.RuleVersion).Scan(&previousStatus)
		if err != nil && err != pgx.ErrNoRows {
			return 0, err
		}
		evidence := analyzer.EvidenceJSON(f.Evidence)
		signature := EvidenceSignature(evidence)
		var actionStatus, storedSignature string
		var suppressUntil *time.Time
		err = tx.QueryRow(ctx, `SELECT COALESCE(a.status,''), COALESCE(a.evidence_sha256,''), a.suppress_until
FROM finding_action a
JOIN finding existing ON existing.id=a.finding_id
WHERE existing.environment_id=$1::uuid AND existing.dedup_key=$2 AND existing.rule_version=$3`, f.EnvironmentID, f.DedupKey, f.RuleVersion).Scan(&actionStatus, &storedSignature, &suppressUntil)
		if err != nil && err != pgx.ErrNoRows {
			return 0, err
		}
		until := time.Time{}
		hasUntil := suppressUntil != nil
		if hasUntil {
			until = *suppressUntil
		}
		switch DecideDiscard(actionStatus, storedSignature, signature, until, hasUntil, time.Now()) {
		case DiscardHold:
			var id string
			err = tx.QueryRow(ctx, `SELECT id::text FROM finding WHERE environment_id=$1::uuid AND dedup_key=$2 AND rule_version=$3`, f.EnvironmentID, f.DedupKey, f.RuleVersion).Scan(&id)
			if err != nil && err != pgx.ErrNoRows {
				return 0, err
			}
			if id != "" && f.AuditRunID != "" {
				if _, err = tx.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,reason,evidence) VALUES($1::uuid,$2::uuid,'suppressed','identical evidence remains discarded',$3::jsonb) ON CONFLICT DO NOTHING`, id, f.AuditRunID, evidence); err != nil {
					return 0, err
				}
			}
			continue
		case DiscardReopen:
			if _, err = tx.Exec(ctx, `UPDATE finding SET status='suppressed', suppressed_until=now()
WHERE environment_id=$1::uuid AND dedup_key=$2 AND rule_version=$3 AND status='suppressed'`, f.EnvironmentID, f.DedupKey, f.RuleVersion); err != nil {
				return 0, err
			}
			if _, err = tx.Exec(ctx, `UPDATE finding_action SET status='suggested', evidence_sha256=NULL, suppress_until=NULL, updated_at=now()
WHERE finding_id=(SELECT id FROM finding WHERE environment_id=$1::uuid AND dedup_key=$2 AND rule_version=$3)`, f.EnvironmentID, f.DedupKey, f.RuleVersion); err != nil {
				return 0, err
			}
		}
		persisted, err := scanFinding(tx.QueryRow(ctx, upsertFindingSQL,
			f.EnvironmentID, f.AuditRunID, f.FindingType, string(f.Severity),
			f.Title, f.Summary, f.ObjectType, f.ObjectKey,
			f.DatabaseName, f.SchemaName, f.ObjectName, evidence, f.DedupKey,
			f.RuleID, f.RuleVersion, f.Category, f.Confidence, f.Impact, f.Risk, f.Recommendation, f.Validation, refs, params))
		if err != nil {
			return 0, fmt.Errorf("finding %d (%s): %w", i+1, f.FindingType, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO finding_event (finding_id,audit_run_id,event_type,category,severity,database_name,schema_name,object_name,rule_version,title,summary,recommendation,confidence,evidence,finding_status,impact,risk,validation,reference_urls,rule_parameters) VALUES ($1::uuid,$2::uuid,'observed',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,$15,$16,$17,$18::jsonb,$19::jsonb) ON CONFLICT DO NOTHING`, persisted.ID, f.AuditRunID, f.Category, string(f.Severity), f.DatabaseName, f.SchemaName, f.ObjectName, f.RuleVersion, f.Title, f.Summary, f.Recommendation, f.Confidence, analyzer.EvidenceJSON(f.Evidence), persisted.Status, f.Impact, f.Risk, f.Validation, refs, params); err != nil {
			return 0, err
		}
		if previousStatus == "resolved" || actionStatus == "discarded" && storedSignature != signature {
			if _, err = tx.Exec(ctx, `INSERT INTO finding_event (finding_id,audit_run_id,event_type,reason) VALUES ($1::uuid,$2::uuid,'reopened','evidence changed or suppression expired') ON CONFLICT DO NOTHING`, persisted.ID, f.AuditRunID); err != nil {
				return 0, err
			}
		}
		rows, err := tx.Query(ctx, `UPDATE finding SET status='resolved', resolved_at=now(), superseded_by=$1::uuid, updated_at=now()
WHERE environment_id=$2::uuid AND rule_id=$3 AND object_key=$4 AND rule_version<>$5 AND superseded_by IS NULL AND $3<>''
RETURNING id::text`, persisted.ID, f.EnvironmentID, f.RuleID, f.ObjectKey, f.RuleVersion)
		if err != nil {
			return 0, err
		}
		var oldIDs []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return 0, err
			}
			oldIDs = append(oldIDs, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, err
		}
		for _, id := range oldIDs {
			if _, err = tx.Exec(ctx, `INSERT INTO finding_event (finding_id,audit_run_id,event_type,reason) VALUES ($1::uuid,$2::uuid,'superseded',$3) ON CONFLICT DO NOTHING`, id, f.AuditRunID, "rule version changed to "+f.RuleVersion); err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	s.NotifyFindings(ctx, findings)
	return len(findings), nil
}
