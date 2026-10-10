export type NavigationSection =
  | "Dashboard"
  | "Documentação"
  | "Ambientes"
  | "Execuções"
  | "Relatórios"
  | "Inventário"
  | "Mapeamentos"
  | "Desvio de schema"
  | "Comparar"
  | "Findings"
  | "Performance"
  | "Segurança"
  | "Regras"
  | "Contas"
  | "Acompanhamento"
  | "Ações assistidas"
  | "Status";

export interface HealthResponse {
  status: string;
}

export interface ConnectionStatus {
  environment_id: string;
  environment_name: string;
  dsn_configured: boolean;
  reachable: boolean;
  latency_ms?: number;
  server_version?: string;
  error?: string;
}

export interface StatusRun {
  id: string;
  environment_id: string;
  profile: string;
  status: string;
  started_at: string;
  finished_at?: string | null;
}

export interface StatusResponse {
  service: string;
  version: string;
  time_utc: string;
  api_status: string;
  database_status: string;
  environments_count: number;
  recent_runs: StatusRun[];
  open_findings: number;
  failed_runs_recent: number;
  notes?: string[];
  storage?: {
    snapshots_bytes: number;
    report_jobs_bytes: number;
    pdf_artifacts_bytes: number;
    quality_history_bytes: number;
    action_history_bytes: number;
  };
}

export interface DashboardKPIs {
  generated_at_utc: string;
  environments: number;
  open_findings: number;
  critical_findings: number;
  high_findings: number;
  failed_runs_recent: number;
  successful_runs_recent: number;
  total_storage_bytes: number;
  databases: number | null;
  schemas: number | null;
  tables: number | null;
  inventory_status: "complete" | "partial" | "empty";
  hypertables: number;
  jobs_scheduled: number;
  policies: number;
  notes?: string[];
}

export interface StorageSeriesPoint {
  label: string;
  size_bytes: number;
  object_kind: string;
}

export interface StorageGrowthResponse {
  generated_at_utc: string;
  by_environment: StorageSeriesPoint[];
  by_database: StorageSeriesPoint[];
  top_consumers: StorageSeriesPoint[];
  series: RunTrendPoint[];
  notes?: string[];
}

export interface FindingTrendBucket {
  key: string;
  count: number;
}

export interface FindingsTrendResponse {
  generated_at_utc: string;
  by_severity: FindingTrendBucket[];
  by_status: FindingTrendBucket[];
  by_type: FindingTrendBucket[];
  total: number;
  series: RunTrendPoint[];
}

export interface RunTrendPoint {
  environment_id: string;
  environment_name: string;
  audit_run_id: string;
  at: string;
  status: "success" | "partial_success";
  profile: string;
  collector_version: string;
  rule_version?: string;
  coverage: "complete" | "partial";
  comparable: boolean;
  comparison_note?: string;
  size_bytes?: number;
  findings?: number;
  critical?: number;
  high?: number;
  score?: number;
  score_version?: string;
  score_confidence?: number;
  counter_reset?: boolean;
}

export interface FindingAction {
  finding_id: string;
  environment_id: string;
  source_run_id?: string;
  rule_id: string;
  target: string;
  severity: string;
  confidence: number;
  evidence: Record<string, unknown>;
  coverage: string;
  suggestion: string;
  plan: {
    category: string;
    expected_benefit: string;
    risk: string;
    prerequisites: string;
    confirmation: string;
    validation: string;
    false_positive_risk: string;
    read_only_query?: string;
  };
  status: string;
  owner: string;
  justification: string;
  result: string;
  updated_by?: string;
  updated_at?: string;
}

export interface ActionEvent {
  status: string;
  owner: string;
  justification: string;
  result: string;
  actor: string;
  recorded_at: string;
}

export interface AuditorAccount {
  id: string;
  username: string;
  role: "viewer" | "auditor" | "operator";
  active: boolean;
  environments: string[];
  updated_at: string;
}

export interface EngineCapability {
  name: string;
  applicable: boolean;
  reason?: string;
}

export interface EnvironmentCapabilities {
  environment_id: string;
  engine: string;
  audit_run_id?: string;
  items: EngineCapability[];
}

export interface QualityPlan {
  meaning: string;
  confirmation: string;
  external_steps: string;
  validation: string;
  risk: string;
}

export interface QualityIssue {
  id: string;
  check_kind: "null" | "duplicate" | "orphan" | "date_range" | "distribution";
  column_name: string;
  affected_rows: number;
  sampled_rows: number;
  status: string;
  owner: string;
  justification: string;
  result: string;
  updated_by: string;
  validation_scan_id?: string;
  plan: QualityPlan;
  previous_affected_rows?: number;
  comparable: boolean;
  comparison_note: string;
}

export interface QualityScan {
  id: string;
  environment_id: string;
  database: string;
  schema: string;
  table: string;
  sample_limit: number;
  sampled_rows: number;
  sample_method: string;
  sampling_note: string;
  actor: string;
  created_at: string;
  issues: QualityIssue[];
}

export interface QualityRequest {
  database: string;
  schema: string;
  table: string;
  limit: number;
  expected_non_null: string[];
  candidate_keys: string[];
  date_ranges: Array<{ column: string; from: string; to: string }>;
}

export interface ActionMeasurement {
  id: string;
  finding_id: string;
  before_run_id: string;
  after_run_id: string;
  metric:
    | "table_size_bytes"
    | "finding_observed"
    | "query_mean_latency_us"
    | "query_reads_per_1000_calls";
  before_value: number | null;
  after_value: number | null;
  comparable: boolean;
  comparison_note: string;
  hypothesis: string;
  window_note: string;
  workload_comparable: boolean;
  recorded_by: string;
  recorded_at: string;
}

export interface TrackedAction {
  finding_id: string;
  environment_id: string;
  title: string;
  severity: string;
  status: string;
  owner: string;
  result: string;
  recurrences: number;
  due_at?: string;
  latest_measurement?: ActionMeasurement;
  measurements: ActionMeasurement[];
  potential_reclaim_bytes?: number;
  estimate_note: string;
}

export interface AuditAnnotation {
  id: string;
  environment_id: string;
  audit_run_id?: string;
  kind: "deployment" | "maintenance" | "incident" | "note";
  note: string;
  actor: string;
  occurred_at: string;
  created_at: string;
}

export interface RegressionAlert {
  id: string;
  environment_id: string;
  baseline_run_id: string;
  audit_run_id: string;
  category: "structure" | "findings";
  evidence: Record<string, unknown>;
  status: "open" | "acknowledged";
  reason: string;
  created_at: string;
}

export interface JobHealthItem {
  environment_id: string;
  environment_name?: string;
  jobs_total: number;
  jobs_scheduled: number;
  policies_total: number;
}

export interface JobHealthResponse {
  generated_at_utc: string;
  items: JobHealthItem[];
}

export interface Environment {
  id: string;
  name: string;
  type: string;
  engine: string;
  discovery_mode: string;
  active: boolean;
  created_at: string;
  updated_at: string;
}

export interface EnvironmentsResponse {
  items: Environment[];
}

export interface PageMeta {
  limit: number;
  offset: number;
  total: number;
  has_more: boolean;
}

export interface PagedResponse<T> {
  items: T[];
  page: PageMeta;
}

export interface DatabaseSnapshot {
  id: string;
  audit_run_id: string;
  environment_id: string;
  database_name: string;
  owner_name: string | null;
  encoding: string | null;
  size_bytes: number;
  connection_count: number;
  allow_connections: boolean;
  is_template: boolean;
  collected_at: string;
}

export interface SchemaSnapshot {
  id: string;
  audit_run_id: string;
  environment_id: string;
  database_name: string;
  schema_name: string;
  owner_name: string | null;
  table_count: number;
  view_count: number;
  materialized_view_count: number;
  sequence_count: number;
  function_count: number;
  size_bytes: number;
  collected_at: string;
}

export interface TableSnapshot {
  id: string;
  audit_run_id: string;
  environment_id: string;
  database_name: string;
  schema_name: string;
  table_name: string;
  owner_name: string | null;
  relkind: string;
  /** table | partitioned_table | partition | foreign_table */
  relation_class?: string;
  is_partition?: boolean;
  total_size_bytes: number;
  data_size_bytes: number;
  index_size_bytes: number;
  row_estimate: number;
  column_count: number;
  has_primary_key: boolean;
  collected_at: string;
}

export interface TableHistoryPoint {
  bucket: string;
  audit_run_id: string;
  run_status: string;
  total_size_bytes: number;
  row_estimate: number;
  size_delta_bytes?: number;
  row_delta?: number;
  seq_scan_delta?: number;
  idx_scan_delta?: number;
  dml_delta?: number;
  stats_reset?: string | null;
  counters_reset: boolean;
  complete: boolean;
}

export interface ScopeHistoryPoint {
  bucket: string;
  scope: "environment" | "database" | "schema" | "table";
  label: string;
  total_size_bytes: number;
  row_estimate: number;
}

export interface ColumnStatSnapshot {
  column_name: string;
  null_fraction: number;
  distinct_estimate: number;
  average_width: number;
  correlation?: number | null;
  source: string;
  quality: string;
  collected_at: string;
}

export interface WorkloadSnapshot {
  query_fingerprint: string;
  query_kind: string;
  extension_version: string;
  calls: number;
  total_exec_time_ms: number;
  mean_exec_time_ms: number;
  rows_total: number;
  shared_blocks_read: number;
  shared_blocks_hit: number;
  stats_reset?: string;
  referenced_objects: string[];
  evidence_quality: string;
  collected_at: string;
}

export interface ColumnSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  table_name: string;
  column_name: string;
  ordinal_position: number;
  data_type: string;
  is_nullable: boolean;
  column_default: string | null;
  is_generated: boolean;
  identity_generation: string | null;
  collation_name: string | null;
  comment?: string | null;
  collected_at: string;
}

export interface ConstraintSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  table_name: string;
  constraint_name: string;
  constraint_type: string;
  constraint_definition: string;
  is_validated: boolean;
  is_deferrable: boolean;
  is_deferred: boolean;
  constrained_columns: string[];
  referenced_schema_name?: string | null;
  referenced_table_name?: string | null;
  referenced_columns?: string[];
}

export interface TableAssessment {
  version: number;
  run: { id: string; status: string; started_at: string; partial: boolean };
  table: TableSnapshot & {
    parent_schema_name?: string | null;
    parent_table_name?: string | null;
    tablespace_name?: string | null;
    persistence?: string | null;
    rls_enabled: boolean;
    rls_forced: boolean;
    comment?: string | null;
    storage_parameters: string[];
    n_live_tup: number;
    n_dead_tup: number;
    seq_scan: number;
    idx_scan: number;
  };
  summary: {
    columns: number;
    constraints: number;
    indexes: number;
    findings: number;
    grants: number;
    dependencies: number;
    triggers: number;
    rls_policies: number;
    score: number | null;
    score_status: string;
    score_version: string;
    score_confidence: number | null;
    score_factors: Array<{
      code: string;
      description: string;
      penalty: number;
    }>;
    score_missing_collectors: string[];
  };
  links: Record<string, string>;
}

export interface RelationshipNode {
  database: string;
  schema: string;
  table: string;
}
export interface RelationshipEdge {
  from: RelationshipNode;
  to: RelationshipNode;
  constraint_name: string;
  columns: string[];
  referenced_columns: string[];
}
export interface RelationshipGraph {
  nodes: RelationshipNode[];
  edges: RelationshipEdge[];
  truncated: boolean;
}
export interface GrantSnapshot {
  grantee: string;
  privileges: string[];
}
export interface DependencySnapshot {
  source_schema: string;
  source_name: string;
  source_kind: string;
  target_schema: string;
  target_name: string;
  target_kind: string;
}
export interface TriggerSnapshot {
  name: string;
  enabled: string;
  timing?: string;
  events?: string;
}
export interface RLSPolicySnapshot {
  name: string;
  command?: string;
  permissive?: string;
  roles: string[];
}

export interface IndexSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  table_name: string;
  index_name: string;
  access_method: string | null;
  is_unique: boolean;
  is_primary: boolean;
  size_bytes: number;
  idx_scan: number;
  collected_at: string;
}

export interface IndexHistoryPoint {
  audit_run_id: string;
  run_status: string;
  definition_fingerprint: string;
  size_bytes: number;
  idx_scan: number;
  idx_tup_read: number;
  idx_tup_fetch: number;
  stats_reset: string | null;
  usage_observed: boolean | null;
  collected_at: string;
}

export interface ViewSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  view_name: string;
  owner_name: string | null;
  relkind: string;
  size_bytes: number;
  collected_at: string;
}

export interface ViewDetailSnapshot extends ViewSnapshot {
  audit_run_id: string;
  environment_id: string;
  columns: { name: string; type: string; position: number }[] | null;
  security_invoker: boolean | null;
  security_barrier: boolean | null;
  is_populated: boolean | null;
  definition_fingerprint: string;
}

export interface FunctionSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  function_name: string;
  identity_arguments: string;
  owner_name: string | null;
  language_name: string | null;
  is_security_definer: boolean;
  kind: string | null;
  collected_at: string;
}

export interface HypertableSnapshot {
  id: string;
  audit_run_id: string;
  environment_id: string;
  database_name: string;
  schema_name: string;
  hypertable_name: string;
  owner_name: string | null;
  num_dimensions: number;
  num_chunks: number;
  compression_enabled: boolean;
  is_distributed: boolean;
  total_size_bytes: number;
  data_size_bytes: number;
  index_size_bytes: number;
  collected_at: string;
}

export interface HypertableDimensionDetail {
  dimension_number: number;
  column_name: string;
  column_type: string | null;
  dimension_type: string | null;
  time_interval: string | null;
  integer_interval: string | null;
  num_slices: number | null;
}

export interface HypertableChunkDetail {
  chunk_schema: string;
  chunk_name: string;
  range_start: string | null;
  range_end: string | null;
  is_compressed: boolean;
  total_size_bytes: number;
  index_size_bytes: number;
}

export interface HypertablePolicyDetail {
  job_id: number;
  policy_type: string;
  scheduled: boolean;
  schedule_interval: string | null;
  next_start: string | null;
  last_run_status: string | null;
  total_failures: number | null;
}

export interface HypertableJobDetail {
  job_id: number;
  application_name: string | null;
  proc_name: string | null;
  scheduled: boolean;
  schedule_interval: string | null;
  next_start: string | null;
  last_run_status: string | null;
  total_failures: number;
}

export interface HypertableHistoryPoint {
  audit_run_id: string;
  run_status: string;
  total_size_bytes: number;
  data_size_bytes: number;
  index_size_bytes: number;
  num_chunks: number;
  collected_at: string;
}

export interface DimensionSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  hypertable_name: string;
  dimension_number: number;
  column_name: string;
  column_type: string | null;
  dimension_type: string | null;
  time_interval: string | null;
  collected_at: string;
}

export interface ChunkSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  hypertable_name: string;
  chunk_schema: string;
  chunk_name: string;
  range_start: string | null;
  range_end: string | null;
  is_compressed: boolean;
  total_size_bytes: number;
  collected_at: string;
}

export interface CAGGSnapshot {
  id: string;
  database_name: string;
  schema_name: string;
  view_name: string;
  owner_name: string | null;
  materialization_schema: string | null;
  materialization_hypertable: string | null;
  materialized_only: boolean;
  compression_enabled: boolean;
  collected_at: string;
}

export interface CAGGRefreshPolicy {
  job_id: number;
  scheduled: boolean;
  schedule_interval: string | null;
  next_start: string | null;
  last_run_status: string | null;
  total_failures: number | null;
}

export interface CAGGHistoryPoint {
  audit_run_id: string;
  run_status: string;
  materialization_size_bytes: number | null;
  lag_interval: string;
  collected_at: string;
}

export interface JobSnapshot {
  id: string;
  database_name: string;
  job_id: number;
  application_name: string | null;
  proc_name: string | null;
  scheduled: boolean;
  schedule_interval: string | null;
  next_start: string | null;
  hypertable_schema: string | null;
  hypertable_name: string | null;
  collected_at: string;
}

export interface PolicySnapshot {
  id: string;
  database_name: string;
  job_id: number;
  policy_type: string;
  proc_name: string | null;
  hypertable_schema: string | null;
  hypertable_name: string | null;
  schedule_interval: string | null;
  scheduled: boolean;
  next_start: string | null;
  collected_at: string;
}

export interface AuditRun {
  id: string;
  environment_id: string;
  environment_name?: string;
  profile: string;
  status: string;
  service_version: string;
  collector_version: string;
  started_at: string;
  finished_at?: string | null;
  warnings: string[];
  errors: string[];
}

export interface CollectorRun {
  id: string;
  audit_run_id: string;
  collector_name: string;
  collector_version: string;
  status: string;
  started_at: string;
  finished_at?: string | null;
  rows_collected: number;
  warning?: string | null;
  error?: string | null;
}

export interface AuditRunCoverage {
  collector_name: string;
  database_name?: string;
  status: string;
  rows_collected: number;
  warning?: string | null;
  error?: string | null;
  collected_at: string;
}

export interface AuditBaseline {
  environment_id: string;
  database_name: string;
  schema_name: string;
  table_name: string;
  audit_run_id: string;
  selected_by: string;
  selected_at: string;
}

export interface BaselineComparison {
  environment_id: string;
  database_name: string;
  schema_name: string;
  table_name: string;
  baseline_run_id: string;
  audit_run_id: string;
  status: "complete" | "partial" | "incompatible";
  added_tables: number;
  removed_tables: number;
  changed_tables: number;
}

export interface ScopeAggregate {
  database_name: string;
  schema_name?: string;
  status: string;
  score: number | null;
  confidence: number;
  tables: number;
  missing_collectors: string[];
}

export interface EffectiveRule {
  rule_id: string;
  rule_version: string;
  category: string;
  enabled: boolean;
  confidence: number;
  effective_parameters: Record<string, unknown>;
  default_parameters: Record<string, unknown>;
}

export interface ScopeScore {
  version: string;
  status: string;
  score: number | null;
  confidence: number;
  categories: Array<{
    category: string;
    score: number;
    penalty: number;
    positive: number;
    findings: number;
  }>;
  missing_collectors: string[];
}

export interface ReportFilters {
  database?: string;
  schema?: string;
  table?: string;
  severity?: string;
  redaction?: "none" | "identifiers" | "strict";
}

export interface ReportJob {
  id: string;
  environment_id: string;
  audit_run_id: string;
  report_type: "executive" | "technical" | "table";
  filters: ReportFilters;
  requested_by: string;
  rule_version: string;
  status: "queued" | "running" | "success" | "failed" | "cancelled";
  attempts: number;
  error?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  expires_at: string;
  sha256?: string;
  size_bytes?: number;
}

export interface FindingEvent {
  id: string;
  finding_id: string;
  audit_run_id?: string;
  event_type: string;
  reason: string;
  recorded_at: string;
}

export interface AnalysisRun {
  audit_run_id: string;
  environment_id: string;
  status: string;
  analyzer_version: string;
  findings_produced: number;
  findings_saved: number;
  error?: string | null;
  started_at: string;
  finished_at?: string | null;
}

export interface SnapshotCompleteness {
  audit_run_id: string;
  run_status: string;
  completeness: "complete" | "partial" | "empty" | "unknown";
  failed_databases: number;
  failed_collectors: number;
  coverage_rows: number;
  analysis_status?: string | null;
  analysis_findings_produced: number;
  analysis_findings_saved: number;
  started_at?: string;
  finished_at?: string | null;
}

export interface ObjectMapping {
  id: string;
  source_environment_id: string;
  target_environment_id: string;
  source_database: string;
  source_schema: string;
  source_object_type: string;
  source_object_name: string;
  target_database: string;
  target_schema: string;
  target_object_type: string;
  target_object_name: string;
  relation_type: string;
  confidence: number;
  status: string;
  source_fingerprint?: string | null;
  target_fingerprint?: string | null;
  fingerprint_algorithm?: string | null;
  notes?: string | null;
  created_at: string;
  updated_at: string;
}

export interface MappingCandidate {
  source_database: string;
  source_schema: string;
  source_object_type: string;
  source_object_name: string;
  target_database: string;
  target_schema: string;
  target_object_type: string;
  target_object_name: string;
  relation_type: string;
  confidence: number;
  source_fingerprint?: string;
  target_fingerprint?: string;
}

export interface CompareObjectItem {
  object_type: string;
  key: string;
  name: string;
  fingerprint?: string;
  fields?: Record<string, string>;
}

export interface FieldDiff {
  field: string;
  source?: string;
  target?: string;
}

export interface ObjectDiff {
  object_type: string;
  object_key: string;
  status: string;
  source_name?: string;
  target_name?: string;
  source_fingerprint?: string;
  target_fingerprint?: string;
  field_diffs?: FieldDiff[];
}

export interface CompareSummary {
  match: number;
  drift: number;
  only_source: number;
  only_target: number;
  unknown: number;
  total: number;
}

export interface ServerCompareRow {
  kind: string;
  name: string;
  left: string;
  right: string;
  status: string;
  informational: boolean;
  detail?: string;
}

export interface ServerCompareReport {
  left_run_id: string;
  right_run_id: string;
  rows: ServerCompareRow[];
}

export interface CompareResult {
  source_run_id?: string;
  target_run_id?: string;
  summary: CompareSummary;
  objects: ObjectDiff[];
}

export interface Finding {
  id: string;
  environment_id: string;
  audit_run_id?: string | null;
  finding_type: string;
  rule_id?: string;
  rule_version?: string;
  category?: string;
  confidence?: number;
  impact?: string;
  risk?: string;
  recommendation?: string;
  friendly_meaning?: string;
  friendly_next?: string;
  validation?: string;
  references?: string[];
  rule_parameters?: Record<string, unknown>;
  severity: string;
  status: string;
  title: string;
  summary: string;
  object_type?: string;
  object_key?: string;
  database_name?: string;
  schema_name?: string;
  object_name?: string;
  evidence?: Record<string, unknown>;
  dedup_key: string;
  first_seen_at: string;
  last_seen_at: string;
  resolved_at?: string | null;
  recurrence_count?: number;
  suppression_reason?: string | null;
  suppressed_until?: string | null;
  assignee?: string;
  due_at?: string | null;
  superseded_by?: string | null;
  notes?: string | null;
  created_at: string;
  updated_at: string;
}

export interface AnalyzeResult {
  produced: number;
  saved: number;
  items: Finding[];
}

export interface ItemsResponse<T> {
  items: T[];
}

export type InventoryObjectKind =
  | "tables"
  | "hypertables"
  | "indexes"
  | "views"
  | "functions"
  | "caggs";
