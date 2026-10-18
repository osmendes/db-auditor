import {
  type InputHTMLAttributes,
  useCallback,
  useEffect,
  useState,
} from "react";
import { useApp } from "../../context/AppContext";
import { useEmptyContext } from "../../hooks/useEmptyContext";
import { useRunCoverage } from "../../hooks/useRunCoverage";
import { resolveEmptyCause } from "../../lib/emptyCause";
import { formatError } from "../../lib/errors";
import { downloadBlob } from "../../lib/export";
import { api } from "../../services/api";
import type { AuditRun, ReportFilters, ReportJob } from "../../types";

/** State and effects of ReportsPage (UI-06 extraction). */
export const severityLabels: Record<string, string> = {
  critical: "Crítica",
  high: "Alta",
  medium: "Média",
  low: "Baixa",
  info: "Informativa",
};

export const runStatusLabels: Record<string, string> = {
  success: "Concluída",
  partial_success: "Parcial",
};

export function ReportTextField({
  label,
  ...props
}: { label: string } & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="grid gap-1.5 text-sm text-slate-300">
      <span className="text-xs font-medium tracking-wide text-slate-400">
        {label}
      </span>
      <input
        className="h-10 w-full rounded-md border border-slate-600 bg-slate-900 px-3 text-sm text-slate-100 placeholder:text-slate-500 focus:border-white focus:outline-none focus:ring-1 focus:ring-white"
        {...props}
      />
    </label>
  );
}

export function reportJobActions(
  job: ReportJob,
  now = Date.now(),
): Array<"download" | "retry" | "cancel"> {
  if (Date.parse(job.expires_at) <= now) return [];
  if (job.status === "success") return ["download"];
  if (job.status === "failed" && job.attempts < 3) return ["retry"];
  if (job.status === "queued" || job.status === "running") return ["cancel"];
  return [];
}

export function reportFormReady(
  env: string,
  runId: string,
  token: string,
  kind: ReportJob["report_type"],
  filters: ReportFilters,
): boolean {
  if (!env || !runId || !token) return false;
  if (filters.schema && !filters.database) return false;
  if (filters.table && !filters.schema) return false;
  if (kind === "architecture")
    return Boolean(filters.database && !filters.table);
  return (
    kind !== "table" ||
    Boolean(filters.database && filters.schema && filters.table)
  );
}

export function useReportsPage() {
  const { environmentId, environments, setSection } = useApp();
  const [env, setEnv] = useState(environmentId ?? "");
  const [token, setToken] = useState("");
  const [runs, setRuns] = useState<AuditRun[]>([]);
  const [runId, setRunId] = useState("");
  const coverage = useRunCoverage(env || null, runId || null);
  const [runsLoaded, setRunsLoaded] = useState(false);
  const noCompletedRun = Boolean(env) && runsLoaded && runs.length === 0;
  const emptyContext = useEmptyContext(env || null, noCompletedRun);
  const reportEmptyCause = noCompletedRun
    ? resolveEmptyCause({
        filtered: false,
        dsnMissing: emptyContext.dsnMissing,
        collected: false,
      })
    : null;
  const [kind, setKind] = useState<ReportJob["report_type"]>("executive");
  const [database, setDatabase] = useState("");
  const [databaseOptions, setDatabaseOptions] = useState<string[]>([]);
  const [schema, setSchema] = useState("");
  const [table, setTable] = useState("");
  const [severity, setSeverity] = useState("");
  const [redaction, setRedaction] = useState<"none" | "identifiers" | "strict">(
    "none",
  );
  const [jobs, setJobs] = useState<ReportJob[]>([]);
  const [preview, setPreview] = useState("");
  const [decisionObject, setDecisionObject] = useState("");
  const [decisionSubject, setDecisionSubject] = useState("semantics");
  const [decisionValue, setDecisionValue] = useState("");
  const [decisions, setDecisions] = useState<
    Array<{
      object_ref: string;
      subject: string;
      value: string;
      version: number;
      author: string;
      created_at: string;
    }>
  >([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (environmentId) setEnv(environmentId);
  }, [environmentId]);
  useEffect(() => {
    setRunsLoaded(false);
    if (!env) {
      setRuns([]);
      setRunId("");
      return;
    }
    void api
      .auditRuns({ environment_id: env })
      .then((result) => {
        const completed = result.items.filter(
          (run) => run.status === "success" || run.status === "partial_success",
        );
        setRuns(completed);
        setRunsLoaded(true);
        setRunId((current) =>
          completed.some((run) => run.id === current)
            ? current
            : (completed[0]?.id ?? ""),
        );
      })
      .catch(() => {
        setRuns([]);
        setRunsLoaded(false);
      });
  }, [env]);
  useEffect(() => {
    if (!env || !runId || kind !== "architecture") {
      setDatabaseOptions([]);
      return;
    }
    void api
      .databases(env, runId)
      .then((response) => {
        const names = response.items.map((item) => item.database_name);
        setDatabaseOptions(names);
        setDatabase((current) =>
          names.includes(current) ? current : (names[0] ?? ""),
        );
      })
      .catch(() => {
        setDatabaseOptions([]);
        setDatabase("");
      });
  }, [env, runId, kind]);

  const refresh = useCallback(async () => {
    if (!env || (!token && !api.hasSession())) return;
    api.setReportToken(token);
    try {
      setJobs((await api.reportJobs(env)).items);
      setError(null);
    } catch (cause: unknown) {
      setError(formatError(cause, "Não foi possível consultar os relatórios"));
    }
  }, [env, token]);

  useEffect(() => {
    void refresh();
  }, [refresh]);
  useEffect(() => {
    if (
      !jobs.some((job) => job.status === "queued" || job.status === "running")
    )
      return;
    const timer = window.setInterval(() => {
      void refresh();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [jobs, refresh]);

  const request = async () => {
    const filters: ReportFilters = {
      database: database.trim() || undefined,
      schema: schema.trim() || undefined,
      table: kind === "architecture" ? undefined : table.trim() || undefined,
      severity: kind === "architecture" ? undefined : severity || undefined,
      redaction,
    };
    if (
      !reportFormReady(
        env,
        runId,
        token || (api.hasSession() ? "session" : ""),
        kind,
        filters,
      )
    )
      return;
    setBusy(true);
    try {
      api.setReportToken(token);
      await api.requestPDFReport(env, runId, kind, filters);
      await refresh();
    } catch (cause: unknown) {
      setError(formatError(cause, "Falha ao solicitar relatório"));
    } finally {
      setBusy(false);
    }
  };

  const act = async (
    job: ReportJob,
    action: "cancel" | "retry" | "download",
  ) => {
    setBusy(true);
    try {
      api.setReportToken(token);
      if (action === "cancel") await api.cancelPDFReport(env, job.id);
      if (action === "retry") await api.retryPDFReport(env, job.id);
      if (action === "download")
        downloadBlob(
          `db-auditor-${job.report_type}-${job.id}.${job.report_type === "architecture" ? "md" : "pdf"}`,
          await api.downloadPDFReport(env, job.id),
        );
      await refresh();
    } catch (cause: unknown) {
      setError(formatError(cause, "Falha na operação do relatório"));
    } finally {
      setBusy(false);
    }
  };

  const loadDecisions = async () => {
    if (!env || !database.trim()) return;
    try {
      setDecisions(
        (await api.architectureDecisions(env, database.trim())).items,
      );
      setError(null);
    } catch (cause: unknown) {
      setError(formatError(cause, "Falha ao consultar decisões"));
    }
  };
  const saveDecision = async () => {
    if (
      !env ||
      !database.trim() ||
      !decisionObject.trim() ||
      !decisionValue.trim()
    )
      return;
    setBusy(true);
    try {
      await api.saveArchitectureDecision(env, {
        database: database.trim(),
        object_ref: decisionObject.trim(),
        subject: decisionSubject,
        value: decisionValue.trim(),
      });
      setDecisionValue("");
      await loadDecisions();
    } catch (cause: unknown) {
      setError(formatError(cause, "Falha ao registrar decisão"));
    } finally {
      setBusy(false);
    }
  };
  const showPreview = async (job: ReportJob) => {
    try {
      setPreview(await (await api.downloadPDFReport(env, job.id)).text());
    } catch (cause: unknown) {
      setError(formatError(cause, "Falha ao abrir prévia"));
    }
  };

  return {
    act,
    busy,
    coverage,
    database,
    databaseOptions,
    decisionObject,
    decisions,
    decisionSubject,
    decisionValue,
    emptyContext,
    env,
    environmentId,
    environments,
    error,
    jobs,
    kind,
    loadDecisions,
    noCompletedRun,
    preview,
    redaction,
    refresh,
    reportEmptyCause,
    request,
    runId,
    runs,
    runsLoaded,
    saveDecision,
    schema,
    setBusy,
    setDatabase,
    setDatabaseOptions,
    setDecisionObject,
    setDecisions,
    setDecisionSubject,
    setDecisionValue,
    setEnv,
    setError,
    setJobs,
    setKind,
    setPreview,
    setRedaction,
    setRunId,
    setRuns,
    setRunsLoaded,
    setSchema,
    setSection,
    setSeverity,
    setTable,
    setToken,
    severity,
    showPreview,
    table,
    token,
  };
}

export type ReportsPageState = ReturnType<typeof useReportsPage>;
