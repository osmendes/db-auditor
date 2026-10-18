package report

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/guidance"
)

// legacyRenderPDF is the pre-PF-07 in-memory renderer, kept verbatim as the
// byte-for-byte oracle of WritePDF. It writes a deterministic, paginated PDF 1.4 document with no
// external font or native runtime dependency. All text is WinAnsi encoded.
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
	y := 790
	for _, line := range lines {
		if line.Style == "heading" && y < 110 {
			if len(pages) >= maxPDFPages {
				return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
			}
			pages = append(pages, []string{})
			y = 790
		}
		if line.Style == "bar_bytes" {
			if y < 75 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
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
					command := fmt.Sprintf("0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 165 %d 250 11 re f 0.09 0.55 0.42 rg 165 %d %d 11 re f 0.12 0.18 0.24 rg BT /F1 8 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d B) Tj ET\n", y-2, y-2, y-2, width*250/300, y, escapePDFText(parts[0]), y, value)
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
					command := fmt.Sprintf("0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 115 %d 300 11 re f 0.09 0.55 0.42 rg 115 %d %d 11 re f 0.12 0.18 0.24 rg BT /F1 9 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d) Tj ET\n", y-2, y-2, y-2, width, y, escapePDFText(parts[0]), y, value)
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
		for _, part := range wrapText(line.Text, width) {
			if y < 55 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
				y = 790
			}
			color := "0.15 0.20 0.26"
			if line.Style == "title" || line.Style == "heading" {
				color = "0.08 0.45 0.35"
			}
			command := fmt.Sprintf("%s rg BT /F1 %d Tf 42 %d Td (%s) Tj ET\n", color, fontSize, y, escapePDFText(part))
			pages[len(pages)-1] = append(pages[len(pages)-1], command)
			y -= step
			fontSize, step = 10, 14
		}
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
	contentID := func(index int) int { return index + 1 }
	pageID := func(index int) int { return pageCount + index + 1 }
	fontID := 2*pageCount + 1
	pagesID := 2*pageCount + 2
	catalogID := 2*pageCount + 3
	for index := range pages {
		commands := pages[index]
		pages[index] = nil
		stream := "1 1 1 rg 0 0 595 842 re f\n"
		if index > 0 {
			run := document.RunID
			if len(run) > 8 {
				run = run[:8]
			}
			stream += fmt.Sprintf("0.45 0.50 0.55 rg BT /F1 9 Tf 42 815 Td (%s) Tj ET\n", escapePDFText("DB Auditor · "+reportTypeLabel(document.Type)+" · execução "+run))
		}
		stream += strings.Join(commands, "") + fmt.Sprintf("0.45 0.50 0.55 rg BT /F1 9 Tf 42 30 Td (%s) Tj ET\n", escapePDFText(fmt.Sprintf("Página %d de %d", index+1, pageCount)))
		if err := writeObject(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)); err != nil {
			return nil, err
		}
	}
	kids := make([]string, pageCount)
	for index := 0; index < pageCount; index++ {
		kids[index] = fmt.Sprintf("%d 0 R", pageID(index))
		body := fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pagesID, fontID, contentID(index))
		if err := writeObject(body); err != nil {
			return nil, err
		}
	}
	if err := writeObject("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"); err != nil {
		return nil, err
	}
	if err := writeObject(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pageCount)); err != nil {
		return nil, err
	}
	if err := writeObject(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID)); err != nil {
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
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), catalogID, xref)
	if out.Len() > maxPDFBytes {
		return nil, fmt.Errorf("relatório excede %d bytes; reduza o escopo ou use filtros", maxPDFBytes)
	}
	return out.Bytes(), nil
}

// legacyBuildLines is the pre-#65 whole-document line builder, kept verbatim
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
		lines = append(lines, Line{"Evolução de armazenamento", "heading"}, Line{"Barras: tamanho total das tabelas na execução (bytes); período e coleta parcial estão identificados em cada linha.", "body"})
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
			lines = append(lines, Line{fmt.Sprintf("%s|%d|%d", label, point.SizeBytes, maximum), "bar_bytes"}, Line{fmt.Sprintf("Execução %s: %d bytes em %d tabelas", point.RunID, point.SizeBytes, point.Tables), "body"})
		}
		if len(d.Growth) > 1 {
			first, last := d.Growth[0], d.Growth[len(d.Growth)-1]
			lines = append(lines, Line{fmt.Sprintf("Variação entre o primeiro e o último ponto: %+d bytes. Confirme que perfil e cobertura são comparáveis antes de atribuir a mudança a uma ação.", last.SizeBytes-first.SizeBytes), "body"})
		}
	}
	lines = append(lines, Line{"Riscos e prioridades", "heading"})
	lines = append(lines, Line{"Matriz de decisão: impacto é a severidade observada; esforço é uma faixa preliminar para planejar a investigação. Confirme dependências, equipe e janela antes de executar uma mudança.", "body"})
	if len(d.Findings) == 0 {
		lines = append(lines, Line{"Nenhum achado observado neste escopo da execução.", "body"})
	}
	for _, f := range d.Findings {
		friendly := guidance.For(f.Type)
		plan := actions.For(f.Type)
		lines = append(lines, Line{fmt.Sprintf("[%s] %s - %s.%s.%s", strings.ToUpper(severityLabel(f.Severity)), friendly.Meaning, f.Database, f.Schema, f.Object), "subheading"})
		lines = append(lines, Line{fmt.Sprintf("ID do achado: %s | impacto: %s | esforço preliminar: %s", f.ID, severityLabel(f.Severity), effortLabel(plan.Category)), "body"})
		lines = append(lines, Line{"Risco da mudança: " + plan.Risk, "body"}, Line{"Benefício esperado: " + plan.Benefit, "body"})
		if strings.TrimSpace(f.Evidence) != "" {
			lines = append(lines, Line{"Evidência técnica: " + f.Evidence, "body"})
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
		for index, f := range d.Findings {
			if index >= 5 {
				break
			}
			friendly := guidance.For(f.Type)
			lines = append(lines, Line{Text: fmt.Sprintf("Prioridade %d (%s): %s em %s.%s.%s. Próximo passo: %s", index+1, severityLabel(f.Severity), friendly.Meaning, f.Database, f.Schema, f.Object, friendly.Next), Style: "body"})
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
