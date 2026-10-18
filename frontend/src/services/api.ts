import { networkApiError, toApiError } from "../lib/errors";
import type {
  ActionEvent,
  ActionMeasurement,
  AnalysisRun,
  AnalyzeResult,
  AuditAnnotation,
  AuditBaseline,
  AuditorAccount,
  AuditRun,
  AuditRunCoverage,
  BaselineComparison,
  CAGGHistoryPoint,
  CAGGRefreshPolicy,
  CAGGSnapshot,
  ChunkSnapshot,
  CollectorRun,
  ColumnSnapshot,
  ColumnStatSnapshot,
  CompareObjectItem,
  CompareResult,
  ConnectionStatus,
  ConstraintSnapshot,
  DashboardKPIs,
  DatabaseSnapshot,
  DependencySnapshot,
  DimensionSnapshot,
  EffectiveRule,
  EnvironmentCapabilities,
  EnvironmentsResponse,
  Finding,
  FindingAction,
  FindingEvent,
  FindingsTrendResponse,
  FunctionSnapshot,
  GrantSnapshot,
  HealthResponse,
  HypertableChunkDetail,
  HypertableDimensionDetail,
  HypertableHistoryPoint,
  HypertableJobDetail,
  HypertablePolicyDetail,
  HypertableSnapshot,
  IndexHistoryPoint,
  IndexSnapshot,
  ItemsResponse,
  JobHealthResponse,
  JobSnapshot,
  MappingCandidate,
  ObjectMapping,
  PagedResponse,
  PolicySnapshot,
  PrioritiesResponse,
  QualityRequest,
  QualityScan,
  RegressionAlert,
  RelationshipGraph,
  ReportFilters,
  ReportJob,
  RLSPolicySnapshot,
  SchemaSnapshot,
  ScopeAggregate,
  ScopeHistoryPoint,
  ServerCompareReport,
  SnapshotCompleteness,
  StatusResponse,
  StorageGrowthResponse,
  TableAssessment,
  TableHistoryPoint,
  TableSnapshot,
  TrackedAction,
  TriggerSnapshot,
  ViewDetailSnapshot,
  ViewSnapshot,
  WorkloadSnapshot,
} from "../types";
import {
  apiContractVersion,
  type CAGGDetailSnapshot,
  type FunctionDetailSnapshot,
  type HypertableDetailSnapshot,
  type IndexDetailSnapshot,
  type ScopeScore,
} from "../types/openapi";

const API_BASE = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/$/, "");

let reportToken = "";
let sessionActive = false;
let csrfToken = "";
let sessionRole = "";
let sessionUser = "";
let restorePromise: Promise<{ username: string; role: string } | null> | null =
  null;
const sessionChannel =
  typeof window !== "undefined" &&
  typeof document !== "undefined" &&
  typeof BroadcastChannel !== "undefined"
    ? new BroadcastChannel("db-auditor-session")
    : null;

function clearSession(): void {
  sessionActive = false;
  csrfToken = "";
  sessionRole = "";
  sessionUser = "";
}

function announceSessionRevocation(): void {
  sessionChannel?.postMessage("revoked");
  if (typeof window !== "undefined")
    window.dispatchEvent(new Event("auditor:session-expired"));
}

if (sessionChannel) {
  sessionChannel.onmessage = (event: MessageEvent) => {
    if (event.data !== "revoked") return;
    clearSession();
    if (typeof window !== "undefined")
      window.dispatchEvent(new Event("auditor:session-expired"));
  };
}

function authHeaders(method = "GET"): Record<string, string> {
  return sessionActive && method !== "GET" && method !== "HEAD"
    ? { "X-CSRF-Token": csrfToken }
    : {};
}

function handleSessionExpiry(response: Response, path: string): void {
  if (
    response.status === 401 &&
    !path.startsWith("/api/v1/auth/login") &&
    sessionActive
  ) {
    clearSession();
    announceSessionRevocation();
  }
}

async function reportRequest<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  if (!reportToken && !sessionActive)
    throw new Error("Informe o token de relatórios para esta sessão.");
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      headers: {
        ...(!sessionActive && reportToken
          ? { Authorization: `Bearer ${reportToken}` }
          : {}),
        ...authHeaders(method),
        ...(body ? { "Content-Type": "application/json" } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
      cache: "no-store",
      credentials: "include",
      signal,
    });
  } catch (cause) {
    throw networkApiError(path, cause);
  }
  if (!response.ok) {
    handleSessionExpiry(response, path);
    throw await toApiError(response, path);
  }
  return response.json() as Promise<T>;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      headers: authHeaders(),
      credentials: "include",
      signal,
    });
  } catch (cause) {
    throw networkApiError(path, cause);
  }
  if (!response.ok) {
    handleSessionExpiry(response, path);
    throw await toApiError(response, path);
  }
  return response.json() as Promise<T>;
}

async function putJSON<T>(path: string, body: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...authHeaders("PUT") },
      body: JSON.stringify(body),
      credentials: "include",
    });
  } catch (cause) {
    throw networkApiError(path, cause);
  }
  if (!response.ok) {
    handleSessionExpiry(response, path);
    throw await toApiError(response, path);
  }
  return response.json() as Promise<T>;
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders("POST") },
      body: JSON.stringify(body),
      credentials: "include",
    });
  } catch (cause) {
    throw networkApiError(path, cause);
  }
  if (!response.ok) {
    handleSessionExpiry(response, path);
    throw await toApiError(response, path);
  }
  return response.json() as Promise<T>;
}

async function patchJSON<T>(path: string, body: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", ...authHeaders("PATCH") },
      body: JSON.stringify(body),
      credentials: "include",
    });
  } catch (cause) {
    throw networkApiError(path, cause);
  }
  if (!response.ok) {
    handleSessionExpiry(response, path);
    throw await toApiError(response, path);
  }
  return response.json() as Promise<T>;
}

function qs(params?: Record<string, string | number | undefined>): string {
  if (!params) {
    return "";
  }
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") {
      q.set(k, String(v));
    }
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}

function tableScopePath(
  env: string,
  run: string,
  database: string,
  schema: string,
  table: string,
): string {
  return `/api/v1/environments/${encodeURIComponent(env)}/runs/${encodeURIComponent(run)}/databases/${encodeURIComponent(database)}/schemas/${encodeURIComponent(schema)}/tables/${encodeURIComponent(table)}`;
}

function viewScopePath(
  env: string,
  run: string,
  database: string,
  schema: string,
  view: string,
): string {
  return `/api/v1/environments/${encodeURIComponent(env)}/runs/${encodeURIComponent(run)}/databases/${encodeURIComponent(database)}/schemas/${encodeURIComponent(schema)}/views/${encodeURIComponent(view)}`;
}

function indexScopePath(
  env: string,
  run: string,
  database: string,
  schema: string,
  index: string,
): string {
  return `/api/v1/environments/${encodeURIComponent(env)}/runs/${encodeURIComponent(run)}/databases/${encodeURIComponent(database)}/schemas/${encodeURIComponent(schema)}/indexes/${encodeURIComponent(index)}`;
}

function functionScopePath(
  env: string,
  run: string,
  database: string,
  schema: string,
  name: string,
): string {
  return `/api/v1/environments/${encodeURIComponent(env)}/runs/${encodeURIComponent(run)}/databases/${encodeURIComponent(database)}/schemas/${encodeURIComponent(schema)}/functions/${encodeURIComponent(name)}`;
}

function hypertableScopePath(
  env: string,
  run: string,
  database: string,
  schema: string,
  name: string,
): string {
  return `/api/v1/environments/${encodeURIComponent(env)}/runs/${encodeURIComponent(run)}/databases/${encodeURIComponent(database)}/schemas/${encodeURIComponent(schema)}/hypertables/${encodeURIComponent(name)}`;
}

function caggScopePath(
  env: string,
  run: string,
  database: string,
  schema: string,
  name: string,
): string {
  return `/api/v1/environments/${encodeURIComponent(env)}/runs/${encodeURIComponent(run)}/databases/${encodeURIComponent(database)}/schemas/${encodeURIComponent(schema)}/continuous-aggregates/${encodeURIComponent(name)}`;
}

export type InventoryListParams = {
  audit_run_id?: string;
  limit?: number;
  offset?: number;
  q?: string;
  database?: string;
  schema?: string;
  table?: string;
  order_by?: string;
};

export type AnalyticsParams = {
  environment_id?: string;
  from?: string;
  to?: string;
  granularity?: "hour" | "day" | "week" | "month";
};

export type TableScopeParams = {
  database: string;
  schema: string;
  table: string;
  from?: string;
  to?: string;
  granularity?: "hour" | "day" | "week" | "month";
};

/** Typed API client — frontend never talks to databases directly. */
export const api = {
  contractVersion: apiContractVersion,
  actionExportURL: (environmentId: string, format: "csv" | "jsonl") =>
    `${API_BASE}/api/v1/environments/${encodeURIComponent(environmentId)}/actions/export?format=${format}`,
  hasSession: () => sessionActive,
  currentUser: () => sessionUser,
  restoreSession: () => {
    if (restorePromise) return restorePromise;
    restorePromise = (async () => {
      try {
        const result = await getJSON<{
          user: { username: string; role: string };
          csrf_token: string;
        }>("/api/v1/auth/me");
        sessionActive = true;
        csrfToken = result.csrf_token;
        sessionRole = result.user.role;
        sessionUser = result.user.username;
        return result.user;
      } catch (error) {
        if (
          error &&
          typeof error === "object" &&
          "status" in error &&
          error.status === 401
        ) {
          clearSession();
          return null;
        }
        throw error;
      }
    })().finally(() => {
      restorePromise = null;
    });
    return restorePromise;
  },
  hasRole: (minimum: "auditor" | "operator") =>
    ({ viewer: 1, auditor: 2, operator: 3 })[
      sessionRole as "viewer" | "auditor" | "operator"
    ] >= { auditor: 2, operator: 3 }[minimum],
  login: async (username: string, password: string) => {
    const result = await postJSON<{
      user?: { username: string; role: string };
      csrf_token?: string;
      mfa_required?: boolean;
      mfa_token?: string;
    }>("/api/v1/auth/login?mode=cookie", { username, password });
    if (result.mfa_required) {
      return { mfa_required: true as const, mfa_token: result.mfa_token ?? "" };
    }
    sessionActive = true;
    csrfToken = result.csrf_token ?? "";
    sessionRole = result.user?.role ?? "";
    sessionUser = result.user?.username ?? "";
    return result.user ?? { username, role: "" };
  },
  confirmTotp: async (mfaToken: string, code: string) => {
    const result = await postJSON<{
      user: { username: string; role: string };
      csrf_token: string;
    }>("/api/v1/auth/totp?mode=cookie", { mfa_token: mfaToken, code });
    sessionActive = true;
    csrfToken = result.csrf_token;
    sessionRole = result.user.role;
    sessionUser = result.user.username;
    return result.user;
  },
  enrollTotp: () =>
    postJSON<{ secret: string; otpauth_uri: string; recovery_codes: string[] }>(
      "/api/v1/auth/totp/enroll",
      {},
    ),
  confirmTotpEnrollment: (code: string) =>
    postJSON<{ totp_enabled: boolean }>("/api/v1/auth/totp/confirm", { code }),
  requireTotp: (required: boolean) =>
    postJSON<{ totp_required: boolean }>("/api/v1/auth/totp/require", {
      required,
    }),
  disableTotp: (password: string, code: string) =>
    postJSON<{ totp_enabled: boolean }>("/api/v1/auth/totp/disable", {
      password,
      code,
    }),
  findingsDiff: (
    environmentId: string,
    from: string,
    to: string,
    change = "",
    signal?: AbortSignal,
  ) =>
    getJSON<{
      items: {
        change: string;
        object_key: string;
        title: string;
        severity: string;
        finding_id?: string;
      }[];
    }>(
      `/api/v1/environments/${environmentId}/findings-diff?from=${from}&to=${to}${change ? `&change=${change}` : ""}`,
      signal,
    ),
  logout: async () => {
    try {
      if (sessionActive) await postJSON("/api/v1/auth/logout", {});
    } finally {
      clearSession();
      reportToken = "";
      announceSessionRevocation();
    }
  },
  changePassword: async (current: string, next: string) => {
    const result = await postJSON<{ status: string; next: string }>(
      "/api/v1/auth/password",
      { current, new: next },
    );
    clearSession();
    announceSessionRevocation();
    return result;
  },
  accounts: (signal?: AbortSignal) =>
    getJSON<ItemsResponse<AuditorAccount>>("/api/v1/auth/users", signal),
  createAccount: (body: {
    username: string;
    password: string;
    role: AuditorAccount["role"];
    environments: string[];
  }) => postJSON<{ user: AuditorAccount }>("/api/v1/auth/users", body),
  updateAccount: (
    id: string,
    body: {
      role: AuditorAccount["role"];
      active: boolean;
      environments: string[];
      new_password?: string;
    },
  ) => patchJSON<{ user: AuditorAccount }>(`/api/v1/auth/users/${id}`, body),
  revokeAccountSessions: (id: string) =>
    postJSON<{ status: string }>(
      `/api/v1/auth/users/${id}/sessions/revoke`,
      {},
    ),
  setReportToken: (value: string) => {
    reportToken = value;
  },
  reportJobs: (environmentId: string, signal?: AbortSignal) =>
    reportRequest<ItemsResponse<ReportJob>>(
      `/api/v1/environments/${environmentId}/reports`,
      "GET",
      undefined,
      signal,
    ),
  requestPDFReport: (
    environmentId: string,
    auditRunId: string,
    reportType: ReportJob["report_type"],
    filters: ReportFilters,
  ) =>
    reportRequest<ReportJob>(
      `/api/v1/environments/${environmentId}/reports`,
      "POST",
      {
        audit_run_id: auditRunId,
        report_type: reportType,
        format: reportType === "architecture" ? "markdown" : "pdf",
        filters,
      },
    ),
  cancelPDFReport: (environmentId: string, id: string) =>
    reportRequest<{ status: string }>(
      `/api/v1/environments/${environmentId}/reports/${id}/cancel`,
      "POST",
    ),
  retryPDFReport: (environmentId: string, id: string) =>
    reportRequest<ReportJob>(
      `/api/v1/environments/${environmentId}/reports/${id}/retry`,
      "POST",
    ),
  downloadPDFReport: async (
    environmentId: string,
    id: string,
  ): Promise<Blob> => {
    if (!reportToken && !sessionActive)
      throw new Error("Informe o token de relatórios para esta sessão.");
    const path = `/api/v1/environments/${environmentId}/reports/${id}/download`;
    const response = await fetch(`${API_BASE}${path}`, {
      headers:
        !sessionActive && reportToken
          ? { Authorization: `Bearer ${reportToken}` }
          : {},
      cache: "no-store",
      credentials: "include",
    });
    if (!response.ok) {
      handleSessionExpiry(response, path);
      throw await toApiError(response, path);
    }
    return new Blob([await response.arrayBuffer()], {
      type: response.headers.get("Content-Type") ?? "application/octet-stream",
    });
  },
  architectureDecisions: (
    environmentId: string,
    database: string,
    signal?: AbortSignal,
  ) =>
    reportRequest<{
      items: Array<{
        object_ref: string;
        subject: string;
        value: string;
        version: number;
        author: string;
        created_at: string;
      }>;
    }>(
      `/api/v1/environments/${environmentId}/reports/architecture-decisions?database=${encodeURIComponent(database)}&history=true`,
      "GET",
      undefined,
      signal,
    ),
  saveArchitectureDecision: (
    environmentId: string,
    input: {
      database: string;
      object_ref: string;
      subject: string;
      value: string;
    },
  ) =>
    reportRequest<{
      object_ref: string;
      subject: string;
      value: string;
      version: number;
      author: string;
      created_at: string;
    }>(
      `/api/v1/environments/${environmentId}/reports/architecture-decisions`,
      "POST",
      input,
    ),
  health: (signal?: AbortSignal) => getJSON<HealthResponse>("/health", signal),
  ready: (signal?: AbortSignal) => getJSON<HealthResponse>("/ready", signal),
  status: (signal?: AbortSignal) =>
    getJSON<StatusResponse>("/api/v1/status", signal),
  connectionStatus: (signal?: AbortSignal) =>
    getJSON<ItemsResponse<ConnectionStatus>>(
      "/api/v1/environments/connection-status",
      signal,
    ),
  analyticsKpis: (params?: AnalyticsParams, signal?: AbortSignal) =>
    getJSON<DashboardKPIs>(`/api/v1/analytics/kpis${qs(params)}`, signal),
  analyticsStorage: (params?: AnalyticsParams, signal?: AbortSignal) =>
    getJSON<StorageGrowthResponse>(
      `/api/v1/analytics/storage${qs(params)}`,
      signal,
    ),
  analyticsFindingsTrends: (params?: AnalyticsParams, signal?: AbortSignal) =>
    getJSON<FindingsTrendResponse>(
      `/api/v1/analytics/findings-trends${qs(params)}`,
      signal,
    ),
  analyticsJobHealth: (params?: AnalyticsParams, signal?: AbortSignal) =>
    getJSON<JobHealthResponse>(
      `/api/v1/analytics/job-health${qs(params)}`,
      signal,
    ),
  reportInventory: (params?: AnalyticsParams, signal?: AbortSignal) =>
    getJSON<Record<string, unknown>>(
      `/api/v1/reports/inventory${qs(params)}`,
      signal,
    ),
  reportFindings: (params?: AnalyticsParams, signal?: AbortSignal) =>
    getJSON<Record<string, unknown>>(
      `/api/v1/reports/findings${qs(params)}`,
      signal,
    ),
  environments: (signal?: AbortSignal) =>
    getJSON<EnvironmentsResponse>("/api/v1/environments", signal),
  createEnvironmentLabel: (name: string) =>
    postJSON<{ id: string; name: string; dsn: null }>("/api/v1/environments", {
      name,
    }),
  environmentCapabilities: (env: string, signal?: AbortSignal) =>
    getJSON<EnvironmentCapabilities>(
      `/api/v1/environments/${env}/capabilities`,
      signal,
    ),
  qualityScans: (env: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<QualityScan> & { enabled: boolean }>(
      `/api/v1/environments/${env}/quality-scans`,
      signal,
    ),
  runQualityScan: (env: string, body: QualityRequest) =>
    postJSON<QualityScan>(`/api/v1/environments/${env}/quality-scans`, body),
  updateQualityIssue: (
    env: string,
    issue: string,
    body: {
      status: string;
      owner: string;
      justification: string;
      result: string;
    },
  ) =>
    patchJSON<QualityScan>(
      `/api/v1/environments/${env}/quality-issues/${issue}`,
      body,
    ),
  trackedActions: (env: string, offset = 0, limit = 50, signal?: AbortSignal) =>
    getJSON<PagedResponse<TrackedAction>>(
      `/api/v1/environments/${env}/actions?offset=${offset}&limit=${limit}`,
      signal,
    ),
  databases: (
    environmentId: string,
    auditRunId?: string,
    signal?: AbortSignal,
  ) =>
    getJSON<ItemsResponse<DatabaseSnapshot>>(
      `/api/v1/environments/${environmentId}/databases${auditRunId ? `?audit_run_id=${encodeURIComponent(auditRunId)}` : ""}`,
      signal,
    ),
  schemas: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<SchemaSnapshot>>(
      `/api/v1/environments/${environmentId}/schemas`,
      signal,
    ),
  snapshotStatus: (
    environmentId: string,
    auditRunId?: string,
    signal?: AbortSignal,
  ) =>
    getJSON<SnapshotCompleteness>(
      `/api/v1/environments/${environmentId}/snapshot-status${qs({
        audit_run_id: auditRunId,
      })}`,
      signal,
    ),
  hypertables: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<HypertableSnapshot>>(
      `/api/v1/environments/${environmentId}/hypertables`,
      signal,
    ),
  hypertablesPage: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<HypertableSnapshot>>(
      `/api/v1/environments/${environmentId}/hypertables/page${qs(params)}`,
      signal,
    ),
  hypertableDetail: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    signal?: AbortSignal,
  ) =>
    getJSON<HypertableDetailSnapshot>(
      `${hypertableScopePath(env, run, database, schema, name)}/detail`,
      signal,
    ),
  hypertableDimensions: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<HypertableDimensionDetail>>(
      `${hypertableScopePath(env, run, database, schema, name)}/dimensions${qs({ limit, offset })}`,
      signal,
    ),
  hypertableChunks: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<HypertableChunkDetail>>(
      `${hypertableScopePath(env, run, database, schema, name)}/chunks${qs({ limit, offset })}`,
      signal,
    ),
  hypertablePolicies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<HypertablePolicyDetail>>(
      `${hypertableScopePath(env, run, database, schema, name)}/policies${qs({ limit, offset })}`,
      signal,
    ),
  hypertableJobs: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<HypertableJobDetail>>(
      `${hypertableScopePath(env, run, database, schema, name)}/jobs${qs({ limit, offset })}`,
      signal,
    ),
  hypertableHistory: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<HypertableHistoryPoint>>(
      `${hypertableScopePath(env, run, database, schema, name)}/history${qs({ limit, offset })}`,
      signal,
    ),
  hypertableIndexes: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<IndexSnapshot>>(
      `${hypertableScopePath(env, run, database, schema, name)}/indexes${qs({ limit, offset })}`,
      signal,
    ),
  hypertableGrants: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<GrantSnapshot>>(
      `${hypertableScopePath(env, run, database, schema, name)}/grants${qs({ limit, offset })}`,
      signal,
    ),
  hypertableRLSPolicies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<RLSPolicySnapshot>>(
      `${hypertableScopePath(env, run, database, schema, name)}/rls-policies${qs({ limit, offset })}`,
      signal,
    ),
  hypertableFindings: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<Finding>>(
      `${hypertableScopePath(env, run, database, schema, name)}/findings${qs({ limit, offset })}`,
      signal,
    ),
  dimensions: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<DimensionSnapshot>>(
      `/api/v1/environments/${environmentId}/dimensions`,
      signal,
    ),
  chunks: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<ChunkSnapshot>>(
      `/api/v1/environments/${environmentId}/chunks`,
      signal,
    ),
  continuousAggregates: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<CAGGSnapshot>>(
      `/api/v1/environments/${environmentId}/continuous-aggregates`,
      signal,
    ),
  continuousAggregatesPage: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<CAGGSnapshot>>(
      `/api/v1/environments/${environmentId}/continuous-aggregates/page${qs(params)}`,
      signal,
    ),
  caggDetail: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    signal?: AbortSignal,
  ) =>
    getJSON<CAGGDetailSnapshot>(
      `${caggScopePath(env, run, database, schema, name)}/detail`,
      signal,
    ),
  caggRefreshPolicies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<CAGGRefreshPolicy>>(
      `${caggScopePath(env, run, database, schema, name)}/refresh-policies${qs({ limit, offset })}`,
      signal,
    ),
  caggHistory: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<CAGGHistoryPoint>>(
      `${caggScopePath(env, run, database, schema, name)}/history${qs({ limit, offset })}`,
      signal,
    ),
  caggDependencies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<DependencySnapshot>>(
      `${caggScopePath(env, run, database, schema, name)}/dependencies${qs({ limit, offset })}`,
      signal,
    ),
  caggGrants: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<GrantSnapshot>>(
      `${caggScopePath(env, run, database, schema, name)}/grants${qs({ limit, offset })}`,
      signal,
    ),
  caggFindings: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<Finding>>(
      `${caggScopePath(env, run, database, schema, name)}/findings${qs({ limit, offset })}`,
      signal,
    ),
  jobs: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<JobSnapshot>>(
      `/api/v1/environments/${environmentId}/jobs`,
      signal,
    ),
  policies: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<PolicySnapshot>>(
      `/api/v1/environments/${environmentId}/policies`,
      signal,
    ),
  tables: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<TableSnapshot>>(
      `/api/v1/environments/${environmentId}/tables${qs(params)}`,
      signal,
    ),
  tableAssessment: (
    env: string,
    run: string,
    database: string,
    schema: string,
    table: string,
    signal?: AbortSignal,
  ) =>
    getJSON<TableAssessment>(
      `${tableScopePath(env, run, database, schema, table)}/assessment`,
      signal,
    ),
  tableGraph: (
    env: string,
    run: string,
    database: string,
    schema: string,
    table: string,
    limit = 50,
    signal?: AbortSignal,
  ) =>
    getJSON<RelationshipGraph>(
      `${tableScopePath(env, run, database, schema, table)}/graph${qs({ limit })}`,
      signal,
    ),
  tableFindings: (
    env: string,
    run: string,
    database: string,
    schema: string,
    table: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<Finding>>(
      `${tableScopePath(env, run, database, schema, table)}/findings${qs({ limit, offset })}`,
      signal,
    ),
  tableGrants: (
    env: string,
    run: string,
    database: string,
    schema: string,
    table: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<GrantSnapshot>>(
      `${tableScopePath(env, run, database, schema, table)}/grants${qs({ limit, offset })}`,
      signal,
    ),
  tableDependencies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    table: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<DependencySnapshot>>(
      `${tableScopePath(env, run, database, schema, table)}/dependencies${qs({ limit, offset })}`,
      signal,
    ),
  tableTriggers: (
    env: string,
    run: string,
    database: string,
    schema: string,
    table: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<TriggerSnapshot>>(
      `${tableScopePath(env, run, database, schema, table)}/triggers${qs({ limit, offset })}`,
      signal,
    ),
  tableRLSPolicies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    table: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<RLSPolicySnapshot>>(
      `${tableScopePath(env, run, database, schema, table)}/rls-policies${qs({ limit, offset })}`,
      signal,
    ),
  constraints: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<ConstraintSnapshot>>(
      `/api/v1/environments/${environmentId}/constraints${qs(params)}`,
      signal,
    ),
  tableHistory: (
    environmentId: string,
    params: TableScopeParams,
    signal?: AbortSignal,
  ) =>
    getJSON<ItemsResponse<TableHistoryPoint>>(
      `/api/v1/environments/${environmentId}/tables/history${qs(params)}`,
      signal,
    ),
  storageHistory: (
    environmentId: string,
    params: Partial<TableScopeParams> & {
      scope: "environment" | "database" | "schema" | "table";
    },
    signal?: AbortSignal,
  ) =>
    getJSON<ItemsResponse<ScopeHistoryPoint>>(
      `/api/v1/environments/${environmentId}/history${qs(params)}`,
      signal,
    ),
  tableColumnStats: (
    environmentId: string,
    params: TableScopeParams,
    signal?: AbortSignal,
  ) =>
    getJSON<ItemsResponse<ColumnStatSnapshot>>(
      `/api/v1/environments/${environmentId}/tables/column-stats${qs(params)}`,
      signal,
    ),
  tableWorkload: (
    environmentId: string,
    params: TableScopeParams,
    signal?: AbortSignal,
  ) =>
    getJSON<ItemsResponse<WorkloadSnapshot>>(
      `/api/v1/environments/${environmentId}/tables/workload${qs(params)}`,
      signal,
    ),
  columns: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<ColumnSnapshot>>(
      `/api/v1/environments/${environmentId}/columns${qs(params)}`,
      signal,
    ),
  indexes: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<IndexSnapshot>>(
      `/api/v1/environments/${environmentId}/indexes${qs(params)}`,
      signal,
    ),
  indexDetail: (
    env: string,
    run: string,
    database: string,
    schema: string,
    index: string,
    signal?: AbortSignal,
  ) =>
    getJSON<IndexDetailSnapshot>(
      `${indexScopePath(env, run, database, schema, index)}/detail`,
      signal,
    ),
  indexHistory: (
    env: string,
    run: string,
    database: string,
    schema: string,
    index: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<IndexHistoryPoint>>(
      `${indexScopePath(env, run, database, schema, index)}/history${qs({ limit, offset })}`,
      signal,
    ),
  indexFindings: (
    env: string,
    run: string,
    database: string,
    schema: string,
    index: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<Finding>>(
      `${indexScopePath(env, run, database, schema, index)}/findings${qs({ limit, offset })}`,
      signal,
    ),
  views: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<ViewSnapshot>>(
      `/api/v1/environments/${environmentId}/views${qs(params)}`,
      signal,
    ),
  viewDetail: (
    env: string,
    run: string,
    database: string,
    schema: string,
    view: string,
    signal?: AbortSignal,
  ) =>
    getJSON<ViewDetailSnapshot>(
      `${viewScopePath(env, run, database, schema, view)}/detail`,
      signal,
    ),
  viewDependencies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    view: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<DependencySnapshot>>(
      `${viewScopePath(env, run, database, schema, view)}/dependencies${qs({ limit, offset })}`,
      signal,
    ),
  viewGrants: (
    env: string,
    run: string,
    database: string,
    schema: string,
    view: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<GrantSnapshot>>(
      `${viewScopePath(env, run, database, schema, view)}/grants${qs({ limit, offset })}`,
      signal,
    ),
  viewFindings: (
    env: string,
    run: string,
    database: string,
    schema: string,
    view: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<Finding>>(
      `${viewScopePath(env, run, database, schema, view)}/findings${qs({ limit, offset })}`,
      signal,
    ),
  functions: (
    environmentId: string,
    params?: InventoryListParams,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<FunctionSnapshot>>(
      `/api/v1/environments/${environmentId}/functions${qs(params)}`,
      signal,
    ),
  functionDetail: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    signature: string,
    signal?: AbortSignal,
  ) =>
    getJSON<FunctionDetailSnapshot>(
      `${functionScopePath(env, run, database, schema, name)}/detail${qs({ signature })}`,
      signal,
    ),
  functionDependencies: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    signature: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<DependencySnapshot>>(
      `${functionScopePath(env, run, database, schema, name)}/dependencies${qs({ signature, limit, offset })}`,
      signal,
    ),
  functionGrants: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    signature: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<GrantSnapshot>>(
      `${functionScopePath(env, run, database, schema, name)}/grants${qs({ signature, limit, offset })}`,
      signal,
    ),
  functionFindings: (
    env: string,
    run: string,
    database: string,
    schema: string,
    name: string,
    signature: string,
    limit = 50,
    offset = 0,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<Finding>>(
      `${functionScopePath(env, run, database, schema, name)}/findings${qs({ signature, limit, offset })}`,
      signal,
    ),
  auditRuns: (
    params?: {
      environment_id?: string;
      profile?: string;
      status?: string;
    },
    signal?: AbortSignal,
  ) => {
    const q = new URLSearchParams();
    if (params?.environment_id) {
      q.set("environment_id", params.environment_id);
    }
    if (params?.profile) {
      q.set("profile", params.profile);
    }
    if (params?.status) {
      q.set("status", params.status);
    }
    const s = q.toString();
    return getJSON<ItemsResponse<AuditRun>>(
      `/api/v1/audit-runs${s ? `?${s}` : ""}`,
      signal,
    );
  },
  auditRunsPage: (
    params: {
      environment_id?: string;
      profile?: string;
      status?: string;
      limit: number;
      offset: number;
    },
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<AuditRun>>(`/api/v1/audit-runs${qs(params)}`, signal),
  auditRun: (id: string, signal?: AbortSignal) =>
    getJSON<AuditRun>(`/api/v1/audit-runs/${id}`, signal),
  cancelAuditRun: (id: string) =>
    postJSON<{ status: string }>(`/api/v1/audit-runs/${id}/cancel`, {}),
  cancelAuditDatabase: (id: string, database: string) =>
    postJSON<{ status: string }>(
      `/api/v1/audit-runs/${id}/databases/${encodeURIComponent(database)}/cancel`,
      {},
    ),
  auditRunCollectors: (id: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<CollectorRun>>(
      `/api/v1/audit-runs/${id}/collectors`,
      signal,
    ),
  auditRunCoverage: (id: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<AuditRunCoverage>>(
      `/api/v1/audit-runs/${id}/coverage`,
      signal,
    ),
  auditRunAnalysis: (id: string, signal?: AbortSignal) =>
    getJSON<AnalysisRun>(`/api/v1/audit-runs/${id}/analysis`, signal),
  auditBaseline: (
    environmentId: string,
    database = "",
    schema = "",
    table = "",
    signal?: AbortSignal,
  ) =>
    getJSON<AuditBaseline>(
      `/api/v1/environments/${environmentId}/baseline${qs({ database, schema, table })}`,
      signal,
    ),
  selectAuditBaseline: (
    environmentId: string,
    auditRunId: string,
    database = "",
    schema = "",
    table = "",
  ) =>
    reportRequest<AuditBaseline>(
      `/api/v1/environments/${environmentId}/baseline`,
      "PUT",
      {
        audit_run_id: auditRunId,
        database,
        schema,
        table,
        confirm: true,
      },
    ),
  baselineComparisons: (
    environmentId: string,
    auditRunId?: string,
    signal?: AbortSignal,
  ) =>
    getJSON<ItemsResponse<BaselineComparison>>(
      `/api/v1/environments/${environmentId}/baseline/comparisons${qs({ audit_run_id: auditRunId })}`,
      signal,
    ),
  annotations: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<AuditAnnotation>>(
      `/api/v1/environments/${environmentId}/annotations`,
      signal,
    ),
  createAnnotation: (
    environmentId: string,
    body: {
      audit_run_id?: string;
      kind: AuditAnnotation["kind"];
      note: string;
      occurred_at: string;
    },
  ) =>
    postJSON<AuditAnnotation>(
      `/api/v1/environments/${environmentId}/annotations`,
      body,
    ),
  regressions: (environmentId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<RegressionAlert>>(
      `/api/v1/environments/${environmentId}/regressions`,
      signal,
    ),
  acknowledgeRegression: (environmentId: string, id: string, reason: string) =>
    postJSON<{ status: string }>(
      `/api/v1/environments/${environmentId}/regressions/${id}/acknowledge`,
      { reason },
    ),
  scopeScore: (
    environmentId: string,
    runId: string,
    database?: string,
    schema?: string,
    table?: string,
    signal?: AbortSignal,
  ) =>
    getJSON<ScopeScore>(
      `/api/v1/environments/${environmentId}/runs/${runId}/score${qs({ database, schema, table })}`,
      signal,
    ),
  compareScopeScore: (
    environmentId: string,
    runId: string,
    database?: string,
    schema?: string,
    table?: string,
    signal?: AbortSignal,
  ) =>
    getJSON<{
      status: string;
      reason: string;
      delta?: number;
      current?: ScopeScore;
      baseline?: ScopeScore;
    }>(
      `/api/v1/environments/${environmentId}/runs/${runId}/score/compare${qs({ database, schema, table })}`,
      signal,
    ),
  scopeScores: (environmentId: string, runId: string, signal?: AbortSignal) =>
    getJSON<{ items: ScopeAggregate[]; version: string }>(
      `/api/v1/environments/${environmentId}/runs/${runId}/scores`,
      signal,
    ),
  rules: (environmentId: string, schema?: string, signal?: AbortSignal) =>
    getJSON<{ items: EffectiveRule[]; schema: string }>(
      `/api/v1/environments/${environmentId}/rules${qs({ schema })}`,
      signal,
    ),
  putRule: (
    environmentId: string,
    ruleId: string,
    body: {
      schema: string;
      enabled: boolean;
      parameters: Record<string, unknown>;
    },
  ) =>
    putJSON<{ rule_id: string; enabled: boolean; note: string }>(
      `/api/v1/environments/${environmentId}/rules/${encodeURIComponent(ruleId)}`,
      body,
    ),
  reprocessAuditRun: (id: string) =>
    postJSON<{ audit_run_id: string; produced: number; saved: number }>(
      `/api/v1/audit-runs/${id}/reprocess`,
      {},
    ),
  triggerAuditRun: (environmentId: string, profile = "manual") =>
    postJSON<{ audit_run_id: string; status: string }>("/api/v1/audit-runs", {
      environment_id: environmentId,
      profile,
    }),
  schedules: (environmentId: string, signal?: AbortSignal) =>
    getJSON<{
      items: {
        environment_id: string;
        profile: string;
        enabled: boolean;
        last_status?: string;
        last_run_at?: string;
        next_run_at?: string;
      }[];
    }>(`/api/v1/environments/${environmentId}/schedules`, signal),
  saveSchedule: (environmentId: string, profile: string, enabled: boolean) =>
    putJSON<{
      profile: string;
      enabled: boolean;
      next_run_at?: string;
      last_status?: string;
    }>(`/api/v1/environments/${environmentId}/schedules`, { profile, enabled }),
  mappings: (
    params?: {
      source_environment_id?: string;
      target_environment_id?: string;
      status?: string;
    },
    signal?: AbortSignal,
  ) => {
    const q = new URLSearchParams();
    if (params?.source_environment_id) {
      q.set("source_environment_id", params.source_environment_id);
    }
    if (params?.target_environment_id) {
      q.set("target_environment_id", params.target_environment_id);
    }
    if (params?.status) {
      q.set("status", params.status);
    }
    const s = q.toString();
    return getJSON<ItemsResponse<ObjectMapping>>(
      `/api/v1/mappings${s ? `?${s}` : ""}`,
      signal,
    );
  },
  createMapping: (body: Partial<ObjectMapping>) =>
    postJSON<ObjectMapping>("/api/v1/mappings", body),
  updateMappingStatus: (id: string, status: string, notes?: string) =>
    patchJSON<ObjectMapping>(`/api/v1/mappings/${id}`, { status, notes }),
  suggestMappings: (body: {
    source: Array<Record<string, string>>;
    target: Array<Record<string, string>>;
    target_default_db?: string;
  }) =>
    postJSON<ItemsResponse<MappingCandidate>>("/api/v1/mappings/suggest", body),
  compare: (body: {
    source_run_id?: string;
    target_run_id?: string;
    source: CompareObjectItem[];
    target: CompareObjectItem[];
    statuses?: string[];
    object_type?: string;
  }) => postJSON<CompareResult>("/api/v1/compare", body),
  serverCompare: (left: string, right: string, signal?: AbortSignal) =>
    getJSON<ServerCompareReport>(
      `/api/v1/server-compare${qs({ left, right })}`,
      signal,
    ),
  findings: (
    params?: {
      environment_id?: string;
      finding_type?: string;
      severity?: string;
      status?: string;
      limit?: number;
    },
    signal?: AbortSignal,
  ) => {
    const q = new URLSearchParams();
    if (params?.environment_id) {
      q.set("environment_id", params.environment_id);
    }
    if (params?.finding_type) {
      q.set("finding_type", params.finding_type);
    }
    if (params?.severity) {
      q.set("severity", params.severity);
    }
    if (params?.status) {
      q.set("status", params.status);
    }
    if (params?.limit) {
      q.set("limit", String(params.limit));
    }
    const s = q.toString();
    return getJSON<ItemsResponse<Finding>>(
      `/api/v1/findings${s ? `?${s}` : ""}`,
      signal,
    );
  },
  findingPriorities: (
    params?: { environment_id?: string; limit?: number },
    signal?: AbortSignal,
  ) =>
    getJSON<PrioritiesResponse>(
      `/api/v1/findings/priorities${qs({
        environment_id: params?.environment_id || undefined,
        limit: params?.limit,
      })}`,
      signal,
    ),
  runFindings: (auditRunId: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<Finding>>(
      `/api/v1/findings?audit_run_id=${encodeURIComponent(auditRunId)}`,
      signal,
    ),
  findingsPage: (
    params: {
      environment_id?: string;
      finding_type?: string;
      severity?: string;
      status?: string;
      assignee?: string;
      overdue?: string;
      limit: number;
      offset: number;
    },
    signal?: AbortSignal,
  ) => getJSON<PagedResponse<Finding>>(`/api/v1/findings${qs(params)}`, signal),
  findingsCategoryPage: (
    category: "security" | "performance",
    params: {
      environment_id?: string;
      status?: string;
      limit: number;
      offset: number;
    },
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<Finding>>(
      `/api/v1/finding-categories/${category}${qs(params)}`,
      signal,
    ),
  finding: (id: string, signal?: AbortSignal) =>
    getJSON<Finding>(`/api/v1/findings/${id}`, signal),
  findingAction: (id: string, signal?: AbortSignal) =>
    getJSON<FindingAction>(`/api/v1/findings/${id}/action`, signal),
  findingActionEvents: (id: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<ActionEvent>>(
      `/api/v1/findings/${id}/action/events`,
      signal,
    ),
  actionMeasurements: (
    id: string,
    offset = 0,
    limit = 50,
    signal?: AbortSignal,
  ) =>
    getJSON<PagedResponse<ActionMeasurement>>(
      `/api/v1/findings/${id}/action/measurements?offset=${offset}&limit=${limit}`,
      signal,
    ),
  recordActionMeasurement: (
    id: string,
    body: {
      before_run_id: string;
      after_run_id: string;
      metric: ActionMeasurement["metric"];
      hypothesis: string;
      window_note: string;
      workload_comparable: boolean;
    },
  ) =>
    postJSON<ActionMeasurement>(
      `/api/v1/findings/${id}/action/measurements`,
      body,
    ),
  updateFindingAction: (
    id: string,
    body: {
      status: string;
      owner: string;
      justification: string;
      result: string;
    },
  ) => patchJSON<FindingAction>(`/api/v1/findings/${id}/action`, body),
  findingTimeline: (id: string, signal?: AbortSignal) =>
    getJSON<ItemsResponse<FindingEvent>>(
      `/api/v1/findings/${id}/timeline`,
      signal,
    ),
  suppressFinding: (id: string, reason: string, suppressedUntil: string) =>
    patchJSON<Finding>(`/api/v1/findings/${id}`, {
      status: "suppressed",
      suppression_reason: reason,
      suppressed_until: suppressedUntil,
    }),
  updateFindingStatus: (id: string, status: string, notes?: string) =>
    patchJSON<Finding>(`/api/v1/findings/${id}`, { status, notes }),
  updateFindingWorkflow: (id: string, assignee: string, dueAt?: string) =>
    patchJSON<Finding>(`/api/v1/findings/${id}`, {
      assignee,
      due_at: dueAt,
    }),
  analyzeFindings: (body: {
    environment_id: string;
    audit_run_id?: string;
    tables?: unknown[];
    indexes?: unknown[];
    hypertables?: unknown[];
    chunks?: unknown[];
    caggs?: unknown[];
    policies?: unknown[];
    jobs?: unknown[];
    activity?: unknown[];
    vacuum?: unknown[];
    locks?: unknown[];
    connections?: unknown[];
    query_stats?: unknown[];
    roles?: unknown[];
    grants?: unknown[];
    functions?: unknown[];
  }) => postJSON<AnalyzeResult>("/api/v1/findings/analyze", body),
};
