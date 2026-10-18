package reportworker

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/osmendes/db-auditor/internal/report"
	"github.com/osmendes/db-auditor/internal/repository"
)

// #65: the worker reads the report section by section from one snapshot. The
// stored PDF must equal the reference path (whole document loaded, redacted
// and rendered in memory) for every redaction level, with tied findings.
func TestReportSectionsMatchWholeDocumentIntegration(t *testing.T) {
	ctx, pool, store := openReportTestStore(t)
	env, run := seedReportRun(ctx, t, pool, 12, 400, 40)
	for _, level := range []string{"", "identifiers", "strict"} {
		for _, kind := range []string{"technical", "executive"} {
			t.Run(kind+"-"+level, func(t *testing.T) {
				job, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: kind, IdempotencyKey: fmt.Sprintf("sections-%s-%s", kind, level), Filters: repository.ReportFilters{Redaction: level}})
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
					t.Fatalf("artifact: %v", err)
				}
				document, err := store.LoadReportDocument(ctx, *finished)
				if err != nil {
					t.Fatal(err)
				}
				if len(document.Findings) != 40 || len(document.Tables) == 0 {
					t.Fatalf("fixture not loaded: %d findings, %d tables", len(document.Findings), len(document.Tables))
				}
				document.RedactMetadata(os.Getenv("AUDITOR_REPORT_REDACT_METADATA"))
				document.RedactSensitive(finished.Filters.Redaction)
				expected, err := report.RenderPDF(document)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(expected, artifact.Content) {
					t.Fatalf("section-by-section artifact (%d bytes) differs from the whole-document render (%d bytes)", len(artifact.Content), len(expected))
				}
			})
		}
	}
}
