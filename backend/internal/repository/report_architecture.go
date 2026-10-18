package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/osmendes/db-auditor/internal/report"
)

// LoadArchitectureDocument reads only the control database and one immutable run.
func (s *Store) LoadArchitectureDocument(ctx context.Context, job ReportJob) (report.ArchitectureDocument, error) {
	d := report.ArchitectureDocument{RunID: job.AuditRunID, Database: job.Filters.Database, ScopeSchema: job.Filters.Schema, RuleVersion: job.RuleVersion, Redaction: job.Filters.Redaction}
	d.Decisions = job.Decisions
	if d.Database == "" {
		return d, fmt.Errorf("architecture report requires a database")
	}
	err := s.pool.QueryRow(ctx, `SELECT e.name,r.started_at,r.status FROM audit_run r JOIN audit_environment e ON e.id=r.environment_id WHERE r.id=$1::uuid AND r.environment_id=$2::uuid AND r.status IN ('success','partial_success')`, job.AuditRunID, job.EnvironmentID).Scan(&d.Environment, &d.RunStarted, &d.RunStatus)
	if err != nil {
		return d, err
	}
	var present bool
	err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3)`, job.EnvironmentID, job.AuditRunID, d.Database).Scan(&present)
	if err != nil {
		return d, err
	}
	if !present {
		return d, fmt.Errorf("database not present in requested run")
	}
	schemaRows, err := s.pool.Query(ctx, `SELECT schema_name FROM schema_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4) ORDER BY schema_name LIMIT 501`, job.EnvironmentID, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	for schemaRows.Next() {
		var name string
		if err = schemaRows.Scan(&name); err != nil {
			break
		}
		d.Schemas = append(d.Schemas, name)
	}
	if err == nil {
		err = schemaRows.Err()
	}
	schemaRows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Schemas) > 500 {
		return d, fmt.Errorf("architecture scope exceeds 500 schemas")
	}
	coverage, err := s.GetSnapshotCompleteness(ctx, job.EnvironmentID, job.AuditRunID)
	if err != nil {
		return d, err
	}
	d.Coverage = coverage.Completeness
	if d.Coverage != "complete" {
		d.CoverageNotes = append(d.CoverageNotes, "Execução parcial; ausência de fatos não comprova ausência de problemas.")
	}
	rows, err := s.pool.Query(ctx, `SELECT schema_name,table_name,total_size_bytes,row_estimate,collected_at FROM table_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4) ORDER BY schema_name,table_name LIMIT 501`, job.EnvironmentID, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var t report.ArchitectureTable
		if err = rows.Scan(&t.Schema, &t.Name, &t.SizeBytes, &t.Rows, &t.CollectedAt); err != nil {
			break
		}
		t.ID = d.Database + "." + t.Schema + "." + t.Name
		d.Tables = append(d.Tables, t)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Tables) > 500 {
		return d, fmt.Errorf("architecture scope exceeds 500 tables; select a schema")
	}
	insertionVersions := make(map[string]int)
	for _, decision := range d.Decisions {
		if decision.Subject == "insertion" {
			insertionVersions[decision.ObjectID] = decision.Version
		}
	}
	rows, err = s.pool.Query(ctx, `SELECT database_name||'.'||schema_name||'.'||table_name,column_name,latest_at,collected_at,status,reason,append_only,decision_version
FROM architecture_insertion_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
ORDER BY schema_name,table_name LIMIT 501`, job.EnvironmentID, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var item report.InsertionEvidence
		var version int
		if err = rows.Scan(&item.ObjectID, &item.Column, &item.Latest, &item.CollectedAt, &item.Status, &item.Reason, &item.AppendOnly, &version); err != nil {
			break
		}
		if insertionVersions[item.ObjectID] != version {
			item.Status = "invalid_mapping"
			item.Reason = "contrato de inserção editado após a coleta; nova execução necessária"
			item.Latest = nil
		}
		d.Insertions = append(d.Insertions, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Insertions) > 500 {
		return d, fmt.Errorf("architecture scope exceeds 500 insertion observations")
	}
	rows, err = s.pool.Query(ctx, `SELECT schema_name,table_name,column_name,data_type FROM column_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4) ORDER BY schema_name,table_name,ordinal_position LIMIT 10001`, job.EnvironmentID, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	columns := map[string][]report.ArchitectureColumn{}
	count := 0
	for rows.Next() {
		var schema, table, name, kind string
		if err = rows.Scan(&schema, &table, &name, &kind); err != nil {
			break
		}
		count++
		columns[schema+"\x00"+table] = append(columns[schema+"\x00"+table], report.ArchitectureColumn{Name: name, Type: kind})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	if count > 10000 {
		return d, fmt.Errorf("architecture scope exceeds 10000 columns; select a schema")
	}
	for i := range d.Tables {
		d.Tables[i].Columns = columns[d.Tables[i].Schema+"\x00"+d.Tables[i].Name]
	}
	rows, err = s.pool.Query(ctx, `SELECT id::text,schema_name,table_name,constraint_name,constraint_type,COALESCE(supporting_index,''),COALESCE(constrained_columns,ARRAY[]::text[]),COALESCE(referenced_schema_name,''),COALESCE(referenced_table_name,''),COALESCE(referenced_columns,ARRAY[]::text[]),is_validated,collected_at FROM constraint_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4) AND constraint_type IN ('p','u','f') ORDER BY schema_name,table_name,constraint_name LIMIT 5001`, job.EnvironmentID, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var c report.ArchitectureConstraint
		if err = rows.Scan(&c.ID, &c.Schema, &c.Table, &c.Name, &c.Kind, &c.SupportingIndex, &c.Columns, &c.RefSchema, &c.RefTable, &c.RefColumns, &c.Validated, &c.CollectedAt); err != nil {
			break
		}
		d.Constraints = append(d.Constraints, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Constraints) > 5000 {
		return d, fmt.Errorf("architecture scope exceeds 5000 constraints; select a schema")
	}
	rows, err = s.pool.Query(ctx, `SELECT f.id::text,f.finding_type,e.severity,e.category,e.database_name,e.schema_name,e.object_name,left(e.title,512),left(e.summary,1024),left(e.recommendation,1024),e.confidence,left(e.evidence::text,1000),e.rule_version FROM finding_event e JOIN finding f ON f.id=e.finding_id WHERE e.audit_run_id=$1::uuid AND e.event_type='observed' AND e.database_name=$2 AND ($3='' OR e.schema_name=$3) ORDER BY e.severity,e.schema_name,e.object_name LIMIT 1001`, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var f report.Finding
		if err = rows.Scan(&f.ID, &f.Type, &f.Severity, &f.Category, &f.Database, &f.Schema, &f.Object, &f.Title, &f.Summary, &f.Recommendation, &f.Confidence, &f.Evidence, &f.RuleVersion); err != nil {
			break
		}
		d.Findings = append(d.Findings, f)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Findings) > 1000 {
		return d, fmt.Errorf("architecture scope exceeds 1000 findings; select a schema")
	}
	rows, err = s.pool.Query(ctx, `SELECT extension_name FROM extension_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 ORDER BY extension_name LIMIT 101`, job.EnvironmentID, job.AuditRunID, d.Database)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			break
		}
		d.Extensions = append(d.Extensions, name)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Extensions) > 100 {
		return d, fmt.Errorf("architecture scope exceeds 100 extensions")
	}
	const detailSQL = `SELECT object_id,fact,value,source,collected_at FROM (
SELECT database_name||'.'||schema_name||'.'||hypertable_name object_id,'hypertable' fact,
'chunks='||num_chunks||'; compression='||compression_enabled::text||'; bytes='||total_size_bytes value,
'hypertable_snapshot' source,collected_at FROM hypertable_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
UNION ALL SELECT database_name||'.'||schema_name||'.'||view_name,'continuous_aggregate',
'materialization='||COALESCE(materialization_schema,'?')||'.'||COALESCE(materialization_hypertable,'?')||'; materialized_only='||materialized_only::text,
'continuous_aggregate_snapshot',collected_at FROM continuous_aggregate_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
UNION ALL SELECT database_name||'.job.'||job_id::text,'timescale_job',
'scheduled='||scheduled::text||'; interval='||COALESCE(schedule_interval,'?')||'; hypertable='||COALESCE(hypertable_schema,'?')||'.'||COALESCE(hypertable_name,'?'),
'job_snapshot',collected_at FROM job_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
UNION ALL SELECT database_name||'.'||schema_name||'.'||hypertable_name,'chunk_summary',
'observed='||count(*)::text||'; compressed='||(count(*) FILTER (WHERE is_compressed))::text||'; bytes='||COALESCE(sum(total_size_bytes),0)::text,
'chunk_snapshot',max(collected_at) FROM chunk_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
GROUP BY database_name,schema_name,hypertable_name
) x ORDER BY object_id,fact LIMIT 1001`
	rows, err = s.pool.Query(ctx, detailSQL, job.EnvironmentID, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var item report.ArchitectureDetail
		if err = rows.Scan(&item.ObjectID, &item.Fact, &item.Value, &item.Source, &item.CollectedAt); err != nil {
			break
		}
		d.Details = append(d.Details, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Details) > 1000 {
		return d, fmt.Errorf("architecture scope exceeds 1000 technical objects; select a schema")
	}
	metricQuery := `SELECT kind,item_count,bytes,at FROM (
SELECT 'hypertables' kind,count(*) item_count,COALESCE(sum(total_size_bytes),0) bytes,max(collected_at) at FROM hypertable_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
UNION ALL SELECT 'chunks',count(*),COALESCE(sum(total_size_bytes),0),max(collected_at) FROM chunk_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
UNION ALL SELECT 'caggs',count(*),0,max(collected_at) FROM continuous_aggregate_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
UNION ALL SELECT 'jobs',count(*),0,max(collected_at) FROM job_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
UNION ALL SELECT 'workload_fingerprints',count(*),0,max(collected_at) FROM workload_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3
UNION ALL SELECT 'temp_written',count(*),COALESCE(sum(temp_blks_written::numeric*block_size_bytes),0)::bigint,max(collected_at) FROM workload_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND block_size_bytes>0 AND temp_blks_written IS NOT NULL
UNION ALL SELECT 'temp_read',count(*),COALESCE(sum(temp_blks_read::numeric*block_size_bytes),0)::bigint,max(collected_at) FROM workload_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND block_size_bytes>0 AND temp_blks_read IS NOT NULL
UNION ALL SELECT 'indexes',count(*),COALESCE(sum(size_bytes),0),max(collected_at) FROM index_snapshot WHERE environment_id=$1::uuid AND audit_run_id=$2::uuid AND database_name=$3 AND ($4='' OR schema_name=$4)
) x ORDER BY kind`
	rows, err = s.pool.Query(ctx, metricQuery, job.EnvironmentID, job.AuditRunID, d.Database, job.Filters.Schema)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var kind string
		var count, bytes int64
		var at *time.Time
		if err = rows.Scan(&kind, &count, &bytes, &at); err != nil {
			break
		}
		if count == 0 {
			if kind == "workload_fingerprints" {
				d.CoverageNotes = append(d.CoverageNotes, "pg_stat_statements sem evidência nesta execução; métricas temporárias indisponíveis.")
			}
			continue
		}
		scope := d.Database
		if job.Filters.Schema != "" && kind != "jobs" && kind != "workload_fingerprints" && kind != "temp_written" && kind != "temp_read" {
			scope += "." + job.Filters.Schema
		}
		source := map[string]string{"hypertables": "hypertable_snapshot", "chunks": "chunk_snapshot", "caggs": "continuous_aggregate_snapshot", "jobs": "job_snapshot", "workload_fingerprints": "workload_snapshot", "temp_written": "workload_snapshot", "temp_read": "workload_snapshot", "indexes": "index_snapshot"}[kind]
		d.Metrics = append(d.Metrics, report.ArchitectureMetric{Name: kind, Value: count, Unit: "objetos", Source: source, Scope: scope, CollectedAt: *at})
		if bytes > 0 {
			d.Metrics = append(d.Metrics, report.ArchitectureMetric{Name: kind + "_size", Value: bytes, Unit: "bytes", Source: source, Scope: scope, CollectedAt: *at})
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return d, err
	}
	timescaleObserved := false
	for _, name := range d.Extensions {
		if name == "timescaledb" {
			timescaleObserved = true
			break
		}
	}
	if !timescaleObserved {
		d.CoverageNotes = append(d.CoverageNotes, "TimescaleDB: não aplicável ou extensão não observada.")
	}
	if d.Redaction == "identifiers" || d.Redaction == "strict" {
		redactArchitecture(&d)
	}
	return d, nil
}

func redactArchitecture(d *report.ArchitectureDocument) {
	d.Environment = "[oculto]"
	d.Database = "[oculto]"
	d.ScopeSchema = "[oculto]"
	for i := range d.Tables {
		t := &d.Tables[i]
		t.ID = fmt.Sprintf("objeto_%d", i+1)
		t.Schema = "escopo"
		t.Name = t.ID
		for j := range t.Columns {
			t.Columns[j].Name = fmt.Sprintf("coluna_%d", j+1)
			t.Columns[j].Type = "oculto"
		}
	}
	// Constraint endpoints cannot be reconstructed safely after redaction. Omit
	// edges instead of risking disclosure in either Mermaid or the text fallback.
	d.Constraints = nil
	for i := range d.Findings {
		f := &d.Findings[i]
		f.Database = "[oculto]"
		f.Schema = "[oculto]"
		f.Object = "[oculto]"
		f.Title = "Achado com identificadores ocultos"
		f.Evidence = "[evidência omitida]"
	}
	d.Extensions = nil
	d.Schemas = nil
	d.Decisions = nil
	d.Insertions = nil
	d.Details = nil
	for i := range d.Metrics {
		d.Metrics[i].Scope = "[oculto]"
	}
	for i := range d.CoverageNotes {
		d.CoverageNotes[i] = "Detalhes de cobertura omitidos neste nível de redação."
	}
	if d.Redaction == "strict" {
		d.Tables = nil
	}
}
