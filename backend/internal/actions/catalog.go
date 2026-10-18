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
	{"integrity.constraint_unvalidated", "structure", "médio", "Uma constraint foi criada como NOT VALID e ainda não cobre as linhas antigas.", "pg_constraint.convalidated = false na coleta.", notEstimated("a contagem de linhas que violam a regra (não coletada)"), "Conte as linhas incompatíveis em homologação e planeje VALIDATE CONSTRAINT fora do horário de pico.", qUnvalidated, nil},
	{"integrity.fk_without_index", "structure", "médio", "A chave estrangeira não tem índice com as mesmas colunas iniciais na tabela filha.", "conkey da FK comparado a indkey dos índices da tabela filha.", notEstimated("tempo de DELETE/UPDATE na tabela pai e volume da filha"), "Meça o custo de DELETE/UPDATE na tabela pai e de JOINs antes de avaliar um índice.", qFKIndex, nil},
	{"integrity.fk_type_mismatch", "structure", "alto", "Colunas ligadas pela FK têm tipos diferentes.", "atttypid das colunas da FK e das colunas referenciadas.", notEstimated("planos que mostrem conversão implícita"), "Confirme a semântica dos tipos com o dono dos dados antes de migrar.", qFKTypes, nil},
	{"integrity.orphan_sequence", "structure", "baixo", "A sequência não tem dependência OWNED BY nem coluna que a use como padrão.", "pg_depend sem deptype 'a' para a sequência.", "não estimado: uma sequência isolada ocupa poucos bytes.", "Procure nextval na aplicação e em funções antes de vincular ou aposentar a sequência.", qSequenceDepend, nil},
	{"integrity.sequence_default_mismatch", "structure", "médio", "O DEFAULT da coluna chama uma sequência diferente da que pertence a ela.", "pg_attrdef da coluna comparado a pg_depend da sequência.", notEstimated("risco de colisão medido por last_value das duas sequências"), "Compare last_value das sequências e o maior valor da coluna antes de corrigir o vínculo.", qColumnDefault, nil},
	{"sequence.near_limit", "structure", "alto", "A sequência consumiu uma fração alta do seu valor máximo.", "last_value e max_value de pg_sequences na coleta.", notEstimated("taxa de consumo entre duas coletas completas"), "Calcule o ritmo de consumo entre coletas e planeje a troca de tipo com antecedência.", qSequence, nil},
	{"index.invalid", "structure", "médio", "O índice está marcado como inválido e não é usado pelo planejador.", "pg_index.indisvalid = false na coleta (CREATE INDEX CONCURRENTLY interrompido ou falha).", notEstimated("bytes do índice inválido"), "Descubra por que a criação falhou; reconstrua com CONCURRENTLY em janela, fora do auditor.", qIndexValidity, nil},
	{"model.naming_inconsistent", "structure", "baixo", "O nome do objeto foge da convenção configurada para o schema.", "Nome do objeto comparado à convenção da regra (padrão snake_case).", "não estimado: convenção de nomes não tem métrica de custo.", "Registre a exceção ou planeje renomeação com camada de compatibilidade.", qNaming, nil},
	{"model.implicit_relationship", "structure", "médio", "Uma coluna parece referenciar outra tabela sem FK declarada.", "Nome e tipo da coluna combinam com a chave de outra tabela; nenhuma FK em pg_constraint.", notEstimated("contagem de linhas órfãs (não coletada)"), "Confirme a regra de negócio e conte órfãos em homologação antes de criar a FK.", qForeignKeys, nil},
	{"model.wide_table", "structure", "alto", "A tabela tem mais colunas que o limite configurado.", "Contagem de colunas de pg_attribute na coleta.", notEstimated("padrão de leitura por coluna"), "Revise com o domínio quais colunas são lidas juntas antes de propor divisão.", qColumns, nil},
	{"model.repeated_columns", "structure", "médio", "Colunas com nomes numerados sugerem grupo repetido.", "Nomes de colunas com sufixo numérico na mesma tabela.", "não estimado: a regra observa só nomes.", "Confirme se os valores representam uma lista antes de normalizar.", qColumns, nil},
	{"model.duplicate_entity", "structure", "alto", "Duas tabelas parecem representar a mesma entidade.", "Nomes e colunas semelhantes entre as tabelas.", "não estimado: a regra compara só estrutura.", "Valide com os responsáveis e as aplicações antes de consolidar dados.", qColumns, nil},
	{"model.type_review", "structure", "médio", "O tipo de uma coluna merece revisão (money, timestamp sem fuso ou status em texto).", "atttypid da coluna na coleta.", "não estimado: o tipo não indica erro sem os valores.", "Discuta a semântica do valor com o domínio antes de propor migração.", qColumns, nil},
	{"model.jsonb_critical", "structure", "médio", "Campos essenciais parecem concentrados em JSONB.", "Colunas jsonb em tabela grande ou central.", notEstimated("consultas que filtram por chaves do JSON"), "Liste as chaves usadas em filtros e contratos antes de extrair colunas.", qColumns, nil},
	{"model.undocumented_critical", "structure", "baixo", "Uma tabela grande não tem comentário de finalidade ou dono.", "obj_description vazio e tamanho acima do limite.", "não estimado: documentação não tem métrica de custo.", "Registre finalidade, dono e retenção com COMMENT ON após confirmação do responsável.", qComment, nil},
	{"inactivity.possibly_inactive", "structure", "baixo", "O objeto não mostrou escrita nem leitura recente nas estatísticas.", "n_tup_ins/upd/del e scans sem variação; data da última mudança de dados.", "não estimado: ausência de uso observado não comprova inatividade.", "Confirme com a aplicação; não remova nada a partir deste sinal.", qActivity, nil},
	// query and index
	{"index.unused", "query_and_index", "médio", "O índice não teve uso observado na janela entre duas coletas.", "idx_scan igual nas duas coletas, sem stats_reset no intervalo.", notEstimated("bytes do índice, janela de idx_scan e tamanho total dos índices da tabela"), "Confirme reset de estatísticas, réplicas e consultas raras (mensais, fechamento) antes de considerar remoção.", qIndexUsage, nil},
	{"index.overlap", "query_and_index", "médio", "Dois índices têm a mesma definição de colunas.", "pg_get_indexdef normalizado igual para os índices listados.", notEstimated("bytes de cada índice e uso de cada um"), "Compare unicidade, predicados e uso; mantenha o que sustenta constraints.", qIndexDefinitions, nil},
	{"index.prefix_overlap", "query_and_index", "médio", "As colunas de um índice são o início de outro índice.", "indkey de um índice é prefixo do indkey de outro na mesma tabela.", notEstimated("bytes e uso de cada índice"), "Confira se o índice menor atende consultas que o maior não cobre bem.", qIndexDefinitions, nil},
	{"index.write_burden", "query_and_index", "médio", "A tabela tem muitos índices e escrita intensa.", "Número de índices e n_tup_ins/upd/del da coleta.", notEstimated("latência de escrita medida"), "Meça latência de escrita e uso de cada índice antes de mudar o conjunto.", qIndexWrites, nil},
	{"index.investigate_missing", "query_and_index", "médio", "Uma consulta frequente lê muitos blocos e pode não ter índice adequado.", "calls e shared_blks_read do fingerprint em pg_stat_statements.", notEstimated("plano de execução representativo"), "Analise EXPLAIN (sem ANALYZE) em homologação; o fingerprint não define as colunas do índice.", qStatements, nil},
	{"index.candidate", "query_and_index", "médio", "Uma tabela recebe muitas varreduras sequenciais e pode se beneficiar de índice.", "seq_scan e seq_tup_read da tabela na coleta.", notEstimated("razão seq_scan/idx_scan dentro de uma janela conhecida"), "Identifique as consultas e os filtros antes de desenhar um índice.", qTableScans, nil},
	{"performance.lock_wait", "query_and_index", "médio", "Sessões aguardaram bloqueio no momento da coleta.", "pg_locks com granted = false e tempo de espera.", notEstimated("frequência das esperas em várias coletas"), "Identifique a transação bloqueadora e o horário; repita a coleta antes de mudar parâmetros.", qLocks, nil},
	{"performance.high_connections", "query_and_index", "médio", "O número de conexões chegou perto de max_connections.", "Contagem em pg_stat_activity comparada a max_connections.", notEstimated("série de conexões ao longo do tempo"), "Revise pool de conexões e sessões ociosas; não aumente max_connections sem medir memória.", qConnections, nil},
	{"performance.slow_query", "query_and_index", "médio", "Um fingerprint tem tempo médio de execução alto.", "mean_exec_time e calls de pg_stat_statements.", notEstimated("janela desde stats_reset e tempo total do banco"), "Compare o plano em janela representativa antes de criar índice ou reescrever a consulta.", qStatements, nil},
	{"performance.workload_scan", "query_and_index", "médio", "Uma consulta lê mais blocos do disco que do cache.", "shared_blks_read e shared_blks_hit do fingerprint, com a janela de observação.", notEstimated("blocos lidos, total de blocos acessados e janela de observação"), "Inspecione um plano representativo e os filtros usados pela aplicação.", qStatements, nil},
	{"performance.workload_write", "query_and_index", "médio", "Uma carga de escrita frequente pode elevar custo de índices e vacuum.", "calls de INSERT/UPDATE/DELETE do fingerprint.", notEstimated("latência de escrita e volume por janela"), "Avalie latência, índices e volume de alterações antes de agir.", qStatements, nil},
	{"performance.workload_cost", "query_and_index", "médio", "Uma consulta concentra tempo de execução acumulado.", "total_exec_time do fingerprint na janela das estatísticas.", notEstimated("tempo total do banco na mesma janela"), "Valide plano e frequência em ambiente autorizado.", qStatements, nil},
	// maintenance
	{"storage.large_table", "maintenance", "alto", "A relação passou do limite de tamanho da regra.", "pg_total_relation_size da coleta.", notEstimated("tamanho total do banco na mesma coleta"), "Compare crescimento em coletas completas e a retenção aprovada. Não proponha DROP nem particionamento automático.", qSize, costPartition},
	{"storage.top_consumer", "maintenance", "médio", "O objeto está entre os maiores consumidores de espaço do banco.", "pg_total_relation_size e posição no ranking da coleta.", notEstimated("tamanho total do banco na mesma coleta"), "Acompanhe a evolução e confirme quais dados precisam permanecer.", qSize, nil},
	{"vacuum.high_dead_tuples", "maintenance", "médio", "A relação acumulou tuplas mortas acima do limiar da regra.", "n_dead_tup e n_live_tup da coleta. Estimativa, não bloat medido.", notEstimated("n_dead_tup, n_live_tup e o instante do último (auto)vacuum"), "Confira autovacuum, transações longas e a carga de escrita. Não agende VACUUM a partir daqui.", qDeadTuples, costAutovacuum},
	{"maintenance.dead_tuple_pressure", "maintenance", "médio", "Tuplas mortas pressionam a manutenção da tabela.", "n_dead_tup e n_live_tup marcados como estimativa.", notEstimated("n_dead_tup, n_live_tup e o instante do último (auto)vacuum"), "Confirme a tendência e a configuração de autovacuum da tabela.", qDeadTuples, costAutovacuum},
	{"timescale.chunk_dead_tuples", "maintenance", "médio", "Um chunk amostrado acumulou tuplas mortas.", "n_dead_tup e n_live_tup do chunk (amostra limitada, não a hypertable inteira).", notEstimated("n_dead_tup, n_live_tup e o instante do último autovacuum do chunk"), "Revise autovacuum e escrita tardia nesse chunk.", qDeadTuples, costAutovacuum},
	{"maintenance.stale_analyze", "maintenance", "baixo", "As estatísticas do planejador estão mais antigas que o limite.", "last_analyze/last_autoanalyze da coleta.", notEstimated("n_mod_since_analyze e planos afetados"), "Confira n_mod_since_analyze e os limiares de autoanalyze antes de ajustar.", qAnalyze, costAnalyze},
	{"maintenance.growth_trend", "maintenance", "médio", "Duas coletas completas mostram mudança de tamanho.", "Tamanhos das duas coletas, janela e stats_reset. Tuplas mortas são estimativa.", notEstimated("duas coletas completas, denominador e janela"), "Confirme tendência, janela e backup. Não há DROP, DELETE nem retenção automática.", qGrowth, costPartition},
	{"chunk.high_count", "maintenance", "médio", "A hypertable tem muitos chunks.", "num_chunks de timescaledb_information.hypertables.", notEstimated("tempo de planejamento das consultas"), "Revise chunk_time_interval com a equipe; a mudança só vale para chunks novos.", qTSHypertable, nil},
	{"chunk.size_skew", "maintenance", "médio", "Os chunks têm tamanhos muito diferentes.", "Maior chunk comparado à média dos chunks coletados.", notEstimated("padrão de ingestão por período"), "Confirme o padrão de ingestão e cargas retroativas.", qTSChunks, nil},
	{"chunk.inventory_truncated", "maintenance", "baixo", "A coleta de chunks atingiu o limite configurado.", "Contagem de chunks igual ao teto da coleta.", "não estimado: o inventário está incompleto.", "Aumente a cobertura de forma controlada antes de concluir sobre todos os chunks.", qTSChunks, nil},
	{"cagg.missing_refresh_policy", "maintenance", "baixo", "O agregado contínuo não tem política de refresh.", "Nenhum job policy_refresh_continuous_aggregate ligado ao agregado.", notEstimated("frescor exigido pelos usuários"), "Confirme a frequência esperada antes de criar a política.", qTSCagg, nil},
	{"cagg.overlap", "maintenance", "médio", "Políticas de refresh trabalham sobre períodos sobrepostos.", "start_offset/end_offset das políticas do agregado.", notEstimated("duração dos jobs de refresh"), "Revise janelas e custo dos jobs antes de ajustá-las.", qTSCaggJobs, nil},
	{"cagg.materialization_lag", "maintenance", "médio", "O agregado contínuo está atrasado em relação aos dados.", "Último refresh comparado ao dado mais recente.", notEstimated("frescor exigido e duração do refresh"), "Compare atraso, política e necessidade dos usuários.", qTSCaggJobs, nil},
	{"cagg.realtime_hypothesis", "maintenance", "baixo", "O modo tempo real do agregado merece validação.", "materialized_only do agregado.", notEstimated("latência das consultas ao agregado"), "Confirme o frescor necessário e o custo das consultas.", qTSCagg, nil},
	{"cagg.refresh_window_exceeded", "maintenance", "médio", "O refresh do agregado dura mais que o intervalo agendado.", "last_run_duration comparado a schedule_interval.", notEstimated("série de durações do job"), "Revise volume, frequência e duração dos refreshes.", qTSCaggJobs, nil},
	{"policy.missing_retention", "maintenance", "médio", "A hypertable não tem política de retenção.", "Nenhum job policy_retention para a hypertable.", notEstimated("volume por período e prazo legal aprovado"), "Confirme o prazo de guarda com negócio/compliance. Retenção exige aprovação e backup restaurável.", qTSJobs, costRetention},
	{"policy.missing_compression", "maintenance", "médio", "A hypertable não tem política de compressão.", "compression_enabled e ausência de job policy_compression.", notEstimated("razão de compressão medida em amostra"), "Teste compressão em homologação com dados reais antes de configurar a política.", qTSHypertable, costCompression},
	{"policy.job_failed", "maintenance", "médio", "Um job de política falhou na última execução.", "last_run_status e total_failures de job_stats.", "não estimado: a falha é operacional, sem métrica de impacto.", "Leia o erro do job no log do servidor e confirme a próxima execução.", qTSJobs, nil},
	{"policy.retention_chunk_mismatch", "maintenance", "baixo", "O prazo de retenção não é múltiplo do intervalo dos chunks.", "drop_after da política comparado a chunk_time_interval.", notEstimated("dados retidos além do prazo"), "Compare períodos e prazo aprovado antes de ajustar.", qTSJobs, costRetention},
	{"policy.reorder_hypothesis", "maintenance", "baixo", "Uma política de reordenação pode ajudar consultas por faixa.", "Hypertable sem reorder com índice dominante.", notEstimated("planos das consultas dominantes"), "Confirme as consultas e o custo de manutenção antes de criar a política.", qTSJobs, costReorder},
	{"policy.compression_not_applied", "maintenance", "médio", "Chunks elegíveis continuam sem compressão.", "Chunks fechados com is_compressed = false e política existente.", notEstimated("bytes dos chunks não comprimidos"), "Confira agenda e falhas do job de compressão.", qTSChunks, costCompression},
	{"policy.compression_ratio", "maintenance", "médio", "A compressão observada está abaixo do esperado.", "Bytes antes e depois da compressão por hypertable.", notEstimated("razão por chunk e segmentby testado"), "Compare dados, segmentos e consultas antes de ajustar a política.", qTSHypertable, costCompression},
	{"policy.compression_settings", "maintenance", "médio", "segmentby/orderby podem não combinar com as consultas.", "Configuração de compressão comparada às colunas filtradas.", notEstimated("teste de configuração em homologação"), "Teste segmentação e ordenação em ambiente controlado.", qTSHypertable, costCompression},
	{"job.unhealthy", "maintenance", "médio", "Um job não está executando como esperado.", "scheduled, last_run_status e next_start do job.", "não estimado: o estado do job não tem métrica de impacto.", "Confira histórico, agenda e erros do job.", qTSJobByID, nil},
	{"job.slo_exceeded", "maintenance", "médio", "Um job ultrapassou a janela de execução esperada.", "last_run_duration comparado a schedule_interval.", notEstimated("série de durações"), "Compare duração, agenda e concorrência dos jobs.", qTSJobByID, nil},
	{"job.workers_saturated", "maintenance", "médio", "Os workers de background podem estar ocupados em excesso.", "Jobs agendados comparados a timescaledb.max_background_workers.", notEstimated("fila de jobs atrasados"), "Confira filas e limites antes de aumentar a concorrência.", qTSWorkers, nil},
	{"replication.lag_high", "maintenance", "alto", "A réplica está atrasada na aplicação do WAL.", "pg_wal_lsn_diff(sent_lsn, replay_lsn) em pg_stat_replication.", notEstimated("série de atraso e RPO acordado"), "Verifique rede, carga da réplica e conflitos de replay antes de mudar parâmetros.", qReplication, nil},
	{"replication.archive_stalled", "maintenance", "alto", "O arquivamento de WAL parou ou está falhando.", "last_archived_time e failed_count de pg_stat_archiver.", notEstimated("WAL acumulado e RPO acordado"), "Confira logs e destino do archive_command; o disco de WAL pode encher.", qArchiver, nil},
	// security
	{"security.excessive_privilege", "security", "alto", "Um papel tem privilégio mais amplo do que o uso conhecido da aplicação.", "Grants efetivos gravados na coleta. A API não executa SQL no alvo.", "não estimado: exposição não vira nota sem o privilégio e o papel.", "Confirme se o privilégio é operacional antes de qualquer revogação.", qTableGrants, nil},
	{"security.powerful_role", "security", "alto", "Um papel tem atributos elevados (superuser, createrole, bypassrls ou replication).", "Atributos de pg_roles na coleta.", "não estimado: o atributo não indica uso indevido por si.", "Liste quem usa o papel e justifique cada atributo.", qRole, nil},
	{"security.security_definer", "security", "alto", "Uma função executa com os privilégios do dono.", "prosecdef = true em pg_proc.", "não estimado: o risco depende do código e de quem pode executar.", "Revise código, dono, proacl e search_path da função.", qDefiner, nil},
	{"security.definer_search_path", "security", "alto", "Uma função SECURITY DEFINER não fixa search_path.", "proconfig sem search_path e definição sem SET search_path.", "não estimado: o risco depende de schemas graváveis por terceiros.", "Fixe search_path na função após revisão do dono.", qDefiner, nil},
	{"security.auditor_not_readonly", "security", "alto", "A conta de auditoria tem privilégio de escrita ou elevado.", "Atributos e associação a papéis de escrita da conta atual.", "não estimado: o auditor não escreve, mas a credencial vazada poderia.", "Troque a credencial por uma conta somente leitura dedicada.", qAuditorRole, nil},
	{"security.auditor_privilege_unknown", "security", "médio", "Não foi possível confirmar as permissões da conta de auditoria.", "A verificação de privilégios falhou ou foi negada.", "não estimado: sem leitura de pg_roles.", "Repita a verificação; não presuma que a conta seja somente leitura.", qAuditorRole, nil},
	{"security.rls_disabled_hypothesis", "security", "médio", "Uma tabela com dados por cliente pode exigir RLS.", "relrowsecurity = false e colunas de tenant na tabela.", "não estimado: depende da política de acesso.", "Confirme a política de acesso antes de habilitar RLS.", qRLS, nil},
	{"security.public_schema_create", "security", "médio", "PUBLIC pode criar objetos no schema.", "nspacl concede CREATE a PUBLIC.", "não estimado: depende de quem conecta ao banco.", "Confirme dependências de criação antes de restringir CREATE.", qPublicSchema, nil},
	{"security.account_inactive_review", "security", "baixo", "Uma conta expirou ou não apareceu ativa nas amostras.", "rolvaliduntil e amostras de pg_stat_activity entre coletas.", "não estimado: amostras não comprovam ausência de uso.", "Confirme o uso real com a equipe antes de desativar a conta.", qRoleMembers, nil},
	{"config.version_drift", "security", "médio", "Os ambientes comparados rodam versões diferentes do servidor.", "server_version de cada ambiente.", "não estimado: a diferença pode ser planejada.", "Confirme se a diferença é planejada e revise compatibilidade.", qVersion, nil},
	{"config.extension_drift", "security", "médio", "Extensões ou versões diferem entre ambientes.", "pg_extension de cada ambiente.", "não estimado: a diferença pode ser planejada.", "Confirme necessidade e versão antes de alinhar.", qExtensions, nil},
	{"config.guc_drift", "security", "médio", "Um parâmetro de configuração difere da referência.", "pg_settings de cada ambiente.", "não estimado: o efeito depende da carga.", "Confirme a intenção da diferença e o efeito antes de alinhar.", qSetting, nil},
}

// rulePlans is the only match table. Unknown ids use the investigation fallback.
var rulePlans = map[string]Plan{}

func init() {
	for _, e := range entries {
		p := base(e.category)
		p.Meaning, p.Evidence, p.Impact, p.Next = e.meaning, e.evidence, e.impact, e.next
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
