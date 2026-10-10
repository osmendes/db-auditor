package report

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRenderPDFDeterministicAndPaginated(t *testing.T) {
	d := Document{Type: "technical", Environment: "Produção", RunID: "12345678-1234-4123-8123-123456789012", RunStarted: time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC), RunStatus: "success", ServiceVersion: "1.0", RuleVersion: "1.0", RequestedBy: "report-token", Coverage: "complete", Databases: []Database{{Name: "db", Schemas: 1, Tables: 200, SizeBytes: 1000}}, Baseline: &Baseline{RunID: "baseline", Status: "complete", AddedTables: 1, RemovedTables: 0, ChangedTables: 2}}
	for i := 0; i < 200; i++ {
		d.Tables = append(d.Tables, Table{Database: "db", Schema: "public", Name: strings.Repeat("orders", 5), SizeBytes: int64(i), Rows: 100, HasPrimaryKey: true})
	}
	d.Findings = []Finding{{Severity: "high", Category: "security", Database: "db", Schema: "public", Object: "orders", Title: "Permissão excessiva", Summary: "Revise privilégios", Recommendation: "Validar em ambiente controlado", Confidence: 0.9, Evidence: `{"role":"public"}`, RuleVersion: "1.0"}}
	first, err := RenderPDF(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidatePDF(first); err != nil {
		t.Fatal(err)
	}
	second, err := RenderPDF(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("same document did not produce identical PDF bytes")
	}
	if PageCount(first) < 2 {
		t.Fatalf("expected multiple pages; got %d", PageCount(first))
	}
	if path := os.Getenv("AUDITOR_REPORT_SAMPLE_PDF"); path != "" {
		if err = os.WriteFile(path, first, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPDFVariantsAndBoundaryContent(t *testing.T) {
	base := Document{Environment: "Ambiente de teste", RunID: "00000000-0000-0000-0000-000000000001", RunStarted: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), ServiceVersion: "teste", RuleVersion: "regra-teste", RequestedBy: "operador", Coverage: "complete", RunStatus: "success"}
	variants := []struct {
		name string
		doc  Document
	}{
		{"executivo-vazio", base},
		{"tecnico-longo", base},
		{"tabela-parcial", base},
	}
	variants[0].doc.Type = "executive"
	variants[1].doc.Type = "technical"
	for i := 0; i < 180; i++ {
		variants[1].doc.Tables = append(variants[1].doc.Tables, Table{Database: "db", Schema: "public", Name: fmt.Sprintf("tabela_de_vendas_%03d", i), SizeBytes: int64(i + 100)})
	}
	variants[1].doc.Findings = append(variants[1].doc.Findings, Finding{ID: "id-1", Type: "index.unused", Severity: "medium", Database: "db", Schema: "public", Object: "indice_vendas", Evidence: strings.Repeat("evidência revisável ", 45)})
	variants[2].doc.Type = "table"
	variants[2].doc.Coverage = "partial"
	variants[2].doc.RunStatus = "partial_success"
	variants[2].doc.CoverageNotes = []string{"A coleta não pôde ler índices nesta execução."}
	variants[2].doc.Findings = []Finding{{ID: "id-2", Type: "integrity.missing_primary_key", Severity: "high", Database: "db", Schema: "public", Object: strings.Repeat("objeto_longo_", 15), Summary: strings.Repeat("Explicação técnica extensa. ", 35)}}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			pdf, err := RenderPDF(variant.doc)
			if err != nil || ValidatePDF(pdf) != nil || PageCount(pdf) == 0 {
				t.Fatalf("PDF inválido: %v", err)
			}
			joined := ""
			for _, line := range BuildLines(variant.doc) {
				joined += line.Text + "\n"
			}
			switch variant.name {
			case "executivo-vazio":
				if !strings.Contains(joined, "Nenhum achado observado neste escopo da execução.") {
					t.Fatal("relatório vazio sem alternativa textual")
				}
			case "tecnico-longo":
				if !strings.Contains(joined, "ID do achado: id-1") {
					t.Fatal("relatório longo sem ID de evidência")
				}
			case "tabela-parcial":
				if !strings.Contains(joined, "ID do achado: id-2") || !strings.Contains(joined, "A cobertura é parcial") {
					t.Fatal("relatório parcial sem ID ou aviso de cobertura")
				}
			}
			if dir := os.Getenv("AUDITOR_REPORT_VARIANTS_DIR"); dir != "" {
				if err := os.WriteFile(dir+"/"+variant.name+".pdf", pdf, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestWrapTextBounded(t *testing.T) {
	for _, part := range wrapText(strings.Repeat("x", 220), 90) {
		if len(part) > 90 {
			t.Fatalf("unbounded part: %d", len(part))
		}
	}
}

func TestPDFEscapesUntrustedMetadata(t *testing.T) {
	name := `) Tj 0 0 m (inject`
	pdf, err := RenderPDF(Document{Type: "executive", Environment: name, RunID: "safe", RunStarted: time.Unix(0, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdf, []byte(name)) || !bytes.Contains(pdf, []byte(`\) Tj 0 0 m \(inject`)) {
		t.Fatal("untrusted metadata was not escaped as PDF text")
	}
}

func TestLargeInventoryPDFIsBounded(t *testing.T) {
	d := Document{Type: "technical", Environment: "large", RunID: "fixed", RunStarted: time.Unix(0, 0), RunStatus: "success", Coverage: "complete"}
	for i := 0; i < 5000; i++ {
		d.Tables = append(d.Tables, Table{Database: "db", Schema: "public", Name: strings.Repeat("large_table_", 3) + string(rune('a'+i%26)), SizeBytes: int64(i)})
	}
	pdf, err := RenderPDF(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) > maxPDFBytes || PageCount(pdf) > 200 {
		t.Fatalf("large report exceeded bounds: %d bytes, %d pages", len(pdf), PageCount(pdf))
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	rendered, err := RenderPDF(d)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) > maxPDFBytes {
		t.Fatalf("streamed report exceeded %d bytes", maxPDFBytes)
	}
	if growth := after.TotalAlloc - before.TotalAlloc; growth > 96<<20 {
		t.Fatalf("relatório volumoso alocou %d bytes; o worker deve falhar antes de estourar a memória", growth)
	}
}

func TestPDFRejectsOversizedTextBeforeLayout(t *testing.T) {
	d := Document{Type: "executive", Environment: strings.Repeat("x", maxReportTextBytes+1)}
	if _, err := RenderPDF(d); err == nil || !strings.Contains(err.Error(), "limite de texto") {
		t.Fatalf("expected a bounded report error, got %v", err)
	}
}

func BenchmarkLargeInventoryPDF(b *testing.B) {
	d := Document{Type: "technical", Environment: "large", RunID: "fixed", RunStarted: time.Unix(0, 0), RunStatus: "success", Coverage: "complete"}
	for i := 0; i < 5000; i++ {
		d.Tables = append(d.Tables, Table{Database: "db", Schema: "public", Name: strings.Repeat("large_table_", 3) + string(rune('a'+i%26)), SizeBytes: int64(i)})
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := RenderPDF(d); err != nil {
			b.Fatal(err)
		}
	}
}

func TestReportContentGolden(t *testing.T) {
	d := Document{Type: "executive", Environment: "env", RunID: "run", RunStarted: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), RunStatus: "partial_success", ServiceVersion: "1.0", RuleVersion: "rule-v2", RequestedBy: "report-token", Coverage: "partial", CoverageNotes: []string{"coverage incomplete"}, TotalTables: 4, TotalFindings: 1, Findings: []Finding{{Type: "security.excessive_privilege", Severity: "critical", Title: "risk", Recommendation: "validate", Evidence: "proof", Confidence: 0.8}}, Baseline: &Baseline{RunID: "base", Status: "partial"}, Growth: []GrowthPoint{{RunID: "run", SizeBytes: 100, Tables: 4}}}
	lines := BuildLines(d)
	joined := ""
	for _, line := range lines {
		joined += line.Text + "\n"
	}
	for _, want := range []string{"DB Auditor - Relatório executivo", "4 tabelas | 1 achado", "Cobertura: parcial", "Índice indisponível", "Referência base", "Evolução de armazenamento", "Evidência técnica: proof", "Confirme as funções da conta", "Metodologia e glossário", "Alternativa textual: Críticos = 1 de 1 itens."} {
		if !strings.Contains(joined, want) {
			t.Fatalf("report missing %q", want)
		}
	}
	if !strings.Contains(joined, "Análise final e próximos passos") || !strings.Contains(joined, "Prioridade 1 (crítica): Uma conta pode ter mais permissões") {
		t.Fatal("report is missing its final analysis")
	}
	if !strings.Contains(string(mustRenderPDF(t, d)), " re f ") {
		t.Fatal("report is missing vector chart bars")
	}
}

func mustRenderPDF(t *testing.T, d Document) []byte {
	t.Helper()
	pdf, err := RenderPDF(d)
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}
