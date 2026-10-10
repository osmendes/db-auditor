// Package guidance provides user-facing language without changing the
// versioned technical evidence stored by the analyzer.
package guidance

import "strings"

type Text struct {
	Meaning string
	Next    string
}

var portuguese = map[string]Text{
	"storage.large_table":                 {"Esta tabela ocupa muito espaço.", "Compare o crescimento, o uso e a política de retenção antes de alterar o armazenamento."},
	"storage.top_consumer":                {"Este objeto está entre os maiores consumidores de espaço.", "Acompanhe sua evolução e confirme quais dados precisam permanecer."},
	"index.unused":                        {"Este índice não teve uso observado no período disponível.", "Confira a janela das estatísticas e as consultas antes de considerar sua remoção."},
	"index.overlap":                       {"Dois índices podem atender consultas semelhantes.", "Compare colunas, filtros, unicidade, uso e planos antes de alterar os índices."},
	"chunk.high_count":                    {"A hypertable possui muitos chunks.", "Revise intervalo dos chunks e consultas típicas com a equipe responsável."},
	"chunk.size_skew":                     {"Os chunks têm tamanhos muito diferentes.", "Confirme o padrão de ingestão e a distribuição por período."},
	"cagg.missing_refresh_policy":         {"O agregado contínuo pode estar sem atualização automática.", "Confirme a frequência esperada e a política de atualização."},
	"cagg.overlap":                        {"Políticas de atualização podem trabalhar sobre períodos sobrepostos.", "Revise janelas e custo dos jobs antes de ajustá-las."},
	"policy.missing_retention":            {"Não foi identificada política automática de retenção.", "Confirme a obrigação de guardar dados e o prazo com a área responsável."},
	"policy.missing_compression":          {"Não foi identificada política de compressão.", "Avalie idade dos dados, leitura histórica e benefício estimado antes de configurar compressão."},
	"policy.job_failed":                   {"Um job de política apresentou falha.", "Leia o erro e confirme se a política voltou a executar."},
	"job.unhealthy":                       {"Um job pode não estar executando como esperado.", "Confira histórico, agenda e erros do job."},
	"inactivity.possibly_inactive":        {"Este objeto pode estar sem uso recente.", "Confirme com a aplicação e seus responsáveis antes de qualquer remoção."},
	"vacuum.high_dead_tuples":             {"Há muitos registros antigos aguardando limpeza.", "Revise autovacuum, escrita recente e estatísticas da tabela."},
	"performance.lock_wait":               {"Sessões esperaram por bloqueios.", "Identifique transações e horários envolvidos antes de mudar consultas ou parâmetros."},
	"performance.high_connections":        {"O número de conexões se aproximou de um nível de atenção.", "Verifique pool de conexões e picos de uso."},
	"performance.slow_query":              {"Uma consulta apresentou tempo elevado.", "Compare plano de execução e métricas em janela representativa."},
	"performance.workload_scan":           {"Uma consulta pode fazer leituras amplas.", "Inspecione um plano representativo e os filtros usados pela aplicação."},
	"performance.workload_write":          {"Uma carga de escrita pode ter custo elevado.", "Avalie latência, índices e volume de alterações antes de agir."},
	"performance.workload_cost":           {"Uma consulta concentra custo de execução.", "Valide seu plano e frequência em ambiente autorizado."},
	"security.excessive_privilege":        {"Uma conta pode ter mais permissões do que precisa.", "Confirme as funções da conta antes de solicitar redução de privilégios."},
	"security.powerful_role":              {"Um papel possui privilégios elevados.", "Revise quem utiliza esse papel e justifique cada privilégio."},
	"security.security_definer":           {"Uma função executa com privilégios de seu proprietário.", "Revise código, proprietário e permissões de execução da função."},
	"integrity.missing_primary_key":       {"Esta tabela não tem chave primária identificada.", "Confirme como os registros são identificados e valide os dados antes de criar uma chave."},
	"integrity.fk_without_index":          {"Uma relação entre tabelas pode estar sem índice útil.", "Confira consultas e planos de execução antes de avaliar um novo índice."},
	"integrity.constraint_unvalidated":    {"Uma restrição ainda não foi validada para todos os dados.", "Investigue registros incompatíveis e planeje validação controlada."},
	"integrity.fk_type_mismatch":          {"Colunas relacionadas usam tipos que merecem revisão.", "Confirme conversões e semântica com quem conhece os dados."},
	"integrity.orphan_sequence":           {"Uma sequência pode não ter vínculo claro com uma coluna.", "Verifique chamadas da aplicação e valores padrão antes de alterá-la."},
	"integrity.sequence_default_mismatch": {"O valor padrão de uma coluna pode apontar para outra sequência.", "Confira dependências e uso da aplicação antes de corrigir o vínculo."},
	"index.prefix_overlap":                {"Um índice pode estar coberto pelo início de outro.", "Compare filtros, colunas incluídas, unicidade, uso e planos."},
	"index.invalid":                       {"Este índice está marcado como inválido.", "Investigue a causa e planeje reconstrução segura, se necessária."},
	"index.write_burden":                  {"Muitos índices podem aumentar o custo de escrita.", "Meça latência de escrita e uso dos índices antes de mudar o conjunto."},
	"index.investigate_missing":           {"Uma consulta pode se beneficiar de índice ainda não identificado.", "Analise o plano; o fingerprint sozinho não determina as colunas do índice."},
	"maintenance.stale_analyze":           {"As estatísticas da tabela podem estar antigas.", "Revise atividade, autovacuum e necessidade de ANALYZE."},
	"maintenance.dead_tuple_pressure":     {"Registros antigos podem pressionar a manutenção da tabela.", "Confirme a tendência e a configuração de autovacuum."},
	"maintenance.growth_trend":            {"O tamanho ou a atividade mudou entre duas coletas completas.", "Trate o número como hipótese. Confirme período, backup e aprovação antes de qualquer manutenção."},
	"model.wide_table":                    {"Esta tabela possui muitas colunas.", "Converse com a equipe de domínio sobre padrões de leitura e limites do modelo."},
	"model.repeated_columns":              {"Há colunas com nomes ou funções aparentemente repetidos.", "Confirme a semântica antes de propor normalização."},
	"model.duplicate_entity":              {"Dois objetos podem representar a mesma entidade.", "Valide com responsáveis e aplicações antes de consolidar dados."},
	"model.implicit_relationship":         {"Uma relação entre tabelas pode não estar declarada.", "Confirme a regra de negócio antes de criar uma chave estrangeira."},
	"model.naming_inconsistent":           {"Nomes de objetos divergem da convenção adotada.", "Planeje compatibilidade com aplicações antes de renomear."},
	"model.undocumented_critical":         {"Um objeto importante pode estar sem documentação.", "Registre finalidade, proprietário e retenção após confirmar com a equipe."},
	"model.type_review":                   {"O tipo de uma coluna merece revisão de semântica.", "Discuta valores e regras de negócio antes de uma migração."},
	"model.jsonb_critical":                {"Dados essenciais podem estar concentrados em JSONB.", "Avalie consultas e contratos antes de mudar a estrutura."},
	"chunk.inventory_truncated":           {"A coleta de chunks atingiu um limite.", "Aumente a cobertura de forma controlada antes de concluir sobre todos os chunks."},
	"policy.retention_chunk_mismatch":     {"A retenção pode não combinar com o tamanho temporal dos chunks.", "Compare períodos, política e requisitos de guarda antes de ajustar."},
	"policy.reorder_hypothesis":           {"A ordenação de chunks pode merecer estudo.", "Confirme consultas e custo de manutenção antes de criar uma política."},
	"policy.compression_not_applied":      {"Chunks elegíveis podem ainda não estar comprimidos.", "Confira agenda e falhas dos jobs de compressão."},
	"policy.compression_ratio":            {"A compressão observada pode estar abaixo do esperado.", "Compare dados, segmentos e consultas antes de ajustar a política."},
	"policy.compression_settings":         {"A configuração de compressão pode não combinar com a carga.", "Teste segmentação e ordenação em ambiente controlado."},
	"job.slo_exceeded":                    {"Um job ultrapassou a janela de execução esperada.", "Compare duração, agenda e concorrência dos jobs."},
	"job.workers_saturated":               {"Os workers de jobs podem estar ocupados em excesso.", "Confira filas e limites antes de aumentar concorrência."},
	"cagg.materialization_lag":            {"O agregado contínuo pode estar desatualizado.", "Compare atraso, política de refresh e necessidade dos usuários."},
	"cagg.realtime_hypothesis":            {"A configuração de dados em tempo real merece validação.", "Confirme frescor necessário e custo das consultas."},
	"cagg.refresh_window_exceeded":        {"A atualização do agregado pode exceder sua janela.", "Revise volume, frequência e duração dos refreshes."},
	"security.auditor_not_readonly":       {"A conta de auditoria possui privilégio de escrita ou elevado.", "Peça ao administrador que use uma conta de somente leitura para a coleta."},
	"security.auditor_privilege_unknown":  {"Não foi possível confirmar as permissões da conta de auditoria.", "Repita a verificação; não presuma que a conta seja de somente leitura."},
	"config.version_drift":                {"Versões dos ambientes comparados são diferentes.", "Confirme se a diferença é planejada e revise compatibilidade."},
	"config.extension_drift":              {"Extensões diferem entre os ambientes.", "Confirme necessidade e versão antes de alinhar configurações."},
	"config.guc_drift":                    {"Um parâmetro de configuração difere da referência.", "Verifique se o ajuste é intencional e meça o impacto."},
	"index.candidate":                     {"Uma consulta pode justificar um novo índice.", "Confirme plano, seletividade e custo de escrita antes de criar o índice."},
	"sequence.near_limit":                 {"Uma sequência pode estar próxima do limite de valores.", "Projete o crescimento e planeje a ampliação antes de esgotar valores."},
	"security.definer_search_path":        {"Uma função privilegiada pode depender de um caminho de busca inseguro.", "Revise search_path e resolução de objetos com o administrador."},
	"security.rls_disabled_hypothesis":    {"Uma tabela pode exigir controle de acesso por linha.", "Confirme a política de acesso antes de habilitar RLS."},
	"security.public_schema_create":       {"Contas podem criar objetos no schema público.", "Confirme dependências antes de restringir CREATE."},
	"security.account_inactive_review":    {"Uma conta expirou ou não apareceu ativa nas amostras disponíveis.", "Confirme o uso real com a equipe antes de desativar a conta."},
	"replication.lag_high":                {"A réplica apresenta atraso elevado.", "Verifique origem, rede, aplicação do WAL e impacto esperado."},
	"replication.archive_stalled":         {"O arquivamento de WAL pode estar parado.", "Confira logs, destino e recuperação antes de alterar configuração."},
	"timescale.chunk_dead_tuples":         {"Um chunk tem muitos registros antigos.", "Revise autovacuum e atividade desse chunk."},
}

func For(ruleID string) Text {
	if text, ok := portuguese[ruleID]; ok {
		return text
	}
	switch strings.SplitN(ruleID, ".", 2)[0] {
	case "security":
		return Text{"Foi observado um ponto de atenção de segurança.", "Revise a evidência com o administrador antes de alterar permissões."}
	case "performance", "index":
		return Text{"Uma métrica sugere possível impacto de desempenho.", "Compare métricas e planos de consulta antes de ajustar o banco."}
	default:
		return Text{"A auditoria encontrou um sinal que precisa de análise humana.", "Confira a evidência e valide o impacto com a equipe responsável."}
	}
}

func Has(ruleID string) bool {
	_, ok := portuguese[ruleID]
	return ok
}
