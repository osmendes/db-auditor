package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/quality"
)

var ErrQualityNotFound = errors.New("quality issue not found in environment")
var ErrQualityEvidenceRequired = errors.New("later comparable quality scan required")

type QualityIssue struct {
	ID               string       `json:"id"`
	Kind             string       `json:"check_kind"`
	Column           string       `json:"column_name"`
	AffectedRows     int          `json:"affected_rows"`
	SampledRows      int          `json:"sampled_rows"`
	Status           string       `json:"status"`
	Owner            string       `json:"owner"`
	Justification    string       `json:"justification"`
	Result           string       `json:"result"`
	UpdatedBy        string       `json:"updated_by"`
	ValidationScanID *string      `json:"validation_scan_id,omitempty"`
	Plan             quality.Plan `json:"plan"`
	PreviousAffected *int         `json:"previous_affected_rows,omitempty"`
	Comparable       bool         `json:"comparable"`
	ComparisonNote   string       `json:"comparison_note"`
}

type QualityScan struct {
	ID            string         `json:"id"`
	EnvironmentID string         `json:"environment_id"`
	Database      string         `json:"database"`
	Schema        string         `json:"schema"`
	Table         string         `json:"table"`
	SampleLimit   int            `json:"sample_limit"`
	SampledRows   int            `json:"sampled_rows"`
	SampleMethod  string         `json:"sample_method"`
	SamplingNote  string         `json:"sampling_note"`
	Actor         string         `json:"actor"`
	CreatedAt     time.Time      `json:"created_at"`
	Issues        []QualityIssue `json:"issues"`
}

func (s *Store) SaveQualityScan(ctx context.Context, env, actor string, request quality.Request, result quality.Result) (*QualityScan, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	if err = tx.QueryRow(ctx, `INSERT INTO quality_scan(environment_id,database_name,schema_name,table_name,sample_limit,sampled_rows,sample_method,actor) VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, env, request.Database, request.Schema, request.Table, request.Limit, result.SampledRows, result.SampleMethod, actor).Scan(&id); err != nil {
		return nil, err
	}
	for _, issue := range result.Issues {
		if _, err = tx.Exec(ctx, `INSERT INTO quality_issue(scan_id,check_kind,column_name,affected_rows,sampled_rows) VALUES($1::uuid,$2,$3,$4,$5)`, id, issue.Kind, issue.Column, issue.AffectedRows, issue.SampledRows); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetQualityScan(ctx, env, id)
}

func (s *Store) GetQualityScan(ctx context.Context, env, id string) (*QualityScan, error) {
	var item QualityScan
	err := s.pool.QueryRow(ctx, `SELECT id::text,environment_id::text,database_name,schema_name,table_name,sample_limit,sampled_rows,sample_method,actor,created_at FROM quality_scan WHERE environment_id=$1::uuid AND id=$2::uuid`, env, id).Scan(&item.ID, &item.EnvironmentID, &item.Database, &item.Schema, &item.Table, &item.SampleLimit, &item.SampledRows, &item.SampleMethod, &item.Actor, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.SamplingNote = "Margem de erro amostral desconhecida: a seleção por páginas ou ordem física não é uma amostra aleatória independente. As contagens descrevem somente as linhas lidas."
	rows, err := s.pool.Query(ctx, `SELECT id::text,check_kind,column_name,affected_rows,sampled_rows,status,owner_name,justification,result_note,updated_by,validation_scan_id::text FROM quality_issue WHERE scan_id=$1::uuid ORDER BY check_kind,column_name,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	item.Issues = []QualityIssue{}
	for rows.Next() {
		var issue QualityIssue
		if err = rows.Scan(&issue.ID, &issue.Kind, &issue.Column, &issue.AffectedRows, &issue.SampledRows, &issue.Status, &issue.Owner, &issue.Justification, &issue.Result, &issue.UpdatedBy, &issue.ValidationScanID); err != nil {
			return nil, err
		}
		issue.Plan = quality.PlanFor(issue.Kind)
		item.Issues = append(item.Issues, issue)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range item.Issues {
		issue := &item.Issues[i]
		var previousAffected, previousRows, previousLimit int
		var previousMethod string
		err = s.pool.QueryRow(ctx, `SELECT q.affected_rows,q.sampled_rows,s.sample_limit,s.sample_method FROM quality_issue q JOIN quality_scan s ON s.id=q.scan_id WHERE s.environment_id=$1::uuid AND s.database_name=$2 AND s.schema_name=$3 AND s.table_name=$4 AND s.created_at<$5 AND q.check_kind=$6 AND q.column_name=$7 ORDER BY s.created_at DESC,s.id DESC LIMIT 1`, env, item.Database, item.Schema, item.Table, item.CreatedAt, issue.Kind, issue.Column).Scan(&previousAffected, &previousRows, &previousLimit, &previousMethod)
		if errors.Is(err, pgx.ErrNoRows) {
			issue.ComparisonNote = "Sem diagnóstico anterior comparável."
			continue
		}
		if err != nil {
			return nil, err
		}
		// A prior count alone cannot establish a trend when sample sizes differ.
		issue.PreviousAffected = &previousAffected
		issue.Comparable = previousRows == issue.SampledRows && previousLimit == item.SampleLimit && previousMethod == item.SampleMethod
		if issue.Comparable {
			issue.ComparisonNote = "Mesmo limite e número de linhas; páginas amostradas podem mudar após alterações na tabela."
		} else {
			issue.ComparisonNote = "Método, limite ou número de linhas diferente; não compare contagens diretamente."
		}
	}
	return &item, nil
}

func (s *Store) ListQualityScans(ctx context.Context, env string) ([]QualityScan, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM quality_scan WHERE environment_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 50`, env)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []QualityScan{}
	for _, id := range ids {
		item, err := s.GetQualityScan(ctx, env, id)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (s *Store) UpdateQualityIssue(ctx context.Context, env, id, actor, status, owner, justification, result string) (*QualityScan, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var scanID, previous string
	err = tx.QueryRow(ctx, `SELECT s.id::text,q.status FROM quality_issue q JOIN quality_scan s ON s.id=q.scan_id WHERE s.environment_id=$1::uuid AND q.id=$2::uuid FOR UPDATE OF q`, env, id).Scan(&scanID, &previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrQualityNotFound
	}
	if err != nil {
		return nil, err
	}
	var validationScanID *string
	if status == "validated" {
		if previous != "executed_externally" {
			return nil, ErrQualityEvidenceRequired
		}
		var later string
		err = tx.QueryRow(ctx, `SELECT newer.id::text FROM quality_issue original
JOIN quality_scan old ON old.id=original.scan_id
JOIN quality_scan newer ON newer.environment_id=old.environment_id AND newer.database_name=old.database_name
  AND newer.schema_name=old.schema_name AND newer.table_name=old.table_name AND newer.created_at>original.updated_at
  AND newer.sample_limit=old.sample_limit AND newer.sample_method=old.sample_method AND newer.sampled_rows=old.sampled_rows
JOIN quality_issue newer_issue ON newer_issue.scan_id=newer.id AND newer_issue.check_kind=original.check_kind
  AND newer_issue.column_name=original.column_name AND newer_issue.sampled_rows=original.sampled_rows
WHERE original.id=$1::uuid AND old.environment_id=$2::uuid
ORDER BY newer.created_at DESC,newer.id DESC LIMIT 1`, id, env).Scan(&later)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrQualityEvidenceRequired
		}
		if err != nil {
			return nil, err
		}
		validationScanID = &later
	}
	if _, err = tx.Exec(ctx, `UPDATE quality_issue SET status=$2,owner_name=$3,justification=$4,result_note=$5,updated_by=$6,updated_at=now(),validation_scan_id=$7::uuid WHERE id=$1::uuid`, id, status, owner, justification, result, actor, validationScanID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO quality_issue_event(issue_id,previous_status,new_status,actor,owner_name,justification,result_note,validation_scan_id) VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8::uuid)`, id, previous, status, actor, owner, justification, result, validationScanID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetQualityScan(ctx, env, scanID)
}
