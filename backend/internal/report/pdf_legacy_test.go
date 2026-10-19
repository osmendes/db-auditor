package report

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/guidance"
	"github.com/osmendes/db-auditor/internal/impact"
	"github.com/osmendes/db-auditor/internal/priority"
)

// legacyRenderPDF is the pre-PF-07 in-memory renderer, kept as the
// byte-for-byte oracle of WritePDF: it builds every line and every page in
// memory before writing. #107 changed the output on purpose (artifact and
// marked-content operators in the page streams, structure tree, embedded
// font, Info/XMP title), so the page commands and the structure bookkeeping
// were updated here independently of pdfLayout; the objects after the
// content streams come from the shared writeDocumentObjects, whose content
// is pinned by the #107 tests in pdf_tagged_test.go.
func legacyRenderPDF(document Document) ([]byte, error) {
	lines := legacyBuildLines(document)
	textBytes := 0
	for _, line := range lines {
		textBytes += len(line.Text)
		if textBytes > maxReportTextBytes {
			return nil, fmt.Errorf("relatório excede o limite de texto (%d bytes); reduza o escopo ou use filtros", maxReportTextBytes)
		}
	}
	pages := [][]string{{}}
	tags := newTagging()
	mark := func(elem *int, role string) string {
		if *elem < 0 {
			*elem = tags.open(role)
		}
		return fmt.Sprintf("/%s << /MCID %d >> BDC ", role, tags.mark(*elem))
	}
	y := 790
	for _, line := range lines {
		if line.Style == "heading" && y < 110 {
			if len(pages) >= maxPDFPages {
				return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
			}
			pages = append(pages, []string{})
			tags.newPage()
			y = 790
		}
		if line.Style == "bar_bytes" {
			if y < 75 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
				tags.newPage()
				y = 790
			}
			parts := strings.Split(line.Text, "|")
			if len(parts) == 3 {
				value, valueErr := strconv.ParseInt(parts[1], 10, 64)
				maximum, maxErr := strconv.ParseInt(parts[2], 10, 64)
				if valueErr == nil && maxErr == nil && value >= 0 && maximum >= 0 {
					width := 0
					if maximum > 0 {
						width = int(300 * float64(value) / float64(maximum))
					}
					elem := -1
					command := fmt.Sprintf("/Artifact BMC 0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 165 %d 250 11 re f 0.09 0.55 0.42 rg 165 %d %d 11 re f EMC %s0.12 0.18 0.24 rg BT /F1 8 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d B) Tj ET EMC\n", y-2, y-2, y-2, width*250/300, mark(&elem, "P"), y, escapePDFText(parts[0]), y, value)
					pages[len(pages)-1] = append(pages[len(pages)-1], command)
					y -= 20
					continue
				}
			}
		}
		if line.Style == "bar" {
			if y < 75 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
				tags.newPage()
				y = 790
			}
			parts := strings.Split(line.Text, "|")
			if len(parts) == 3 {
				value, valueErr := strconv.Atoi(parts[1])
				maximum, maxErr := strconv.Atoi(parts[2])
				if valueErr == nil && maxErr == nil && value >= 0 && maximum >= 0 {
					width := 0
					if maximum > 0 {
						width = 300 * value / maximum
					}
					elem := -1
					command := fmt.Sprintf("/Artifact BMC 0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 115 %d 300 11 re f 0.09 0.55 0.42 rg 115 %d %d 11 re f EMC %s0.12 0.18 0.24 rg BT /F1 9 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d) Tj ET EMC\n", y-2, y-2, y-2, width, mark(&elem, "P"), y, escapePDFText(parts[0]), y, value)
					pages[len(pages)-1] = append(pages[len(pages)-1], command)
					y -= 20
					continue
				}
			}
		}
		fontSize, step := 10, 14
		if line.Style == "title" {
			fontSize, step = 18, 26
		}
		if line.Style == "heading" {
			fontSize, step = 13, 21
		}
		if line.Style == "subheading" {
			fontSize, step = 10, 17
		}
		if y < 780 {
			switch line.Style {
			case "heading":
				y -= 10
			case "subheading":
				y -= 4
			}
		}
		width := 90
		if fontSize == 18 {
			width = 54
		}
		if fontSize == 13 {
			width = 75
		}
		elem := -1
		role := map[string]string{"title": "H1", "heading": "H2", "subheading": "H3"}[line.Style]
		if role == "" {
			role = "P"
		}
		for _, part := range wrapText(line.Text, width) {
			if y < 55 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
				tags.newPage()
				y = 790
			}
			color := "0.15 0.20 0.26"
			if line.Style == "title" || line.Style == "heading" {
				color = "0.08 0.45 0.35"
			}
			open := "/Artifact BMC "
			if strings.TrimSpace(part) != "" {
				if elem < 0 && line.Style == "heading" {
					// Bookmark at the top of the section heading (#107).
					tags.section(line.Text, len(pages)-1, y+fontSize+4)
				}
				open = mark(&elem, role)
			}
			command := fmt.Sprintf("%s%s rg BT /F1 %d Tf 42 %d Td (%s) Tj ET EMC\n", open, color, fontSize, y, escapePDFText(part))
			pages[len(pages)-1] = append(pages[len(pages)-1], command)
			y -= step
			fontSize, step = 10, 14
		}
	}
	font, err := embeddedFont()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := []int{0}
	writeObject := func(body string) error {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body)
		if out.Len() > maxPDFBytes {
			return fmt.Errorf("relatório excede %d bytes; reduza o escopo ou use filtros", maxPDFBytes)
		}
		return nil
	}
	pageCount := len(pages)
	run := document.RunID
	if len(run) > 8 {
		run = run[:8]
	}
	header := fmt.Sprintf("/Artifact BMC 0.45 0.50 0.55 rg BT /F1 9 Tf 42 815 Td (%s) Tj ET EMC\n", escapePDFText("DB Auditor · "+reportTypeLabel(document.Type)+" · execução "+run))
	for index := range pages {
		commands := pages[index]
		pages[index] = nil
		stream := "/Artifact BMC 1 1 1 rg 0 0 595 842 re f EMC\n"
		if index > 0 {
			stream += header
		}
		stream += strings.Join(commands, "") + fmt.Sprintf("/Artifact BMC 0.45 0.50 0.55 rg BT /F1 9 Tf 42 30 Td (%s) Tj ET EMC\n", escapePDFText(fmt.Sprintf("Página %d de %d", index+1, pageCount)))
		if err := writeObject(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)); err != nil {
			return nil, err
		}
	}
	catalogID, infoID, err := writeDocumentObjects(writeObject, pageCount, document, tags, font)
	if err != nil {
		return nil, err
	}
	if catalogID != len(offsets)-1 {
		return nil, fmt.Errorf("relatório PDF inconsistente")
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	out.WriteString(pdfTrailer(len(offsets), catalogID, infoID, int64(xref)))
	if out.Len() > maxPDFBytes {
		return nil, fmt.Errorf("relatório excede %d bytes; reduza o escopo ou use filtros", maxPDFBytes)
	}
	return out.Bytes(), nil
}

// legacyBuildLines is the whole-document line builder (the #118 report
// content: priority matrix, impact and gain, maintenance costs, growth gaps,
// detail limit) as it was before the section-by-section split, kept verbatim
// as the oracle of the section-by-section builder.
func legacyBuildLines(d Document) []Line {
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
		lines = append(lines, growthLines(d.Growth, growthSubject(d))...)
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
			// #107: Portuguese rule summary (intentional output change).
			if summary := strings.TrimSpace(actions.For(f.Type).Evidence); summary != "" {
				lines = append(lines, Line{"Resumo da regra: " + summary, "body"})
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

// legacyRedactSensitive is the pre-#65 whole-document redaction, kept verbatim.
func legacyRedactSensitive(d *Document, level string) {
	if level != "identifiers" && level != "strict" {
		d.RedactionLevel = "none"
		return
	}
	d.RedactionLevel = level
	mask := "[oculto]"
	d.Environment, d.RequestedBy = mask, mask
	d.DatabaseFilter, d.SchemaFilter, d.TableFilter = mask, mask, mask
	for i := range d.Databases {
		d.Databases[i].Name = mask
	}
	for i := range d.Tables {
		d.Tables[i].Database, d.Tables[i].Schema, d.Tables[i].Name = mask, mask, mask
	}
	for i := range d.Findings {
		d.Findings[i].Database, d.Findings[i].Schema, d.Findings[i].Object = mask, mask, mask
		d.Findings[i].Evidence = "[evidência omitida]"
		d.Findings[i].Title = mask
		d.Findings[i].Summary = "[resumo omitido]"
		d.Findings[i].Recommendation = "[texto original omitido]"
	}
	for i := range d.CoverageNotes {
		d.CoverageNotes[i] = "Detalhes de cobertura omitidos neste nível de redação."
	}
	for i := range d.Regressions {
		d.Regressions[i].Object = mask
	}
	if level == "strict" {
		if d.TotalDatabases == 0 {
			d.TotalDatabases = len(d.Databases)
		}
		// The strict copy keeps counts and provenance, but removes object lists,
		// storage evolution and baseline details that could reveal topology.
		d.Databases = nil
		d.Tables = nil
		d.Growth = nil
		d.Baseline = nil
		d.Regressions = nil
	}
}
