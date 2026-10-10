/** Generated from backend/api/openapi.json. Do not edit. */
export const apiContractVersion = "0.1.0";

export type ApiPath =
  | "/api/v1/analytics/findings-trends"
  | "/api/v1/analytics/job-health"
  | "/api/v1/analytics/kpis"
  | "/api/v1/analytics/storage"
  | "/api/v1/audit-runs"
  | "/api/v1/audit-runs/{id}"
  | "/api/v1/audit-runs/{id}/analysis"
  | "/api/v1/audit-runs/{id}/cancel"
  | "/api/v1/audit-runs/{id}/collectors"
  | "/api/v1/audit-runs/{id}/coverage"
  | "/api/v1/audit-runs/{id}/databases/{database}/cancel"
  | "/api/v1/audit-runs/{id}/reprocess"
  | "/api/v1/auth/login"
  | "/api/v1/auth/logout"
  | "/api/v1/auth/me"
  | "/api/v1/auth/password"
  | "/api/v1/auth/totp"
  | "/api/v1/auth/totp/confirm"
  | "/api/v1/auth/totp/disable"
  | "/api/v1/auth/totp/enroll"
  | "/api/v1/auth/totp/require"
  | "/api/v1/auth/users"
  | "/api/v1/auth/users/{id}"
  | "/api/v1/auth/users/{id}/sessions/revoke"
  | "/api/v1/compare"
  | "/api/v1/environments"
  | "/api/v1/environments/connection-status"
  | "/api/v1/environments/{id}/actions"
  | "/api/v1/environments/{id}/actions/export"
  | "/api/v1/environments/{id}/annotations"
  | "/api/v1/environments/{id}/baseline"
  | "/api/v1/environments/{id}/baseline/comparisons"
  | "/api/v1/environments/{id}/capabilities"
  | "/api/v1/environments/{id}/chunks"
  | "/api/v1/environments/{id}/columns"
  | "/api/v1/environments/{id}/constraints"
  | "/api/v1/environments/{id}/continuous-aggregates"
  | "/api/v1/environments/{id}/continuous-aggregates/page"
  | "/api/v1/environments/{id}/databases"
  | "/api/v1/environments/{id}/dimensions"
  | "/api/v1/environments/{id}/findings-diff"
  | "/api/v1/environments/{id}/functions"
  | "/api/v1/environments/{id}/history"
  | "/api/v1/environments/{id}/hypertables"
  | "/api/v1/environments/{id}/hypertables/page"
  | "/api/v1/environments/{id}/indexes"
  | "/api/v1/environments/{id}/jobs"
  | "/api/v1/environments/{id}/policies"
  | "/api/v1/environments/{id}/quality-issues/{issue}"
  | "/api/v1/environments/{id}/quality-scans"
  | "/api/v1/environments/{id}/regressions"
  | "/api/v1/environments/{id}/regressions/{alert}/acknowledge"
  | "/api/v1/environments/{id}/reports"
  | "/api/v1/environments/{id}/reports/{job}"
  | "/api/v1/environments/{id}/reports/{job}/cancel"
  | "/api/v1/environments/{id}/reports/{job}/download"
  | "/api/v1/environments/{id}/reports/{job}/retry"
  | "/api/v1/environments/{id}/rules"
  | "/api/v1/environments/{id}/rules/{rule}"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/continuous-aggregates/{cagg}/dependencies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/continuous-aggregates/{cagg}/detail"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/continuous-aggregates/{cagg}/findings"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/continuous-aggregates/{cagg}/grants"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/continuous-aggregates/{cagg}/history"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/continuous-aggregates/{cagg}/refresh-policies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/functions/{function}/dependencies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/functions/{function}/detail"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/functions/{function}/findings"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/functions/{function}/grants"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/chunks"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/detail"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/dimensions"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/findings"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/grants"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/history"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/indexes"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/jobs"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/policies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/hypertables/{hypertable}/rls-policies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/indexes/{index}/detail"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/indexes/{index}/findings"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/indexes/{index}/history"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}/assessment"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}/dependencies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}/findings"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}/grants"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}/graph"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}/rls-policies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/tables/{table}/triggers"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/views/{view}/dependencies"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/views/{view}/detail"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/views/{view}/findings"
  | "/api/v1/environments/{id}/runs/{run}/databases/{database}/schemas/{schema}/views/{view}/grants"
  | "/api/v1/environments/{id}/runs/{run}/score"
  | "/api/v1/environments/{id}/runs/{run}/score/compare"
  | "/api/v1/environments/{id}/runs/{run}/scores"
  | "/api/v1/environments/{id}/schedules"
  | "/api/v1/environments/{id}/schemas"
  | "/api/v1/environments/{id}/snapshot-status"
  | "/api/v1/environments/{id}/tables"
  | "/api/v1/environments/{id}/tables/column-stats"
  | "/api/v1/environments/{id}/tables/history"
  | "/api/v1/environments/{id}/tables/workload"
  | "/api/v1/environments/{id}/views"
  | "/api/v1/finding-categories/{category}"
  | "/api/v1/findings"
  | "/api/v1/findings/analyze"
  | "/api/v1/findings/{id}"
  | "/api/v1/findings/{id}/action"
  | "/api/v1/findings/{id}/action/events"
  | "/api/v1/findings/{id}/action/measurements"
  | "/api/v1/findings/{id}/timeline"
  | "/api/v1/mappings"
  | "/api/v1/mappings/suggest"
  | "/api/v1/mappings/{id}"
  | "/api/v1/reports/findings"
  | "/api/v1/reports/inventory"
  | "/api/v1/server-compare"
  | "/api/v1/status";

export interface CAGGDetailSnapshot {
  id: string;
  environment_id: string;
  audit_run_id: string;
  database_name: string;
  schema_name: string;
  view_name: string;
  owner_name?: string | null;
  materialization_schema?: string | null;
  materialization_hypertable?: string | null;
  materialized_only: boolean;
  compression_enabled: boolean;
  finalized?: boolean | null;
  source_hypertable_schema?: string | null;
  source_hypertable_name?: string | null;
  bucket_interval?: string | null;
  lag_interval: string;
  definition_fingerprint: string;
  materialization_size_bytes?: number | null;
  collected_at: string;
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
  severity: string;
  status: string;
  title: string;
  summary: string;
  object_type?: string;
  object_key?: string;
  database_name?: string;
  schema_name?: string;
  object_name?: string;
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
  evidence?: Record<string, unknown>;
  references?: string[];
  rule_parameters?: Record<string, unknown>;
}

export interface FindingChange {
  change: string;
  object_key: string;
  title: string;
  severity: string;
  finding_id?: string;
}

export interface FunctionDetailSnapshot {
  id: string;
  environment_id: string;
  audit_run_id: string;
  database_name: string;
  schema_name: string;
  function_name: string;
  identity_arguments: string;
  owner_name?: string | null;
  language_name?: string | null;
  is_security_definer: boolean;
  volatility?: string | null;
  parallel_safety?: string | null;
  kind?: string | null;
  return_type?: string | null;
  search_path_pinned?: boolean | null;
  execute_role_count?: number | null;
  calls?: number | null;
  total_time_ms?: number | null;
  self_time_ms?: number | null;
  stats_reset?: string | null;
  stats_observed?: boolean | null;
  definition_fingerprint?: string;
  collected_at: string;
}

export interface HypertableDetailSnapshot {
  id: string;
  audit_run_id: string;
  environment_id: string;
  database_name: string;
  schema_name: string;
  hypertable_name: string;
  owner_name?: string | null;
  num_dimensions: number;
  num_chunks: number;
  compression_enabled: boolean;
  is_distributed: boolean;
  total_size_bytes: number;
  data_size_bytes: number;
  index_size_bytes: number;
  collected_at: string;
  base_table_observed: boolean;
}

export interface IndexDetailSnapshot {
  id: string;
  environment_id: string;
  audit_run_id: string;
  database_name: string;
  schema_name: string;
  table_name: string;
  table_owner_name?: string | null;
  index_name: string;
  access_method?: string | null;
  is_unique: boolean;
  is_primary: boolean;
  is_valid: boolean;
  is_ready: boolean;
  key_columns?: string[] | null;
  include_columns?: string[] | null;
  is_partial: boolean;
  definition_fingerprint?: string;
  predicate_fingerprint?: string;
  size_bytes: number;
  idx_scan: number;
  idx_tup_read: number;
  idx_tup_fetch: number;
  stats_reset?: string | null;
  usage_observed?: boolean | null;
  collected_at: string;
}

export interface ScopeScore {
  version: string;
  profile: string;
  collector_version: string;
  rule_version: string;
  environment_id: string;
  audit_run_id: string;
  database_name?: string;
  schema_name?: string;
  table_name?: string;
  status: string;
  score: number | null;
  confidence: number;
  explanation: string;
  categories: ScopeScoreCategory[];
  missing_collectors: string[];
}

export interface ScopeScoreCategory {
  category: string;
  weight: number;
  score: number;
  penalty: number;
  positive: number;
  findings: number;
}

export interface ViewDetailSnapshot {
  id: string;
  environment_id: string;
  audit_run_id: string;
  database_name: string;
  schema_name: string;
  view_name: string;
  owner_name?: string | null;
  relkind: string;
  size_bytes: number;
  columns?: Record<string, unknown>[] | null;
  security_invoker?: boolean | null;
  security_barrier?: boolean | null;
  is_populated?: boolean | null;
  definition_fingerprint?: string;
  collected_at: string;
}
