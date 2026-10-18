import {
  type InputHTMLAttributes,
  useCallback,
  useEffect,
  useState,
} from "react";
import { CoverageBanner } from "../components/CoverageBanner";
import { PageHeader } from "../components/PageHeader";
import { Button, Card, ErrorBanner, Select } from "../components/ui";
import { useApp } from "../context/AppContext";
import { useEmptyContext } from "../hooks/useEmptyContext";
import { useRunCoverage } from "../hooks/useRunCoverage";
import { resolveEmptyCause } from "../lib/emptyCause";
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

function ReportTextField({
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

export function ReportsPage() {
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

  return (
    <>
      <PageHeader
        eyebrow="Relatórios"
        title="Relatórios"
        description="Gere relatórios versionados a partir de uma execução específica. Acesso controlado pela conta local."
      />
      <div className="mt-8 space-y-5">
        {/* Coverage of the selected run: the PDF inherits these limits. */}
        {env && runId ? <CoverageBanner signals={coverage.signals} /> : null}
        <Card
          title="Solicitar relatório"
          contentClassName="flex flex-col gap-3"
        >
          <p className="-mt-1.5 text-sm text-slate-400">
            O processamento é assíncrono e o arquivo expira após 30 dias.
          </p>
          <div className="grid gap-3 sm:grid-cols-2">
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
              onChange={(event) => {
                const next = event.target.value as ReportJob["report_type"];
                setKind(next);
                if (next === "architecture") {
                  setTable("");
                  setSeverity("");
                }
              }}
              options={[
                { value: "executive", label: "Executivo" },
                { value: "technical", label: "Técnico" },
                { value: "table", label: "Tabela" },
                ...(import.meta.env.VITE_ARCHITECTURE_REPORT_ENABLED === "true"
                  ? [
                      {
                        value: "architecture",
                        label: "Análise completa e modelagem",
                      },
                    ]
                  : []),
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
            {kind === "architecture" ? (
              <Select
                label="Banco da execução"
                value={database}
                onChange={(event) => setDatabase(event.target.value)}
                options={[
                  { value: "", label: "Selecione" },
                  ...databaseOptions.map((name) => ({
                    value: name,
                    label: name,
                  })),
                ]}
              />
            ) : (
              <ReportTextField
                label="Banco (opcional)"
                aria-label="Banco"
                value={database}
                onChange={(event) => setDatabase(event.target.value)}
              />
            )}
            <ReportTextField
              label="Esquema (opcional)"
              aria-label="Esquema"
              value={schema}
              onChange={(event) => setSchema(event.target.value)}
            />
            <ReportTextField
              label="Tabela"
              aria-label="Tabela"
              placeholder="Obrigatória no relatório de tabela"
              value={table}
              onChange={(event) => setTable(event.target.value)}
            />
            {!api.hasSession() && (
              <ReportTextField
                label="Token de relatórios"
                aria-label="Token de relatórios"
                type="password"
                autoComplete="off"
                value={token}
                onChange={(event) => setToken(event.target.value)}
              />
            )}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Button
              className="h-10 w-full"
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
              {kind === "architecture" ? "Gerar Markdown" : "Gerar PDF"}
            </Button>
            <Button
              className="h-10 w-full"
              variant="secondary"
              disabled={busy || !env || (!token && !api.hasSession())}
              onClick={() => void refresh()}
            >
              Atualizar histórico
            </Button>
          </div>
        </Card>
        {kind === "architecture" &&
        import.meta.env.VITE_ARCHITECTURE_REPORT_ENABLED === "true" ? (
          <Card title="Decisões de domínio" contentClassName="space-y-3">
            <p className="text-sm text-slate-400">
              Registre uma interpretação confirmada com autoria e histórico. O
              pedido congela a versão atual; nenhuma decisão altera o ERD
              observado.
            </p>
            {decisionSubject === "insertion" ? (
              <p className="text-xs text-slate-400">
                Contrato de inserção: informe JSON com column, timezone,
                validated, filled_on_insert e append_only. Exemplo:{" "}
                <code>
                  {
                    '{"column":"inserted_at","timezone":"UTC","validated":true,"filled_on_insert":true,"append_only":false}'
                  }
                </code>
                . Com exclusões possíveis, a data só representa os registros
                presentes.
              </p>
            ) : null}
            <div className="grid gap-3 sm:grid-cols-2">
              <ReportTextField
                label="Objeto (database.schema.tabela)"
                value={decisionObject}
                onChange={(event) => setDecisionObject(event.target.value)}
              />
              <Select
                label="Assunto"
                value={decisionSubject}
                onChange={(event) => setDecisionSubject(event.target.value)}
                options={[
                  { value: "semantics", label: "Semântica" },
                  { value: "insertion", label: "Contrato de inserção" },
                  { value: "unit", label: "Unidade" },
                  { value: "retention", label: "Retenção" },
                  { value: "consumer", label: "Consumidor" },
                ]}
              />
              <ReportTextField
                label="Decisão confirmada"
                value={decisionValue}
                onChange={(event) => setDecisionValue(event.target.value)}
              />
            </div>
            <div className="flex gap-3">
              <Button
                disabled={
                  busy || !database || !decisionObject || !decisionValue
                }
                onClick={() => void saveDecision()}
              >
                Registrar decisão
              </Button>
              <Button
                variant="secondary"
                disabled={!database}
                onClick={() => void loadDecisions()}
              >
                Ver decisões
              </Button>
            </div>
            <ul className="text-sm text-slate-300">
              {decisions.map((item) => (
                <li key={`${item.object_ref}/${item.subject}/${item.version}`}>
                  {item.object_ref} · {item.subject} v{item.version}:{" "}
                  {item.value} · {item.author} · {item.created_at}
                </li>
              ))}
            </ul>
          </Card>
        ) : null}
        {error ? (
          <ErrorBanner message={error} onRetry={() => void refresh()} />
        ) : null}
        <ReportJobList
          jobs={jobs}
          act={act}
          reportJobActions={reportJobActions}
          emptyCause={reportEmptyCause}
          onEmptyAction={() =>
            setSection(reportEmptyCause === "dsn" ? "Ambientes" : "Execuções")
          }
        />
        {jobs
          .filter(
            (job) =>
              job.report_type === "architecture" && job.status === "success",
          )
          .map((job) => (
            <Button
              key={job.id}
              variant="secondary"
              onClick={() => void showPreview(job)}
            >
              Prévia textual · {job.id.slice(0, 8)}
            </Button>
          ))}
        {preview ? (
          <Card title="Prévia Markdown">
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs text-slate-300">
              {preview}
            </pre>
          </Card>
        ) : null}
      </div>
    </>
  );
}
