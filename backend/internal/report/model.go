package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/impact"
	"github.com/osmendes/db-auditor/internal/priority"
)

// Document is the renderer-independent, run-scoped report representation.
// Every collection is bounded at the database layer and sorted here so the
// same run, version and filters always produce the same PDF bytes.
type Document struct {
	Type            string
	Environment     string
	RunID           string
	RunStarted      time.Time
	RunStatus       string
	ServiceVersion  string
	RuleVersion     string
	RequestedBy     string
	DatabaseFilter  string
	SchemaFilter    string
	TableFilter     string
	SeverityFilter  string
	Coverage        string
	CoverageNotes   []string
	Databases       []Database
	Tables          []Table
	Findings        []Finding
	Growth          []GrowthPoint
	Score           *int
	ScoreConfidence float64
	ScoreCategories []ScoreCategory
	TotalTables     int
	TotalDatabases  int
	TotalFindings   int
	Baseline        *Baseline
	Regressions     []Regression
	Truncated       bool
	RedactionLevel  string
}

type Database struct {
	Name      string
	Schemas   int
	Tables    int
	SizeBytes int64
}
type Table struct {
	Database      string
	Schema        string
	Name          string
	SizeBytes     int64
	Rows          int64
	HasPrimaryKey bool
}
type Finding struct {
	ID             string
	Type           string
	Severity       string
	Category       string
	Database       string
	Schema         string
	Object         string
	Title          string
	Summary        string
	Recommendation string
	Confidence     float64
	Evidence       string
	RuleVersion    string
	// Impact is the uniform estimate (value, unit, denominator, period).
	// A zero value renders as "impacto não estimado".
	Impact impact.Estimate
	// Gain keeps hypothesis, predicted and measured gain apart (BA-21).
	Gain impact.Gain
}
type Baseline struct {
	RunID         string
	Status        string
	AddedTables   int
	RemovedTables int
	ChangedTables int
}
type Regression struct {
	FindingID string
	Severity  string
	Type      string
	Object    string
}

type GrowthPoint struct {
	RunID     string
	At        time.Time
	Status    string
	SizeBytes int64
	Tables    int
}
type ScoreCategory struct {
	Category string
	Score    int
	Penalty  int
	Positive int
	Findings int
}

type Line struct {
	Text  string
	Style string
}

// BuildLines returns every report line of an in-memory document. The worker
// uses the section-by-section path (WritePDFSections) instead; both share
// emitLines, so they produce the same lines.
func BuildLines(d Document) []Line {
	source := MemorySource(d)
	plan, err := planSections(context.Background(), source, nil)
	if err != nil {
		return nil
	}
	lines := []Line{}
	_ = plan.emitLines(context.Background(), func(line Line) error {
		lines = append(lines, line)
		return nil
	})
	return lines
}

// MaxDetailedFindings bounds the per-finding detail; the rest are listed in one
// line each. DetailLimited reports when that happened so the job records it.
const MaxDetailedFindings = 100

// LimitNote summarizes every limit applied to the document; empty when the
// report is complete for its scope. The worker stores it on the job.
func (d Document) LimitNote() string { return limitNote(d, len(d.Findings)) }

// limitNote builds the note from the overview and the number of findings in
// the report, so the section-by-section renderer can produce it without the
// whole document.
func limitNote(d Document, findings int) string {
	notes := []string{}
	if d.Truncated {
		notes = append(notes, "listagens limitadas (500 bancos, 5000 tabelas, 2000 achados)")
	}
	if DetailLimitedCount(findings) {
		notes = append(notes, fmt.Sprintf("detalhamento limitado aos %d achados de maior prioridade", MaxDetailedFindings))
	}
	if d.Coverage != "" && d.Coverage != "complete" {
		notes = append(notes, "cobertura "+coverageLabel(d.Coverage))
	}
	return strings.Join(notes, "; ")
}

// DetailLimited reports whether BuildLines will summarize findings beyond
// MaxDetailedFindings.
func DetailLimited(d Document) bool { return DetailLimitedCount(len(d.Findings)) }

// DetailLimitedCount is DetailLimited for a report with that many findings.
func DetailLimitedCount(findings int) bool { return findings > MaxDetailedFindings }

const (
	matrixRows       = 20
	maxEvidenceChars = 500
)

func effortLabel(plan actions.Plan) string {
	switch plan.Effort {
	case "alto":
		return "alto (dependências e janela)"
	case "médio":
		return "médio (teste e retorno)"
	case "baixo":
		return "baixo para investigar; execução não estimada"
	}
	switch plan.Category {
	case "structure", "maintenance":
		return "alto (dependências e janela)"
	case "query_and_index", "security":
		return "médio (teste e retorno)"
	default:
		return "baixo para investigar; execução não estimada"
	}
}

func rankFindings(findings []Finding) []priority.Ranked {
	inputs := make([]priority.Input, 0, len(findings))
	for _, f := range findings {
		inputs = append(inputs, priority.Input{FindingID: f.ID, RuleID: f.Type, Severity: f.Severity, Confidence: f.Confidence, Impact: f.Impact})
	}
	return priority.Top(inputs, 0)
}

func unitLabel(e impact.Estimate) string {
	if !e.Estimated() || e.Unit == "" {
		return "não informada"
	}
	if e.DenominatorUnit != "" && e.DenominatorUnit != e.Unit {
		return e.Unit + " / " + e.DenominatorUnit
	}
	return e.Unit
}

func maintenanceKindLabel(kind string) string {
	switch kind {
	case "partitioning":
		return "particionamento"
	case "compression":
		return "compressão"
	case "retention":
		return "retenção"
	case "autovacuum":
		return "autovacuum"
	case "analyze":
		return "ANALYZE/estatísticas"
	case "reorder":
		return "reordenação"
	default:
		return kind
	}
}

func truncateRunes(value string, limit int) string {
	r := []rune(value)
	if len(r) <= limit {
		return value
	}
	return string(r[:limit]) + "…"
}

// growthLines renders the storage chart with its text alternative. A partial
// collection is a gap: it is never plotted (so never reads as zero) and never
// enters the variation, which is computed only between complete points.
//
// subject names what each bar sums (growthSubject): the series follows the
// report scope, so a table report plots that one table (#135).
func growthLines(points []GrowthPoint, subject string) []Line {
	lines := []Line{{"Evolução de armazenamento", "heading"}, {"Barras: " + subject + " (unidade: bytes) por coleta completa. Coletas parciais aparecem como lacuna e não entram na variação.", "body"}}
	var maximum int64
	complete := []GrowthPoint{}
	for _, point := range points {
		if point.Status == "success" {
			complete = append(complete, point)
			if point.SizeBytes > maximum {
				maximum = point.SizeBytes
			}
		}
	}
	for _, point := range points {
		at := point.At.UTC().Format("02/01/2006")
		if point.At.IsZero() {
			at = "data indisponível"
		}
		shortID := point.RunID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		if point.Status != "success" {
			lines = append(lines, Line{fmt.Sprintf("%s %s: lacuna (coleta parcial) — valor não plotado e fora da comparação.", at, shortID), "body"})
			continue
		}
		lines = append(lines, Line{fmt.Sprintf("%s %s|%d|%d", at, shortID, point.SizeBytes, maximum), "bar_bytes"},
			Line{fmt.Sprintf("Alternativa textual: execução %s em %s = %d bytes em %d %s (coleta completa).", point.RunID, at, point.SizeBytes, point.Tables, plural(point.Tables, "tabela", "tabelas")), "body"})
	}
	if len(complete) > 1 {
		first, last := complete[0], complete[len(complete)-1]
		period := impact.PeriodLabel(first.At, last.At)
		change := last.SizeBytes - first.SizeBytes
		percent := "sem base para percentual"
		if first.SizeBytes > 0 {
			percent = fmt.Sprintf("%+.1f%%", float64(change)*100/float64(first.SizeBytes))
		}
		lines = append(lines, Line{fmt.Sprintf("Variação entre coletas completas: %+d bytes (%s) no período %s. Confirme que perfil e cobertura são comparáveis antes de atribuir a mudança a uma ação.", change, percent, period), "body"})
	} else {
		lines = append(lines, Line{"Variação não calculada: são necessárias ao menos duas coletas completas.", "body"})
	}
	return lines
}

// growthSubject describes what the storage series sums, following the same
// database/schema/table filters as the report. It names no object, so it is
// safe at every redaction level.
func growthSubject(d Document) string {
	switch {
	case d.TableFilter != "":
		return "tamanho total da tabela do escopo"
	case d.SchemaFilter != "":
		return "tamanho total das tabelas do schema do escopo"
	case d.DatabaseFilter != "":
		return "tamanho total das tabelas do banco do escopo"
	default:
		return "tamanho total das tabelas"
	}
}

func reportTypeLabel(kind string) string {
	switch kind {
	case "executive":
		return "executivo"
	case "technical":
		return "técnico"
	case "table":
		return "de tabela"
	default:
		return kind
	}
}

func redactionLabel(value string) string {
	switch value {
	case "", "none":
		return "sem redação"
	case "identifiers":
		return "identificadores ocultos"
	case "strict":
		return "estrita"
	default:
		return "configuração desconhecida"
	}
}

func runStatusLabel(value string) string {
	switch value {
	case "success":
		return "concluída"
	case "partial_success":
		return "concluída parcialmente"
	case "failed":
		return "falhou"
	default:
		return value
	}
}

func plural(count int, singular, many string) string {
	if count == 1 {
		return singular
	}
	return many
}

func coverageLabel(value string) string {
	switch value {
	case "complete":
		return "completa"
	case "partial":
		return "parcial"
	default:
		return value
	}
}

func severityRank(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

func severityLabel(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "crítica"
	case "high":
		return "alta"
	case "medium":
		return "média"
	case "low":
		return "baixa"
	case "info":
		return "informativa"
	default:
		return severity
	}
}

func scope(d Document) string {
	parts := []string{}
	for _, p := range []string{d.DatabaseFilter, d.SchemaFilter, d.TableFilter} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "ambiente inteiro")
	}
	if d.SeverityFilter != "" {
		parts = append(parts, "severidade: "+d.SeverityFilter)
	}
	return strings.Join(parts, " / ")
}
