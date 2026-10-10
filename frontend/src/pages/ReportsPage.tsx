import { useCallback, useEffect, useState } from "react";
import { CoverageBanner } from "../components/CoverageBanner";
import { PageHeader } from "../components/PageHeader";
import { Button, Card, ErrorBanner, Select } from "../components/ui";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { downloadBlob } from "../lib/export";
import { api } from "../services/api";
import type { AuditRun, ReportFilters, ReportJob } from "../types";
import { ReportJobList } from "./parts/ReportJobList";

const severityLabels: Record<string, string> = {
  critical: "Crítica",
  high: "Alta",
  medium: "Média",
  low: "Baixa",
  info: "Informativa",
};

const runStatusLabels: Record<string, string> = {
  success: "Concluída",
  partial_success: "Parcial",
};

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
  return (
    kind !== "table" ||
    Boolean(filters.database && filters.schema && filters.table)
  );
}

export function ReportsPage() {
  const { environmentId, environments } = useApp();
  const [env, setEnv] = useState(environmentId ?? "");
  const [token, setToken] = useState("");
  const [runs, setRuns] = useState<AuditRun[]>([]);
  const [runId, setRunId] = useState("");
  const [kind, setKind] = useState<ReportJob["report_type"]>("executive");
  const [database, setDatabase] = useState("");
  const [schema, setSchema] = useState("");
  const [table, setTable] = useState("");
  const [severity, setSeverity] = useState("");
  const [redaction, setRedaction] = useState<"none" | "identifiers" | "strict">(
    "none",
  );
  const [jobs, setJobs] = useState<ReportJob[]>([]);
  const [coverageKind, setCoverageKind] = useState<
    "partial" | "gap" | "permission" | null
  >(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (environmentId) setEnv(environmentId);
  }, [environmentId]);
  useEffect(() => {
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
        setCoverageKind(
          completed.some((run) => run.status === "partial_success")
            ? "partial"
            : null,
        );
        setRunId((current) =>
          completed.some((run) => run.id === current)
            ? current
            : (completed[0]?.id ?? ""),
        );
      })
      .catch(() => setRuns([]));
  }, [env]);

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
      table: table.trim() || undefined,
      severity: severity || undefined,
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
          `db-auditor-${job.report_type}-${job.id}.pdf`,
          await api.downloadPDFReport(env, job.id),
        );
      await refresh();
    } catch (cause: unknown) {
      setError(formatError(cause, "Falha na operação do relatório"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <PageHeader
        eyebrow="Relatórios"
        title="Relatórios PDF"
        description="Gere relatórios versionados a partir de uma execução específica. Acesso controlado pela conta local."
      />
      <div className="mt-8 space-y-5">
        <Card
          title="Solicitar relatório"
          subtitle="O processamento é assíncrono e o PDF expira após 30 dias."
        >
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <Select
              label="Ambiente"
              value={env}
              onChange={(event) => setEnv(event.target.value)}
              options={[
                { value: "", label: "Selecione" },
                ...environments.map((item) => ({
                  value: item.id,
                  label: item.name,
                })),
              ]}
            />
            <Select
              label="Execução"
              value={runId}
              onChange={(event) => setRunId(event.target.value)}
              options={[
                { value: "", label: "Selecione" },
                ...runs.map((run) => ({
                  value: run.id,
                  label: `${run.id.slice(0, 8)}… · ${runStatusLabels[run.status] ?? run.status}`,
                })),
              ]}
            />
            <Select
              label="Tipo"
              value={kind}
              onChange={(event) =>
                setKind(event.target.value as ReportJob["report_type"])
              }
              options={[
                { value: "executive", label: "Executivo" },
                { value: "technical", label: "Técnico" },
                { value: "table", label: "Tabela" },
              ]}
            />
            <Select
              label="Severidade"
              value={severity}
              onChange={(event) => setSeverity(event.target.value)}
              options={[
                { value: "", label: "Todas" },
                ...["critical", "high", "medium", "low", "info"].map(
                  (value) => ({ value, label: severityLabels[value] }),
                ),
              ]}
            />
            <Select
              label="Redação de dados sensíveis"
              value={redaction}
              onChange={(event) =>
                setRedaction(
                  event.target.value as "none" | "identifiers" | "strict",
                )
              }
              options={[
                { value: "none", label: "Sem redação" },
                {
                  value: "identifiers",
                  label: "Ocultar identificadores e textos livres",
                },
                {
                  value: "strict",
                  label: "Rigorosa: ocultar também notas de cobertura",
                },
              ]}
            />
            <input
              aria-label="Banco"
              placeholder="Banco (opcional)"
              value={database}
              onChange={(event) => setDatabase(event.target.value)}
              className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
            />
            <input
              aria-label="Esquema"
              placeholder="Esquema (opcional)"
              value={schema}
              onChange={(event) => setSchema(event.target.value)}
              className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
            />
            <input
              aria-label="Tabela"
              placeholder="Tabela (obrigatória para relatório de tabela)"
              value={table}
              onChange={(event) => setTable(event.target.value)}
              className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
            />
            {!api.hasSession() && (
              <input
                aria-label="Token de relatórios"
                type="password"
                autoComplete="off"
                placeholder="Token de relatórios"
                value={token}
                onChange={(event) => setToken(event.target.value)}
                className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
              />
            )}
          </div>
          <div className="mt-4 flex gap-2">
            <Button
              disabled={
                busy ||
                !reportFormReady(
                  env,
                  runId,
                  token || (api.hasSession() ? "session" : ""),
                  kind,
                  {
                    database,
                    schema,
                    table,
                    severity,
                  },
                )
              }
              onClick={() => void request()}
            >
              Gerar PDF
            </Button>
            <Button
              variant="secondary"
              disabled={busy || !env || (!token && !api.hasSession())}
              onClick={() => void refresh()}
            >
              Atualizar histórico
            </Button>
          </div>
        </Card>
        {error ? (
          <ErrorBanner message={error} onRetry={() => void refresh()} />
        ) : null}
        <CoverageBanner kind={coverageKind} />
        <ReportJobList
          jobs={jobs}
          act={act}
          reportJobActions={reportJobActions}
        />
      </div>
    </>
  );
}
