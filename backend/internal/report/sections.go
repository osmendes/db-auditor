package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/guidance"
	"github.com/osmendes/db-auditor/internal/impact"
	"github.com/osmendes/db-auditor/internal/priority"
)

// SectionSource loads the report data one section at a time (#65). Overview
// returns every field except Databases, Tables and Findings, which come from
// their own calls. Each call must return the same data every time it is made
// during one render; the renderer walks the sections more than once.
type SectionSource interface {
	Overview(ctx context.Context) (Document, error)
	Databases(ctx context.Context) ([]Database, error)
	Tables(ctx context.Context) ([]Table, error)
	Findings(ctx context.Context) ([]Finding, error)
}

// Redaction is applied per section with the same rules as
// Document.RedactMetadata(Metadata) followed by Document.RedactSensitive(Level).
type Redaction struct {
	Metadata string
	Level    string
}

// MemorySource serves an already loaded document through SectionSource.
func MemorySource(d Document) SectionSource { return memorySource{d: d} }

type memorySource struct{ d Document }

func (m memorySource) Overview(context.Context) (Document, error) {
	overview := m.d
	overview.Databases, overview.Tables, overview.Findings = nil, nil, nil
	return overview, nil
}
func (m memorySource) Databases(context.Context) ([]Database, error) { return m.d.Databases, nil }
func (m memorySource) Tables(context.Context) ([]Table, error)       { return m.d.Tables, nil }
func (m memorySource) Findings(context.Context) ([]Finding, error)   { return m.d.Findings, nil }

// sectionPlan holds the overview and the few aggregates the summary and the
// closing section need from the large sections, so those sections can be
// loaded again later, one at a time, while the lines are emitted.
type sectionPlan struct {
	source         SectionSource
	redaction      *Redaction
	overview       Document
	databases      int
	tables         int
	findings       int
	critical, high int
	severityCounts map[string]int
	// top holds the five highest-ranked findings with their rank, for the
	// closing section.
	top []rankedFinding
}

type rankedFinding struct {
	rank    priority.Ranked
	finding Finding
}

func planSections(ctx context.Context, source SectionSource, redaction *Redaction) (*sectionPlan, error) {
	plan := &sectionPlan{source: source, redaction: redaction, severityCounts: map[string]int{}}
	overview, err := source.Overview(ctx)
	if err != nil {
		return nil, err
	}
	databases, err := source.Databases(ctx)
	if err != nil {
		return nil, err
	}
	rawDatabases := len(databases)
	plan.databases = len(plan.prepareDatabases(databases))
	if err = sectionCause(ctx); err != nil {
		return nil, err
	}
	tables, err := source.Tables(ctx)
	if err != nil {
		return nil, err
	}
	plan.tables = len(plan.prepareTables(tables))
	if err = sectionCause(ctx); err != nil {
		return nil, err
	}
	findings, err := source.Findings(ctx)
	if err != nil {
		return nil, err
	}
	findings = plan.prepareFindings(findings)
	plan.findings = len(findings)
	for _, f := range findings {
		plan.severityCounts[strings.ToLower(f.Severity)]++
		if f.Severity == "critical" {
			plan.critical++
		}
		if f.Severity == "high" {
			plan.high++
		}
	}
	plan.top = topRanked(findings, 5)
	if redaction != nil {
		overview.RedactMetadata(redaction.Metadata)
		overview.redactOverview(redaction.Level, rawDatabases)
	}
	if overview.TotalDatabases == 0 {
		overview.TotalDatabases = plan.databases
	}
	if overview.TotalTables == 0 {
		overview.TotalTables = plan.tables
	}
	if overview.TotalFindings == 0 {
		overview.TotalFindings = plan.findings
	}
	plan.overview = overview
	return plan, sectionCause(ctx)
}

func sectionCause(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return err
	}
	return nil
}

func (p *sectionPlan) level() string {
	if p.redaction == nil {
		return ""
	}
	return p.redaction.Level
}

func (p *sectionPlan) prepareDatabases(databases []Database) []Database {
	if p.redaction != nil {
		databases = redactDatabases(p.level(), databases)
	}
	sort.Slice(databases, func(i, j int) bool { return databases[i].Name < databases[j].Name })
	return databases
}

func (p *sectionPlan) prepareTables(tables []Table) []Table {
	if p.redaction != nil {
		tables = redactTables(p.level(), tables)
	}
	sort.Slice(tables, func(i, j int) bool {
		a, b := tables[i], tables[j]
		return a.Database+"/"+a.Schema+"/"+a.Name < b.Database+"/"+b.Schema+"/"+b.Name
	})
	return tables
}

func (p *sectionPlan) prepareFindings(findings []Finding) []Finding {
	if p.redaction != nil {
		findings = redactFindings(p.level(), findings)
	}
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) < severityRank(b.Severity)
		}
		return a.Severity+a.Category+a.Database+a.Schema+a.Object+a.Title < b.Severity+b.Category+b.Database+b.Schema+b.Object+b.Title
	})
	return findings
}

// emitLines produces the report lines in order. The findings, databases and
// tables sections are loaded from the source when their turn comes and
// released right after, so at most one large section is held at a time.
func (p *sectionPlan) emitLines(ctx context.Context, emit func(Line) error) error {
	d := p.overview
	out := func(lines ...Line) error {
		for _, line := range lines {
			if err := emit(line); err != nil {
				return err
			}
		}
		return nil
	}
	if err := out(Line{"DB Auditor - Relatório " + reportTypeLabel(d.Type), "title"}, Line{"Nível de redação: " + redactionLabel(d.RedactionLevel), "body"}, Line{"Ambiente: " + d.Environment, "body"}, Line{"Execução: " + d.RunID, "body"}, Line{"Início: " + d.RunStarted.UTC().Format("02/01/2006 15:04 UTC") + " | Estado: " + runStatusLabel(d.RunStatus), "body"}, Line{"Versão do serviço: " + d.ServiceVersion + " | Regras: " + d.RuleVersion, "body"}, Line{"Solicitante: " + d.RequestedBy, "body"}, Line{"Escopo: " + scope(d), "body"}, Line{"", "body"}, Line{"Resumo executivo", "heading"}); err != nil {
		return err
	}
	critical, high := p.critical, p.high
	if err := out(Line{fmt.Sprintf("%d bancos | %d tabelas | %d %s (%d %s, %d %s)", d.TotalDatabases, d.TotalTables, d.TotalFindings, plural(d.TotalFindings, "achado", "achados"), critical, plural(critical, "crítico", "críticos"), high, plural(high, "alto", "altos")), "body"}); err != nil {
		return err
	}
	if err := out(Line{"Como ler: confirme primeiro a cobertura e os itens críticos; a nota resume sinais observados, não substitui a investigação.", "body"}, Line{Text: "Achados por severidade (itens incluídos no PDF)", Style: "subheading"}); err != nil {
		return err
	}
	for _, severity := range []struct{ key, label string }{{"critical", "Críticos"}, {"high", "Altos"}, {"medium", "Médios"}, {"low", "Baixos"}, {"info", "Informativos"}} {
		if err := out(Line{Text: fmt.Sprintf("%s|%d|%d", severity.label, p.severityCounts[severity.key], p.findings), Style: "bar"}, Line{Text: fmt.Sprintf("Alternativa textual: %s = %d de %d itens.", severity.label, p.severityCounts[severity.key], p.findings), Style: "body"}); err != nil {
			return err
		}
	}
	if err := out(Line{"Cobertura: " + coverageLabel(d.Coverage), "body"}); err != nil {
		return err
	}
	if d.Score != nil {
		if err := out(Line{fmt.Sprintf("Índice: %d/100 | confiança %.0f%%", *d.Score, d.ScoreConfidence*100), "body"}); err != nil {
			return err
		}
	} else if err := out(Line{fmt.Sprintf("Índice indisponível | confiança %.0f%%", d.ScoreConfidence*100), "body"}); err != nil {
		return err
	}
	for _, c := range d.ScoreCategories {
		if err := out(Line{fmt.Sprintf("%s: %d/100 (%d achados, penalidade %d, evidência positiva +%d)", c.Category, c.Score, c.Findings, c.Penalty, c.Positive), "body"}); err != nil {
			return err
		}
	}
	for _, note := range d.CoverageNotes {
		if err := out(Line{"Limite: " + note, "body"}); err != nil {
			return err
		}
	}
	if d.Truncated {
		if err := out(Line{"Limite: inventário resumido para manter o consumo de memória previsível.", "body"}); err != nil {
			return err
		}
	}
	if d.Baseline != nil {
		if err := out(Line{"Histórico e referência de comparação", "heading"}, Line{fmt.Sprintf("Referência %s | %s | +%d / -%d tabelas, %d alteradas", d.Baseline.RunID, coverageLabel(d.Baseline.Status), d.Baseline.AddedTables, d.Baseline.RemovedTables, d.Baseline.ChangedTables), "body"}); err != nil {
			return err
		}
		if len(d.Regressions) > 0 {
			if err := out(Line{"Novos achados de maior prioridade desde a referência", "subheading"}); err != nil {
				return err
			}
			for _, regression := range d.Regressions {
				if err := out(Line{fmt.Sprintf("%s · %s · objeto %s · achado %s", severityLabel(regression.Severity), guidance.For(regression.Type).Meaning, regression.Object, regression.FindingID), "body"}); err != nil {
					return err
				}
			}
		} else if err := out(Line{"Nenhum novo achado alto ou crítico identificado neste recorte; confirme a cobertura antes de concluir melhora.", "body"}); err != nil {
			return err
		}
	}
	if len(d.Growth) > 0 {
		if err := out(growthLines(d.Growth)...); err != nil {
			return err
		}
	}
	if err := out(Line{"Riscos e prioridades", "heading"}, Line{"Matriz de decisão (" + priority.FormulaVersion + "): pontuação = peso da severidade × fator de impacto medido × confiança ÷ esforço. Impacto sem métrica não eleva a prioridade. Confirme dependências, equipe e janela antes de executar uma mudança.", "body"}); err != nil {
		return err
	}
	if err := p.emitFindings(ctx, d, out); err != nil {
		return err
	}
	if (d.Type == "technical" || d.Type == "table") && (p.databases > 0 || p.tables > 0) {
		if err := out(Line{"Inventário técnico (apêndice)", "heading"}); err != nil {
			return err
		}
		if err := p.emitInventory(ctx, out); err != nil {
			return err
		}
	}
	if err := out(Line{Text: "Análise final e próximos passos", Style: "heading"}); err != nil {
		return err
	}
	if p.findings == 0 {
		if err := out(Line{Text: "Não há achados neste recorte. Confira a cobertura antes de concluir que o ambiente não possui riscos.", Style: "body"}); err != nil {
			return err
		}
	} else {
		observation := "Foram observados"
		if p.findings == 1 {
			observation = "Foi observado"
		}
		if err := out(Line{Text: fmt.Sprintf("%s %d %s neste recorte, incluindo %d %s e %d %s. Priorize a validação dos itens de maior severidade com a equipe responsável pelo banco.", observation, p.findings, plural(p.findings, "achado", "achados"), critical, plural(critical, "crítico", "críticos"), high, plural(high, "alto", "altos")), Style: "body"}); err != nil {
			return err
		}
		for _, top := range p.top {
			r, f := top.rank, top.finding
			friendly := guidance.For(f.Type)
			if err := out(Line{Text: fmt.Sprintf("Prioridade %d (achado %s, %s, pontuação %.2f): %s em %s.%s.%s. Próximo passo: %s", r.Rank, f.ID, severityLabel(f.Severity), r.Score, friendly.Meaning, f.Database, f.Schema, f.Object, friendly.Next), Style: "body"}); err != nil {
				return err
			}
		}
	}
	if d.Truncated || d.Coverage != "complete" {
		if err := out(Line{Text: "A cobertura é parcial ou o conteúdo foi limitado. Execute uma coleta completa antes de decidir alterações com base na ausência de achados.", Style: "body"}); err != nil {
			return err
		}
	}
	return out(Line{Text: "Valide cada recomendação com métricas, plano de execução e teste em ambiente controlado. O auditor não aplica mudanças no banco auditado.", Style: "body"}, Line{"Metodologia e glossário", "heading"}, Line{"Somente dados e observações da execução informada foram usados.", "body"}, Line{"Achado: sinal de diagnóstico que exige validação humana.", "body"}, Line{"Baseline: execução aprovada para comparação temporal.", "body"}, Line{"Cobertura parcial impede conclusões sobre ausência de objetos e achados.", "body"})
}

// emitFindings loads the findings section and emits the priority matrix and
// the per-finding detail in rank order (AN-02), the first MaxDetailedFindings
// in full and the rest in one line each.
func (p *sectionPlan) emitFindings(ctx context.Context, d Document, out func(...Line) error) error {
	if err := sectionCause(ctx); err != nil {
		return err
	}
	findings, err := p.source.Findings(ctx)
	if err != nil {
		return err
	}
	findings = p.prepareFindings(findings)
	ranked := rankFindings(findings)
	if len(findings) == 0 {
		if err := out(Line{"Nenhum achado observado neste escopo da execução.", "body"}); err != nil {
			return err
		}
	} else {
		if err := out(Line{Text: "Matriz impacto × esforço × confiança × risco", Style: "subheading"}); err != nil {
			return err
		}
		for i, r := range ranked {
			if i >= matrixRows {
				if err := out(Line{fmt.Sprintf("Matriz limitada aos %d itens de maior pontuação; os demais aparecem no detalhamento abaixo.", matrixRows), "body"}); err != nil {
					return err
				}
				break
			}
			if err := out(Line{fmt.Sprintf("#%d | achado %s | pontuação %.2f/%.1f | severidade %s | impacto %s | esforço %s | confiança %s (%.0f%%) | risco: %s", r.Rank, r.FindingID, r.Score, r.MaxScore, severityLabel(r.Severity), r.ImpactLevel, r.Effort, r.ConfidenceLabel, r.Confidence*100, r.Risk), "body"}); err != nil {
				return err
			}
		}
	}
	byID := findingsByID(findings)
	for index, r := range ranked {
		f := byID[r.FindingID]
		if index >= MaxDetailedFindings {
			if index == MaxDetailedFindings {
				if err := out(Line{"Demais achados (resumo de uma linha)", "subheading"}, Line{fmt.Sprintf("Detalhamento limitado aos %d achados de maior pontuação para manter o PDF dentro de %d páginas; consulte os demais na interface.", MaxDetailedFindings, maxPDFPages), "body"}); err != nil {
					return err
				}
			}
			if err := out(Line{fmt.Sprintf("#%d %s | %s | %s.%s.%s | pontuação %.2f", r.Rank, f.ID, severityLabel(f.Severity), f.Database, f.Schema, f.Object, r.Score), "body"}); err != nil {
				return err
			}
			continue
		}
		friendly := guidance.For(f.Type)
		plan := actions.For(f.Type)
		estimate := f.Impact
		if estimate.Status == "" {
			estimate = impact.For(f.Type, nil, time.Time{}, time.Time{})
		}
		if err := out(Line{fmt.Sprintf("[%s] %s - %s.%s.%s", strings.ToUpper(severityLabel(f.Severity)), friendly.Meaning, f.Database, f.Schema, f.Object), "subheading"},
			Line{fmt.Sprintf("ID do achado: %s | severidade: %s | esforço preliminar: %s", f.ID, severityLabel(f.Severity), effortLabel(plan)), "body"},
			Line{"Impacto: " + estimate.Label, "body"}, Line{"Unidade: " + unitLabel(estimate) + " | Período de referência: " + estimate.Period, "body"},
			Line{"Risco da mudança: " + plan.Risk, "body"}); err != nil {
			return err
		}
		gain := f.Gain
		if gain.Hypothesis == "" {
			gain = impact.BuildGain(friendly.Meaning, nil, estimate, nil)
		}
		if err := out(Line{"Hipótese: " + gain.Hypothesis, "body"}, Line{"Ganho previsto: " + gain.Predicted, "body"}, Line{"Ganho medido: " + gain.Measured, "body"}); err != nil {
			return err
		}
		if cost := plan.Maintenance; cost != nil {
			if err := out(Line{"Custos de " + maintenanceKindLabel(cost.Kind) + " (" + cost.Capability + ")", "subheading"},
				Line{"Pré-condições: " + cost.Preconditions, "body"}, Line{"Custo de leitura: " + cost.ReadCost, "body"}, Line{"Custo de escrita: " + cost.WriteCost, "body"},
				Line{"Custo de armazenamento: " + cost.StorageCost, "body"}, Line{"Janela: " + cost.Window, "body"}, Line{"Risco: " + cost.Risk, "body"},
				Line{"Validação posterior: " + cost.PostValidation, "body"}); err != nil {
				return err
			}
			if cost.BusinessApproval {
				if err := out(Line{"Exige aprovação de negócio antes de qualquer execução; o auditor não aplica a mudança.", "body"}); err != nil {
					return err
				}
			}
		}
		if strings.TrimSpace(f.Evidence) != "" {
			if err := out(Line{"Evidência técnica: " + truncateRunes(f.Evidence, maxEvidenceChars), "body"}); err != nil {
				return err
			}
		}
		if err := out(Line{"Próximo passo: " + friendly.Next, "body"}, Line{fmt.Sprintf("Confiança: %.0f%% | Regra: %s", f.Confidence*100, f.RuleVersion), "body"}); err != nil {
			return err
		}
		if d.Type == "technical" || d.Type == "table" {
			if strings.TrimSpace(f.Summary) != "" {
				if err := out(Line{"Resumo original da regra: " + f.Summary, "body"}); err != nil {
					return err
				}
			}
			if strings.TrimSpace(f.Recommendation) != "" {
				if err := out(Line{"Recomendação técnica original: " + f.Recommendation, "body"}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// findingsByID mirrors the whole-document builder: a later finding with a
// repeated id replaces the earlier one.
func findingsByID(findings []Finding) map[string]Finding {
	byID := make(map[string]Finding, len(findings))
	for _, f := range findings {
		byID[f.ID] = f
	}
	return byID
}

// topRanked returns the first n findings in rank order with their rank.
func topRanked(findings []Finding, n int) []rankedFinding {
	ranked := rankFindings(findings)
	byID := findingsByID(findings)
	top := []rankedFinding{}
	for index, r := range ranked {
		if index >= n {
			break
		}
		top = append(top, rankedFinding{rank: r, finding: byID[r.FindingID]})
	}
	return top
}

func (p *sectionPlan) emitInventory(ctx context.Context, out func(...Line) error) error {
	if err := sectionCause(ctx); err != nil {
		return err
	}
	databases, err := p.source.Databases(ctx)
	if err != nil {
		return err
	}
	for _, db := range p.prepareDatabases(databases) {
		if err := out(Line{fmt.Sprintf("Banco %s: %d esquemas, %d tabelas, %d bytes", db.Name, db.Schemas, db.Tables, db.SizeBytes), "body"}); err != nil {
			return err
		}
	}
	if err := sectionCause(ctx); err != nil {
		return err
	}
	tables, err := p.source.Tables(ctx)
	if err != nil {
		return err
	}
	for _, table := range p.prepareTables(tables) {
		primaryKey := "Não"
		if table.HasPrimaryKey {
			primaryKey = "Sim"
		}
		if err := out(Line{fmt.Sprintf("%s.%s.%s | %d bytes | %d linhas estimadas | PK: %s", table.Database, table.Schema, table.Name, table.SizeBytes, table.Rows, primaryKey), "body"}); err != nil {
			return err
		}
	}
	return nil
}
