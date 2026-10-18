// Package actions turns a diagnostic into a reviewable, read-only action plan.
// The auditor never executes these steps against a target database.
package actions

import (
	"regexp"
	"strings"
)

// Plan is the versioned guidance for one rule. The five operator fields are
// Meaning (constatação), Evidence (evidência), Impact (impacto), Next
// (próximo passo) and Query (consulta de confirmação somente leitura).
// Impact stays "não estimado" in the catalog: a number only appears when the
// finding carries a metric, a window and a denominator (see package impact).
type Plan struct {
	Category      string           `json:"category"`
	Meaning       string           `json:"meaning"`
	Evidence      string           `json:"evidence"`
	Impact        string           `json:"impact"`
	Next          string           `json:"next"`
	Benefit       string           `json:"expected_benefit"`
	Effort        string           `json:"effort"`
	Risk          string           `json:"risk"`
	Prerequisites string           `json:"prerequisites"`
	Confirmation  string           `json:"confirmation"`
	Validation    string           `json:"validation"`
	FalsePositive string           `json:"false_positive_risk"`
	ReadOnlyQuery string           `json:"read_only_query,omitempty"`
	Query         *QuerySpec       `json:"query,omitempty"`
	Maintenance   *MaintenanceCost `json:"maintenance,omitempty"`
}

// QuerySpec describes how a human may run the confirmation query. The auditor
// itself never runs it. Every query is a single SELECT over catalog or
// statistics views, bounded by LIMIT, meant for a READ ONLY transaction.
type QuerySpec struct {
	SQL           string   `json:"sql"`
	Engine        string   `json:"engine"`
	Capability    string   `json:"capability"`
	MinPermission string   `json:"min_permission"`
	Params        []string `json:"params"`
	RowLimit      int      `json:"row_limit"`
	TimeoutMs     int      `json:"timeout_ms"`
	ImpactWarning string   `json:"impact_warning"`
}

// MaintenanceCost lists what an operator must weigh before partitioning,
// compression, retention or autovacuum/analyze review. Nothing is automatic.
type MaintenanceCost struct {
	Kind             string `json:"kind"`
	Capability       string `json:"capability"`
	Preconditions    string `json:"preconditions"`
	ReadCost         string `json:"read_cost"`
	WriteCost        string `json:"write_cost"`
	StorageCost      string `json:"storage_cost"`
	Window           string `json:"window"`
	Risk             string `json:"risk"`
	PostValidation   string `json:"post_validation"`
	BusinessApproval bool   `json:"business_approval"`
}

const (
	defaultRowLimit  = 100
	defaultTimeoutMs = 5000
	readOnlyWarning  = "Rode em BEGIN READ ONLY com SET LOCAL statement_timeout. Lê só catálogo/estatísticas; em catálogos muito grandes pode custar CPU, prefira réplica ou horário de baixa carga."
)

func pgQuery(sql, capability, permission string, params ...string) *QuerySpec {
	return &QuerySpec{SQL: sql, Engine: "postgresql", Capability: capability, MinPermission: permission, Params: params,
		RowLimit: defaultRowLimit, TimeoutMs: defaultTimeoutMs, ImpactWarning: readOnlyWarning}
}

func tsQuery(sql, permission string, params ...string) *QuerySpec {
	q := pgQuery(sql, "timescaledb_information", permission, params...)
	q.Engine = "timescaledb"
	return q
}

const (
	permCatalog = "CONNECT no banco; catálogo pg_catalog é legível por qualquer papel"
	permStats   = "CONNECT e USAGE no schema; pg_read_all_stats para estatísticas de outros papéis"
	permMonitor = "pg_monitor (inclui pg_read_all_stats e pg_read_all_settings)"
	permTS      = "CONNECT; views timescaledb_information legíveis pelo papel de leitura"
	permACL     = "CONNECT e USAGE no schema; information_schema mostra só objetos visíveis ao papel"
)

var (
	qPrimaryKey       = pgQuery("SELECT c.conname, c.contype, c.convalidated FROM pg_catalog.pg_constraint c WHERE c.conrelid = $1::regclass AND c.contype = 'p' LIMIT 100;", "catalog", permCatalog, "$1 relação (schema.tabela)")
	qUnvalidated      = pgQuery("SELECT c.conname, c.contype, c.convalidated FROM pg_catalog.pg_constraint c WHERE c.conrelid = $1::regclass AND NOT c.convalidated LIMIT 100;", "constraints", permCatalog, "$1 relação (schema.tabela)")
	qForeignKeys      = pgQuery("SELECT c.conname, c.conkey, c.confrelid::regclass, c.confkey, c.convalidated FROM pg_catalog.pg_constraint c WHERE c.conrelid = $1::regclass AND c.contype = 'f' LIMIT 100;", "constraints", permCatalog, "$1 relação (schema.tabela)")
	qFKIndex          = pgQuery("SELECT c.conname, c.conkey, i.indexrelid::regclass, i.indkey FROM pg_catalog.pg_constraint c LEFT JOIN pg_catalog.pg_index i ON i.indrelid = c.conrelid WHERE c.conrelid = $1::regclass AND c.contype = 'f' LIMIT 100;", "indexes", permCatalog, "$1 relação filha (schema.tabela)")
	qFKTypes          = pgQuery("SELECT c.conname, a.attname, a.atttypid::regtype, r.attname AS ref_column, r.atttypid::regtype AS ref_type FROM pg_catalog.pg_constraint c JOIN pg_catalog.pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey) JOIN pg_catalog.pg_attribute r ON r.attrelid = c.confrelid AND r.attnum = ANY (c.confkey) WHERE c.conrelid = $1::regclass AND c.contype = 'f' LIMIT 100;", "constraints", permCatalog, "$1 relação filha (schema.tabela)")
	qSequenceDepend   = pgQuery("SELECT d.deptype, d.refobjid::regclass, d.refobjsubid FROM pg_catalog.pg_depend d WHERE d.objid = $1::regclass AND d.classid = 'pg_catalog.pg_class'::regclass LIMIT 100;", "catalog", permCatalog, "$1 sequência (schema.sequencia)")
	qSequence         = pgQuery("SELECT s.schemaname, s.sequencename, s.data_type, s.last_value, s.max_value, s.increment_by FROM pg_catalog.pg_sequences s WHERE s.schemaname = $1 AND s.sequencename = $2 LIMIT 100;", "catalog", permCatalog, "$1 schema", "$2 sequência")
	qColumnDefault    = pgQuery("SELECT a.attname, pg_catalog.pg_get_expr(d.adbin, d.adrelid) AS default_expr FROM pg_catalog.pg_attrdef d JOIN pg_catalog.pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum WHERE d.adrelid = $1::regclass LIMIT 100;", "catalog", permCatalog, "$1 relação (schema.tabela)")
	qIndexValidity    = pgQuery("SELECT i.indexrelid::regclass, i.indisvalid, i.indisready, i.indislive FROM pg_catalog.pg_index i WHERE i.indexrelid = $1::regclass LIMIT 100;", "indexes", permCatalog, "$1 índice (schema.indice)")
	qIndexUsage       = pgQuery("SELECT s.indexrelid::regclass, s.idx_scan, s.last_idx_scan, pg_catalog.pg_relation_size(s.indexrelid) AS index_bytes, d.stats_reset FROM pg_catalog.pg_stat_user_indexes s CROSS JOIN pg_catalog.pg_stat_database d WHERE s.indexrelid = $1::regclass AND d.datname = pg_catalog.current_database() LIMIT 100;", "indexes", permStats, "$1 índice (schema.indice)")
	qIndexDefinitions = pgQuery("SELECT i.indexrelid::regclass, i.indisunique, i.indisvalid, i.indkey, pg_catalog.pg_get_indexdef(i.indexrelid) FROM pg_catalog.pg_index i WHERE i.indrelid = $1::regclass LIMIT 100;", "indexes", permCatalog, "$1 relação (schema.tabela)")
	qIndexWrites      = pgQuery("SELECT t.relid::regclass, t.n_tup_ins, t.n_tup_upd, t.n_tup_del, t.n_tup_hot_upd, (SELECT count(*) FROM pg_catalog.pg_index i WHERE i.indrelid = t.relid) AS index_count FROM pg_catalog.pg_stat_user_tables t WHERE t.relid = $1::regclass LIMIT 100;", "indexes", permStats, "$1 relação (schema.tabela)")
	qTableScans       = pgQuery("SELECT t.relid::regclass, t.seq_scan, t.seq_tup_read, t.idx_scan, t.n_live_tup, d.stats_reset FROM pg_catalog.pg_stat_user_tables t CROSS JOIN pg_catalog.pg_stat_database d WHERE t.relid = $1::regclass AND d.datname = pg_catalog.current_database() LIMIT 100;", "workload", permStats, "$1 relação (schema.tabela)")
	qStatements       = pgQuery("SELECT s.queryid, s.calls, s.total_exec_time, s.mean_exec_time, s.shared_blks_read, s.shared_blks_hit, s.rows FROM pg_stat_statements s WHERE s.queryid = $1::bigint LIMIT 100;", "workload", "pg_read_all_stats e extensão pg_stat_statements instalada (o texto da consulta pode conter literais: não copie para tickets)", "$1 queryid do fingerprint")
	qLocks            = pgQuery("SELECT l.locktype, l.relation::regclass, l.mode, l.granted, a.wait_event_type, a.state, pg_catalog.age(pg_catalog.now(), a.query_start) AS waiting_for FROM pg_catalog.pg_locks l JOIN pg_catalog.pg_stat_activity a ON a.pid = l.pid WHERE NOT l.granted LIMIT 100;", "workload", permMonitor)
	qConnections      = pgQuery("SELECT a.datname, a.usename, a.state, count(*) AS sessions, pg_catalog.current_setting('max_connections') AS max_connections FROM pg_catalog.pg_stat_activity a GROUP BY a.datname, a.usename, a.state ORDER BY sessions DESC LIMIT 100;", "workload", permMonitor)
	qColumns          = pgQuery("SELECT a.attname, a.atttypid::regtype, a.attnotnull, pg_catalog.col_description(a.attrelid, a.attnum) FROM pg_catalog.pg_attribute a WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum LIMIT 100;", "catalog", permCatalog, "$1 relação (schema.tabela)")
	qNaming           = pgQuery("SELECT n.nspname, c.relname, c.relkind FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE c.oid = $1::regclass LIMIT 100;", "catalog", permCatalog, "$1 relação (schema.tabela)")
	qComment          = pgQuery("SELECT c.oid::regclass, pg_catalog.obj_description(c.oid, 'pg_class') AS description, pg_catalog.pg_total_relation_size(c.oid) AS bytes FROM pg_catalog.pg_class c WHERE c.oid = $1::regclass LIMIT 100;", "catalog", permCatalog, "$1 relação (schema.tabela)")
	qActivity         = pgQuery("SELECT t.relid::regclass, t.n_tup_ins, t.n_tup_upd, t.n_tup_del, t.seq_scan, t.idx_scan, t.last_autoanalyze, d.stats_reset FROM pg_catalog.pg_stat_user_tables t CROSS JOIN pg_catalog.pg_stat_database d WHERE t.relid = $1::regclass AND d.datname = pg_catalog.current_database() LIMIT 100;", "workload", permStats, "$1 relação (schema.tabela)")
	qSize             = pgQuery("SELECT c.oid::regclass, c.relkind, c.reltuples::bigint, pg_catalog.pg_table_size(c.oid) AS table_bytes, pg_catalog.pg_indexes_size(c.oid) AS index_bytes, pg_catalog.pg_total_relation_size(c.oid) AS total_bytes FROM pg_catalog.pg_class c WHERE c.oid = $1::regclass LIMIT 100;", "catalog", permCatalog, "$1 relação (schema.tabela)")
	qDeadTuples       = pgQuery("SELECT t.relid::regclass, t.n_live_tup, t.n_dead_tup, t.last_vacuum, t.last_autovacuum, t.autovacuum_count, c.reloptions FROM pg_catalog.pg_stat_user_tables t JOIN pg_catalog.pg_class c ON c.oid = t.relid WHERE t.relid = $1::regclass LIMIT 100;", "workload", permStats, "$1 relação (schema.tabela)")
	qAnalyze          = pgQuery("SELECT t.relid::regclass, t.last_analyze, t.last_autoanalyze, t.n_mod_since_analyze, t.n_live_tup, c.reloptions FROM pg_catalog.pg_stat_user_tables t JOIN pg_catalog.pg_class c ON c.oid = t.relid WHERE t.relid = $1::regclass LIMIT 100;", "workload", permStats, "$1 relação (schema.tabela)")
	qGrowth           = pgQuery("SELECT c.oid::regclass, pg_catalog.pg_total_relation_size(c.oid) AS total_bytes, t.n_live_tup, t.n_dead_tup, d.stats_reset FROM pg_catalog.pg_class c LEFT JOIN pg_catalog.pg_stat_user_tables t ON t.relid = c.oid CROSS JOIN pg_catalog.pg_stat_database d WHERE c.oid = $1::regclass AND d.datname = pg_catalog.current_database() LIMIT 100;", "catalog", permStats, "$1 relação (schema.tabela)")
	qTableGrants      = pgQuery("SELECT g.grantee, g.privilege_type, g.is_grantable FROM information_schema.role_table_grants g WHERE g.table_schema = $1 AND g.table_name = $2 LIMIT 100;", "security", permACL, "$1 schema", "$2 tabela")
	qRole             = pgQuery("SELECT r.rolname, r.rolsuper, r.rolcreaterole, r.rolcreatedb, r.rolreplication, r.rolbypassrls, r.rolcanlogin, r.rolvaliduntil FROM pg_catalog.pg_roles r WHERE r.rolname = $1 LIMIT 100;", "security", permCatalog, "$1 papel")
	qRoleMembers      = pgQuery("SELECT m.roleid::regrole AS granted_role, m.member::regrole AS member, m.admin_option FROM pg_catalog.pg_auth_members m WHERE m.member = $1::regrole OR m.roleid = $1::regrole LIMIT 100;", "security", permCatalog, "$1 papel")
	qDefiner          = pgQuery("SELECT p.oid::regprocedure, p.prosecdef, p.proowner::regrole, p.proconfig, p.proacl FROM pg_catalog.pg_proc p WHERE p.oid = $1::regprocedure LIMIT 100;", "security", permCatalog, "$1 assinatura da função (schema.funcao(args))")
	qAuditorRole      = pgQuery("SELECT r.rolname, r.rolsuper, r.rolcreaterole, r.rolcreatedb, r.rolbypassrls, pg_catalog.pg_has_role(pg_catalog.current_user(), 'pg_write_all_data', 'MEMBER') AS can_write_all FROM pg_catalog.pg_roles r WHERE r.rolname = pg_catalog.current_user() LIMIT 100;", "security", permCatalog)
	qRLS              = pgQuery("SELECT c.oid::regclass, c.relrowsecurity, c.relforcerowsecurity, p.polname, p.polcmd FROM pg_catalog.pg_class c LEFT JOIN pg_catalog.pg_policy p ON p.polrelid = c.oid WHERE c.oid = $1::regclass LIMIT 100;", "security", permCatalog, "$1 relação (schema.tabela)")
	qPublicSchema     = pgQuery("SELECT n.nspname, n.nspacl, pg_catalog.has_schema_privilege('public', n.oid, 'CREATE') AS public_can_create FROM pg_catalog.pg_namespace n WHERE n.nspname = $1 LIMIT 100;", "security", permCatalog, "$1 schema")
	qVersion          = pgQuery("SELECT pg_catalog.current_setting('server_version') AS server_version, pg_catalog.current_setting('server_version_num') AS server_version_num LIMIT 1;", "catalog", permCatalog)
	qExtensions       = pgQuery("SELECT e.extname, e.extversion, e.extnamespace::regnamespace FROM pg_catalog.pg_extension e ORDER BY e.extname LIMIT 100;", "catalog", permCatalog)
	qSetting          = pgQuery("SELECT s.name, s.setting, s.unit, s.source, s.pending_restart FROM pg_catalog.pg_settings s WHERE s.name = $1 LIMIT 100;", "catalog", "pg_read_all_settings para parâmetros restritos", "$1 nome do parâmetro")
	qReplication      = pgQuery("SELECT r.application_name, r.state, r.sync_state, pg_catalog.pg_wal_lsn_diff(r.sent_lsn, r.replay_lsn) AS replay_lag_bytes, r.replay_lag FROM pg_catalog.pg_stat_replication r LIMIT 100;", "workload", permMonitor)
	qArchiver         = pgQuery("SELECT a.archived_count, a.last_archived_wal, a.last_archived_time, a.failed_count, a.last_failed_wal, a.last_failed_time FROM pg_catalog.pg_stat_archiver a LIMIT 1;", "workload", permMonitor)
	qTSHypertable     = tsQuery("SELECT h.hypertable_schema, h.hypertable_name, h.num_chunks, h.compression_enabled FROM timescaledb_information.hypertables h WHERE h.hypertable_schema = $1 AND h.hypertable_name = $2 LIMIT 100;", permTS, "$1 schema", "$2 hypertable")
	qTSChunks         = tsQuery("SELECT c.chunk_schema, c.chunk_name, c.range_start, c.range_end, c.is_compressed FROM timescaledb_information.chunks c WHERE c.hypertable_schema = $1 AND c.hypertable_name = $2 ORDER BY c.range_end DESC LIMIT 100;", permTS, "$1 schema", "$2 hypertable")
	qTSJobs           = tsQuery("SELECT j.job_id, j.proc_name, j.schedule_interval, j.scheduled, j.config, s.last_run_status, s.last_successful_finish, s.total_failures FROM timescaledb_information.jobs j LEFT JOIN timescaledb_information.job_stats s ON s.job_id = j.job_id WHERE j.hypertable_schema = $1 AND j.hypertable_name = $2 LIMIT 100;", permTS, "$1 schema", "$2 hypertable")
	qTSJobByID        = tsQuery("SELECT j.job_id, j.proc_name, j.schedule_interval, j.scheduled, s.last_run_status, s.last_run_duration, s.next_start, s.total_failures FROM timescaledb_information.jobs j LEFT JOIN timescaledb_information.job_stats s ON s.job_id = j.job_id WHERE j.job_id = $1::integer LIMIT 100;", permTS, "$1 job_id")
	qTSWorkers        = tsQuery("SELECT count(*) FILTER (WHERE j.scheduled) AS scheduled_jobs, pg_catalog.current_setting('timescaledb.max_background_workers') AS max_background_workers FROM timescaledb_information.jobs j LIMIT 1;", permTS)
	qTSCagg           = tsQuery("SELECT a.view_schema, a.view_name, a.materialized_only, a.materialization_hypertable_schema, a.materialization_hypertable_name FROM timescaledb_information.continuous_aggregates a WHERE a.view_schema = $1 AND a.view_name = $2 LIMIT 100;", permTS, "$1 schema", "$2 agregado contínuo")
	qTSCaggJobs       = tsQuery("SELECT j.job_id, j.schedule_interval, j.config, s.last_run_status, s.last_successful_finish, s.last_run_duration FROM timescaledb_information.jobs j LEFT JOIN timescaledb_information.job_stats s ON s.job_id = j.job_id WHERE j.proc_name = 'policy_refresh_continuous_aggregate' AND j.hypertable_schema = $1 AND j.hypertable_name = $2 LIMIT 100;", permTS, "$1 schema da hypertable de materialização", "$2 hypertable de materialização")
)

// Maintenance cost templates (BA-21). They never authorize an automatic change.
var (
	costPartition = &MaintenanceCost{Kind: "partitioning", Capability: "PostgreSQL 10+ (particionamento declarativo) ou hypertable TimescaleDB",
		Preconditions:  "Duas coletas completas mostrando crescimento; chave temporal ou de negócio estável; consultas filtram por essa chave; backup restaurável; dono e janela aprovados.",
		ReadCost:       "Consultas sem filtro pela chave passam a ler todas as partições; planos precisam ser revalidados.",
		WriteCost:      "Migração reescreve a tabela (cópia ou anexação); gatilhos e FKs podem exigir ajustes; carga de escrita durante a migração.",
		StorageCost:    "Espaço temporário de até 2x a tabela durante a cópia; índices recriados por partição.",
		Window:         "Janela de manutenção dedicada, com estimativa de duração medida em homologação com volume representativo.",
		Risk:           "Bloqueios longos, quebra de FKs/aplicação e plano pior para consultas sem a chave.",
		PostValidation: "Nova coleta completa: tamanho por partição, planos das consultas principais e latência comparados ao período anterior.", BusinessApproval: true}
	costCompression = &MaintenanceCost{Kind: "compression", Capability: "TimescaleDB com compressão nativa (columnstore)",
		Preconditions:  "Chunks fechados há tempo suficiente para não receber UPDATE frequente; segmentby/orderby testados; backup restaurável.",
		ReadCost:       "Leituras analíticas costumam melhorar; leituras pontuais e por colunas fora de segmentby podem piorar.",
		WriteCost:      "UPDATE/DELETE em chunks comprimidos custa mais; o job de compressão consome CPU e I/O.",
		StorageCost:    "Redução esperada só é conhecida após compressão de amostra; meça a razão em homologação.",
		Window:         "Job de compressão em horário de baixa carga; primeira compressão de histórico pode levar horas.",
		Risk:           "Configuração inadequada de segmentby piora consultas; descompressão em massa exige espaço livre.",
		PostValidation: "Nova coleta completa: tamanho antes/depois, razão de compressão e latência das consultas críticas."}
	costRetention = &MaintenanceCost{Kind: "retention", Capability: "TimescaleDB (drop_chunks via política) ou rotina externa aprovada",
		Preconditions:  "Prazo legal/negócio aprovado por escrito; backup restaurável e testado; consumidores do histórico identificados.",
		ReadCost:       "Consultas a períodos removidos deixam de retornar dados.",
		WriteCost:      "Remoção de chunks é rápida, mas irreversível sem backup.",
		StorageCost:    "Libera o espaço dos chunks além do prazo; o valor depende do volume por período observado.",
		Window:         "Agenda acordada com negócio/compliance; primeira execução acompanhada.",
		Risk:           "Perda permanente de dados se o prazo estiver errado ou o backup não restaurar.",
		PostValidation: "Confirme que só chunks além do prazo saíram e que o backup restaura o período removido.", BusinessApproval: true}
	costAutovacuum = &MaintenanceCost{Kind: "autovacuum", Capability: "PostgreSQL/TimescaleDB (pg_stat_user_tables)",
		Preconditions:  "Tendência de tuplas mortas em duas coletas completas; transações longas e slots de replicação verificados.",
		ReadCost:       "VACUUM lê a tabela inteira ou as páginas marcadas; pode competir por I/O.",
		WriteCost:      "Ajustes mais agressivos aumentam I/O de manutenção; menos agressivos acumulam tuplas mortas.",
		StorageCost:    "VACUUM comum não devolve espaço ao sistema; só reorganização (fora do auditor) reduz arquivo.",
		Window:         "Ajuste de parâmetro por tabela pode ser feito a qualquer hora; reorganizações exigem janela.",
		Risk:           "Parâmetros errados elevam I/O ou deixam bloat crescer; VACUUM FULL bloqueia a tabela.",
		PostValidation: "Compare n_dead_tup, last_autovacuum e tamanho em novas coletas completas."}
	costAnalyze = &MaintenanceCost{Kind: "analyze", Capability: "PostgreSQL/TimescaleDB (pg_stat_user_tables)",
		Preconditions:  "n_mod_since_analyze alto ou estatísticas antigas confirmadas; planos ruins observados.",
		ReadCost:       "ANALYZE lê uma amostra da tabela; custo moderado.",
		WriteCost:      "Atualiza pg_statistic; sem reescrita da tabela.",
		StorageCost:    "Desprezível.",
		Window:         "Horário de baixa carga em tabelas grandes.",
		Risk:           "Planos podem mudar após novas estatísticas; valide consultas críticas.",
		PostValidation: "Confira last_analyze e os planos das consultas afetadas na nova coleta."}
	costReorder = &MaintenanceCost{Kind: "reorder", Capability: "TimescaleDB (add_reorder_policy)",
		Preconditions:  "Consultas dominantes por um índice específico; chunks fechados.",
		ReadCost:       "Pode melhorar varreduras por faixa no índice escolhido.",
		WriteCost:      "Reescreve cada chunk reordenado; consome I/O.",
		StorageCost:    "Espaço temporário do tamanho do chunk durante a reordenação.",
		Window:         "Job em horário de baixa carga.",
		Risk:           "Bloqueio do chunk durante a reordenação.",
		PostValidation: "Compare planos e latência das consultas dominantes em nova coleta."}
)

// narratedRules are rules whose entry states its own meaning and next step.
// Every other rule takes them from ruleText (text_pt.go), the single source
// of user-facing text that guidance.For also reads (AR-06).
var narratedRules = map[string]bool{}

// entry: meaning and next are empty for rules declared in ruleText.
type entry struct {
	id, category, effort      string
	meaning, evidence, impact string
	next                      string
	query                     *QuerySpec
	cost                      *MaintenanceCost
}

func notEstimated(metric string) string {
	return "não estimado sem " + metric + "."
}

var entries = []entry{
	// structure / integrity
	{"integrity.missing_primary_key", "structure", "alto", "Esta relação não tem chave primária observada na coleta.", "pg_constraint sem contype='p' para a relação; a consulta lista as constraints primárias e zero linhas confirma a ausência.", "não estimado: ausência de chave não tem tamanho nem latência mensuráveis nesta coleta.", "Valide duplicatas e o identificador usado pela aplicação antes de propor uma chave.", qPrimaryKey, nil},
	{"model.no_primary_key", "structure", "alto", "Esta relação não tem chave primária observada na coleta.", "pg_constraint sem contype='p' para a relação; a consulta lista as constraints primárias e zero linhas confirma a ausência.", "não estimado: ausência de chave não tem tamanho nem latência mensuráveis nesta coleta.", "Valide duplicatas e o identificador usado pela aplicação antes de propor uma chave.", qPrimaryKey, nil},
	{"integrity.constraint_unvalidated", "structure", "médio", "", "pg_constraint.convalidated = false na coleta.", notEstimated("a contagem de linhas que violam a regra (não coletada)"), "", qUnvalidated, nil},
	{"integrity.fk_without_index", "structure", "médio", "", "conkey da FK comparado a indkey dos índices da tabela filha.", notEstimated("tempo de DELETE/UPDATE na tabela pai e volume da filha"), "", qFKIndex, nil},
	{"integrity.fk_type_mismatch", "structure", "alto", "", "atttypid das colunas da FK e das colunas referenciadas.", notEstimated("planos que mostrem conversão implícita"), "", qFKTypes, nil},
	{"integrity.orphan_sequence", "structure", "baixo", "", "pg_depend sem deptype 'a' para a sequência.", "não estimado: uma sequência isolada ocupa poucos bytes.", "", qSequenceDepend, nil},
	{"integrity.sequence_default_mismatch", "structure", "médio", "", "pg_attrdef da coluna comparado a pg_depend da sequência.", notEstimated("risco de colisão medido por last_value das duas sequências"), "", qColumnDefault, nil},
	{"sequence.near_limit", "structure", "alto", "", "last_value e max_value de pg_sequences na coleta.", notEstimated("taxa de consumo entre duas coletas completas"), "", qSequence, nil},
	{"index.invalid", "structure", "médio", "", "pg_index.indisvalid = false na coleta (CREATE INDEX CONCURRENTLY interrompido ou falha).", notEstimated("bytes do índice inválido"), "", qIndexValidity, nil},
	{"model.naming_inconsistent", "structure", "baixo", "", "Nome do objeto comparado à convenção da regra (padrão snake_case).", "não estimado: convenção de nomes não tem métrica de custo.", "", qNaming, nil},
	{"model.implicit_relationship", "structure", "médio", "", "Nome e tipo da coluna combinam com a chave de outra tabela; nenhuma FK em pg_constraint.", notEstimated("contagem de linhas órfãs (não coletada)"), "", qForeignKeys, nil},
	{"model.wide_table", "structure", "alto", "", "Contagem de colunas de pg_attribute na coleta.", notEstimated("padrão de leitura por coluna"), "", qColumns, nil},
	{"model.repeated_columns", "structure", "médio", "", "Nomes de colunas com sufixo numérico na mesma tabela.", "não estimado: a regra observa só nomes.", "", qColumns, nil},
	{"model.duplicate_entity", "structure", "alto", "", "Nomes e colunas semelhantes entre as tabelas.", "não estimado: a regra compara só estrutura.", "", qColumns, nil},
	{"model.type_review", "structure", "médio", "", "atttypid da coluna na coleta.", "não estimado: o tipo não indica erro sem os valores.", "", qColumns, nil},
	{"model.jsonb_critical", "structure", "médio", "", "Colunas jsonb em tabela grande ou central.", notEstimated("consultas que filtram por chaves do JSON"), "", qColumns, nil},
	{"model.undocumented_critical", "structure", "baixo", "", "obj_description vazio e tamanho acima do limite.", "não estimado: documentação não tem métrica de custo.", "", qComment, nil},
	{"inactivity.possibly_inactive", "structure", "baixo", "", "n_tup_ins/upd/del e scans sem variação; data da última mudança de dados.", "não estimado: ausência de uso observado não comprova inatividade.", "", qActivity, nil},
	// query and index
	{"index.unused", "query_and_index", "médio", "O índice não teve uso observado na janela entre duas coletas.", "idx_scan igual nas duas coletas, sem stats_reset no intervalo.", notEstimated("bytes do índice, janela de idx_scan e tamanho total dos índices da tabela"), "Confirme reset de estatísticas, réplicas e consultas raras (mensais, fechamento) antes de considerar remoção.", qIndexUsage, nil},
	{"index.overlap", "query_and_index", "médio", "", "pg_get_indexdef normalizado igual para os índices listados.", notEstimated("bytes de cada índice e uso de cada um"), "", qIndexDefinitions, nil},
	{"index.prefix_overlap", "query_and_index", "médio", "", "indkey de um índice é prefixo do indkey de outro na mesma tabela.", notEstimated("bytes e uso de cada índice"), "", qIndexDefinitions, nil},
	{"index.write_burden", "query_and_index", "médio", "", "Número de índices e n_tup_ins/upd/del da coleta.", notEstimated("latência de escrita medida"), "", qIndexWrites, nil},
	{"index.investigate_missing", "query_and_index", "médio", "", "calls e shared_blks_read do fingerprint em pg_stat_statements.", notEstimated("plano de execução representativo"), "", qStatements, nil},
	{"index.candidate", "query_and_index", "médio", "", "seq_scan e seq_tup_read da tabela na coleta.", notEstimated("razão seq_scan/idx_scan dentro de uma janela conhecida"), "", qTableScans, nil},
	{"performance.lock_wait", "query_and_index", "médio", "", "pg_locks com granted = false e tempo de espera.", notEstimated("frequência das esperas em várias coletas"), "", qLocks, nil},
	{"performance.high_connections", "query_and_index", "médio", "", "Contagem em pg_stat_activity comparada a max_connections.", notEstimated("série de conexões ao longo do tempo"), "", qConnections, nil},
	{"performance.slow_query", "query_and_index", "médio", "", "mean_exec_time e calls de pg_stat_statements.", notEstimated("janela desde stats_reset e tempo total do banco"), "", qStatements, nil},
	{"performance.workload_scan", "query_and_index", "médio", "", "shared_blks_read e shared_blks_hit do fingerprint, com a janela de observação.", notEstimated("blocos lidos, total de blocos acessados e janela de observação"), "", qStatements, nil},
	{"performance.workload_write", "query_and_index", "médio", "", "calls de INSERT/UPDATE/DELETE do fingerprint.", notEstimated("latência de escrita e volume por janela"), "", qStatements, nil},
	{"performance.workload_cost", "query_and_index", "médio", "", "total_exec_time do fingerprint na janela das estatísticas.", notEstimated("tempo total do banco na mesma janela"), "", qStatements, nil},
	// maintenance
	{"storage.large_table", "maintenance", "alto", "A relação passou do limite de tamanho da regra.", "pg_total_relation_size da coleta.", notEstimated("tamanho total do banco na mesma coleta"), "Compare crescimento em coletas completas e a retenção aprovada. Não proponha DROP nem particionamento automático.", qSize, costPartition},
	{"storage.top_consumer", "maintenance", "médio", "", "pg_total_relation_size e posição no ranking da coleta.", notEstimated("tamanho total do banco na mesma coleta"), "", qSize, nil},
	{"vacuum.high_dead_tuples", "maintenance", "médio", "A relação acumulou tuplas mortas acima do limiar da regra.", "n_dead_tup e n_live_tup da coleta. Estimativa, não bloat medido.", notEstimated("n_dead_tup, n_live_tup e o instante do último (auto)vacuum"), "Confira autovacuum, transações longas e a carga de escrita. Não agende VACUUM a partir daqui.", qDeadTuples, costAutovacuum},
	{"maintenance.dead_tuple_pressure", "maintenance", "médio", "", "n_dead_tup e n_live_tup marcados como estimativa.", notEstimated("n_dead_tup, n_live_tup e o instante do último (auto)vacuum"), "", qDeadTuples, costAutovacuum},
	{"timescale.chunk_dead_tuples", "maintenance", "médio", "", "n_dead_tup e n_live_tup do chunk (amostra limitada, não a hypertable inteira).", notEstimated("n_dead_tup, n_live_tup e o instante do último autovacuum do chunk"), "", qDeadTuples, costAutovacuum},
	{"maintenance.stale_analyze", "maintenance", "baixo", "", "last_analyze/last_autoanalyze da coleta.", notEstimated("n_mod_since_analyze e planos afetados"), "", qAnalyze, costAnalyze},
	{"maintenance.growth_trend", "maintenance", "médio", "Duas coletas completas mostram mudança de tamanho.", "Tamanhos das duas coletas, janela e stats_reset. Tuplas mortas são estimativa.", notEstimated("duas coletas completas, denominador e janela"), "Confirme tendência, janela e backup. Não há DROP, DELETE nem retenção automática.", qGrowth, costPartition},
	{"chunk.high_count", "maintenance", "médio", "", "num_chunks de timescaledb_information.hypertables.", notEstimated("tempo de planejamento das consultas"), "", qTSHypertable, nil},
	{"chunk.size_skew", "maintenance", "médio", "", "Maior chunk comparado à média dos chunks coletados.", notEstimated("padrão de ingestão por período"), "", qTSChunks, nil},
	{"chunk.inventory_truncated", "maintenance", "baixo", "", "Contagem de chunks igual ao teto da coleta.", "não estimado: o inventário está incompleto.", "", qTSChunks, nil},
	{"cagg.missing_refresh_policy", "maintenance", "baixo", "", "Nenhum job policy_refresh_continuous_aggregate ligado ao agregado.", notEstimated("frescor exigido pelos usuários"), "", qTSCagg, nil},
	{"cagg.overlap", "maintenance", "médio", "", "start_offset/end_offset das políticas do agregado.", notEstimated("duração dos jobs de refresh"), "", qTSCaggJobs, nil},
	{"cagg.materialization_lag", "maintenance", "médio", "", "Último refresh comparado ao dado mais recente.", notEstimated("frescor exigido e duração do refresh"), "", qTSCaggJobs, nil},
	{"cagg.realtime_hypothesis", "maintenance", "baixo", "", "materialized_only do agregado.", notEstimated("latência das consultas ao agregado"), "", qTSCagg, nil},
	{"cagg.refresh_window_exceeded", "maintenance", "médio", "", "last_run_duration comparado a schedule_interval.", notEstimated("série de durações do job"), "", qTSCaggJobs, nil},
	{"policy.missing_retention", "maintenance", "médio", "", "Nenhum job policy_retention para a hypertable.", notEstimated("volume por período e prazo legal aprovado"), "", qTSJobs, costRetention},
	{"policy.missing_compression", "maintenance", "médio", "", "compression_enabled e ausência de job policy_compression.", notEstimated("razão de compressão medida em amostra"), "", qTSHypertable, costCompression},
	{"policy.job_failed", "maintenance", "médio", "", "last_run_status e total_failures de job_stats.", "não estimado: a falha é operacional, sem métrica de impacto.", "", qTSJobs, nil},
	{"policy.retention_chunk_mismatch", "maintenance", "baixo", "", "drop_after da política comparado a chunk_time_interval.", notEstimated("dados retidos além do prazo"), "", qTSJobs, costRetention},
	{"policy.reorder_hypothesis", "maintenance", "baixo", "", "Hypertable sem reorder com índice dominante.", notEstimated("planos das consultas dominantes"), "", qTSJobs, costReorder},
	{"policy.compression_not_applied", "maintenance", "médio", "", "Chunks fechados com is_compressed = false e política existente.", notEstimated("bytes dos chunks não comprimidos"), "", qTSChunks, costCompression},
	{"policy.compression_ratio", "maintenance", "médio", "", "Bytes antes e depois da compressão por hypertable.", notEstimated("razão por chunk e segmentby testado"), "", qTSHypertable, costCompression},
	{"policy.compression_settings", "maintenance", "médio", "", "Configuração de compressão comparada às colunas filtradas.", notEstimated("teste de configuração em homologação"), "", qTSHypertable, costCompression},
	{"job.unhealthy", "maintenance", "médio", "", "scheduled, last_run_status e next_start do job.", "não estimado: o estado do job não tem métrica de impacto.", "", qTSJobByID, nil},
	{"job.slo_exceeded", "maintenance", "médio", "", "last_run_duration comparado a schedule_interval.", notEstimated("série de durações"), "", qTSJobByID, nil},
	{"job.workers_saturated", "maintenance", "médio", "", "Jobs agendados comparados a timescaledb.max_background_workers.", notEstimated("fila de jobs atrasados"), "", qTSWorkers, nil},
	{"replication.lag_high", "maintenance", "alto", "", "pg_wal_lsn_diff(sent_lsn, replay_lsn) em pg_stat_replication.", notEstimated("série de atraso e RPO acordado"), "", qReplication, nil},
	{"replication.archive_stalled", "maintenance", "alto", "", "last_archived_time e failed_count de pg_stat_archiver.", notEstimated("WAL acumulado e RPO acordado"), "", qArchiver, nil},
	// security
	{"security.excessive_privilege", "security", "alto", "Um papel tem privilégio mais amplo do que o uso conhecido da aplicação.", "Grants efetivos gravados na coleta. A API não executa SQL no alvo.", "não estimado: exposição não vira nota sem o privilégio e o papel.", "Confirme se o privilégio é operacional antes de qualquer revogação.", qTableGrants, nil},
	{"security.powerful_role", "security", "alto", "", "Atributos de pg_roles na coleta.", "não estimado: o atributo não indica uso indevido por si.", "", qRole, nil},
	{"security.security_definer", "security", "alto", "", "prosecdef = true em pg_proc.", "não estimado: o risco depende do código e de quem pode executar.", "", qDefiner, nil},
	{"security.definer_search_path", "security", "alto", "", "proconfig sem search_path e definição sem SET search_path.", "não estimado: o risco depende de schemas graváveis por terceiros.", "", qDefiner, nil},
	{"security.auditor_not_readonly", "security", "alto", "", "Atributos e associação a papéis de escrita da conta atual.", "não estimado: o auditor não escreve, mas a credencial vazada poderia.", "", qAuditorRole, nil},
	{"security.auditor_privilege_unknown", "security", "médio", "", "A verificação de privilégios falhou ou foi negada.", "não estimado: sem leitura de pg_roles.", "", qAuditorRole, nil},
	{"security.rls_disabled_hypothesis", "security", "médio", "", "relrowsecurity = false e colunas de tenant na tabela.", "não estimado: depende da política de acesso.", "", qRLS, nil},
	{"security.public_schema_create", "security", "médio", "", "nspacl concede CREATE a PUBLIC.", "não estimado: depende de quem conecta ao banco.", "", qPublicSchema, nil},
	{"security.account_inactive_review", "security", "baixo", "", "rolvaliduntil e amostras de pg_stat_activity entre coletas.", "não estimado: amostras não comprovam ausência de uso.", "", qRoleMembers, nil},
	{"config.version_drift", "security", "médio", "", "server_version de cada ambiente.", "não estimado: a diferença pode ser planejada.", "", qVersion, nil},
	{"config.extension_drift", "security", "médio", "", "pg_extension de cada ambiente.", "não estimado: a diferença pode ser planejada.", "", qExtensions, nil},
	{"config.guc_drift", "security", "médio", "", "pg_settings de cada ambiente.", "não estimado: o efeito depende da carga.", "", qSetting, nil},
}

// rulePlans is the only match table. Unknown ids use the investigation fallback.
var rulePlans = map[string]Plan{}

func init() {
	for _, e := range entries {
		p := base(e.category)
		p.Meaning, p.Evidence, p.Impact, p.Next = e.meaning, e.evidence, e.impact, e.next
		if text, ok := ruleText[e.id]; ok {
			p.Meaning, p.Next = text.Meaning, text.Next
		} else if e.meaning != "" {
			narratedRules[e.id] = true
		}
		if e.effort != "" {
			p.Effort = e.effort
		}
		if e.query != nil {
			q := *e.query
			p.Query = &q
			p.ReadOnlyQuery = q.SQL
		}
		if e.cost != nil {
			c := *e.cost
			p.Maintenance = &c
			p.Risk = c.Risk
		}
		rulePlans[e.id] = p
	}
	storage := rulePlans["storage.large_table"]
	storage.Confirmation = "Compare crescimento em coletas completas e confirme a retenção com a área de negócio antes de qualquer mudança externa."
	storage.Prerequisites = "Estime custo de migração, índices e espaço temporário; obtenha backup restaurável, janela e plano de retorno."
	storage.Validation = "Após mudança externa, compare volume, latência e cobertura em novas coletas completas."
	rulePlans["storage.large_table"] = storage
	growth := rulePlans["maintenance.growth_trend"]
	growth.Confirmation = "Rode a consulta de confirmação nas duas pontas da janela e compare com os tamanhos coletados."
	growth.FalsePositive = "Carga atípica, reset de estatísticas, lacuna entre coletas ou coleta parcial."
	rulePlans["maintenance.growth_trend"] = growth
}

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
		p.Confirmation = "Inspecione as dependências e o catálogo do objeto; confirme com o dono da aplicação antes de alterar chaves, constraints ou índices."
		p.Validation = "Após correção externa, confira o catálogo e compare uma nova coleta com a anterior."
		p.FalsePositive = "Ausência de uso observado não comprova que um objeto está órfão ou pode ser removido."
		p.Effort = "alto"
	case "query_and_index":
		p.Prerequisites = "Confira estatísticas, taxa de escrita, espaço e plano da consulta em ambiente seguro."
		p.Confirmation = "Compare EXPLAIN (sem executar alterações) e o fingerprint da consulta; confirme cobertura e período das estatísticas."
		p.Validation = "Meça latência, leituras, escrita e espaço antes/depois sob carga comparável; documente rollback."
		p.Risk = "Criar ou remover índice pode aumentar escrita, ocupar espaço ou causar bloqueios; planeje execução concorrente quando aplicável."
		p.FalsePositive = "Contadores zerados após reset ou uma janela curta não provam inutilidade de índice."
	case "maintenance":
		p.Prerequisites = "Confirme tendência, volume, custo operacional, SLA e janela aprovada. Retenção depende de decisão de negócio."
		p.Confirmation = "Compare duas ou mais coletas completas e verifique a configuração e as estatísticas do objeto."
		p.Validation = "Após ação externa, compare tamanho, atraso e saúde dos jobs em novas coletas."
		p.Risk = "Manutenção e retenção podem consumir recursos ou remover dados; exija backup e plano de recuperação."
		p.FalsePositive = "Um único pico ou coleta parcial não estabelece tendência de crescimento."
	case "security":
		p.Prerequisites = "Confirme o papel da aplicação, a permissão efetiva e a política de acesso antes de revogar privilégios."
		p.Confirmation = "Revise grants, políticas e dependências com o responsável pela aplicação."
		p.Validation = "Teste o fluxo autorizado e repita a coleta para verificar a permissão efetiva."
		p.Risk = "Uma revogação pode interromper um serviço legítimo; mudanças devem ter rollback externo."
		p.FalsePositive = "Permissão aparentemente ampla pode ser necessária a uma função operacional conhecida."
		p.Effort = "alto"
	}
	return p
}

// For returns the plan registered for rule. Anything else is the single investigation fallback.
func For(rule string) Plan {
	if p, ok := rulePlans[rule]; ok {
		return p
	}
	return base("investigation")
}

// Rules lists every rule id with an explicit plan.
// Has reports whether rule has its own Portuguese meaning and next step,
// either narrated in its entry or declared in ruleText.
func Has(rule string) bool {
	if _, ok := rulePlans[rule]; !ok {
		return false
	}
	_, declared := ruleText[rule]
	return declared || narratedRules[rule]
}

func Rules() []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.id)
	}
	return out
}

var (
	bannedStatement = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|MERGE|DROP|ALTER|CREATE|GRANT|REVOKE|TRUNCATE|VACUUM|ANALYZE|EXPLAIN|COPY|CALL|DO|SET|RESET|LOCK|REINDEX|CLUSTER|REFRESH|COMMENT|SECURITY|LISTEN|NOTIFY|PREPARE|EXECUTE|INTO)\b`)
	bannedFunction  = regexp.MustCompile(`(?i)\b(pg_terminate_backend|pg_cancel_backend|pg_reload_conf|pg_rotate_logfile|set_config|nextval|setval|pg_sleep\w*|lo_\w+|dblink\w*|pg_read_file|pg_read_binary_file|pg_ls_dir|pg_stat_reset\w*|pg_advisory\w*|txid_current|pg_switch_wal|pg_create\w*|pg_drop\w*|query_to_xml|pg_logical_emit_message|pg_promote|pg_backup_\w+)\s*\(`)
	relationRef     = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*(?:\.[a-z_][a-z0-9_]*)?|\()`)
	allowedSchemas  = []string{"pg_catalog.", "information_schema.", "timescaledb_information.", "pg_stat_statements"}
)

// QueryIsReadOnly rejects any statement that is not a single SELECT. It also
// rejects data-changing keywords anywhere and side-effect functions. An empty
// query is allowed (the rule has nothing to confirm).
func QueryIsReadOnly(query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return true
	}
	upper := strings.ToUpper(q)
	if !strings.HasPrefix(upper, "SELECT ") {
		return false
	}
	stripped := stripStringLiterals(q)
	if bannedStatement.MatchString(stripped) || bannedFunction.MatchString(stripped) {
		return false
	}
	return strings.Count(q, ";") == 1 && strings.HasSuffix(q, ";")
}

// ReadsOnlyCatalog reports whether every FROM/JOIN target is a schema-qualified
// catalog or statistics view. Subqueries in FROM are rejected so a review
// cannot miss a nested table read.
func ReadsOnlyCatalog(query string) bool {
	for _, m := range relationRef.FindAllStringSubmatch(stripStringLiterals(query), -1) {
		ref := strings.ToLower(m[1])
		ok := false
		for _, schema := range allowedSchemas {
			if strings.HasPrefix(ref, schema) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// stripStringLiterals blanks single-quoted literals so 'INSERT' inside a
// comparison value does not count as a statement, and a keyword hidden in a
// literal cannot hide a real one.
func stripStringLiterals(q string) string {
	var b strings.Builder
	in := false
	for _, r := range q {
		if r == '\'' {
			in = !in
			b.WriteRune(' ')
			continue
		}
		if in {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
