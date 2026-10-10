package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/report"
)

func (s *Store) LoadReportDocument(ctx context.Context, job ReportJob) (report.Document, error) {
	d := report.Document{Type: job.Type, RunID: job.AuditRunID, RuleVersion: job.RuleVersion, RequestedBy: job.RequestedBy, DatabaseFilter: job.Filters.Database, SchemaFilter: job.Filters.Schema, TableFilter: job.Filters.Table, SeverityFilter: job.Filters.Severity, RedactionLevel: job.Filters.Redaction, Databases: []report.Database{}, Tables: []report.Table{}, Findings: []report.Finding{}, CoverageNotes: []string{}}
	err := s.pool.QueryRow(ctx, `SELECT e.name,r.started_at,r.status,r.service_version FROM audit_run r JOIN audit_environment e ON e.id=r.environment_id WHERE r.id=$1::uuid AND r.environment_id=$2::uuid AND r.status IN ('success','partial_success')`, job.AuditRunID, job.EnvironmentID).Scan(&d.Environment, &d.RunStarted, &d.RunStatus, &d.ServiceVersion)
	if err != nil {
		return d, fmt.Errorf("report run: %w", err)
	}
	completeness, err := s.GetSnapshotCompleteness(ctx, job.EnvironmentID, job.AuditRunID)
	if err != nil {
		return d, err
	}
	d.Coverage = completeness.Completeness
	if completeness.Completeness != "complete" {
		d.CoverageNotes = append(d.CoverageNotes, "Execucao parcial; ausencia de objetos/findings nao e prova de resolucao.")
	}
	if job.Filters.Severity != "" {
		d.CoverageNotes = append(d.CoverageNotes, "O score considera todas as severidades do escopo, inclusive as filtradas desta lista.")
	}
	rows, err := s.pool.Query(ctx, `SELECT d.database_name,
(SELECT count(*) FROM schema_snapshot x WHERE x.audit_run_id=d.audit_run_id AND x.database_name=d.database_name),
(SELECT count(*) FROM table_snapshot x WHERE x.audit_run_id=d.audit_run_id AND x.database_name=d.database_name),d.size_bytes
FROM database_snapshot d WHERE d.audit_run_id=$1::uuid AND ($2='' OR d.database_name=$2) ORDER BY d.database_name LIMIT 501`, job.AuditRunID, job.Filters.Database)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var item report.Database
		if err = rows.Scan(&item.Name, &item.Schemas, &item.Tables, &item.SizeBytes); err != nil {
			rows.Close()
			return d, err
		}
		d.Databases = append(d.Databases, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Databases) > 500 {
		d.Databases = d.Databases[:500]
		d.Truncated = true
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM database_snapshot WHERE audit_run_id=$1::uuid AND ($2='' OR database_name=$2) AND NOT is_template`, job.AuditRunID, job.Filters.Database).Scan(&d.TotalDatabases); err != nil {
		return d, err
	}
	limit := 5001
	if job.Type == "executive" {
		limit = 21
	}
	rows, err = s.pool.Query(ctx, `SELECT database_name,schema_name,table_name,total_size_bytes,row_estimate,has_primary_key FROM table_snapshot WHERE audit_run_id=$1::uuid AND ($2='' OR database_name=$2) AND ($3='' OR schema_name=$3) AND ($4='' OR table_name=$4) ORDER BY CASE WHEN $6='executive' THEN total_size_bytes END DESC NULLS LAST,database_name,schema_name,table_name LIMIT $5`, job.AuditRunID, job.Filters.Database, job.Filters.Schema, job.Filters.Table, limit, job.Type)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var item report.Table
		if err = rows.Scan(&item.Database, &item.Schema, &item.Name, &item.SizeBytes, &item.Rows, &item.HasPrimaryKey); err != nil {
			rows.Close()
			return d, err
		}
		d.Tables = append(d.Tables, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Tables) >= limit {
		d.Tables = d.Tables[:limit-1]
		d.Truncated = true
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM table_snapshot WHERE audit_run_id=$1::uuid AND ($2='' OR database_name=$2) AND ($3='' OR schema_name=$3) AND ($4='' OR table_name=$4)`, job.AuditRunID, job.Filters.Database, job.Filters.Schema, job.Filters.Table).Scan(&d.TotalTables); err != nil {
		return d, err
	}
	rows, err = s.pool.Query(ctx, `SELECT f.id::text,left(f.finding_type,128),e.severity,left(e.category,128),left(e.database_name,256),left(e.schema_name,256),left(e.object_name,256),left(e.title,512),left(e.summary,1024),left(e.recommendation,1024),e.confidence,left(e.evidence::text,500),left(e.rule_version,128) FROM finding_event e JOIN finding f ON f.id=e.finding_id WHERE e.audit_run_id=$1::uuid AND e.event_type='observed' AND ($2='' OR e.database_name=$2) AND ($3='' OR e.schema_name=$3) AND ($4='' OR e.object_name=$4) AND ($5='' OR e.severity=$5) ORDER BY e.severity,e.category,e.database_name,e.schema_name,e.object_name,e.title LIMIT 2001`, job.AuditRunID, job.Filters.Database, job.Filters.Schema, job.Filters.Table, job.Filters.Severity)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var item report.Finding
		if err = rows.Scan(&item.ID, &item.Type, &item.Severity, &item.Category, &item.Database, &item.Schema, &item.Object, &item.Title, &item.Summary, &item.Recommendation, &item.Confidence, &item.Evidence, &item.RuleVersion); err != nil {
			rows.Close()
			return d, err
		}
		d.Findings = append(d.Findings, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Findings) > 2000 {
		d.Findings = d.Findings[:2000]
		d.Truncated = true
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM finding_event WHERE audit_run_id=$1::uuid AND event_type='observed' AND ($2='' OR database_name=$2) AND ($3='' OR schema_name=$3) AND ($4='' OR object_name=$4) AND ($5='' OR severity=$5)`, job.AuditRunID, job.Filters.Database, job.Filters.Schema, job.Filters.Table, job.Filters.Severity).Scan(&d.TotalFindings); err != nil {
		return d, err
	}
	score, err := s.GetScopeScore(ctx, job.EnvironmentID, job.AuditRunID, job.Filters.Database, job.Filters.Schema, job.Filters.Table)
	if err != nil {
		return d, err
	}
	if score != nil {
		d.Score = score.Score
		d.ScoreConfidence = score.Confidence
		for _, category := range score.Categories {
			d.ScoreCategories = append(d.ScoreCategories, report.ScoreCategory{Category: category.Category, Score: category.Score, Penalty: category.Penalty, Positive: category.Positive, Findings: category.Findings})
		}
		if len(score.MissingCollectors) > 0 {
			d.CoverageNotes = append(d.CoverageNotes, "Score sem cobertura suficiente: "+strings.Join(score.MissingCollectors, ", "))
		}
	}
	rows, err = s.pool.Query(ctx, `SELECT r.id::text,r.started_at,r.status,COALESCE(sum(t.total_size_bytes),0),count(t.id) FROM audit_run r LEFT JOIN table_snapshot t ON t.audit_run_id=r.id AND ($3='' OR t.database_name=$3) WHERE r.environment_id=$1::uuid AND r.status IN ('success','partial_success') AND r.started_at<=$2 GROUP BY r.id,r.started_at,r.status ORDER BY r.started_at DESC LIMIT 6`, job.EnvironmentID, d.RunStarted, job.Filters.Database)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var p report.GrowthPoint
		if err = rows.Scan(&p.RunID, &p.At, &p.Status, &p.SizeBytes, &p.Tables); err != nil {
			rows.Close()
			return d, err
		}
		d.Growth = append(d.Growth, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	for left, right := 0, len(d.Growth)-1; left < right; left, right = left+1, right-1 {
		d.Growth[left], d.Growth[right] = d.Growth[right], d.Growth[left]
	}
	var baseline report.Baseline
	err = s.pool.QueryRow(ctx, `SELECT baseline_run_id::text,status,added_tables,removed_tables,changed_tables FROM baseline_comparison WHERE audit_run_id=$1::uuid AND ($2='' OR database_name=$2) ORDER BY database_name LIMIT 1`, job.AuditRunID, job.Filters.Database).Scan(&baseline.RunID, &baseline.Status, &baseline.AddedTables, &baseline.RemovedTables, &baseline.ChangedTables)
	if err == nil {
		d.Baseline = &baseline
		rows, err = s.pool.Query(ctx, `SELECT e.finding_id::text,f.finding_type,e.severity,concat_ws('.',e.database_name,e.schema_name,e.object_name)
FROM finding_event e JOIN finding f ON f.id=e.finding_id
WHERE e.audit_run_id=$1::uuid AND e.event_type='observed' AND e.severity IN ('critical','high')
AND ($3='' OR e.database_name=$3) AND ($4='' OR e.schema_name=$4) AND ($5='' OR e.object_name=$5)
AND NOT EXISTS (SELECT 1 FROM finding_event old WHERE old.audit_run_id=$2::uuid AND old.finding_id=e.finding_id AND old.event_type='observed')
ORDER BY CASE e.severity WHEN 'critical' THEN 0 ELSE 1 END,e.finding_id LIMIT 5`, job.AuditRunID, baseline.RunID, job.Filters.Database, job.Filters.Schema, job.Filters.Table)
		if err != nil {
			return d, err
		}
		for rows.Next() {
			var regression report.Regression
			if err = rows.Scan(&regression.FindingID, &regression.Type, &regression.Severity, &regression.Object); err != nil {
				rows.Close()
				return d, err
			}
			d.Regressions = append(d.Regressions, regression)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return d, err
		}
	} else if err != pgx.ErrNoRows {
		return d, err
	}
	if job.Filters.Table != "" && len(d.Tables) == 0 {
		return d, fmt.Errorf("table not present in requested run")
	}
	if d.Truncated {
		d.CoverageNotes = append(d.CoverageNotes, "Listagens limitadas a 500 bancos, 5000 tabelas e 2000 findings.")
	}
	for i := range d.CoverageNotes {
		d.CoverageNotes[i] = strings.TrimSpace(d.CoverageNotes[i])
	}
	return d, nil
}
