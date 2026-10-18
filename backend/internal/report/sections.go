package report

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/guidance"
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
	top            []Finding
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
	plan.top = append([]Finding(nil), findings[:min(5, len(findings))]...)
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
		if err := out(Line{"Evolução de armazenamento", "heading"}, Line{"Barras: tamanho total das tabelas na execução (bytes); período e coleta parcial estão identificados em cada linha.", "body"}); err != nil {
			return err
		}
		var maximum int64
		for _, point := range d.Growth {
			if point.SizeBytes > maximum {
				maximum = point.SizeBytes
			}
		}
		for _, point := range d.Growth {
			at := point.At.UTC().Format("02/01/2006")
			if point.At.IsZero() {
				at = "data indisponível"
			}
			shortID := point.RunID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			label := at + " " + shortID
			if point.Status == "partial_success" {
				label += " (parcial)"
			}
			if err := out(Line{fmt.Sprintf("%s|%d|%d", label, point.SizeBytes, maximum), "bar_bytes"}, Line{fmt.Sprintf("Execução %s: %d bytes em %d tabelas", point.RunID, point.SizeBytes, point.Tables), "body"}); err != nil {
				return err
			}
		}
		if len(d.Growth) > 1 {
			first, last := d.Growth[0], d.Growth[len(d.Growth)-1]
			if err := out(Line{fmt.Sprintf("Variação entre o primeiro e o último ponto: %+d bytes. Confirme que perfil e cobertura são comparáveis antes de atribuir a mudança a uma ação.", last.SizeBytes-first.SizeBytes), "body"}); err != nil {
				return err
			}
		}
	}
	if err := out(Line{"Riscos e prioridades", "heading"}, Line{"Matriz de decisão: impacto é a severidade observada; esforço é uma faixa preliminar para planejar a investigação. Confirme dependências, equipe e janela antes de executar uma mudança.", "body"}); err != nil {
		return err
	}
	if p.findings == 0 {
		if err := out(Line{"Nenhum achado observado neste escopo da execução.", "body"}); err != nil {
			return err
		}
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
		for index, f := range p.top {
			friendly := guidance.For(f.Type)
			if err := out(Line{Text: fmt.Sprintf("Prioridade %d (%s): %s em %s.%s.%s. Próximo passo: %s", index+1, severityLabel(f.Severity), friendly.Meaning, f.Database, f.Schema, f.Object, friendly.Next), Style: "body"}); err != nil {
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

func (p *sectionPlan) emitFindings(ctx context.Context, d Document, out func(...Line) error) error {
	if err := sectionCause(ctx); err != nil {
		return err
	}
	findings, err := p.source.Findings(ctx)
	if err != nil {
		return err
	}
	for _, f := range p.prepareFindings(findings) {
		friendly := guidance.For(f.Type)
		plan := actions.For(f.Type)
		if err := out(Line{fmt.Sprintf("[%s] %s - %s.%s.%s", strings.ToUpper(severityLabel(f.Severity)), friendly.Meaning, f.Database, f.Schema, f.Object), "subheading"}, Line{fmt.Sprintf("ID do achado: %s | impacto: %s | esforço preliminar: %s", f.ID, severityLabel(f.Severity), effortLabel(plan.Category)), "body"}, Line{"Risco da mudança: " + plan.Risk, "body"}, Line{"Benefício esperado: " + plan.Benefit, "body"}); err != nil {
			return err
		}
		if strings.TrimSpace(f.Evidence) != "" {
			if err := out(Line{"Evidência técnica: " + f.Evidence, "body"}); err != nil {
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
