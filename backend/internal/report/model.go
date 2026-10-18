package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/guidance"
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

func BuildLines(d Document) []Line {
	if d.TotalDatabases == 0 {
		d.TotalDatabases = len(d.Databases)
	}
	if d.TotalTables == 0 {
		d.TotalTables = len(d.Tables)
	}
	if d.TotalFindings == 0 {
		d.TotalFindings = len(d.Findings)
	}
	sort.Slice(d.Databases, func(i, j int) bool { return d.Databases[i].Name < d.Databases[j].Name })
	sort.Slice(d.Tables, func(i, j int) bool {
		a, b := d.Tables[i], d.Tables[j]
		return a.Database+"/"+a.Schema+"/"+a.Name < b.Database+"/"+b.Schema+"/"+b.Name
	})
	sort.Slice(d.Findings, func(i, j int) bool {
		a, b := d.Findings[i], d.Findings[j]
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) < severityRank(b.Severity)
		}
		return a.Severity+a.Category+a.Database+a.Schema+a.Object+a.Title < b.Severity+b.Category+b.Database+b.Schema+b.Object+b.Title
	})
	lines := []Line{{"DB Auditor - Relatório " + reportTypeLabel(d.Type), "title"}, {"Nível de redação: " + redactionLabel(d.RedactionLevel), "body"}, {"Ambiente: " + d.Environment, "body"}, {"Execução: " + d.RunID, "body"}, {"Início: " + d.RunStarted.UTC().Format("02/01/2006 15:04 UTC") + " | Estado: " + runStatusLabel(d.RunStatus), "body"}, {"Versão do serviço: " + d.ServiceVersion + " | Regras: " + d.RuleVersion, "body"}, {"Solicitante: " + d.RequestedBy, "body"}, {"Escopo: " + scope(d), "body"}, {"", "body"}, {"Resumo executivo", "heading"}}
	critical, high := 0, 0
	severityCounts := map[string]int{}
	for _, f := range d.Findings {
		severityCounts[strings.ToLower(f.Severity)]++
		if f.Severity == "critical" {
			critical++
		}
		if f.Severity == "high" {
			high++
		}
	}
	lines = append(lines, Line{fmt.Sprintf("%d bancos | %d tabelas | %d %s (%d %s, %d %s)", d.TotalDatabases, d.TotalTables, d.TotalFindings, plural(d.TotalFindings, "achado", "achados"), critical, plural(critical, "crítico", "críticos"), high, plural(high, "alto", "altos")), "body"})
	lines = append(lines, Line{"Como ler: confirme primeiro a cobertura e os itens críticos; a nota resume sinais observados, não substitui a investigação.", "body"})
	lines = append(lines, Line{Text: "Achados por severidade (itens incluídos no PDF)", Style: "subheading"})
	for _, severity := range []struct{ key, label string }{{"critical", "Críticos"}, {"high", "Altos"}, {"medium", "Médios"}, {"low", "Baixos"}, {"info", "Informativos"}} {
		lines = append(lines, Line{Text: fmt.Sprintf("%s|%d|%d", severity.label, severityCounts[severity.key], len(d.Findings)), Style: "bar"})
		lines = append(lines, Line{Text: fmt.Sprintf("Alternativa textual: %s = %d de %d itens.", severity.label, severityCounts[severity.key], len(d.Findings)), Style: "body"})
	}
	lines = append(lines, Line{"Cobertura: " + coverageLabel(d.Coverage), "body"})
	if d.Score != nil {
		lines = append(lines, Line{fmt.Sprintf("Índice: %d/100 | confiança %.0f%%", *d.Score, d.ScoreConfidence*100), "body"})
	} else {
		lines = append(lines, Line{fmt.Sprintf("Índice indisponível | confiança %.0f%%", d.ScoreConfidence*100), "body"})
	}
	for _, c := range d.ScoreCategories {
		lines = append(lines, Line{fmt.Sprintf("%s: %d/100 (%d achados, penalidade %d, evidência positiva +%d)", c.Category, c.Score, c.Findings, c.Penalty, c.Positive), "body"})
	}
	for _, note := range d.CoverageNotes {
		lines = append(lines, Line{"Limite: " + note, "body"})
	}
	if d.Truncated {
		lines = append(lines, Line{"Limite: inventário resumido para manter o consumo de memória previsível.", "body"})
	}
	if d.Baseline != nil {
		lines = append(lines, Line{"Histórico e referência de comparação", "heading"}, Line{fmt.Sprintf("Referência %s | %s | +%d / -%d tabelas, %d alteradas", d.Baseline.RunID, coverageLabel(d.Baseline.Status), d.Baseline.AddedTables, d.Baseline.RemovedTables, d.Baseline.ChangedTables), "body"})
		if len(d.Regressions) > 0 {
			lines = append(lines, Line{"Novos achados de maior prioridade desde a referência", "subheading"})
			for _, regression := range d.Regressions {
				lines = append(lines, Line{fmt.Sprintf("%s · %s · objeto %s · achado %s", severityLabel(regression.Severity), guidance.For(regression.Type).Meaning, regression.Object, regression.FindingID), "body"})
			}
		} else {
			lines = append(lines, Line{"Nenhum novo achado alto ou crítico identificado neste recorte; confirme a cobertura antes de concluir melhora.", "body"})
		}
	}
	if len(d.Growth) > 0 {
		lines = append(lines, growthLines(d.Growth)...)
	}
	ranked := rankFindings(d.Findings)
	lines = append(lines, Line{"Riscos e prioridades", "heading"})
	lines = append(lines, Line{"Matriz de decisão (" + priority.FormulaVersion + "): pontuação = peso da severidade × fator de impacto medido × confiança ÷ esforço. Impacto sem métrica não eleva a prioridade. Confirme dependências, equipe e janela antes de executar uma mudança.", "body"})
	if len(d.Findings) == 0 {
		lines = append(lines, Line{"Nenhum achado observado neste escopo da execução.", "body"})
	} else {
		lines = append(lines, Line{Text: "Matriz impacto × esforço × confiança × risco", Style: "subheading"})
		for i, r := range ranked {
			if i >= matrixRows {
				lines = append(lines, Line{fmt.Sprintf("Matriz limitada aos %d itens de maior pontuação; os demais aparecem no detalhamento abaixo.", matrixRows), "body"})
				break
			}
			lines = append(lines, Line{fmt.Sprintf("#%d | achado %s | pontuação %.2f/%.1f | severidade %s | impacto %s | esforço %s | confiança %s (%.0f%%) | risco: %s", r.Rank, r.FindingID, r.Score, r.MaxScore, severityLabel(r.Severity), r.ImpactLevel, r.Effort, r.ConfidenceLabel, r.Confidence*100, r.Risk), "body"})
		}
	}
	byID := make(map[string]Finding, len(d.Findings))
	for _, f := range d.Findings {
		byID[f.ID] = f
	}
	for index, r := range ranked {
		f := byID[r.FindingID]
		if index >= MaxDetailedFindings {
			if index == MaxDetailedFindings {
				lines = append(lines, Line{"Demais achados (resumo de uma linha)", "subheading"}, Line{fmt.Sprintf("Detalhamento limitado aos %d achados de maior pontuação para manter o PDF dentro de %d páginas; consulte os demais na interface.", MaxDetailedFindings, maxPDFPages), "body"})
			}
			lines = append(lines, Line{fmt.Sprintf("#%d %s | %s | %s.%s.%s | pontuação %.2f", r.Rank, f.ID, severityLabel(f.Severity), f.Database, f.Schema, f.Object, r.Score), "body"})
			continue
		}
		friendly := guidance.For(f.Type)
		plan := actions.For(f.Type)
		estimate := f.Impact
		if estimate.Status == "" {
			estimate = impact.For(f.Type, nil, time.Time{}, time.Time{})
		}
		lines = append(lines, Line{fmt.Sprintf("[%s] %s - %s.%s.%s", strings.ToUpper(severityLabel(f.Severity)), friendly.Meaning, f.Database, f.Schema, f.Object), "subheading"})
		lines = append(lines, Line{fmt.Sprintf("ID do achado: %s | severidade: %s | esforço preliminar: %s", f.ID, severityLabel(f.Severity), effortLabel(plan)), "body"})
		lines = append(lines, Line{"Impacto: " + estimate.Label, "body"}, Line{"Unidade: " + unitLabel(estimate) + " | Período de referência: " + estimate.Period, "body"})
		lines = append(lines, Line{"Risco da mudança: " + plan.Risk, "body"})
		gain := f.Gain
		if gain.Hypothesis == "" {
			gain = impact.BuildGain(friendly.Meaning, nil, estimate, nil)
		}
		lines = append(lines, Line{"Hipótese: " + gain.Hypothesis, "body"}, Line{"Ganho previsto: " + gain.Predicted, "body"}, Line{"Ganho medido: " + gain.Measured, "body"})
		if cost := plan.Maintenance; cost != nil {
			lines = append(lines, Line{"Custos de " + maintenanceKindLabel(cost.Kind) + " (" + cost.Capability + ")", "subheading"},
				Line{"Pré-condições: " + cost.Preconditions, "body"}, Line{"Custo de leitura: " + cost.ReadCost, "body"}, Line{"Custo de escrita: " + cost.WriteCost, "body"},
				Line{"Custo de armazenamento: " + cost.StorageCost, "body"}, Line{"Janela: " + cost.Window, "body"}, Line{"Risco: " + cost.Risk, "body"},
				Line{"Validação posterior: " + cost.PostValidation, "body"})
			if cost.BusinessApproval {
				lines = append(lines, Line{"Exige aprovação de negócio antes de qualquer execução; o auditor não aplica a mudança.", "body"})
			}
		}
		if strings.TrimSpace(f.Evidence) != "" {
			lines = append(lines, Line{"Evidência técnica: " + truncateRunes(f.Evidence, maxEvidenceChars), "body"})
		}
		lines = append(lines, Line{"Próximo passo: " + friendly.Next, "body"}, Line{fmt.Sprintf("Confiança: %.0f%% | Regra: %s", f.Confidence*100, f.RuleVersion), "body"})
		if d.Type == "technical" || d.Type == "table" {
			if strings.TrimSpace(f.Summary) != "" {
				lines = append(lines, Line{"Resumo original da regra: " + f.Summary, "body"})
			}
			if strings.TrimSpace(f.Recommendation) != "" {
				lines = append(lines, Line{"Recomendação técnica original: " + f.Recommendation, "body"})
			}
		}
	}
	if (d.Type == "technical" || d.Type == "table") && (len(d.Databases) > 0 || len(d.Tables) > 0) {
		lines = append(lines, Line{"Inventário técnico (apêndice)", "heading"})
		for _, db := range d.Databases {
			lines = append(lines, Line{fmt.Sprintf("Banco %s: %d esquemas, %d tabelas, %d bytes", db.Name, db.Schemas, db.Tables, db.SizeBytes), "body"})
		}
		for _, table := range d.Tables {
			primaryKey := "Não"
			if table.HasPrimaryKey {
				primaryKey = "Sim"
			}
			lines = append(lines, Line{fmt.Sprintf("%s.%s.%s | %d bytes | %d linhas estimadas | PK: %s", table.Database, table.Schema, table.Name, table.SizeBytes, table.Rows, primaryKey), "body"})
		}
	}
	lines = append(lines, Line{Text: "Análise final e próximos passos", Style: "heading"})
	if len(d.Findings) == 0 {
		lines = append(lines, Line{Text: "Não há achados neste recorte. Confira a cobertura antes de concluir que o ambiente não possui riscos.", Style: "body"})
	} else {
		observation := "Foram observados"
		if len(d.Findings) == 1 {
			observation = "Foi observado"
		}
		lines = append(lines, Line{Text: fmt.Sprintf("%s %d %s neste recorte, incluindo %d %s e %d %s. Priorize a validação dos itens de maior severidade com a equipe responsável pelo banco.", observation, len(d.Findings), plural(len(d.Findings), "achado", "achados"), critical, plural(critical, "crítico", "críticos"), high, plural(high, "alto", "altos")), Style: "body"})
		for index, r := range ranked {
			if index >= 5 {
				break
			}
			f := byID[r.FindingID]
			friendly := guidance.For(f.Type)
			lines = append(lines, Line{Text: fmt.Sprintf("Prioridade %d (achado %s, %s, pontuação %.2f): %s em %s.%s.%s. Próximo passo: %s", r.Rank, f.ID, severityLabel(f.Severity), r.Score, friendly.Meaning, f.Database, f.Schema, f.Object, friendly.Next), Style: "body"})
		}
	}
	if d.Truncated || d.Coverage != "complete" {
		lines = append(lines, Line{Text: "A cobertura é parcial ou o conteúdo foi limitado. Execute uma coleta completa antes de decidir alterações com base na ausência de achados.", Style: "body"})
	}
	lines = append(lines, Line{Text: "Valide cada recomendação com métricas, plano de execução e teste em ambiente controlado. O auditor não aplica mudanças no banco auditado.", Style: "body"})
	lines = append(lines, Line{"Metodologia e glossário", "heading"}, Line{"Somente dados e observações da execução informada foram usados.", "body"}, Line{"Achado: sinal de diagnóstico que exige validação humana.", "body"}, Line{"Baseline: execução aprovada para comparação temporal.", "body"}, Line{"Cobertura parcial impede conclusões sobre ausência de objetos e achados.", "body"})
	return lines
}

// MaxDetailedFindings bounds the per-finding detail; the rest are listed in one
// line each. DetailLimited reports when that happened so the job records it.
const MaxDetailedFindings = 100

// LimitNote summarizes every limit applied to the document; empty when the
// report is complete for its scope. The worker stores it on the job.
func (d Document) LimitNote() string {
	notes := []string{}
	if d.Truncated {
		notes = append(notes, "listagens limitadas (500 bancos, 5000 tabelas, 2000 achados)")
	}
	if DetailLimited(d) {
		notes = append(notes, fmt.Sprintf("detalhamento limitado aos %d achados de maior prioridade", MaxDetailedFindings))
	}
	if d.Coverage != "" && d.Coverage != "complete" {
		notes = append(notes, "cobertura "+coverageLabel(d.Coverage))
	}
	return strings.Join(notes, "; ")
}

// DetailLimited reports whether BuildLines will summarize findings beyond
// MaxDetailedFindings.
func DetailLimited(d Document) bool { return len(d.Findings) > MaxDetailedFindings }

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
func growthLines(points []GrowthPoint) []Line {
	lines := []Line{{"Evolução de armazenamento", "heading"}, {"Barras: tamanho total das tabelas (unidade: bytes) por coleta completa. Coletas parciais aparecem como lacuna e não entram na variação.", "body"}}
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
			Line{fmt.Sprintf("Alternativa textual: execução %s em %s = %d bytes em %d tabelas (coleta completa).", point.RunID, at, point.SizeBytes, point.Tables), "body"})
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
