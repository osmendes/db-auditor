// Package postgres implements read-only PostgreSQL inventory collectors.
// Collectors emit facts only; analyzers interpret facts elsewhere.
package postgres

import "time"

const CollectorVersion = "1.1.0"

// ServerFacts is the output of the server collector.
type ServerFacts struct {
	ServerVersion      string    `json:"server_version"`
	StartedAt          time.Time `json:"started_at"`
	Uptime             string    `json:"uptime"`
	Timezone           string    `json:"timezone"`
	ServerEncoding     string    `json:"server_encoding"`
	MaxConnections     int       `json:"max_connections"`
	CurrentConnections int       `json:"current_connections"`
	Autovacuum         string    `json:"autovacuum"`
}

// DatabaseFacts is one row from the database collector.
type DatabaseFacts struct {
	Name             string `json:"database_name"`
	Owner            string `json:"owner_name"`
	Encoding         string `json:"encoding"`
	Collate          string `json:"collate_name"`
	CType            string `json:"ctype_name"`
	AllowConnections bool   `json:"allow_connections"`
	IsTemplate       bool   `json:"is_template"`
	SizeBytes        int64  `json:"size_bytes"`
	ConnectionCount  int    `json:"connection_count"`
}

// SchemaFacts is one row from the schema collector (current database).
type SchemaFacts struct {
	DatabaseName          string `json:"database_name"`
	SchemaName            string `json:"schema_name"`
	Owner                 string `json:"owner_name"`
	TableCount            int    `json:"table_count"`
	ViewCount             int    `json:"view_count"`
	MaterializedViewCount int    `json:"materialized_view_count"`
	SequenceCount         int    `json:"sequence_count"`
	FunctionCount         int    `json:"function_count"`
	SizeBytes             int64  `json:"size_bytes"`
	ACL                   string `json:"acl,omitempty"`
}

// TableFacts is one row from the table collector (current database).
type TableFacts struct {
	DatabaseName        string     `json:"database_name"`
	SchemaName          string     `json:"schema_name"`
	TableName           string     `json:"table_name"`
	Owner               string     `json:"owner_name"`
	Relkind             string     `json:"relkind"`
	RelationClass       string     `json:"relation_class"`
	IsPartition         bool       `json:"is_partition"`
	ParentSchemaName    *string    `json:"parent_schema_name,omitempty"`
	ParentTableName     *string    `json:"parent_table_name,omitempty"`
	PartitionBound      *string    `json:"partition_bound,omitempty"`
	TablespaceName      *string    `json:"tablespace_name,omitempty"`
	RelPersistence      string     `json:"relpersistence"`
	RelRowSecurity      bool       `json:"relrowsecurity"`
	RelForceRowSecurity bool       `json:"relforcerowsecurity"`
	TableComment        *string    `json:"table_comment,omitempty"`
	StorageParameters   []string   `json:"storage_parameters,omitempty"`
	DataSizeBytes       int64      `json:"data_size_bytes"`
	IndexSizeBytes      int64      `json:"index_size_bytes"`
	TotalSizeBytes      int64      `json:"total_size_bytes"`
	RowEstimate         int64      `json:"row_estimate"`
	NLiveTup            int64      `json:"n_live_tup"`
	NDeadTup            int64      `json:"n_dead_tup"`
	NTupIns             int64      `json:"n_tup_ins"`
	NTupUpd             int64      `json:"n_tup_upd"`
	NTupDel             int64      `json:"n_tup_del"`
	SeqScan             int64      `json:"seq_scan"`
	IdxScan             int64      `json:"idx_scan"`
	LastVacuum          *time.Time `json:"last_vacuum,omitempty"`
	LastAutovacuum      *time.Time `json:"last_autovacuum,omitempty"`
	LastAnalyze         *time.Time `json:"last_analyze,omitempty"`
	LastAutoanalyze     *time.Time `json:"last_autoanalyze,omitempty"`
	ColumnCount         int        `json:"column_count"`
	HasPrimaryKey       bool       `json:"has_primary_key"`
	StatsReset          *time.Time `json:"stats_reset,omitempty"`
}

// ColumnFacts is one row from the column collector (current database).
type ColumnFacts struct {
	DatabaseName       string  `json:"database_name"`
	SchemaName         string  `json:"schema_name"`
	TableName          string  `json:"table_name"`
	ColumnName         string  `json:"column_name"`
	OrdinalPosition    int     `json:"ordinal_position"`
	DataType           string  `json:"data_type"`
	IsNullable         bool    `json:"is_nullable"`
	ColumnDefault      *string `json:"column_default,omitempty"`
	IsGenerated        bool    `json:"is_generated"`
	IdentityGeneration *string `json:"identity_generation,omitempty"`
	CollationName      *string `json:"collation_name,omitempty"`
	Comment            *string `json:"comment,omitempty"`
}

// ColumnStatFacts contains only aggregate planner estimates. Raw values and
// most_common_vals are deliberately never queried or persisted.
type ColumnStatFacts struct {
	DatabaseName     string   `json:"database_name"`
	SchemaName       string   `json:"schema_name"`
	TableName        string   `json:"table_name"`
	ColumnName       string   `json:"column_name"`
	NullFraction     float64  `json:"null_fraction"`
	DistinctEstimate float64  `json:"distinct_estimate"`
	AverageWidth     int      `json:"average_width"`
	Correlation      *float64 `json:"correlation,omitempty"`
	Source           string   `json:"source"`
	Quality          string   `json:"quality"`
}

// WorkloadFacts is a privacy-safe pg_stat_statements aggregate. Query text is
// used transiently to derive a hash and object references, then discarded.
type WorkloadFacts struct {
	DatabaseName      string     `json:"database_name"`
	ExtensionVersion  string     `json:"extension_version"`
	QueryKind         string     `json:"query_kind"`
	QueryFingerprint  string     `json:"query_fingerprint"`
	QueryID           string     `json:"query_id"`
	Calls             int64      `json:"calls"`
	TotalExecTimeMS   float64    `json:"total_exec_time_ms"`
	MeanExecTimeMS    float64    `json:"mean_exec_time_ms"`
	RowsTotal         int64      `json:"rows_total"`
	SharedBlocksRead  int64      `json:"shared_blocks_read"`
	SharedBlocksHit   int64      `json:"shared_blocks_hit"`
	ReferencedObjects []string   `json:"referenced_objects"`
	StatsReset        *time.Time `json:"stats_reset,omitempty"`
	EvidenceQuality   string     `json:"evidence_quality"`
}

// IndexFacts is one row from the index collector (current database).
type IndexFacts struct {
	DatabaseName    string     `json:"database_name"`
	SchemaName      string     `json:"schema_name"`
	TableName       string     `json:"table_name"`
	IndexName       string     `json:"index_name"`
	IndexDefinition string     `json:"index_definition"`
	AccessMethod    string     `json:"access_method"`
	IsUnique        bool       `json:"is_unique"`
	IsPrimary       bool       `json:"is_primary"`
	SizeBytes       int64      `json:"size_bytes"`
	IdxScan         int64      `json:"idx_scan"`
	IdxTupRead      int64      `json:"idx_tup_read"`
	IdxTupFetch     int64      `json:"idx_tup_fetch"`
	StatsReset      *time.Time `json:"stats_reset,omitempty"`
	IsValid         bool       `json:"is_valid"`
	IsReady         bool       `json:"is_ready"`
	KeyColumns      []string   `json:"key_columns,omitempty"`
	IncludeColumns  []string   `json:"include_columns,omitempty"`
	UsageObserved   bool       `json:"usage_observed"`
	Predicate       string     `json:"predicate,omitempty"`
}

// ConstraintFacts is one row from the constraint collector.
type ConstraintFacts struct {
	DatabaseName         string   `json:"database_name"`
	SchemaName           string   `json:"schema_name"`
	TableName            string   `json:"table_name"`
	ConstraintName       string   `json:"constraint_name"`
	ConstraintType       string   `json:"constraint_type"`
	ConstraintDefinition string   `json:"constraint_definition"`
	IsValidated          bool     `json:"is_validated"`
	IsDeferrable         bool     `json:"is_deferrable"`
	IsDeferred           bool     `json:"is_deferred"`
	ConstrainedColumns   []string `json:"constrained_columns,omitempty"`
	ReferencedSchema     *string  `json:"referenced_schema_name,omitempty"`
	ReferencedTable      *string  `json:"referenced_table_name,omitempty"`
	ReferencedColumns    []string `json:"referenced_columns,omitempty"`
	FKUpdateAction       *string  `json:"fk_update_action,omitempty"`
	FKDeleteAction       *string  `json:"fk_delete_action,omitempty"`
	FKMatchType          *string  `json:"fk_match_type,omitempty"`
}

// ViewFacts is one row from the view collector (views and matviews).
type ViewFacts struct {
	DatabaseName    string `json:"database_name"`
	SchemaName      string `json:"schema_name"`
	ViewName        string `json:"view_name"`
	Owner           string `json:"owner_name"`
	Relkind         string `json:"relkind"`
	ViewDefinition  string `json:"view_definition"`
	SizeBytes       int64  `json:"size_bytes"`
	ColumnsJSON     string `json:"columns_json"`
	SecurityInvoker *bool  `json:"security_invoker"`
	SecurityBarrier *bool  `json:"security_barrier"`
	IsPopulated     *bool  `json:"is_populated"`
}

// FunctionFacts is one row from the function/procedure collector.
type FunctionFacts struct {
	DatabaseName       string     `json:"database_name"`
	SchemaName         string     `json:"schema_name"`
	FunctionName       string     `json:"function_name"`
	IdentityArguments  string     `json:"identity_arguments"`
	Owner              string     `json:"owner_name"`
	LanguageName       string     `json:"language_name"`
	IsSecurityDefiner  bool       `json:"is_security_definer"`
	Volatility         string     `json:"volatility"`
	ParallelSafety     string     `json:"parallel_safety"`
	Kind               string     `json:"kind"`
	FunctionDefinition string     `json:"function_definition"`
	Proconfig          string     `json:"proconfig,omitempty"`
	ReturnType         *string    `json:"return_type,omitempty"`
	SearchPathPinned   bool       `json:"search_path_pinned"`
	ExecuteRoles       []string   `json:"execute_roles,omitempty"`
	Calls              int64      `json:"calls"`
	TotalTimeMS        float64    `json:"total_time_ms"`
	SelfTimeMS         float64    `json:"self_time_ms"`
	StatsReset         *time.Time `json:"stats_reset,omitempty"`
	StatsObserved      bool       `json:"stats_observed"`
}

// ExtensionFacts is one row from the extension collector.
type ExtensionFacts struct {
	DatabaseName     string `json:"database_name"`
	ExtensionName    string `json:"extension_name"`
	ExtensionVersion string `json:"extension_version"`
	SchemaName       string `json:"schema_name"`
	IsRelocatable    bool   `json:"is_relocatable"`
}

// SequenceFacts is one row from the sequence collector (US-052).
type SequenceFacts struct {
	DatabaseName  string  `json:"database_name"`
	SchemaName    string  `json:"schema_name"`
	SequenceName  string  `json:"sequence_name"`
	DataType      *string `json:"data_type,omitempty"`
	StartValue    *string `json:"start_value,omitempty"`
	IncrementBy   *string `json:"increment_by,omitempty"`
	MaxValue      *string `json:"max_value,omitempty"`
	MinValue      *string `json:"min_value,omitempty"`
	Cycle         bool    `json:"cycle"`
	OwnedByTable  *string `json:"owned_by_table,omitempty"`
	OwnedByColumn *string `json:"owned_by_column,omitempty"`
	LastValue     *string `json:"last_value,omitempty"`
}

// TriggerFacts is one row from the trigger collector (US-052).
type TriggerFacts struct {
	DatabaseName      string  `json:"database_name"`
	SchemaName        string  `json:"schema_name"`
	TableName         string  `json:"table_name"`
	TriggerName       string  `json:"trigger_name"`
	Enabled           string  `json:"enabled"`
	Timing            *string `json:"timing,omitempty"`
	EventManipulation *string `json:"event_manipulation,omitempty"`
	ActionStatement   *string `json:"action_statement,omitempty"`
}

// PolicyFacts is one RLS policy row (US-053).
type PolicyFacts struct {
	DatabaseName string   `json:"database_name"`
	SchemaName   string   `json:"schema_name"`
	TableName    string   `json:"table_name"`
	PolicyName   string   `json:"policy_name"`
	Permissive   *string  `json:"permissive,omitempty"`
	Roles        []string `json:"roles,omitempty"`
	Cmd          *string  `json:"cmd,omitempty"`
	Qual         *string  `json:"qual,omitempty"`
	WithCheck    *string  `json:"with_check,omitempty"`
}

// PartialError records a non-fatal failure for one database during discovery.
type PartialError struct {
	Database string `json:"database"`
	Op       string `json:"op"`
	Message  string `json:"error"`
}
