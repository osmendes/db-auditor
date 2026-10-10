// Package actions turns a diagnostic into a reviewable, read-only action plan.
// The auditor never executes these steps against a target database.
package actions

import "strings"

// Plan is the versioned guidance for one rule. Impact stays "não estimado"
// unless a metric, a window and a denominator were actually collected.
type Plan struct {
	Category      string `json:"category"`
	Meaning       string `json:"meaning"`
	Evidence      string `json:"evidence"`
	Impact        string `json:"impact"`
	Next          string `json:"next"`
	Benefit       string `json:"expected_benefit"`
	Effort        string `json:"effort"`
	Risk          string `json:"risk"`
	Prerequisites string `json:"prerequisites"`
	Confirmation  string `json:"confirmation"`
	Validation    string `json:"validation"`
	FalsePositive string `json:"false_positive_risk"`
	ReadOnlyQuery string `json:"read_only_query,omitempty"`
}

const (
	structureQuery = "SELECT c.oid::regclass, c.relkind, c.reltuples::bigint FROM pg_catalog.pg_class c WHERE c.oid=$1::regclass;"
	indexQuery     = "SELECT i.indexrelid::regclass, i.indrelid::regclass, i.indisunique, i.indisvalid, pg_catalog.pg_get_indexdef(i.indexrelid) FROM pg_catalog.pg_index i WHERE i.indexrelid=$1::regclass OR i.indrelid=$1::regclass;"
	columnQuery    = "SELECT a.attname, a.atttypid::regtype, a.attnotnull FROM pg_catalog.pg_attribute a WHERE a.attrelid=$1::regclass AND a.attnum>0 AND NOT a.attisdropped ORDER BY a.attnum;"
	fkQuery        = "SELECT conname, conkey, confrelid::regclass, confkey, convalidated FROM pg_catalog.pg_constraint WHERE conrelid=$1::regclass AND contype='f';"
	sequenceQuery  = "SELECT s.seqrelid::regclass, s.seqtypid::regtype, s.seqstart, s.seqmin, s.seqmax, s.seqincrement FROM pg_catalog.pg_sequence s WHERE s.seqrelid=$1::regclass;"
	sizeQuery      = "SELECT c.oid::regclass,c.relkind,c.reltuples::bigint,pg_catalog.pg_total_relation_size(c.oid) AS bytes FROM pg_catalog.pg_class c WHERE c.oid=$1::regclass;"
	deadQuery      = "SELECT c.relname, s.n_live_tup, s.n_dead_tup FROM pg_catalog.pg_stat_user_tables s JOIN pg_catalog.pg_class c ON c.oid=s.relid WHERE c.oid=$1::regclass;"
	grantQuery     = "SELECT grantee, privilege_type, is_grantable FROM information_schema.role_table_grants WHERE table_schema=$1 AND table_name=$2;"
)

func base(category string) Plan {
	p := Plan{
		Category:      category,
		Meaning:       "A auditoria encontrou um sinal que precisa de análise humana.",
		Evidence:      "Use a evidência gravada na coleta. Sem métrica e denominador, o impacto não foi estimado.",
		Impact:        "não estimado",
		Next:          "Confira as evidências e valide o impacto com a equipe responsável antes de fazer alterações.",
		Benefit:       "Impacto ainda não estimado; meça a condição antes de planejar uma alteração.",
		Effort:        "médio",
		Risk:          "Uma mudança sem conhecer dependências e carga pode afetar a aplicação.",
		Prerequisites: "Confirme a cobertura da coleta, dependências e janela de manutenção.",
		Confirmation:  "Compare o achado com uma nova coleta e com a equipe responsável pelo objeto.",
		Validation:    "Registre as métricas anteriores e posteriores e repita a auditoria.",
		FalsePositive: "Uma observação isolada ou uma coleta parcial pode não representar o uso normal.",
	}
	switch category {
	case "structure":
		p.Meaning = "A estrutura do objeto pode impedir identificação, vínculo ou validação dos registros."
		p.Next = "Confira o catálogo e as dependências com o dono da aplicação antes de alterar chaves ou constraints."
		p.Confirmation = "Inspecione as dependências e o catálogo do objeto; confirme com o dono da aplicação antes de alterar chaves, constraints ou índices."
		p.Validation = "Após correção externa, confira o catálogo e compare uma nova coleta com a anterior."
		p.FalsePositive = "Ausência de uso observado não comprova que um objeto está órfão ou pode ser removido."
		p.ReadOnlyQuery = structureQuery
		p.Effort = "alto"
	case "query_and_index":
		p.Meaning = "O uso ou a ausência de um índice pode mudar o custo de leitura ou de escrita."
		p.Next = "Compare estatísticas e planos somente leitura antes de criar ou remover um índice."
		p.Prerequisites = "Confira estatísticas, taxa de escrita, espaço e plano da consulta em ambiente seguro."
		p.Confirmation = "Compare EXPLAIN (sem executar alterações) e o fingerprint da consulta; confirme cobertura e período das estatísticas."
		p.Validation = "Meça latência, leituras, escrita e espaço antes/depois sob carga comparável; documente rollback."
		p.Risk = "Criar ou remover índice pode aumentar escrita, ocupar espaço ou causar bloqueios; planeje execução concorrente quando aplicável."
		p.FalsePositive = "Contadores zerados após reset ou uma janela curta não provam inutilidade de índice."
		p.ReadOnlyQuery = indexQuery
	case "maintenance":
		p.Meaning = "Manutenção, retenção ou crescimento só se justificam com tendência de coletas completas."
		p.Next = "Compare ao menos duas coletas completas. Sem tendência, o ganho não foi estimado."
		p.Prerequisites = "Confirme tendência, volume, custo operacional, SLA e janela aprovada. Retenção depende de decisão de negócio."
		p.Confirmation = "Compare duas ou mais coletas completas e verifique a configuração e as estatísticas do objeto."
		p.Validation = "Após ação externa, compare tamanho, atraso e saúde dos jobs em novas coletas."
		p.Risk = "Manutenção e retenção podem consumir recursos ou remover dados; exija backup e plano de recuperação."
		p.FalsePositive = "Um único pico ou coleta parcial não estabelece tendência de crescimento."
	case "security":
		p.Meaning = "Uma permissão ou execução privilegiada merece revisão antes de qualquer revogação."
		p.Next = "Confirme o papel da aplicação e a permissão efetiva. Não revogue sem rollback."
		p.Prerequisites = "Confirme o papel da aplicação, a permissão efetiva e a política de acesso antes de revogar privilégios."
		p.Confirmation = "Revise grants, políticas e dependências com o responsável pela aplicação."
		p.Validation = "Teste o fluxo autorizado e repita a coleta para verificar a permissão efetiva."
		p.Risk = "Uma revogação pode interromper um serviço legítimo; mudanças devem ter rollback externo."
		p.FalsePositive = "Permissão aparentemente ampla pode ser necessária a uma função operacional conhecida."
		p.ReadOnlyQuery = grantQuery
		p.Effort = "alto"
	}
	return p
}

func narrate(p Plan, meaning, evidence, impact, next string) Plan {
	p.Meaning, p.Evidence, p.Impact, p.Next = meaning, evidence, impact, next
	return p
}

func withQuery(p Plan, query string) Plan {
	p.ReadOnlyQuery = query
	return p
}

func storagePlan() Plan {
	p := narrate(withQuery(base("maintenance"), sizeQuery),
		"A relação é grande o bastante para merecer tendência, não uma ação imediata.",
		"Tamanho vem de pg_total_relation_size. Crescimento exige duas coletas completas comparáveis.",
		"não estimado com uma única coleta ou com coleta parcial.",
		"Compare o crescimento, a chave natural e a retenção aprovada. Não proponha DROP nem particionamento automático.")
	p.Confirmation = "Compare crescimento em coletas completas e confirme a retenção com a área de negócio antes de qualquer mudança externa."
	p.Prerequisites = "Estime custo de migração, índices e espaço temporário; obtenha backup restaurável, janela e plano de retorno."
	p.Validation = "Após mudança externa, compare volume, latência e cobertura em novas coletas completas."
	return p
}

// rulePlans is the only match table. Unknown ids use the investigation fallback.
var rulePlans = map[string]Plan{}

func init() {
	put := func(id string, p Plan) { rulePlans[id] = p }
	structure := func(id, query string) { put(id, withQuery(base("structure"), query)) }
	index := func(id string) { put(id, withQuery(base("query_and_index"), indexQuery)) }
	maint := func(id string) { put(id, base("maintenance")) }
	sec := func(id string) { put(id, base("security")) }

	structure("integrity.missing_primary_key", "SELECT conname, contype, convalidated FROM pg_catalog.pg_constraint WHERE conrelid = $1::regclass AND contype = 'p';")
	structure("integrity.constraint_unvalidated", "SELECT conname, contype, convalidated FROM pg_catalog.pg_constraint WHERE conrelid = $1::regclass AND NOT convalidated;")
	structure("integrity.fk_without_index", fkQuery)
	structure("integrity.fk_type_mismatch", fkQuery)
	structure("integrity.orphan_sequence", "SELECT d.deptype, d.refobjid::regclass FROM pg_catalog.pg_depend d WHERE d.objid = $1::regclass;")
	structure("integrity.sequence_default_mismatch", sequenceQuery)
	structure("sequence.near_limit", sequenceQuery)
	structure("index.invalid", "SELECT indexrelid::regclass, indisvalid, indisready FROM pg_catalog.pg_index WHERE indexrelid = $1::regclass;")
	structure("model.naming_inconsistent", "SELECT n.nspname, c.relname, c.relkind FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=$1::regclass;")
	structure("model.implicit_relationship", fkQuery)
	for _, id := range []string{"model.wide_table", "model.repeated_columns", "model.duplicate_entity", "model.type_review", "model.jsonb_critical", "model.undocumented_critical"} {
		structure(id, columnQuery)
	}
	put("inactivity.possibly_inactive", base("structure"))

	for _, id := range []string{"index.unused", "index.overlap", "index.prefix_overlap", "index.write_burden"} {
		index(id)
	}
	for _, id := range []string{"index.investigate_missing", "index.candidate", "performance.lock_wait", "performance.high_connections", "performance.slow_query", "performance.workload_scan", "performance.workload_write", "performance.workload_cost"} {
		put(id, base("query_and_index"))
	}

	put("storage.large_table", storagePlan())
	put("storage.top_consumer", withQuery(base("maintenance"), sizeQuery))
	put("vacuum.high_dead_tuples", withQuery(base("maintenance"), deadQuery))
	put("maintenance.dead_tuple_pressure", withQuery(base("maintenance"), deadQuery))
	put("timescale.chunk_dead_tuples", withQuery(base("maintenance"), deadQuery))
	for _, id := range []string{"maintenance.stale_analyze", "chunk.high_count", "chunk.size_skew", "chunk.inventory_truncated", "cagg.missing_refresh_policy", "cagg.overlap", "cagg.materialization_lag", "cagg.realtime_hypothesis", "cagg.refresh_window_exceeded", "policy.missing_retention", "policy.missing_compression", "policy.job_failed", "policy.retention_chunk_mismatch", "policy.reorder_hypothesis", "policy.compression_not_applied", "policy.compression_ratio", "policy.compression_settings", "job.unhealthy", "job.slo_exceeded", "job.workers_saturated", "replication.lag_high", "replication.archive_stalled"} {
		maint(id)
	}
	for _, id := range []string{"security.excessive_privilege", "security.powerful_role", "security.security_definer", "security.auditor_not_readonly", "security.auditor_privilege_unknown", "security.definer_search_path", "security.rls_disabled_hypothesis", "security.public_schema_create", "security.account_inactive_review", "config.version_drift", "config.extension_drift", "config.guc_drift"} {
		sec(id)
	}

	put("integrity.missing_primary_key", narrate(rulePlans["integrity.missing_primary_key"],
		"Esta relação não tem chave primária observada na coleta.",
		"A consulta lista constraints primárias. Zero linhas confirma a ausência nesta relação.",
		"não estimado: ausência de chave não tem tamanho nem latência nesta coleta.",
		"Valide duplicatas e o identificador da aplicação antes de propor uma chave."))
	put("index.unused", narrate(rulePlans["index.unused"],
		"O índice não teve uso observado na janela das estatísticas.",
		"A consulta mostra definição e validade. O uso vem de idx_scan da coleta.",
		"não estimado sem bytes do índice e sem a janela de idx_scan.",
		"Confirme reset de estatísticas e consultas raras antes de considerar remoção."))
	put("vacuum.high_dead_tuples", narrate(rulePlans["vacuum.high_dead_tuples"],
		"A relação acumulou tuplas mortas acima do limiar da regra.",
		"n_dead_tup e n_live_tup da coleta. Estimativa não é bloat medido.",
		"não estimado sem n_dead_tup, n_live_tup e a janela entre coletas.",
		"Confira autovacuum e a carga de escrita. Não agende VACUUM a partir daqui."))
	put("security.excessive_privilege", narrate(rulePlans["security.excessive_privilege"],
		"Um papel tem privilégio mais amplo do que o uso conhecido da aplicação.",
		"Grants efetivos gravados na coleta. A API não executa SQL no alvo.",
		"não estimado: exposição não vira nota sem o privilégio e o papel.",
		"Confirme se o privilégio é operacional antes de qualquer revogação."))
}

// For returns the plan registered for rule. Anything else is the single investigation fallback.
func For(rule string) Plan {
	if p, ok := rulePlans[rule]; ok {
		return p
	}
	return base("investigation")
}

// QueryIsReadOnly rejects any statement that is not a single parameterized SELECT.
func QueryIsReadOnly(query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return true
	}
	upper := strings.ToUpper(q)
	if !strings.HasPrefix(upper, "SELECT ") {
		return false
	}
	for _, banned := range []string{"INSERT ", "UPDATE ", "DELETE ", "DROP ", "ALTER ", "VACUUM", "REVOKE ", "EXPLAIN ANALYZE", "TRUNCATE "} {
		if strings.Contains(upper, banned) {
			return false
		}
	}
	return strings.Count(q, ";") == 1 && strings.HasSuffix(q, ";")
}
