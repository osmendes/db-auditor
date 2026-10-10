package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// RuleDefinition is immutable for a given (ID, Version). A future change in
// thresholds or interpretation must create a new version, not rewrite history.
type RuleDefinition struct {
	ID                string         `json:"rule_id"`
	Version           string         `json:"rule_version"`
	Category          string         `json:"category"`
	Confidence        float64        `json:"confidence"`
	Impact            string         `json:"impact"`
	Risk              string         `json:"risk"`
	Recommendation    string         `json:"recommendation"`
	Validation        string         `json:"validation"`
	References        []string       `json:"references"`
	DefaultParameters map[string]any `json:"default_parameters"`
}

// RulePolicy is an environment or schema override loaded from the internal
// store. Schema-specific policies take precedence over environment defaults.
type RulePolicy struct {
	EnvironmentID string         `json:"environment_id"`
	SchemaName    string         `json:"schema_name"`
	RuleID        string         `json:"rule_id"`
	Enabled       bool           `json:"enabled"`
	Parameters    map[string]any `json:"parameters"`
}

var ruleIDs = []string{
	"storage.large_table", "storage.top_consumer", "index.unused", "index.overlap",
	"chunk.high_count", "chunk.size_skew", "cagg.missing_refresh_policy", "cagg.overlap",
	"policy.missing_retention", "policy.missing_compression", "policy.job_failed", "job.unhealthy",
	"inactivity.possibly_inactive", "vacuum.high_dead_tuples", "performance.lock_wait",
	"performance.high_connections", "performance.slow_query", "performance.workload_scan",
	"performance.workload_write", "performance.workload_cost", "security.excessive_privilege",
	"security.powerful_role", "security.security_definer", "integrity.missing_primary_key",
	"integrity.fk_without_index", "integrity.constraint_unvalidated", "integrity.fk_type_mismatch",
	"integrity.orphan_sequence", "integrity.sequence_default_mismatch", "index.prefix_overlap", "index.invalid", "index.write_burden",
	"index.investigate_missing", "maintenance.stale_analyze", "maintenance.dead_tuple_pressure", "maintenance.growth_trend",
	"model.wide_table", "model.repeated_columns", "model.duplicate_entity",
	"model.implicit_relationship", "model.naming_inconsistent", "model.undocumented_critical",
	"model.type_review", "model.jsonb_critical",
	"chunk.inventory_truncated", "policy.retention_chunk_mismatch", "policy.reorder_hypothesis",
	"policy.compression_not_applied", "policy.compression_ratio", "policy.compression_settings", "job.slo_exceeded", "job.workers_saturated",
	"cagg.materialization_lag", "cagg.realtime_hypothesis", "cagg.refresh_window_exceeded", "security.auditor_not_readonly",
	"security.auditor_privilege_unknown", "config.version_drift", "config.extension_drift", "config.guc_drift",
	"index.candidate", "sequence.near_limit", "security.definer_search_path",
	"security.rls_disabled_hypothesis", "security.public_schema_create", "security.account_inactive_review",
	"replication.lag_high", "replication.archive_stalled", "timescale.chunk_dead_tuples",
}

// Catalog is the versioned, golden-tested source of rule metadata.
func Catalog() []RuleDefinition {
	out := make([]RuleDefinition, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		category := strings.SplitN(id, ".", 2)[0]
		confidence := 0.90
		risk := "baixo"
		if category == "model" || id == "index.prefix_overlap" || id == "index.investigate_missing" || id == "integrity.fk_type_mismatch" {
			confidence = 0.45
			risk = "hipótese"
		}
		if category == "security" {
			confidence = 0.80
			risk = "revisão de segurança"
		}
		impact := "Possível impacto operacional ou estrutural"
		switch category {
		case "integrity":
			impact = "Possível risco para vínculos ou identificação de registros"
		case "index", "performance":
			impact = "Possível lentidão de consultas ou custo adicional de escrita"
		case "maintenance", "vacuum":
			impact = "Possível aumento de espaço ou perda de qualidade das estatísticas"
		case "security":
			impact = "Possível exposição por permissões ou execução privilegiada"
		case "model":
			impact = "Hipótese de dificuldade de manutenção do modelo"
		}
		parameters := map[string]any{}
		switch id {
		case "model.wide_table":
			parameters["min_columns"] = 50
		case "model.undocumented_critical":
			parameters["min_bytes"] = float64(1 << 30)
		case "index.write_burden":
			parameters["min_indexes"] = 8
			parameters["min_writes"] = 100000
		case "maintenance.stale_analyze":
			parameters["max_age_days"] = 30
		case "maintenance.dead_tuple_pressure":
			parameters["min_dead_tuples"] = 10000
			parameters["min_dead_ratio"] = 0.2
		case "index.investigate_missing":
			parameters["min_calls"] = 100
			parameters["min_shared_reads"] = 1000
		case "model.type_review":
			parameters["check_money"] = true
			parameters["check_timestamp_without_timezone"] = true
			parameters["check_status_text"] = true
		case "model.naming_inconsistent":
			parameters["convention"] = "snake_case"
		case "config.guc_drift":
			parameters["numeric_tolerance"] = 0.0
		case "sequence.near_limit":
			parameters["sequence_near_limit_ratio"] = 0.70
			parameters["sequence_integer_high_ratio"] = 0.80
		}
		version := "1.0.0"
		switch id {
		case "storage.large_table", "policy.compression_not_applied", "policy.compression_ratio", "policy.compression_settings", "policy.retention_chunk_mismatch",
			"policy.reorder_hypothesis", "job.slo_exceeded", "job.workers_saturated",
			"cagg.materialization_lag", "cagg.realtime_hypothesis", "cagg.refresh_window_exceeded", "chunk.inventory_truncated",
			"security.auditor_not_readonly", "security.auditor_privilege_unknown",
			"config.version_drift", "config.extension_drift", "config.guc_drift",
			"index.candidate", "sequence.near_limit", "security.definer_search_path",
			"security.rls_disabled_hypothesis", "security.public_schema_create",
			"replication.lag_high", "replication.archive_stalled", "timescale.chunk_dead_tuples":
			version = "1.1.0"
		}
		if id == "policy.reorder_hypothesis" || id == "cagg.realtime_hypothesis" || id == "policy.compression_settings" || id == "index.candidate" || id == "security.rls_disabled_hypothesis" {
			confidence = 0.4
			risk = "hipótese"
		}
		out = append(out, RuleDefinition{
			ID: id, Version: version, Category: category, Confidence: confidence,
			Impact: impact, Risk: risk,
			Recommendation: ruleRecommendation(id),
			Validation:     ruleValidation(id),
			References:     ruleReferences(category), DefaultParameters: parameters,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ruleRecommendation(id string) string {
	switch id {
	case "integrity.missing_primary_key":
		return "Confirme como os registros são identificados e as dependências; avalie uma chave primária somente após validar os dados."
	case "integrity.fk_without_index":
		return "Revise planos das consultas e custo de escrita da tabela filha antes de considerar um índice para a relação."
	case "integrity.constraint_unvalidated":
		return "Investigue registros incompatíveis e planeje a validação da restrição após uma correção controlada."
	case "integrity.fk_type_mismatch":
		return "Revise conversões e o significado da relação com a equipe responsável; não presuma o tipo de negócio correto."
	case "integrity.orphan_sequence", "integrity.sequence_default_mismatch":
		return "Confira chamadas da aplicação e valores padrão antes de vincular ou aposentar a sequência."
	case "index.overlap", "index.prefix_overlap":
		return "Compare filtros, colunas incluídas, unicidade, uso e planos; não remova índices automaticamente."
	case "index.invalid":
		return "Investigue a falha de criação do índice e planeje uma reconstrução segura se necessária."
	case "index.write_burden":
		return "Meça latência de escrita e uso dos índices antes de alterar o conjunto."
	case "index.investigate_missing":
		return "Examine planos de execução representativos; o fingerprint não identifica as colunas do índice."
	case "maintenance.stale_analyze", "maintenance.dead_tuple_pressure":
		return "Confira autovacuum e atividade da tabela; confirme as estimativas em diagnóstico controlado."
	case "model.wide_table", "model.repeated_columns", "model.duplicate_entity", "model.implicit_relationship", "model.jsonb_critical":
		return "Trate como hipótese de modelagem; valide acesso e limites do domínio com os responsáveis."
	case "model.type_review":
		return "Discuta o significado do tipo com a equipe de domínio antes de propor migração."
	case "model.naming_inconsistent":
		return "Revise a convenção de nomes; planeje a compatibilidade antes de renomear."
	case "model.undocumented_critical":
		return "Documente proprietário, finalidade e retenção no catálogo após revisão do responsável."
	case "performance.workload_scan", "performance.workload_write", "performance.workload_cost":
		return "Revise o fingerprint e o plano em ambiente autorizado; o SQL completo não é armazenado."
	case "security.account_inactive_review":
		return "Confirme validade, uso entre coletas e responsável pela conta antes de planejar desativação externa."
	case "storage.large_table":
		return "Confirme crescimento e acesso por período; avalie particionamento, retenção ou compressão somente após medir custos e obter aprovação do negócio."
	default:
		return "Investigue a evidência e confirme a necessidade de negócio antes de alterar estrutura ou configuração."
	}
}

func ruleValidation(id string) string {
	switch {
	case strings.HasPrefix(id, "integrity."):
		return "Confirme o catálogo e verifique relações afetadas em ambiente controlado."
	case strings.HasPrefix(id, "index."), strings.HasPrefix(id, "performance."):
		return "Compare planos de execução e métricas em uma janela de observação suficiente."
	case strings.HasPrefix(id, "model."):
		return "Revise com a equipe de domínio; a regra isolada não comprova um defeito de desenho."
	default:
		return "Confirme com o catálogo, carga representativa e teste controlado fora da produção."
	}
}

func ruleReferences(category string) []string {
	switch category {
	case "integrity", "model":
		return []string{"https://www.postgresql.org/docs/current/ddl-constraints.html"}
	case "index":
		return []string{"https://www.postgresql.org/docs/current/indexes.html"}
	case "maintenance", "vacuum":
		return []string{"https://www.postgresql.org/docs/current/routine-vacuuming.html"}
	case "performance":
		return []string{"https://www.postgresql.org/docs/current/pgstatstatements.html"}
	case "security":
		return []string{"https://www.postgresql.org/docs/current/user-manag.html"}
	default:
		return []string{"https://www.postgresql.org/docs/current/monitoring-stats.html"}
	}
}

func definitionByID(id string) (RuleDefinition, bool) {
	for _, rule := range Catalog() {
		if rule.ID == id {
			return rule, true
		}
	}
	return RuleDefinition{}, false
}

func effectivePolicy(f SnapshotFacts, schema, id string) (bool, map[string]any) {
	def, ok := definitionByID(id)
	if !ok {
		return true, map[string]any{}
	}
	params := make(map[string]any, len(def.DefaultParameters))
	for k, v := range def.DefaultParameters {
		params[k] = v
	}
	enabled := true
	for _, scope := range []string{"", schema} {
		for _, p := range f.RulePolicies {
			if p.RuleID != id || p.SchemaName != scope || p.EnvironmentID != f.EnvironmentID {
				continue
			}
			enabled = p.Enabled
			for k, v := range p.Parameters {
				if _, allowed := def.DefaultParameters[k]; allowed {
					params[k] = v
				}
			}
		}
	}
	return enabled, params
}

func threshold(f SnapshotFacts, schema, id, name string, fallback float64) float64 {
	_, params := effectivePolicy(f, schema, id)
	if v, ok := params[name].(float64); ok && v > 0 {
		return v
	}
	return fallback
}

func ruleFlag(f SnapshotFacts, schema, id, name string, fallback bool) bool {
	_, params := effectivePolicy(f, schema, id)
	if value, ok := params[name].(bool); ok {
		return value
	}
	return fallback
}

func namingConvention(f SnapshotFacts, schema string) string {
	_, params := effectivePolicy(f, schema, "model.naming_inconsistent")
	if value, ok := params["convention"].(string); ok {
		return value
	}
	return "snake_case"
}

// EnrichFindings applies effective policies and freezes rule metadata on each
// finding, so later catalog updates cannot silently mutate historical rows.
func EnrichFindings(f SnapshotFacts, items []Finding) []Finding {
	out := make([]Finding, 0, len(items))
	for _, item := range items {
		def, ok := definitionByID(item.FindingType)
		if !ok {
			category := strings.SplitN(item.FindingType, ".", 2)[0]
			def = RuleDefinition{ID: item.FindingType, Version: "1.0.0", Category: category, Confidence: 0.5,
				Impact: "Requer análise de uma pessoa responsável", Risk: "não classificado", Recommendation: "Confira as evidências antes de fazer alterações.",
				Validation: "Confirme em ambiente controlado.", References: ruleReferences(category), DefaultParameters: map[string]any{}}
		}
		enabled, params := effectivePolicy(f, item.SchemaName, def.ID)
		if !enabled {
			continue
		}
		item.RuleID = def.ID
		item.RuleVersion = def.Version
		item.Category = def.Category
		item.Confidence = def.Confidence
		item.Impact = def.Impact
		item.Risk = def.Risk
		item.Recommendation = def.Recommendation
		item.Validation = def.Validation
		item.References = append([]string(nil), def.References...)
		item.RuleParameters = params
		fingerprint, _ := json.Marshal(params)
		item.DedupKey = DedupKey(item.FindingType+"@"+def.Version, item.ObjectKey, item.Title+string(fingerprint))
		out = append(out, item)
	}
	return out
}

type EffectiveRule struct {
	RuleDefinition
	Enabled             bool           `json:"enabled"`
	EffectiveParameters map[string]any `json:"effective_parameters"`
}

// RuleManifestHash identifies the exact rule metadata bundled with this build.
// encoding/json emits map keys in stable order and Catalog returns sorted rules.
func RuleManifestHash() string {
	payload, _ := json.Marshal(Catalog())
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func EffectiveCatalog(f SnapshotFacts, schema string) []EffectiveRule {
	definitions := Catalog()
	out := make([]EffectiveRule, 0, len(definitions))
	for _, d := range definitions {
		enabled, parameters := effectivePolicy(f, schema, d.ID)
		out = append(out, EffectiveRule{RuleDefinition: d, Enabled: enabled, EffectiveParameters: parameters})
	}
	return out
}
