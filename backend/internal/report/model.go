package report

import (
	"context"
	"strings"
	"time"
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

func effortLabel(category string) string {
	switch category {
	case "structure", "maintenance":
		return "alto (dependências e janela)"
	case "query_and_index", "security":
		return "médio (teste e retorno)"
	default:
		return "baixo para investigar; execução não estimada"
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
