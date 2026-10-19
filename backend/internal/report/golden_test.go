package report

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/impact"
)

var updateGolden = flag.Bool("update", false, "rewrite report golden files")

var goldenStart = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func goldenBase(kind, status, coverage string) Document {
	return Document{Type: kind, Environment: "staging-erp", RunID: "00000000-0000-0000-0000-000000000003", RunStarted: goldenStart, RunStatus: status,
		ServiceVersion: "1.4.0", RuleVersion: "rules-v3", RequestedBy: "operator", Coverage: coverage, RedactionLevel: "none"}
}

func intPtr(v int64) *int64 { return &v }

func goldenDocuments() map[string]Document {
	empty := goldenBase("executive", "success", "complete")

	partial := goldenBase("technical", "partial_success", "partial")
	partial.CoverageNotes = []string{"Execução parcial; ausência de objetos/achados não é prova de resolução."}
	partial.Growth = []GrowthPoint{
		{RunID: "00000000-0000-0000-0000-000000000001", At: goldenStart.Add(-48 * time.Hour), Status: "success", SizeBytes: 1000, Tables: 3},
		{RunID: "00000000-0000-0000-0000-000000000002", At: goldenStart.Add(-24 * time.Hour), Status: "partial_success", SizeBytes: 10, Tables: 1},
		{RunID: "00000000-0000-0000-0000-000000000003", At: goldenStart, Status: "success", SizeBytes: 1500, Tables: 3},
	}
	unusedEvidence := map[string]any{impact.BasisKey: map[string]any{"size_bytes": 40 << 20, "table_index_bytes": 100 << 20,
		"window_start": goldenStart.Add(-7 * 24 * time.Hour).Format(time.RFC3339), "window_end": goldenStart.Format(time.RFC3339)}}
	unusedImpact := impact.For("index.unused", unusedEvidence, time.Time{}, goldenStart)
	partial.Findings = []Finding{
		{ID: "f-unused", Type: "index.unused", Severity: "medium", Category: "query_and_index", Database: "erp", Schema: "public", Object: "orders_old_idx",
			Confidence: 0.7, Evidence: `{"idx_scan":0}`, RuleVersion: "rules-v3", Impact: unusedImpact,
			Gain: impact.BuildGain("Índice sem uso.", unusedEvidence, unusedImpact, &impact.Measurement{Metric: "table_size_bytes", Before: intPtr(1000), After: intPtr(600), Comparable: true, AfterRunID: "run-after"})},
		{ID: "f-growth", Type: "maintenance.growth_trend", Severity: "medium", Category: "maintenance", Database: "erp", Schema: "public", Object: "events",
			Confidence: 0.55, RuleVersion: "rules-v3", Impact: impact.For("maintenance.growth_trend", nil, time.Time{}, goldenStart)},
		{ID: "f-pk", Type: "integrity.missing_primary_key", Severity: "high", Category: "structure", Database: "erp", Schema: "public", Object: "audit_log",
			Confidence: 0.9, RuleVersion: "rules-v3"},
	}

	long := goldenBase("technical", "success", "complete")
	types := []string{"security.excessive_privilege", "index.unused", "vacuum.high_dead_tuples", "storage.large_table", "integrity.missing_primary_key", "performance.workload_scan"}
	severities := []string{"critical", "high", "medium", "low", "info"}
	for i := 0; i < 40; i++ {
		long.Findings = append(long.Findings, Finding{ID: fmt.Sprintf("f-%03d", i), Type: types[i%len(types)], Severity: severities[i%len(severities)], Category: "structure",
			Database: "erp", Schema: "public", Object: fmt.Sprintf("table_%03d", i), Confidence: float64(i%10) / 10, Evidence: strings.Repeat("e", 600), RuleVersion: "rules-v3"})
	}
	for i := 0; i < 30; i++ {
		long.Tables = append(long.Tables, Table{Database: "erp", Schema: "public", Name: fmt.Sprintf("table_%03d", i), SizeBytes: int64(i) * 8192, Rows: int64(i) * 10, HasPrimaryKey: i%4 != 0})
	}
	long.Databases = []Database{{Name: "erp", Schemas: 1, Tables: 30, SizeBytes: 1 << 30}}
	table := goldenBase("table", "success", "complete")
	table.DatabaseFilter, table.SchemaFilter, table.TableFilter = "erp", "public", "orders"
	table.Tables = []Table{{Database: "erp", Schema: "public", Name: "orders", SizeBytes: 1 << 20, Rows: 1000, HasPrimaryKey: true}}
	table.Findings = []Finding{partial.Findings[0]}
	table.Baseline = &Baseline{RunID: "00000000-0000-0000-0000-000000000001", Status: "complete", ChangedTables: 1}
	table.Regressions = []Regression{{FindingID: "f-unused", Severity: "high", Type: "index.unused", Object: "erp.public.orders_old_idx"}}
	// #135: the table report plots that one table, not its database.
	table.Growth = []GrowthPoint{
		{RunID: "00000000-0000-0000-0000-000000000001", At: goldenStart.Add(-7 * 24 * time.Hour), Status: "success", SizeBytes: 900 << 10, Tables: 1},
		{RunID: "00000000-0000-0000-0000-000000000003", At: goldenStart, Status: "success", SizeBytes: 1 << 20, Tables: 1},
	}
	return map[string]Document{"empty": empty, "partial": partial, "long": long, "table": table}
}

func volumousDocument() Document {
	d := goldenBase("technical", "success", "complete")
	d.Truncated = true
	d.TotalTables, d.TotalFindings, d.TotalDatabases = 12000, 9000, 600
	d.CoverageNotes = []string{"Listagens limitadas a 500 bancos, 5000 tabelas e 2000 achados."}
	for i := 0; i < 5000; i++ {
		d.Tables = append(d.Tables, Table{Database: "db", Schema: "public", Name: fmt.Sprintf("table_%05d", i), SizeBytes: int64(i)})
	}
	for i := 0; i < 2000; i++ {
		d.Findings = append(d.Findings, Finding{ID: fmt.Sprintf("f-%05d", i), Type: "index.unused", Severity: "low", Database: "db", Schema: "public", Object: fmt.Sprintf("idx_%05d", i), Confidence: 0.5})
	}
	return d
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run go test ./internal/report -run Golden -update): %v", path, err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from golden; review the change and rerun with -update", path)
	}
}

func writeVariant(t *testing.T, name string, pdf []byte) {
	if dir := os.Getenv("AUDITOR_REPORT_VARIANTS_DIR"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "golden-"+name+".pdf"), pdf, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// AN-12 / BA-21: the text of empty, partial and long reports is pinned in
// golden files; the volumous report is pinned by its deterministic summary.
func TestReportGoldenFiles(t *testing.T) {
	for name, d := range goldenDocuments() {
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			for _, line := range BuildLines(d) {
				b.WriteString(line.Style + "\t" + line.Text + "\n")
			}
			compareGolden(t, name+".golden.txt", b.String())
			pdf := mustRenderPDF(t, d)
			if again := mustRenderPDF(t, d); string(again) != string(pdf) {
				t.Fatal("PDF must be deterministic")
			}
			writeVariant(t, name, pdf)
		})
	}
	t.Run("volumous", func(t *testing.T) {
		d := volumousDocument()
		started := time.Now()
		pdf := mustRenderPDF(t, d)
		if elapsed := time.Since(started); elapsed > 20*time.Second {
			t.Fatalf("volumous report took %s", elapsed)
		}
		lines := BuildLines(d)
		headings := []string{}
		for _, line := range lines {
			if line.Style == "heading" {
				headings = append(headings, line.Text)
			}
		}
		sum := sha256.Sum256(pdf)
		summary := fmt.Sprintf("lines=%d\npages=%d\nbytes=%d\nsha256=%s\nheadings=%s\n", len(lines), PageCount(pdf), len(pdf), hex.EncodeToString(sum[:]), strings.Join(headings, " | "))
		compareGolden(t, "volumous.golden.txt", summary)
		writeVariant(t, "volumous", pdf)
	})
}

func TestReportPartialGrowthIsGapNotZero(t *testing.T) {
	d := goldenDocuments()["partial"]
	joined := ""
	for _, line := range BuildLines(d) {
		if line.Style == "bar_bytes" && strings.Contains(line.Text, "00000000") && strings.HasPrefix(strings.Split(line.Text, "|")[0], "01/10/2026") {
			t.Fatal("partial point must not be plotted")
		}
		joined += line.Text + "\n"
	}
	for _, want := range []string{"lacuna (coleta parcial)", "Variação entre coletas completas: +500 bytes (+50.0%)", "Unidade: bytes", "Ganho medido: ", "Ganho previsto: ", "Hipótese: ", "Custos de ", "Matriz impacto × esforço × confiança × risco", "achado f-pk"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q", want)
		}
	}
}
