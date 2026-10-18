package report

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func cloneDocument(d Document) Document {
	c := d
	c.Databases = append([]Database(nil), d.Databases...)
	c.Tables = append([]Table(nil), d.Tables...)
	c.Findings = append([]Finding(nil), d.Findings...)
	c.CoverageNotes = append([]string(nil), d.CoverageNotes...)
	c.Growth = append([]GrowthPoint(nil), d.Growth...)
	c.Regressions = append([]Regression(nil), d.Regressions...)
	c.ScoreCategories = append([]ScoreCategory(nil), d.ScoreCategories...)
	if d.Baseline != nil {
		baseline := *d.Baseline
		c.Baseline = &baseline
	}
	if len(d.Regressions) == 0 {
		c.Regressions = d.Regressions
	}
	return c
}

func sectionFixtures() map[string]Document {
	fixtures := streamFixtures()
	tied := fixtures["tecnico-completo"]
	tied = cloneDocument(tied)
	for i := 0; i < 30; i++ {
		// Same sort key, different ids: the unstable sort must still agree.
		tied.Findings = append(tied.Findings, Finding{ID: fmt.Sprintf("tie-%02d", i), Type: "storage.bloat", Severity: "high", Category: "storage", Database: "db", Schema: "public", Object: "igual", Title: "Mesmo título", Evidence: fmt.Sprintf("evidência %d", i), Confidence: 0.4, RuleVersion: "r1"})
	}
	tied.Regressions = []Regression{{FindingID: "tie-01", Severity: "high", Type: "storage.bloat", Object: "db.public.igual"}}
	score := 72
	tied.Score = &score
	tied.ScoreCategories = []ScoreCategory{{Category: "performance", Score: 70, Penalty: 30, Positive: 2, Findings: 9}}
	tied.TotalTables, tied.TotalDatabases, tied.TotalFindings = 0, 0, 0
	tied.Truncated = true
	fixtures["empates-e-baseline"] = tied
	table := cloneDocument(tied)
	table.Type = "table"
	table.TableFilter, table.SchemaFilter, table.DatabaseFilter, table.SeverityFilter = "tabela_001", "public", "db", "high"
	fixtures["tabela-filtrada"] = table
	return fixtures
}

func legacyRedactedPDF(d Document, metadata, level string) ([]byte, error) {
	d.RedactMetadata(metadata)
	legacyRedactSensitive(&d, level)
	return legacyRenderPDF(d)
}

// #65: section-by-section rendering, with redaction applied per section, is
// byte-identical to redacting the whole document and rendering it with the
// pre-#65 renderer.
func TestWritePDFSectionsMatchesLegacyWholeDocument(t *testing.T) {
	for name, document := range sectionFixtures() {
		for _, level := range []string{"", "none", "identifiers", "strict", "desconhecido"} {
			for _, metadata := range []string{"", "environment,run_id,requested_by,service_version,rule_version"} {
				t.Run(fmt.Sprintf("%s/%s/%t", name, level, metadata != ""), func(t *testing.T) {
					expected, expectedErr := legacyRedactedPDF(cloneDocument(document), metadata, level)
					var out bytes.Buffer
					result, err := WritePDFSections(context.Background(), &out, MemorySource(cloneDocument(document)), &Redaction{Metadata: metadata, Level: level})
					if (err == nil) != (expectedErr == nil) || err != nil && err.Error() != expectedErr.Error() {
						t.Fatalf("errors differ: %v vs legacy %v", err, expectedErr)
					}
					if err != nil {
						return
					}
					if !bytes.Equal(expected, out.Bytes()) {
						t.Fatalf("section render (%d bytes) differs from legacy (%d bytes)", out.Len(), len(expected))
					}
					if result.Bytes != int64(len(expected)) || result.Pages != PageCount(expected) {
						t.Fatalf("result %+v does not describe the PDF", result)
					}
				})
			}
		}
	}
}

func TestWritePDFSectionsKeepsLegacyErrorPrecedence(t *testing.T) {
	both := largeStreamDocument(20000)
	both.Environment = strings.Repeat("x", maxReportTextBytes)
	_, legacyErr := legacyRenderPDF(cloneDocument(both))
	_, err := WritePDF(context.Background(), io.Discard, cloneDocument(both))
	if err == nil || legacyErr == nil || err.Error() != legacyErr.Error() || !strings.Contains(err.Error(), "limite de texto") {
		t.Fatalf("text limit must win over the page limit: %v vs legacy %v", err, legacyErr)
	}
}

// recordingSource builds each section freshly on every call, like the store,
// and records the order in which sections are requested.
type recordingSource struct {
	document Document
	calls    []string
	drift    bool
	tables   int
}

func (r *recordingSource) Overview(ctx context.Context) (Document, error) {
	r.calls = append(r.calls, "overview")
	return MemorySource(cloneDocument(r.document)).Overview(ctx)
}
func (r *recordingSource) Databases(context.Context) ([]Database, error) {
	r.calls = append(r.calls, "databases")
	return cloneDocument(r.document).Databases, nil
}
func (r *recordingSource) Tables(context.Context) ([]Table, error) {
	r.calls = append(r.calls, "tables")
	tables := cloneDocument(r.document).Tables
	r.tables++
	// Drift on the write pass only, after the pages were counted.
	if r.drift && r.tables >= 3 {
		tables = append(tables, largeStreamDocument(2000).Tables...)
	}
	return tables, nil
}
func (r *recordingSource) Findings(context.Context) ([]Finding, error) {
	r.calls = append(r.calls, "findings")
	return cloneDocument(r.document).Findings, nil
}

func TestWritePDFSectionsLoadsOneSectionAtATime(t *testing.T) {
	document := sectionFixtures()["empates-e-baseline"]
	source := &recordingSource{document: document}
	var out bytes.Buffer
	if _, err := WritePDFSections(context.Background(), &out, source, nil); err != nil {
		t.Fatal(err)
	}
	want := "overview databases tables findings findings databases tables findings databases tables"
	if got := strings.Join(source.calls, " "); got != want {
		t.Fatalf("section calls:\n got %s\nwant %s", got, want)
	}
	expected, err := legacyRenderPDF(cloneDocument(document))
	if err != nil || !bytes.Equal(expected, out.Bytes()) {
		t.Fatalf("fresh-per-call source differs from legacy: %v", err)
	}
	executive := cloneDocument(document)
	executive.Type = "executive"
	source = &recordingSource{document: executive}
	if _, err = WritePDFSections(context.Background(), io.Discard, source, nil); err != nil {
		t.Fatal(err)
	}
	// The inventory appendix is not rendered for executive reports, so its
	// sections are only read once, for the summary counts.
	if got := strings.Join(source.calls, " "); got != "overview databases tables findings findings findings" {
		t.Fatalf("executive section calls: %s", got)
	}
}

func TestWritePDFSectionsRejectsASourceThatChanges(t *testing.T) {
	source := &recordingSource{document: sectionFixtures()["tecnico-completo"], drift: true}
	_, err := WritePDFSections(context.Background(), io.Discard, source, nil)
	if err == nil || !strings.Contains(err.Error(), "inconsistente") {
		t.Fatalf("a source that changes between passes must fail, got %v", err)
	}
}

// generatedSource builds each section on demand, as the store does, so the
// whole document never exists in memory.
type generatedSource struct{ tables int }

func (g generatedSource) Overview(context.Context) (Document, error) {
	return Document{Type: "technical", Environment: "large", RunID: "12345678-aaaa", RunStarted: time.Unix(0, 0), RunStatus: "success", Coverage: "complete"}, nil
}
func (g generatedSource) Databases(context.Context) ([]Database, error) { return nil, nil }
func (g generatedSource) Tables(context.Context) ([]Table, error) {
	return largeStreamDocument(g.tables).Tables, nil
}
func (g generatedSource) Findings(context.Context) ([]Finding, error) { return nil, nil }

// #65: loading the sections on demand keeps the peak heap well below loading
// the whole document and rendering it with the pre-#65 renderer.
func TestWritePDFSectionsPeakHeapBelowWholeDocument(t *testing.T) {
	const tables = 5600
	previous := debug.SetGCPercent(5)
	defer debug.SetGCPercent(previous)
	wholePeak, sectionPeak := uint64(0), uint64(0)
	for attempt := 0; attempt < 3; attempt++ {
		whole := measurePeak(t, func() error {
			out, err := legacyRenderPDF(largeStreamDocument(tables))
			runtime.KeepAlive(out)
			return err
		})
		sections := measurePeak(t, func() error {
			_, err := WritePDFSections(context.Background(), io.Discard, generatedSource{tables: tables}, nil)
			return err
		})
		wholePeak = max(wholePeak, whole)
		if attempt == 0 || sections < sectionPeak {
			sectionPeak = sections
		}
	}
	t.Logf("whole document + legacy render peak heap %d bytes, section-by-section peak heap %d bytes", wholePeak, sectionPeak)
	if sectionPeak*2 > wholePeak {
		t.Fatalf("section peak heap %d is not below half of the whole-document peak %d", sectionPeak, wholePeak)
	}
}

func TestWritePDFSectionsStopsOnSectionError(t *testing.T) {
	failing := errors.New("snapshot indisponível")
	_, err := WritePDFSections(context.Background(), io.Discard, failingSource{err: failing}, nil)
	if !errors.Is(err, failing) {
		t.Fatalf("expected the section error, got %v", err)
	}
}

type failingSource struct {
	generatedSource
	err error
}

func (f failingSource) Findings(context.Context) ([]Finding, error) { return nil, f.err }

// AN-12: the limit note of a section-by-section render equals the note of the
// whole redacted document.
func TestWritePDFSectionsLimitNoteMatchesWholeDocument(t *testing.T) {
	many := cloneDocument(sectionFixtures()["tecnico-completo"])
	for i := 0; i < MaxDetailedFindings+5; i++ {
		many.Findings = append(many.Findings, Finding{ID: fmt.Sprintf("extra-%03d", i), Type: "index.unused", Severity: "low", Database: "db", Schema: "public", Object: "o", Title: "t"})
	}
	many.Coverage = "partial"
	many.Truncated = true
	fixtures := sectionFixtures()
	fixtures["detalhe-limitado"] = many
	for name, document := range fixtures {
		for _, level := range []string{"", "strict"} {
			whole := cloneDocument(document)
			whole.RedactSensitive(level)
			result, err := WritePDFSections(context.Background(), io.Discard, MemorySource(cloneDocument(document)), &Redaction{Level: level})
			if err != nil {
				continue
			}
			if result.LimitNote != whole.LimitNote() {
				t.Fatalf("%s/%s: limit note %q, whole document %q", name, level, result.LimitNote, whole.LimitNote())
			}
		}
	}
	if result, err := WritePDF(context.Background(), io.Discard, many); err != nil || !strings.Contains(result.LimitNote, "detalhamento limitado") || !strings.Contains(result.LimitNote, "cobertura parcial") {
		t.Fatalf("limit note %q (%v)", result.LimitNote, err)
	}
}
