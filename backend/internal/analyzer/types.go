// Package analyzer turns inventory snapshots into actionable findings.
package analyzer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Severity ranks finding impact.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Status is the triage state of a finding.
type Status string

const (
	StatusOpen         Status = "open"
	StatusAcknowledged Status = "acknowledged"
	StatusResolved     Status = "resolved"
	StatusSuppressed   Status = "suppressed"
)

// Finding is a diagnostic produced by an analyzer.
type Finding struct {
	ID             string         `json:"id,omitempty"`
	EnvironmentID  string         `json:"environment_id"`
	AuditRunID     string         `json:"audit_run_id,omitempty"`
	FindingType    string         `json:"finding_type"`
	RuleID         string         `json:"rule_id"`
	RuleVersion    string         `json:"rule_version"`
	Category       string         `json:"category"`
	Confidence     float64        `json:"confidence"`
	Impact         string         `json:"impact"`
	Risk           string         `json:"risk"`
	Recommendation string         `json:"recommendation"`
	Validation     string         `json:"validation"`
	References     []string       `json:"references"`
	RuleParameters map[string]any `json:"rule_parameters"`
	Severity       Severity       `json:"severity"`
	Status         Status         `json:"status"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	ObjectType     string         `json:"object_type,omitempty"`
	ObjectKey      string         `json:"object_key,omitempty"`
	DatabaseName   string         `json:"database_name,omitempty"`
	SchemaName     string         `json:"schema_name,omitempty"`
	ObjectName     string         `json:"object_name,omitempty"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	DedupKey       string         `json:"dedup_key"`
	FirstSeenAt    time.Time      `json:"first_seen_at,omitempty"`
	LastSeenAt     time.Time      `json:"last_seen_at,omitempty"`
	ResolvedAt     *time.Time     `json:"resolved_at,omitempty"`
	Notes          string         `json:"notes,omitempty"`
}

// SnapshotFacts is the read-only input analyzers consume.
type SnapshotFacts struct {
	EnvironmentID      string
	AuditRunID         string
	Engine             string
	RulePolicies       []RulePolicy
	Tables             []TableFact
	Columns            []ColumnFact
	Constraints        []ConstraintFact
	Sequences          []SequenceFact
	Indexes            []IndexFact
	PreviousIndexes    []IndexFact
	Fingerprints       []FingerprintFact
	Schemas            []SchemaACLFact
	ExpectsReplica     bool
	ReplayLagBytes     *int64
	LastArchived       *time.Time
	LastFailedArchive  *time.Time
	ArchiveFailedCount int64
	Hypertables        []HypertableFact
	Chunks             []ChunkFact
	ChunkStats         []ChunkStat
	ChunkVacuum        []ChunkVacuumFact
	ChunksTruncated    bool
	CAGGs              []CAGGFact
	Policies           []PolicyFact
	Jobs               []JobFact
	Activity           []ActivityFact
	// Sprint 9 — performance & security facts (demo or collector-fed).
	Vacuum      []VacuumFact
	Locks       []LockFact
	Connections []ConnectionFact
	QueryStats  []QueryStatFact
	Roles       []RoleFact
	Grants      []GrantFact
	Functions   []FunctionSecurityFact
	// P1 — snapshot-only drift inputs. Peers are other environments' latest success runs.
	ServerVersion    string
	TimescaleVersion string
	Extensions       []ExtensionFact
	Settings         map[string]string
	Peers            []ServerSide
	// Maintenance trend uses only complete, comparable collections.
	CollectionComplete bool
	PreviousComplete   bool
	PreviousTables     []TableFact
}

// TableFact is a minimal table size fact for storage analysis.
type TableFact struct {
	Database       string     `json:"database"`
	Schema         string     `json:"schema"`
	Name           string     `json:"name"`
	SizeBytes      int64      `json:"size_bytes"`
	RowEstimate    int64      `json:"row_estimate"`
	ColumnCount    int        `json:"column_count"`
	HasPrimaryKey  bool       `json:"has_primary_key"`
	IsPartition    bool       `json:"is_partition"`
	RelationClass  string     `json:"relation_class"`
	Comment        string     `json:"comment,omitempty"`
	SeqScan        int64      `json:"seq_scan"`
	NTupIns        int64      `json:"n_tup_ins"`
	NTupUpd        int64      `json:"n_tup_upd"`
	NTupDel        int64      `json:"n_tup_del"`
	LastAnalyze    *time.Time `json:"last_analyze,omitempty"`
	StatsReset     *time.Time `json:"stats_reset,omitempty"`
	CollectedAt    time.Time  `json:"collected_at,omitempty"`
	RelRowSecurity bool       `json:"relrowsecurity"`
}

type ColumnFact struct {
	Database  string `json:"database"`
	Schema    string `json:"schema"`
	TableName string `json:"table_name"`
	Name      string `json:"name"`
	DataType  string `json:"data_type"`
	Nullable  bool   `json:"nullable"`
	Default   string `json:"default,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

type ConstraintFact struct {
	Database          string   `json:"database"`
	Schema            string   `json:"schema"`
	TableName         string   `json:"table_name"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	Validated         bool     `json:"validated"`
	Columns           []string `json:"columns,omitempty"`
	ReferencedSchema  string   `json:"referenced_schema,omitempty"`
	ReferencedTable   string   `json:"referenced_table,omitempty"`
	ReferencedColumns []string `json:"referenced_columns,omitempty"`
}

type SequenceFact struct {
	Database      string `json:"database"`
	Schema        string `json:"schema"`
	Name          string `json:"name"`
	DataType      string `json:"data_type,omitempty"`
	OwnedByTable  string `json:"owned_by_table,omitempty"`
	OwnedByColumn string `json:"owned_by_column,omitempty"`
	// LastValue is pg_sequences.last_value. Nil when the sequence was never used.
	LastValue *int64 `json:"last_value,omitempty"`
	MaxValue  *int64 `json:"max_value,omitempty"`
}

type FingerprintFact struct {
	Hash string
	Text string
}

type SchemaACLFact struct {
	Database string
	Name     string
	ACLs     []string
}

// IndexFact carries scan counters when available.
type IndexFact struct {
	Database    string     `json:"database"`
	Schema      string     `json:"schema"`
	TableName   string     `json:"table_name"`
	IndexName   string     `json:"index_name"`
	IdxScan     int64      `json:"idx_scan"`
	SizeBytes   int64      `json:"size_bytes"`
	IsPrimary   bool       `json:"is_primary"`
	IsUnique    bool       `json:"is_unique"`
	Definition  string     `json:"definition"`
	KeyColumns  []string   `json:"key_columns,omitempty"`
	Predicate   string     `json:"predicate,omitempty"`
	IsValid     bool       `json:"is_valid"`
	IsReady     bool       `json:"is_ready"`
	HasValidity bool       `json:"has_validity"`
	CollectedAt time.Time  `json:"collected_at,omitempty"`
	StatsReset  *time.Time `json:"stats_reset,omitempty"`
}

// HypertableFact summarizes chunk topology.
type HypertableFact struct {
	Database                 string `json:"database"`
	Schema                   string `json:"schema"`
	Name                     string `json:"name"`
	NumChunks                int    `json:"num_chunks"`
	SizeBytes                int64  `json:"size_bytes"`
	ChunkInterval            string `json:"chunk_interval,omitempty"`
	UncompressedClosedChunks int    `json:"uncompressed_closed_chunks,omitempty"`
	ReadsOutsideTime         bool   `json:"reads_outside_time,omitempty"`
}

// ChunkVacuumFact is a capped sample of one chunk, not the whole hypertable.
type ChunkVacuumFact struct {
	Database       string     `json:"database"`
	Schema         string     `json:"schema"`
	Hypertable     string     `json:"hypertable"`
	Chunk          string     `json:"chunk"`
	DeadTuples     int64      `json:"dead_tuples"`
	LiveTuples     int64      `json:"live_tuples"`
	LastAutovacuum *time.Time `json:"last_autovacuum,omitempty"`
	LastAnalyze    *time.Time `json:"last_analyze,omitempty"`
}
type ChunkStat struct {
	Database       string `json:"database"`
	Schema         string `json:"schema"`
	HypertableName string `json:"hypertable_name"`
	Count          int    `json:"count"`
	MaxBytes       int64  `json:"max_bytes"`
	SumBytes       int64  `json:"sum_bytes"`
}

// ChunkFact is a single chunk size observation.
type ChunkFact struct {
	Database       string `json:"database"`
	Schema         string `json:"schema"`
	HypertableName string `json:"hypertable_name"`
	ChunkName      string `json:"chunk_name"`
	SizeBytes      int64  `json:"size_bytes"`
}

// CAGGFact describes a continuous aggregate.
type CAGGFact struct {
	Database                  string `json:"database"`
	Schema                    string `json:"schema"`
	ViewName                  string `json:"view_name"`
	MaterializationSchema     string `json:"materialization_schema"`
	MaterializationHypertable string `json:"materialization_hypertable"`
	MaterializedOnly          bool   `json:"materialized_only"`
	HasRefreshPolicy          bool   `json:"has_refresh_policy"`
	ViewDefinition            string `json:"view_definition"`
	Lag                       string `json:"lag,omitempty"`
	Hierarchical              bool   `json:"hierarchical,omitempty"`
	Realtime                  bool   `json:"realtime,omitempty"`
}

// PolicyFact describes a Timescale retention/compression/refresh policy.
type PolicyFact struct {
	Database         string  `json:"database"`
	JobID            int64   `json:"job_id"`
	PolicyType       string  `json:"policy_type"` // retention, compression, refresh, reorder
	ProcName         string  `json:"proc_name"`
	HypertableSchema string  `json:"hypertable_schema"`
	HypertableName   string  `json:"hypertable_name"`
	ScheduleInterval string  `json:"schedule_interval"`
	Config           string  `json:"config"`
	Scheduled        bool    `json:"scheduled"`
	LastRunStatus    string  `json:"last_run_status"`
	CompressionRatio float64 `json:"compression_ratio,omitempty"`
	SegmentBy        string  `json:"segmentby,omitempty"`
	OrderBy          string  `json:"orderby,omitempty"`
}

// JobFact describes a background job.
type JobFact struct {
	Database             string `json:"database"`
	JobID                int64  `json:"job_id"`
	Application          string `json:"application_name"`
	ProcName             string `json:"proc_name"`
	Scheduled            bool   `json:"scheduled"`
	LastRunStatus        string `json:"last_run_status"`
	TotalFailures        int64  `json:"total_failures"`
	ScheduleInterval     string `json:"schedule_interval,omitempty"`
	LastRunDuration      string `json:"last_run_duration,omitempty"`
	NextStart            string `json:"next_start,omitempty"`
	MaxBackgroundWorkers int    `json:"max_background_workers,omitempty"`
	ScheduledJobCount    int    `json:"scheduled_job_count,omitempty"`
}

// ActivityFact carries DML / temporal signals for inactivity analysis.
// Classification is always POSSIBLY_INACTIVE — never triggers deletion.
type ActivityFact struct {
	Database       string     `json:"database"`
	Schema         string     `json:"schema"`
	Name           string     `json:"name"`
	ObjectType     string     `json:"object_type"` // table, hypertable
	NLiveTup       int64      `json:"n_live_tup"`
	NTupIns        int64      `json:"n_tup_ins"`
	NTupUpd        int64      `json:"n_tup_upd"`
	NTupDel        int64      `json:"n_tup_del"`
	LastDataChange *time.Time `json:"last_data_change,omitempty"`
	MaxTimeValue   *time.Time `json:"max_time_value,omitempty"`
	DaysSinceDML   int        `json:"days_since_dml"`
}

// VacuumFact carries live/dead tuple stats for vacuum analysis.
type VacuumFact struct {
	Database       string `json:"database"`
	Schema         string `json:"schema"`
	Name           string `json:"name"`
	NLiveTup       int64  `json:"n_live_tup"`
	NDeadTup       int64  `json:"n_dead_tup"`
	LastVacuum     string `json:"last_vacuum,omitempty"`
	LastAutovacuum string `json:"last_autovacuum,omitempty"`
}

// LockFact is a waiting lock observation (no sensitive query text).
type LockFact struct {
	Database       string `json:"database"`
	Relation       string `json:"relation,omitempty"`
	Mode           string `json:"mode"`
	Granted        bool   `json:"granted"`
	WaitAgeSeconds int64  `json:"wait_age_seconds"`
	PID            int    `json:"pid,omitempty"`
}

// ConnectionFact summarizes connection pressure per database/role.
type ConnectionFact struct {
	Database       string `json:"database"`
	RoleName       string `json:"role_name,omitempty"`
	State          string `json:"state,omitempty"`
	Count          int    `json:"count"`
	MaxConnections int    `json:"max_connections,omitempty"`
}

// QueryStatFact holds sanitized query fingerprints only — never raw SQL with literals.
type QueryStatFact struct {
	Database          string     `json:"database"`
	QueryKind         string     `json:"query_kind"`
	ExtensionVersion  string     `json:"extension_version"`
	SharedBlocksRead  int64      `json:"shared_blocks_read"`
	SharedBlocksHit   int64      `json:"shared_blocks_hit"`
	QueryFingerprint  string     `json:"query_fingerprint"` // normalized, literals stripped
	Calls             int64      `json:"calls"`
	TotalExecTimeMs   float64    `json:"total_exec_time_ms"`
	MeanExecTimeMs    float64    `json:"mean_exec_time_ms"`
	Rows              int64      `json:"rows"`
	PgStatStatements  bool       `json:"pg_stat_statements"`
	ReferencedObjects []string   `json:"referenced_objects,omitempty"`
	EvidenceQuality   string     `json:"evidence_quality,omitempty"`
	StatsReset        *time.Time `json:"stats_reset,omitempty"`
	CollectedAt       time.Time  `json:"collected_at,omitempty"`
}

// RoleFact describes a database role for privilege review.
type RoleFact struct {
	Database             string     `json:"database"`
	RoleName             string     `json:"role_name"`
	Superuser            bool       `json:"superuser"`
	CreateDB             bool       `json:"createrole,omitempty"`
	CreateRole           bool       `json:"create_role,omitempty"`
	Login                bool       `json:"login"`
	Replication          bool       `json:"replication"`
	BypassRLS            bool       `json:"bypass_rls"`
	Current              bool       `json:"current,omitempty"`
	CanWrite             bool       `json:"can_write,omitempty"`
	PrivilegeCheckFailed bool       `json:"privilege_check_failed,omitempty"`
	ValidUntil           *time.Time `json:"valid_until,omitempty"`
	SampledActive        bool       `json:"sampled_active,omitempty"`
	PossiblyInactive     bool       `json:"possibly_inactive,omitempty"`
	CollectedAt          time.Time  `json:"collected_at,omitempty"`
}

// GrantFact is an ACL entry for review (no automatic REVOKE).
type GrantFact struct {
	Database   string `json:"database"`
	Schema     string `json:"schema,omitempty"`
	ObjectType string `json:"object_type"` // table, schema, database, function
	ObjectName string `json:"object_name"`
	Grantee    string `json:"grantee"`
	Privilege  string `json:"privilege"` // SELECT, INSERT, UPDATE, DELETE, ALL, ...
	Grantable  bool   `json:"grantable"`
}

// FunctionSecurityFact flags SECURITY DEFINER routines for manual review.
// SearchPathPinned is true when proconfig contains search_path= or the stored
// definition contains SET search_path. DefaultParameters are not used here.
type FunctionSecurityFact struct {
	Database           string `json:"database"`
	Schema             string `json:"schema"`
	FunctionName       string `json:"function_name"`
	IdentityArgs       string `json:"identity_arguments,omitempty"`
	IsSecurityDefiner  bool   `json:"is_security_definer"`
	Owner              string `json:"owner,omitempty"`
	Language           string `json:"language,omitempty"`
	FunctionDefinition string `json:"function_definition,omitempty"`
	Config             string `json:"config,omitempty"`
	SearchPathPinned   bool   `json:"search_path_pinned,omitempty"`
}

// Analyzer produces findings from snapshot facts.
type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, facts SnapshotFacts) ([]Finding, error)
}

// DedupKey builds a stable identity for first_seen/last_seen tracking.
func DedupKey(findingType, objectKey, title string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", findingType, objectKey, title)))
	return hex.EncodeToString(h[:16])
}

// EvidenceJSON marshals evidence for persistence.
func EvidenceJSON(ev map[string]any) []byte {
	if ev == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return []byte("{}")
	}
	return b
}
