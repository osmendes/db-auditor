package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/osmendes/db-auditor/internal/guidance"
)

// Finding is a persisted diagnostic.
type Finding struct {
	ID                string          `json:"id"`
	EnvironmentID     string          `json:"environment_id"`
	AuditRunID        *string         `json:"audit_run_id,omitempty"`
	FindingType       string          `json:"finding_type"`
	Severity          string          `json:"severity"`
	Status            string          `json:"status"`
	Title             string          `json:"title"`
	Summary           string          `json:"summary"`
	ObjectType        string          `json:"object_type"`
	ObjectKey         string          `json:"object_key"`
	DatabaseName      string          `json:"database_name"`
	SchemaName        string          `json:"schema_name"`
	ObjectName        string          `json:"object_name"`
	Evidence          json.RawMessage `json:"evidence"`
	DedupKey          string          `json:"dedup_key"`
	RuleID            string          `json:"rule_id"`
	RuleVersion       string          `json:"rule_version"`
	Category          string          `json:"category"`
	Confidence        float64         `json:"confidence"`
	Impact            string          `json:"impact"`
	Risk              string          `json:"risk"`
	Recommendation    string          `json:"recommendation"`
	FriendlyMeaning   string          `json:"friendly_meaning"`
	FriendlyNext      string          `json:"friendly_next"`
	Validation        string          `json:"validation"`
	References        json.RawMessage `json:"references"`
	RuleParameters    json.RawMessage `json:"rule_parameters"`
	FirstSeenAt       time.Time       `json:"first_seen_at"`
	LastSeenAt        time.Time       `json:"last_seen_at"`
	ResolvedAt        *time.Time      `json:"resolved_at,omitempty"`
	RecurrenceCount   int             `json:"recurrence_count"`
	SuppressionReason *string         `json:"suppression_reason,omitempty"`
	SuppressedUntil   *time.Time      `json:"suppressed_until,omitempty"`
	SupersededBy      *string         `json:"superseded_by,omitempty"`
	Assignee          string          `json:"assignee,omitempty"`
	DueAt             *time.Time      `json:"due_at,omitempty"`
	Notes             *string         `json:"notes,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// UpsertFindingParams is the input for inserting or refreshing a finding.
type UpsertFindingParams struct {
	EnvironmentID  string
	AuditRunID     string
	FindingType    string
	Severity       string
	Title          string
	Summary        string
	ObjectType     string
	ObjectKey      string
	DatabaseName   string
	SchemaName     string
	ObjectName     string
	Evidence       []byte
	DedupKey       string
	RuleID         string
	RuleVersion    string
	Category       string
	Confidence     float64
	Impact         string
	Risk           string
	Recommendation string
	Validation     string
	References     []byte
	RuleParameters []byte
}

const upsertFindingSQL = `
INSERT INTO finding (
  environment_id, audit_run_id, finding_type, severity, status,
  title, summary, object_type, object_key,
  database_name, schema_name, object_name,
  evidence, dedup_key, rule_id, rule_version, category, confidence, impact, risk,
  recommendation, validation, reference_urls, rule_parameters, first_seen_at, last_seen_at
) VALUES (
  $1::uuid, NULLIF($2,'')::uuid, $3, $4, 'open',
  $5, $6, $7, $8,
  $9, $10, $11,
  $12::jsonb, $13, $14, $15, $16, $17, $18, $19,
  $20, $21, $22::jsonb, $23::jsonb, now(), now()
)
ON CONFLICT (environment_id, dedup_key, rule_version) DO UPDATE SET
  last_seen_at = now(),
  audit_run_id = COALESCE(EXCLUDED.audit_run_id, finding.audit_run_id),
  severity = EXCLUDED.severity,
  title = EXCLUDED.title,
  summary = EXCLUDED.summary,
  evidence = EXCLUDED.evidence,
  rule_id = EXCLUDED.rule_id,
  rule_version = EXCLUDED.rule_version,
  category = EXCLUDED.category,
  confidence = EXCLUDED.confidence,
  impact = EXCLUDED.impact,
  risk = EXCLUDED.risk,
  recommendation = EXCLUDED.recommendation,
  validation = EXCLUDED.validation,
  reference_urls = EXCLUDED.reference_urls,
  rule_parameters = EXCLUDED.rule_parameters,
  status = CASE WHEN finding.status = 'resolved' OR (finding.status = 'suppressed' AND finding.suppressed_until <= now()) THEN 'open' ELSE finding.status END,
  recurrence_count = finding.recurrence_count + CASE WHEN finding.status = 'resolved' THEN 1 ELSE 0 END,
  resolved_at = CASE WHEN finding.status = 'resolved' THEN NULL ELSE finding.resolved_at END,
  suppression_reason = CASE WHEN finding.status = 'suppressed' AND finding.suppressed_until <= now() THEN NULL ELSE finding.suppression_reason END,
  suppressed_until = CASE WHEN finding.status = 'suppressed' AND finding.suppressed_until <= now() THEN NULL ELSE finding.suppressed_until END,
  updated_at = now()
RETURNING id::text, environment_id::text, audit_run_id::text,
  finding_type, severity, status, title, summary,
  object_type, object_key, database_name, schema_name, object_name,
  evidence, dedup_key, rule_id, rule_version, category, confidence, impact, risk,
  recommendation, validation, reference_urls, rule_parameters, first_seen_at, last_seen_at, resolved_at, notes,
  created_at, updated_at, recurrence_count, suppression_reason, suppressed_until, superseded_by::text, assignee, due_at
`

// UpsertFinding inserts a new open finding or refreshes last_seen on match.
func (s *Store) UpsertFinding(ctx context.Context, p UpsertFindingParams) (*Finding, error) {
	if len(p.Evidence) == 0 {
		p.Evidence = []byte("{}")
	}
	if len(p.References) == 0 {
		p.References = []byte("[]")
	}
	if len(p.RuleParameters) == 0 {
		p.RuleParameters = []byte("{}")
	}
	row := s.pool.QueryRow(ctx, upsertFindingSQL, p.EnvironmentID, p.AuditRunID, p.FindingType, p.Severity,
		p.Title, p.Summary, p.ObjectType, p.ObjectKey,
		p.DatabaseName, p.SchemaName, p.ObjectName,
		p.Evidence, p.DedupKey, p.RuleID, p.RuleVersion, p.Category, p.Confidence, p.Impact, p.Risk,
		p.Recommendation, p.Validation, p.References, p.RuleParameters,
	)
	f, err := scanFinding(row)
	if err != nil {
		return nil, err
	}
	if p.AuditRunID != "" {
		_, err = s.pool.Exec(ctx, `INSERT INTO finding_event (finding_id,audit_run_id,event_type,category,severity,database_name,schema_name,object_name,rule_version,title,summary,recommendation,confidence,evidence,finding_status,impact,risk,validation,reference_urls,rule_parameters) VALUES ($1::uuid,$2::uuid,'observed',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,$15,$16,$17,$18::jsonb,$19::jsonb) ON CONFLICT DO NOTHING`, f.ID, p.AuditRunID, p.Category, p.Severity, p.DatabaseName, p.SchemaName, p.ObjectName, p.RuleVersion, p.Title, p.Summary, p.Recommendation, p.Confidence, p.Evidence, f.Status, p.Impact, p.Risk, p.Validation, p.References, p.RuleParameters)
	}
	return f, err
}

// ListFindings filters findings by environment and optional dimensions.
func (s *Store) ListFindings(ctx context.Context, environmentID, findingType, severity, status string, limit int) ([]Finding, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.listFindings(ctx, environmentID, findingType, severity, status, "", false, limit, 0)
}

func (s *Store) ListFindingsPage(ctx context.Context, environmentID, findingType, severity, status string, limit, offset int) ([]Finding, int, error) {
	if limit < 1 || limit > 500 || offset < 0 {
		return nil, 0, fmt.Errorf("invalid pagination")
	}
	var total int
	err := s.pool.QueryRow(ctx, findingQueueCount, environmentID, findingType, severity, status, "", false).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.listFindings(ctx, environmentID, findingType, severity, status, "", false, limit, offset)
	return items, total, err
}

// ListFindingsQueue applies the open-queue rules plus assignee and overdue filters.
func (s *Store) ListFindingsQueue(ctx context.Context, environmentID, findingType, severity, status, assignee string, overdue bool, limit, offset int) ([]Finding, int, error) {
	if limit < 1 || limit > 500 || offset < 0 {
		return nil, 0, fmt.Errorf("invalid pagination")
	}
	var total int
	err := s.pool.QueryRow(ctx, findingQueueCount, environmentID, findingType, severity, status, assignee, overdue).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.listFindings(ctx, environmentID, findingType, severity, status, assignee, overdue, limit, offset)
	return items, total, err
}

const findingQueueWhere = `
($1 = '' OR environment_id = $1::uuid)
  AND ($2 = '' OR finding_type = $2)
  AND ($3 = '' OR severity = $3)
  AND (
    $4 = ''
    OR ($4 = 'open' AND (status = 'open' OR (status = 'suppressed' AND suppressed_until <= now())))
    OR ($4 = 'suppressed' AND status = 'suppressed' AND (suppressed_until IS NULL OR suppressed_until > now()))
    OR ($4 NOT IN ('', 'open', 'suppressed') AND status = $4)
  )
  AND ($5 = '' OR assignee = $5)
  AND ($6 = false OR (due_at IS NOT NULL AND due_at < now() AND status IN ('open', 'acknowledged')))`

const findingQueueCount = `SELECT count(*) FROM finding WHERE ` + findingQueueWhere

func (s *Store) listFindings(ctx context.Context, environmentID, findingType, severity, status, assignee string, overdue bool, limit, offset int) ([]Finding, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, environment_id::text, audit_run_id::text,
  finding_type, severity, status, title, summary,
  object_type, object_key, database_name, schema_name, object_name,
  evidence, dedup_key, rule_id, rule_version, category, confidence, impact, risk,
  recommendation, validation, reference_urls, rule_parameters, first_seen_at, last_seen_at, resolved_at, notes,
  created_at, updated_at, recurrence_count, suppression_reason, suppressed_until, superseded_by::text, assignee, due_at
FROM finding
WHERE `+findingQueueWhere+`
ORDER BY
  CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END,
  confidence DESC,
  last_seen_at DESC,
  id DESC
LIMIT $7 OFFSET $8
`, environmentID, findingType, severity, status, assignee, overdue, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Finding, 0)
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// GetFinding returns one finding by id.
func (s *Store) GetFinding(ctx context.Context, id string) (*Finding, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id::text, environment_id::text, audit_run_id::text,
  finding_type, severity, status, title, summary,
  object_type, object_key, database_name, schema_name, object_name,
  evidence, dedup_key, rule_id, rule_version, category, confidence, impact, risk,
  recommendation, validation, reference_urls, rule_parameters, first_seen_at, last_seen_at, resolved_at, notes,
  created_at, updated_at, recurrence_count, suppression_reason, suppressed_until, superseded_by::text, assignee, due_at
FROM finding WHERE id = $1::uuid
`, id)
	return scanFinding(row)
}

// UpdateFindingStatus changes triage status and optional notes.
func (s *Store) UpdateFindingStatus(ctx context.Context, id, status, notes string) (*Finding, error) {
	row := s.pool.QueryRow(ctx, `
UPDATE finding
SET status = $2,
    notes = COALESCE(NULLIF($3,''), notes),
    resolved_at = CASE WHEN $2 IN ('resolved', 'suppressed') THEN now() ELSE resolved_at END,
    updated_at = now()
WHERE id = $1::uuid
RETURNING id::text, environment_id::text, audit_run_id::text,
  finding_type, severity, status, title, summary,
  object_type, object_key, database_name, schema_name, object_name,
  evidence, dedup_key, rule_id, rule_version, category, confidence, impact, risk,
  recommendation, validation, reference_urls, rule_parameters, first_seen_at, last_seen_at, resolved_at, notes,
  created_at, updated_at, recurrence_count, suppression_reason, suppressed_until, superseded_by::text, assignee, due_at
`, id, status, notes)
	f, err := scanFinding(row)
	if err != nil {
		return nil, fmt.Errorf("update finding: %w", err)
	}
	eventType := status
	if status == "open" {
		eventType = "reopened"
	}
	if status == "acknowledged" || status == "resolved" || status == "open" {
		if _, err := s.pool.Exec(ctx, `INSERT INTO finding_event(finding_id,event_type,reason) VALUES ($1::uuid,$2,$3)`, id, eventType, notes); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func scanFinding(row scannable) (*Finding, error) {
	var f Finding
	var auditRun, notes, suppressionReason, supersededBy, assignee pgtype.Text
	var resolvedAt, suppressedUntil, dueAt pgtype.Timestamptz
	var evidence []byte
	if err := row.Scan(
		&f.ID, &f.EnvironmentID, &auditRun,
		&f.FindingType, &f.Severity, &f.Status, &f.Title, &f.Summary,
		&f.ObjectType, &f.ObjectKey, &f.DatabaseName, &f.SchemaName, &f.ObjectName,
		&evidence, &f.DedupKey, &f.RuleID, &f.RuleVersion, &f.Category, &f.Confidence, &f.Impact, &f.Risk,
		&f.Recommendation, &f.Validation, &f.References, &f.RuleParameters,
		&f.FirstSeenAt, &f.LastSeenAt, &resolvedAt, &notes,
		&f.CreatedAt, &f.UpdatedAt, &f.RecurrenceCount, &suppressionReason, &suppressedUntil, &supersededBy,
		&assignee, &dueAt,
	); err != nil {
		return nil, err
	}
	if auditRun.Valid {
		s := auditRun.String
		f.AuditRunID = &s
	}
	if notes.Valid {
		s := notes.String
		f.Notes = &s
	}
	if resolvedAt.Valid {
		t := resolvedAt.Time
		f.ResolvedAt = &t
	}
	if suppressionReason.Valid {
		f.SuppressionReason = &suppressionReason.String
	}
	if suppressedUntil.Valid {
		t := suppressedUntil.Time
		f.SuppressedUntil = &t
	}
	if supersededBy.Valid {
		f.SupersededBy = &supersededBy.String
	}
	if assignee.Valid {
		f.Assignee = assignee.String
	}
	if dueAt.Valid {
		t := dueAt.Time
		f.DueAt = &t
	}
	if len(evidence) == 0 {
		f.Evidence = json.RawMessage("{}")
	} else {
		f.Evidence = evidence
	}
	friendly := guidance.For(f.FindingType)
	f.FriendlyMeaning = friendly.Meaning
	f.FriendlyNext = friendly.Next
	return &f, nil
}
