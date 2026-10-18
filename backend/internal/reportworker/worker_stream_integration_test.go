package reportworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/report"
	"github.com/osmendes/db-auditor/internal/repository"
)

// PF-07: the PDF is rendered straight into the report_artifact COPY stream.
// The stored bytes must equal the in-memory renderer, the memory budget must
// stop a job with a readable reason, and a failed stream must leave no artifact.
func TestReportStreamingArtifactIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `UPDATE report_job SET status='cancelled' WHERE status='queued'`); err != nil {
		t.Fatal(err)
	}
	store := repository.NewStore(pool)
	var env, run string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version) VALUES($1::uuid,'manual','success','test','test') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name,size_bytes) VALUES($1::uuid,$2::uuid,'db',1000)`, run, env); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key,total_size_bytes)
SELECT $1::uuid,$2::uuid,'db','public','tabela_'||g,g%2=0,g*1000 FROM generate_series(1,3000) g`, run, env); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version,rule_manifest_hash) VALUES($1::uuid,$2::uuid,'success','test','manifest-test')`, run, env); err != nil {
		t.Fatal(err)
	}

	job, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "technical", IdempotencyKey: "stream-ok"})
	if err != nil {
		t.Fatal(err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	finished, err := store.GetReportJob(ctx, env, job.ID)
	if err != nil || finished.Status != "success" {
		t.Fatalf("finished: %#v %v", finished, err)
	}
	artifact, err := store.GetReportArtifact(ctx, env, job.ID)
	if err != nil || artifact == nil {
		t.Fatalf("artifact: %#v %v", artifact, err)
	}
	if err = report.ValidatePDF(artifact.Content); err != nil || report.PageCount(artifact.Content) < 2 {
		t.Fatalf("stored PDF invalid (%v) or not paginated (%d pages)", err, report.PageCount(artifact.Content))
	}
	sum := sha256.Sum256(artifact.Content)
	if hex.EncodeToString(sum[:]) != artifact.SHA256 {
		t.Fatal("stored hash does not match stored bytes")
	}
	document, err := store.LoadReportDocument(ctx, *finished)
	if err != nil {
		t.Fatal(err)
	}
	document.RedactMetadata(os.Getenv("AUDITOR_REPORT_REDACT_METADATA"))
	document.RedactSensitive(finished.Filters.Redaction)
	expected, err := report.RenderPDF(document)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, artifact.Content) {
		t.Fatalf("streamed artifact (%d bytes) differs from the in-memory render (%d bytes)", len(artifact.Content), len(expected))
	}

	// Memory budget: a job over budget fails with a readable reason and no artifact.
	budgetJob, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "technical", IdempotencyKey: "stream-budget"})
	if err != nil {
		t.Fatal(err)
	}
	err = (Worker{Store: store, MaxHeapBytes: 1}).ProcessOne(ctx)
	if err == nil || !strings.Contains(err.Error(), "orçamento de memória") {
		t.Fatalf("expected the memory budget reason, got %v", err)
	}
	failed, err := store.GetReportJob(ctx, env, budgetJob.ID)
	if err != nil || failed.Status != "failed" || failed.Error == nil || !strings.Contains(*failed.Error, "orçamento de memória") {
		t.Fatalf("budget job: %#v %v", failed, err)
	}
	if got, err := store.GetReportArtifact(ctx, env, budgetJob.ID); err != nil || got != nil {
		t.Fatalf("over-budget job stored an artifact: %v %v", got != nil, err)
	}

	// Store-level guarantees of the streaming write.
	direct, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "executive", IdempotencyKey: "stream-direct"})
	if err != nil {
		t.Fatal(err)
	}
	pdfWriter := func(out io.Writer) (repository.StreamedArtifact, error) {
		result, err := report.WritePDF(ctx, out, document)
		return repository.StreamedArtifact{Bytes: result.Bytes, SHA256: result.SHA256}, err
	}
	if err = store.FinishReportJobStream(ctx, direct.ID, "x.pdf", "application/pdf", pdfWriter); !errors.Is(err, repository.ErrReportConflict) {
		t.Fatalf("queued job accepted an artifact: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE report_job SET status='running' WHERE id=$1::uuid`, direct.ID); err != nil {
		t.Fatal(err)
	}
	limit := errors.New("relatório excede 200 páginas; reduza o escopo ou use filtros")
	err = store.FinishReportJobStream(ctx, direct.ID, "x.pdf", "application/pdf", func(out io.Writer) (repository.StreamedArtifact, error) {
		_, _ = out.Write([]byte("%PDF-1.4 partial"))
		return repository.StreamedArtifact{}, limit
	})
	var writeErr *repository.ArtifactWriteError
	if !errors.As(err, &writeErr) || !errors.Is(err, limit) {
		t.Fatalf("expected the writer error, got %v", err)
	}
	err = store.FinishReportJobStream(ctx, direct.ID, "x.pdf", "application/pdf", func(out io.Writer) (repository.StreamedArtifact, error) {
		result, err := pdfWriter(out)
		result.SHA256 = strings.Repeat("0", 64)
		return result, err
	})
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected a hash mismatch, got %v", err)
	}
	if got, err := store.GetReportJob(ctx, env, direct.ID); err != nil || got.Status != "running" || got.SHA256 != nil {
		t.Fatalf("failed streams changed the job: %#v %v", got, err)
	}
	if err = store.FinishReportJobStream(ctx, direct.ID, "x\tname.pdf", "application/pdf", pdfWriter); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetReportArtifact(ctx, env, direct.ID)
	if err != nil || stored == nil || !bytes.Equal(stored.Content, expected) || stored.Filename != "x\tname.pdf" {
		t.Fatalf("direct stream stored wrong artifact: %v", err)
	}
}
