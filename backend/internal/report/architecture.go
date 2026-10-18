package report

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/actions"
)

const ArchitectureVersion = "architecture-report-v1"
const maxArchitectureBytes = 16 << 20

// ArchitectureDocument contains only facts from one completed audit run.
// No target connection is involved in rendering it.
type ArchitectureDocument struct {
	Environment   string
	Database      string
	ScopeSchema   string
	RunID         string
	RunStarted    time.Time
	RunStatus     string
	RuleVersion   string
	Redaction     string
	Coverage      string
	CoverageNotes []string
	Tables        []ArchitectureTable
	Schemas       []string
	Constraints   []ArchitectureConstraint
	Findings      []Finding
	Extensions    []string
	Metrics       []ArchitectureMetric
	Details       []ArchitectureDetail
	Decisions     []ArchitectureDecision
	Insertions    []InsertionEvidence
}

type ArchitectureMetric struct {
	Name                string
	Value               int64
	Unit, Source, Scope string
	CollectedAt         time.Time
}

type ArchitectureDetail struct {
	ObjectID, Fact, Value, Source string
	CollectedAt                   time.Time
}

type ArchitectureTable struct {
	ID          string
	Schema      string
	Name        string
	SizeBytes   int64
	Rows        int64
	CollectedAt time.Time
	Columns     []ArchitectureColumn
}

type ArchitectureColumn struct{ Name, Type string }

type ArchitectureConstraint struct {
	ID, Schema, Table, Name, Kind string
	SupportingIndex               string
	Columns                       []string
	RefSchema, RefTable           string
	RefColumns                    []string
	Validated                     bool
	CollectedAt                   time.Time
}

type ArchitectureDecision struct {
	ObjectID  string    `json:"object_ref"`
	Subject   string    `json:"subject"`
	Value     string    `json:"value"`
	Version   int       `json:"version"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// Mermaid identifiers are generated from sorted ordinal positions, never from
// catalog text. Catalog values appear only in escaped Markdown tables.
func (d ArchitectureDocument) RenderMarkdown() ([]byte, error) {
	if d.Database == "" || d.RunID == "" {
		return nil, fmt.Errorf("architecture scope requires database and run")
	}
	sort.Slice(d.Tables, func(i, j int) bool { return d.Tables[i].ID < d.Tables[j].ID })
	sort.Slice(d.Constraints, func(i, j int) bool { return d.Constraints[i].ID < d.Constraints[j].ID })
	sort.Slice(d.Findings, func(i, j int) bool { return d.Findings[i].ID < d.Findings[j].ID })
	sort.Strings(d.Extensions)
	sort.Strings(d.Schemas)
	sort.Slice(d.Details, func(i, j int) bool {
		return d.Details[i].ObjectID+d.Details[i].Fact < d.Details[j].ObjectID+d.Details[j].Fact
	})
	var b bytes.Buffer
	var writeErr error
	write := func(s string) error {
		if writeErr != nil {
			return writeErr
		}
		if b.Len()+len(s) > maxArchitectureBytes {
			writeErr = fmt.Errorf("architecture report exceeds size limit")
			return writeErr
		}
		_, err := b.WriteString(s)
		if err != nil {
			writeErr = err
		}
		return err
	}
	line := func(s string) error { return write(s + "\n") }
	if err := line("# Análise completa e modelagem"); err != nil {
		return nil, err
	}
	for _, s := range []string{
		"\n## 1. Capa",
		"- Contrato: " + ArchitectureVersion,
		"- Ambiente: " + md(d.Environment), "- Database: " + md(d.Database), "- Execução: " + md(d.RunID),
		"- Início (UTC): " + d.RunStarted.UTC().Format(time.RFC3339), "- Estado: " + md(d.RunStatus),
		"- Regras: " + md(d.RuleVersion), "- Redação: " + md(d.Redaction),
		"\n## 2. Resumo executivo",
		fmt.Sprintf("%d tabelas e %d achados neste recorte. Recomendações exigem confirmação humana.", len(d.Tables), len(d.Findings)),
		"\n## 3. Cobertura e limites", "- Cobertura: " + md(d.Coverage),
	} {
		if err := line(s); err != nil {
			return nil, err
		}
	}
	for _, note := range d.CoverageNotes {
		if err := line("- Limite: " + md(note)); err != nil {
			return nil, err
		}
	}
	if len(d.CoverageNotes) == 0 && d.Coverage != "complete" {
		_ = line("- Coleta parcial: ausência de fatos não comprova ausência de objetos ou problemas.")
	}
	_ = line("- Tamanhos e contagens vêm dos snapshots da execução; podem ser estimativas. Sem soma de materializações e chunks neste relatório.")
	_ = line("\n## 4. Diagnóstico físico")
	_ = line("| Objeto | Tamanho (bytes) | Linhas estimadas | Coletado (UTC) | Evidência |")
	_ = line("| --- | ---: | ---: | --- | --- |")
	for _, t := range d.Tables {
		if err := line(fmt.Sprintf("| %s | %d | %d | %s | table_snapshot:%s |", md(t.ID), t.SizeBytes, t.Rows, t.CollectedAt.UTC().Format(time.RFC3339), md(t.ID))); err != nil {
			return nil, err
		}
	}
	if len(d.Tables) == 0 {
		_ = line("Inventário indisponível neste escopo; conferir cobertura da execução.")
	}
	if len(d.Extensions) > 0 {
		_ = line("Extensões observadas: " + md(strings.Join(d.Extensions, ", ")) + ". Instalação da extensão, por si só, não classifica um esquema como gerido.")
	} else {
		_ = line("Extensões: indisponível ou não observadas neste recorte.")
	}
	if len(d.Metrics) > 0 {
		_ = line("| Métrica | Valor | Unidade | Escopo | Fonte | Coletado (UTC) |")
		_ = line("| --- | ---: | --- | --- | --- | --- |")
		for _, m := range d.Metrics {
			_ = line(fmt.Sprintf("| %s | %d | %s | %s | %s | %s |", md(m.Name), m.Value, md(m.Unit), md(m.Scope), md(m.Source), m.CollectedAt.UTC().Format(time.RFC3339)))
		}
	}
	_ = line("Métricas Timescale vêm de snapshots próprios. Bytes de hypertables e chunks não são somados entre si. Temporários vêm de agregados por fingerprint na janela de pg_stat_statements; não atribuem culpa a uma aplicação. Tamanho de materializações e tiering: indisponíveis nesta versão da coleta.")
	if len(d.Details) > 0 {
		_ = line("| Objeto técnico | Fato | Valor | Fonte | Coletado (UTC) |")
		_ = line("| --- | --- | --- | --- | --- |")
		for _, item := range d.Details {
			_ = line("| " + md(item.ObjectID) + " | " + md(item.Fact) + " | " + md(item.Value) + " | " + md(item.Source) + " | " + item.CollectedAt.UTC().Format(time.RFC3339) + " |")
		}
	}
	ids := map[string]string{}
	for i, t := range d.Tables {
		ids[t.Schema+"\x00"+t.Name] = fmt.Sprintf("T%d", i+1)
	}
	_ = line("\n## 5. ERD observado")
	_ = line("Somente constraints coletadas geram relações. Os nomes `Tn` correspondem à tabela textual abaixo.")
	_ = line("```mermaid\nerDiagram")
	for _, t := range d.Tables {
		_ = line("    " + ids[t.Schema+"\x00"+t.Name] + " {")
		for i := range t.Columns {
			_ = line(fmt.Sprintf("        string c%d", i+1))
		}
		_ = line("    }")
	}
	for _, c := range d.Constraints {
		if c.Kind != "f" {
			continue
		}
		from, ok1 := ids[c.Schema+"\x00"+c.Table]
		to, ok2 := ids[c.RefSchema+"\x00"+c.RefTable]
		if ok1 && ok2 {
			_ = line("    " + to + " ||--o{ " + from + " : FK_confirmada")
		}
	}
	_ = line("```")
	_ = line("Colunas do ERD (`cN` segue a ordem de coleta):")
	for _, t := range d.Tables {
		for i, c := range t.Columns {
			_ = line(fmt.Sprintf("- %s.c%d = %s (%s)", ids[t.Schema+"\x00"+t.Name], i+1, md(c.Name), md(c.Type)))
		}
	}
	_ = line("| Constraint | Tipo | Origem e colunas na ordem | Destino e colunas na ordem | Índice de suporte | Evidência |")
	_ = line("| --- | --- | --- | --- | --- | --- |")
	for _, c := range d.Constraints {
		dst := "—"
		if c.Kind == "f" {
			dst = d.Database + "." + c.RefSchema + "." + c.RefTable + " (" + strings.Join(c.RefColumns, ", ") + ")"
		}
		_ = line("| " + md(c.Name) + " | " + md(c.Kind) + " | " + md(d.Database+"."+c.Schema+"."+c.Table+" ("+strings.Join(c.Columns, ", ")+")") + " | " + md(dst) + " | " + md(c.SupportingIndex) + " | constraint_snapshot:" + md(c.ID) + " |")
	}
	_ = line("\n## 6. UML observado")
	_ = line("```mermaid\nclassDiagram")
	for _, t := range d.Tables {
		_ = line("    class " + ids[t.Schema+"\x00"+t.Name])
	}
	for _, c := range d.Constraints {
		if c.Kind != "f" {
			continue
		}
		from, ok1 := ids[c.Schema+"\x00"+c.Table]
		to, ok2 := ids[c.RefSchema+"\x00"+c.RefTable]
		if ok1 && ok2 {
			_ = line("    " + from + " --> " + to + " : FK_confirmada")
		}
	}
	_ = line("```")
	_ = line("As associações UML representam somente FKs observadas. Nenhum serviço de ingestão foi inferido.")
	_ = line("\n## 7. Dados antigos e atividade")
	byObject := make(map[string]*InsertionEvidence, len(d.Insertions))
	for i := range d.Insertions {
		byObject[d.Insertions[i].ObjectID] = &d.Insertions[i]
	}
	_ = line("Corte de seis meses calendários (UTC): " + sixMonthsBefore(d.RunStarted).Format(time.RFC3339) + ". A conclusão exige contrato de preenchimento e evidência de leitura da mesma execução.")
	_ = line("| Tabela | Estado da última inserção | Última data comprovada | Evidência ou motivo |")
	_ = line("| --- | --- | --- | --- |")
	bySchema := map[string][]InsertionState{}
	allStates := make([]InsertionState, 0, len(d.Tables))
	for _, t := range d.Tables {
		e := byObject[t.ID]
		state := ClassifyInsertion(d.RunStarted, e)
		date, reason := "—", "Sem timestamp de inserção e contrato validados nesta execução"
		if e != nil {
			reason = e.Status + ": " + e.Reason
			if e.Latest != nil {
				date = e.Latest.UTC().Format(time.RFC3339)
				if e.Latest.After(d.RunStarted) {
					reason += "; data posterior ao início da coleta, fora do corte da execução"
				}
				if !e.AppendOnly {
					reason += "; data entre registros presentes; histórico indeterminado por exclusões possíveis"
				}
			}
		}
		_ = line("| " + md(t.ID) + " | " + string(state) + " | " + date + " | " + md(reason) + " |")
		bySchema[t.Schema] = append(bySchema[t.Schema], state)
		allStates = append(allStates, state)
	}
	if d.Redaction == "strict" {
		_ = line("Estado da última inserção: **indeterminado** neste nível de redação.")
	} else if d.ScopeSchema != "" {
		_ = line("Recorte do esquema " + md(d.ScopeSchema) + ": **" + string(AggregateInsertion(allStates)) + "**. Este recorte não classifica o banco inteiro.")
	} else {
		_ = line("Banco: **" + string(AggregateInsertion(allStates)) + "**.")
	}
	schemaSet := make(map[string]bool, len(bySchema)+len(d.Schemas))
	for schema := range bySchema {
		schemaSet[schema] = true
	}
	for _, schema := range d.Schemas {
		schemaSet[schema] = true
	}
	schemas := make([]string, 0, len(schemaSet))
	for schema := range schemaSet {
		schemas = append(schemas, schema)
	}
	sort.Strings(schemas)
	for _, schema := range schemas {
		_ = line("- Esquema " + md(schema) + ": " + string(AggregateInsertion(bySchema[schema])))
	}
	_ = line("Contadores de DML e datas de evento não comprovam a última inserção. Esquema sem tabelas tem estado próprio, não 'antigo'.")
	_ = line("\n## 8. Melhorias priorizadas")
	equivalent := equivalentKeyConstraints(d.Constraints)
	if len(d.Findings) == 0 && len(equivalent) == 0 {
		_ = line("Nenhuma melhoria sustentada por achados neste recorte; cobertura incompleta impede concluir ausência de problemas.")
	}
	for _, pair := range equivalent {
		_ = line("### Revisar PK e UNIQUE equivalentes — " + md(d.Database+"."+pair[0].Schema+"."+pair[0].Table))
		_ = line("- Evidência: constraint_snapshot:" + md(pair[0].ID) + " e constraint_snapshot:" + md(pair[1].ID) + "; colunas na ordem: " + md(strings.Join(pair[0].Columns, ", ")) + ".")
		_ = line("- Índices de suporte observados: " + md(pair[0].SupportingIndex) + " e " + md(pair[1].SupportingIndex) + ". Equivalência de colunas não comprova que uma constraint pode ser removida.")
		_ = line("- Prioridade: revisão; espaço recuperável não estimado. Confirme dependências, `ON CONFLICT` e contratos externos antes de considerar remoção.")
	}
	for _, f := range d.Findings {
		plan := actions.For(f.Type)
		_ = line("### " + md(f.ID) + " — " + md(f.Title))
		_ = line("- Estado: observado; prioridade: " + md(f.Severity) + "; objeto: " + md(f.Database+"."+f.Schema+"."+f.Object))
		_ = line("- Evidência: finding_event:" + md(f.ID) + "; " + md(f.Evidence))
		_ = line("- Impacto: " + md(plan.Impact) + ". Benefício esperado: " + md(plan.Benefit))
		_ = line("- Risco: " + md(plan.Risk) + ". Pré-requisitos: " + md(plan.Prerequisites))
	}
	_ = line("\n## 9. ERD alvo e UML alvo")
	_ = line("O modelo observado acima é a base preservada. Nenhuma nova entidade ou relação física é proposta sem contrato de domínio confirmado. Cada achado demanda uma decisão antes de alterar o desenho.")
	_ = line("```mermaid\nerDiagram")
	for _, t := range d.Tables {
		_ = line("    " + ids[t.Schema+"\x00"+t.Name] + " {")
		_ = line("        string existente")
		_ = line("    }")
	}
	for _, c := range d.Constraints {
		if c.Kind != "f" {
			continue
		}
		from, ok1 := ids[c.Schema+"\x00"+c.Table]
		to, ok2 := ids[c.RefSchema+"\x00"+c.RefTable]
		if ok1 && ok2 {
			_ = line("    " + to + " ||--o{ " + from + " : FK_confirmada")
		}
	}
	_ = line("```\n```mermaid\nclassDiagram")
	for _, t := range d.Tables {
		_ = line("    class " + ids[t.Schema+"\x00"+t.Name])
	}
	_ = line("```")
	_ = line("Identidades `Tn`: ")
	for _, t := range d.Tables {
		_ = line("- " + ids[t.Schema+"\x00"+t.Name] + " = " + md(t.ID))
	}
	_ = line("Responsabilidades alvo condicionadas por decisões de domínio:")
	roles := 0
	for _, decision := range d.Decisions {
		if decision.Subject == "semantics" {
			roles++
			_ = line("- " + md(decision.ObjectID) + ": " + md(decision.Value) + " (decisão versão " + fmt.Sprint(decision.Version) + "; proposta de interpretação, sem nova FK)")
		}
	}
	if roles == 0 {
		_ = line("- Nenhuma semântica confirmada; modelo alvo permanece estruturalmente igual ao observado.")
	}
	_ = line("\n## 10. Guia para responsáveis")
	for _, pair := range equivalent {
		_ = line("### Revisão de constraints " + md(pair[0].ID) + " / " + md(pair[1].ID))
		_ = line("- Entenda que duas constraints parecem exigir a mesma chave. Confirme índices patrocinadores, nomes usados pela aplicação e versões do TimescaleDB.")
		_ = line("- Teste qualquer mudança num fork fora do DB Auditor. DBA e dono da aplicação aprovam; valide unicidade, inserção idempotente, planos e retorno externo.")
		_ = line("- Não remova automaticamente uma constraint por equivalência de colunas.")
	}
	for _, f := range d.Findings {
		plan := actions.For(f.Type)
		_ = line("### " + md(f.ID))
		for _, s := range []string{plan.Meaning, plan.Confirmation, "Teste a mudança em fork ou restauração, fora do DB Auditor.", "Peça aprovação ao DBA e ao dono da aplicação; retenção exige o dono dos dados.", "Aplique externamente por migração versionada somente após aprovação.", plan.Validation + " Use o plano de retorno externo se falhar.", plan.FalsePositive + " Não aja só por nome de esquema, zero scans, idade do evento ou índice aparentemente sem uso."} {
			_ = line("- " + s)
		}
	}
	_ = line("\n## 11. Apêndice")
	_ = line("Decisões congeladas neste pedido: " + fmt.Sprint(len(d.Decisions)) + ". Consultas de confirmação devem ser somente leitura, aprovadas e executadas fora da geração do arquivo.")
	for _, decision := range d.Decisions {
		_ = line(fmt.Sprintf("- %s / %s: %s (versão %d, %s, %s)", md(decision.ObjectID), md(decision.Subject), md(decision.Value), decision.Version, md(decision.Author), decision.CreatedAt.UTC().Format(time.RFC3339)))
	}
	_ = line("Pendente: semântica de colunas, unidade, fuso, retenção, consumidor, contrato de inserção e classificação de esquemas geridos.")
	seenQueries := map[string]bool{}
	for _, f := range d.Findings {
		if seenQueries[f.Type] {
			continue
		}
		seenQueries[f.Type] = true
		plan := actions.For(f.Type)
		if plan.ReadOnlyQuery != "" {
			_ = line("Consulta de confirmação para " + md(f.Type) + " (somente leitura, parâmetros a validar):")
			_ = line("```sql")
			_ = line(plan.ReadOnlyQuery)
			_ = line("```")
		}
	}
	if writeErr != nil {
		return nil, writeErr
	}
	return b.Bytes(), nil
}

func md(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
	replacer := strings.NewReplacer("\\", "\\\\", "&", "&amp;", "|", "\\|", "<", "&lt;", ">", "&gt;", "`", "\\`", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "!", "\\!", "#", "\\#")
	return replacer.Replace(s)
}

func sixMonthsBefore(at time.Time) time.Time {
	u := at.UTC()
	first := time.Date(u.Year(), u.Month(), 1, u.Hour(), u.Minute(), u.Second(), u.Nanosecond(), time.UTC).AddDate(0, -6, 0)
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := u.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(first.Year(), first.Month(), day, u.Hour(), u.Minute(), u.Second(), u.Nanosecond(), time.UTC)
}

func equivalentKeyConstraints(constraints []ArchitectureConstraint) [][2]ArchitectureConstraint {
	pk := map[string]ArchitectureConstraint{}
	var pairs [][2]ArchitectureConstraint
	for _, c := range constraints {
		if c.Kind == "p" && len(c.Columns) > 0 {
			pk[c.Schema+"\x00"+c.Table+"\x00"+strings.Join(c.Columns, "\x00")] = c
		}
	}
	for _, c := range constraints {
		if c.Kind == "u" && len(c.Columns) > 0 {
			if p, ok := pk[c.Schema+"\x00"+c.Table+"\x00"+strings.Join(c.Columns, "\x00")]; ok {
				pairs = append(pairs, [2]ArchitectureConstraint{p, c})
			}
		}
	}
	return pairs
}
