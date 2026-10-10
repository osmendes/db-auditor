import { lazy, type ReactNode, Suspense, useEffect, useState } from "react";
import { CoverageBanner } from "../components/CoverageBanner";
import { PageHeader } from "../components/PageHeader";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Skeleton,
  Table,
} from "../components/ui";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { labels } from "../lib/labels";
import { trendEntries } from "../lib/trend-gaps";
import { api } from "../services/api";

const StoragePieChart = lazy(() =>
  import("../components/ui/StoragePieChart").then((module) => ({
    default: module.StoragePieChart,
  })),
);

import type {
  ConnectionStatus,
  DashboardKPIs,
  Finding,
  FindingsTrendResponse,
  JobHealthResponse,
  RunTrendPoint,
  ScopeAggregate,
  StorageGrowthResponse,
} from "../types";

function formatBytes(n: number): string {
  if (n <= 0) {
    return "0 B";
  }
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function MiniBar({
  label,
  value,
  max,
  tone = "emerald",
  valueLabel,
}: {
  label: string;
  value: number;
  max: number;
  tone?: "emerald" | "amber" | "rose" | "sky";
  valueLabel?: string;
}) {
  const pct = max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0;
  const colors = {
    emerald: "bg-emerald-500/80",
    amber: "bg-amber-500/80",
    rose: "bg-rose-500/80",
    sky: "bg-sky-500/80",
  };
  return (
    <div className="space-y-1">
      <div className="flex justify-between gap-2 text-xs">
        <span className="truncate text-slate-300" title={label}>
          {label}
        </span>
        <span className="shrink-0 font-mono text-slate-100">
          {valueLabel ?? value}
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
        <div
          className={`h-full rounded-full ${colors[tone]}`}
          style={{ width: `${pct}%` }}
          role="presentation"
        />
      </div>
    </div>
  );
}

function KpiGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <p className="text-xs font-medium text-slate-400">{title}</p>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{children}</div>
    </div>
  );
}

function TrendBars({
  points,
  metric,
  granularity,
}: {
  points: RunTrendPoint[];
  metric: "size_bytes" | "findings" | "score" | "score_confidence";
  granularity: "day" | "week" | "month";
}) {
  const visible = points.slice(-60);
  const entries = trendEntries(visible, granularity);
  const max = Math.max(1, ...visible.map((point) => point[metric] ?? 0));
  if (visible.length === 0) {
    return (
      <p className="text-sm text-slate-400">
        Sem medições comparáveis no período.
      </p>
    );
  }
  return (
    <div className="max-h-72 space-y-3 overflow-y-auto pr-2">
      {entries.map((entry) => {
        if (entry.kind === "gap") {
          return (
            <p
              key={entry.key}
              className="border-l-2 border-dashed border-amber-500 pl-2 text-xs text-amber-200"
            >
              {entry.environment}: {entry.periods}{" "}
              {entry.periods === 1
                ? "período sem coleta"
                : "períodos sem coleta"}
              .
            </p>
          );
        }
        const { point } = entry;
        if (point[metric] == null) {
          return (
            <p
              key={point.audit_run_id}
              className="border-l-2 border-dashed border-amber-500 pl-2 text-xs text-amber-200"
            >
              {new Date(point.at).toLocaleDateString("pt-BR")} ·{" "}
              {point.environment_name}: sem dado para esta métrica (
              {point.coverage === "partial"
                ? "coleta ou análise parcial"
                : "inventário ausente"}
              ).
            </p>
          );
        }
        const value = point[metric] ?? 0;
        const when = new Date(point.at).toLocaleDateString("pt-BR");
        return (
          <div
            key={point.audit_run_id}
            title={`Execução ${point.audit_run_id}`}
          >
            <MiniBar
              label={`${when} · ${point.environment_name}${point.coverage === "partial" ? " · coleta parcial" : ""}${point.counter_reset ? " · contadores reiniciados" : ""}${!point.comparable && point.comparison_note ? ` · ${point.comparison_note}` : ""}`}
              value={value}
              max={max}
              valueLabel={
                metric === "size_bytes"
                  ? formatBytes(value)
                  : metric === "score_confidence"
                    ? `${Math.round(value * 100)}%`
                    : metric === "score"
                      ? `${value}/100`
                      : String(value)
              }
              tone={!point.comparable ? "amber" : "sky"}
            />
          </div>
        );
      })}
      <p className="text-xs text-slate-400">
        Cada barra representa a última execução elegível do período. Períodos
        sem coleta não são tratados como zero. Mostrando {visible.length} de{" "}
        {points.length} pontos; selecione um ambiente ou aumente a granularidade
        para facilitar a leitura.
      </p>
    </div>
  );
}

export function DashboardPage() {
  const {
    environmentId,
    setEnvironmentId,
    setSection,
    selectedEnvironment,
    openFinding,
  } = useApp();
  const [kpis, setKpis] = useState<DashboardKPIs | null>(null);
  const [storage, setStorage] = useState<StorageGrowthResponse | null>(null);
  const [trends, setTrends] = useState<FindingsTrendResponse | null>(null);
  const [jobs, setJobs] = useState<JobHealthResponse | null>(null);
  const [connections, setConnections] = useState<ConnectionStatus[] | null>(
    null,
  );
  const [scores, setScores] = useState<ScopeAggregate[]>([]);
  const [priorities, setPriorities] = useState<Finding[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);
  const [historyDays, setHistoryDays] = useState(90);
  const [historyGranularity, setHistoryGranularity] = useState<
    "day" | "week" | "month"
  >("day");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const params = environmentId
          ? { environment_id: environmentId }
          : undefined;
        const historyParams = {
          ...params,
          from: new Date(Date.now() - historyDays * 86400000).toISOString(),
          to: new Date().toISOString(),
          granularity: historyGranularity,
        };
        const [k, s, t, j, c] = await Promise.all([
          api.analyticsKpis(params),
          api.analyticsStorage(historyParams),
          api.analyticsFindingsTrends(historyParams),
          api.analyticsJobHealth(params),
          api.connectionStatus(),
        ]);
        if (!cancelled) {
          setKpis(k);
          setStorage(s);
          setTrends(t);
          setJobs(j);
          setConnections(c.items);
          const queue = await api.findings({
            environment_id: environmentId ?? undefined,
            status: "open",
            limit: 5,
          });
          if (!cancelled) setPriorities(queue.items.slice(0, 5));
        }
        if (environmentId) {
          const runs = await api.auditRuns({
            environment_id: environmentId,
            status: "success",
          });
          const run = runs.items[0];
          if (run && !cancelled) {
            const agg = await api.scopeScores(environmentId, run.id);
            if (!cancelled) setScores(agg.items);
          } else if (!cancelled) {
            setScores([]);
          }
        } else if (!cancelled) {
          setScores([]);
        }
      } catch (e) {
        if (!cancelled) {
          setError(
            formatError(
              e,
              "Falha ao carregar o dashboard. Verifique a API e tente novamente.",
            ),
          );
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [environmentId, reloadKey, historyDays, historyGranularity]);

  const consumersMax = storage?.top_consumers?.length
    ? Math.max(1, ...storage.top_consumers.map((p) => p.size_bytes))
    : 1;
  const severityMax = trends
    ? Math.max(1, ...trends.by_severity.map((b) => b.count))
    : 1;
  const statusMax = trends
    ? Math.max(1, ...trends.by_status.map((b) => b.count))
    : 1;
  const typeMax = trends?.by_type?.length
    ? Math.max(1, ...trends.by_type.map((b) => b.count))
    : 1;

  const envName =
    selectedEnvironment?.name ??
    (environmentId ? `${environmentId.slice(0, 8)}…` : null);

  return (
    <>
      <PageHeader
        eyebrow="Dashboard"
        title="Auditoria com evidências, sem mudanças automáticas"
        description="Conexões, indicadores, armazenamento, achados e saúde de tarefas. Use o filtro de ambiente na barra lateral para focar um alvo."
      />

      {environmentId && envName ? (
        <div className="mt-4 flex flex-wrap items-center gap-2 rounded-lg border border-slate-700 bg-slate-900/60 px-3 py-2 text-sm text-slate-200">
          <span>
            Mostrando: <strong className="text-slate-50">{envName}</strong>
          </span>
          <Button
            type="button"
            variant="ghost"
            onClick={() => setEnvironmentId(null)}
          >
            Ver todos
          </Button>
        </div>
      ) : null}

      {error ? (
        <div className="mt-8">
          <ErrorBanner
            message={error}
            onRetry={() => setReloadKey((k) => k + 1)}
          />
        </div>
      ) : null}

      {loading ? (
        <div className="mt-8 space-y-6">
          <Skeleton className="h-12 w-full" />
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
        </div>
      ) : null}

      {!loading && connections ? (
        <>
          <section className="mt-8" aria-labelledby="priority-heading">
            <h2
              id="priority-heading"
              className="text-sm font-medium text-slate-300"
            >
              Cinco prioridades
            </h2>
            {priorities.length === 0 ? (
              <div className="mt-3">
                <EmptyState
                  title="Nenhuma prioridade nesta semana"
                  description="Não há achado aberto com evidência suficiente. Isso não significa ausência de risco se a coleta estiver parcial."
                  action={
                    <Button
                      type="button"
                      onClick={() => setSection("Findings")}
                    >
                      Abrir achados
                    </Button>
                  }
                />
              </div>
            ) : (
              <ul className="mt-3 grid gap-3">
                {priorities.map((item) => (
                  <li key={item.id}>
                    <button
                      type="button"
                      className="w-full rounded-xl border border-slate-800 bg-slate-900/60 px-4 py-3 text-left"
                      onClick={() => openFinding(item.id)}
                    >
                      <span className="text-sm font-medium text-slate-50">
                        {item.title}
                      </span>
                      <span className="mt-1 block text-xs text-slate-400">
                        {item.severity} · confiança{" "}
                        {item.confidence != null
                          ? `${Math.round(item.confidence * 100)}%`
                          : "baixa"}{" "}
                        · {item.summary || "impacto não estimado"}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <section className="mt-8" aria-labelledby="conn-heading">
            <h2
              id="conn-heading"
              className="text-sm font-medium text-slate-300"
            >
              Conexões
            </h2>
            {connections.length === 0 ? (
              <div className="mt-3">
                <EmptyState
                  title="Nenhum ambiente configurado"
                  description="Configure AUDITOR_TARGET_DSN_* no backend e reinicie a API. Depois valide em Status."
                  action={
                    <Button type="button" onClick={() => setSection("Status")}>
                      Ir para Status
                    </Button>
                  }
                />
              </div>
            ) : (
              <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {connections.map((c) => {
                  const ok = c.dsn_configured && c.reachable;
                  const statusLabel = ok
                    ? "Conectado"
                    : c.dsn_configured
                      ? "Indisponível"
                      : "DSN ausente";
                  return (
                    <Card key={c.environment_id}>
                      <div className="flex items-center justify-between gap-3">
                        <strong className="min-w-0 truncate text-base font-semibold text-slate-50">
                          {c.environment_name}
                        </strong>
                        <Badge tone={ok ? "success" : "danger"}>
                          {statusLabel}
                        </Badge>
                      </div>
                      {c.server_version ? (
                        <p className="mt-2 font-mono text-xs text-slate-400">
                          PG {c.server_version}
                          {c.latency_ms != null ? ` · ${c.latency_ms} ms` : ""}
                        </p>
                      ) : null}
                      {c.error ? (
                        <p className="mt-2 truncate text-xs text-rose-300">
                          {c.error}
                        </p>
                      ) : null}
                    </Card>
                  );
                })}
              </div>
            )}
          </section>
        </>
      ) : null}

      {!loading && kpis ? (
        <div className="mt-8 space-y-6">
          <KpiGroup title="Risco">
            <Card
              subtitle="Achados abertos"
              title={String(kpis.open_findings)}
              onClick={() => setSection("Findings")}
            />
            <Card
              subtitle="Críticos / altos"
              title={`${kpis.critical_findings} / ${kpis.high_findings}`}
              onClick={() => setSection("Findings")}
            />
            <Card
              subtitle="Execuções concluídas / com falha"
              title={`${kpis.successful_runs_recent} / ${kpis.failed_runs_recent}`}
              onClick={() => setSection("Execuções")}
            />
          </KpiGroup>
          {scores.length > 0 ? (
            <KpiGroup title="Nota por esquema">
              {scores.slice(0, 6).map((item) => (
                <Card
                  key={`${item.database_name}.${item.schema_name ?? ""}`}
                  subtitle={`${item.database_name}.${item.schema_name ?? ""} · ${item.tables} ${item.status === "not_applicable" ? "coleções" : "tabelas"}`}
                  title={
                    item.status === "not_applicable"
                      ? "não aplicável"
                      : item.score == null
                        ? "cobertura insuficiente"
                        : `${item.score}/100`
                  }
                />
              ))}
            </KpiGroup>
          ) : null}
          <KpiGroup title="Capacidade">
            <Card
              subtitle="Total de bancos"
              title={kpis.databases == null ? "—" : String(kpis.databases)}
              onClick={() => setSection("Inventário")}
            />
            <Card
              subtitle="Total de esquemas"
              title={kpis.schemas == null ? "—" : String(kpis.schemas)}
              onClick={() => setSection("Inventário")}
            />
            <Card
              subtitle="Total de Tabelas"
              title={kpis.tables == null ? "—" : String(kpis.tables)}
              onClick={() => setSection("Inventário")}
            />
            <Card
              subtitle="Armazenamento total"
              title={formatBytes(kpis.total_storage_bytes)}
              onClick={() => setSection("Inventário")}
            />
            <Card
              subtitle="Tabelas temporais"
              title={String(kpis.hypertables)}
            />
            <Card subtitle="Políticas" title={String(kpis.policies)} />
          </KpiGroup>
          {kpis.inventory_status !== "complete" ? (
            <CoverageBanner
              kind={kpis.inventory_status === "partial" ? "partial" : null}
            />
          ) : null}
          {kpis.inventory_status === "empty" ? (
            <p className="text-sm text-amber-300" role="status">
              Ainda não há inventário concluído para o ambiente selecionado.
            </p>
          ) : null}
          <KpiGroup title="Operação">
            <Card
              subtitle="Ambientes"
              title={String(kpis.environments)}
              onClick={() => setSection("Ambientes")}
            />
            <Card
              subtitle="Tarefas agendadas"
              title={String(kpis.jobs_scheduled)}
            />
          </KpiGroup>
        </div>
      ) : null}

      {!loading ? (
        <div className="mt-8 grid gap-4 lg:grid-cols-2 lg:items-stretch">
          {storage ? (
            <Card title="Storage por ambiente" className="min-h-[22rem]">
              <div className="mt-2 flex flex-1 flex-col">
                <Suspense fallback={<p role="status">Carregando gráfico…</p>}>
                  <StoragePieChart items={storage.by_environment} />
                </Suspense>
              </div>
            </Card>
          ) : null}

          {trends ? (
            <Card
              title={`Achados (total ${trends.total})`}
              className="min-h-[22rem]"
            >
              <div className="mt-2 grid flex-1 gap-4 sm:grid-cols-2">
                <div>
                  <p className="mb-2 text-xs font-medium text-slate-400">
                    Severidade
                  </p>
                  <ul className="space-y-2.5">
                    {trends.by_severity.map((b) => (
                      <li key={b.key}>
                        <MiniBar
                          label={labels.severity(b.key)}
                          value={b.count}
                          max={severityMax}
                          tone={
                            b.key === "critical" || b.key === "high"
                              ? "rose"
                              : b.key === "medium"
                                ? "amber"
                                : "emerald"
                          }
                        />
                      </li>
                    ))}
                  </ul>
                </div>
                <div>
                  <p className="mb-2 text-xs font-medium text-slate-400">
                    Status
                  </p>
                  <ul className="space-y-2.5">
                    {trends.by_status.map((b) => (
                      <li key={b.key}>
                        <MiniBar
                          label={labels.findingStatus(b.key)}
                          value={b.count}
                          max={statusMax}
                          tone="amber"
                        />
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            </Card>
          ) : null}

          {storage?.top_consumers && storage.top_consumers.length > 0 ? (
            <Card
              title="Maiores consumidores (tabelas)"
              subtitle="A barra é relativa ao maior objeto da lista; o tamanho aparece ao lado."
              className="min-h-[22rem]"
            >
              <ul className="mt-1 min-h-0 flex-1 space-y-2.5 overflow-y-auto pr-1">
                {storage.top_consumers.slice(0, 12).map((p) => (
                  <li key={`${p.object_kind}-${p.label}`}>
                    <MiniBar
                      label={p.label}
                      value={p.size_bytes}
                      max={consumersMax}
                      tone="sky"
                      valueLabel={formatBytes(p.size_bytes)}
                    />
                  </li>
                ))}
              </ul>
            </Card>
          ) : null}

          {trends?.by_type && trends.by_type.length > 0 ? (
            <Card title="Achados por tipo" className="min-h-[22rem]">
              <ul className="mt-1 min-h-0 flex-1 space-y-2.5 overflow-y-auto pr-1">
                {trends.by_type.slice(0, 12).map((b) => (
                  <li key={b.key}>
                    <MiniBar
                      label={b.key}
                      value={b.count}
                      max={typeMax}
                      tone="emerald"
                    />
                  </li>
                ))}
              </ul>
            </Card>
          ) : null}
        </div>
      ) : null}

      {!loading && storage && trends ? (
        <section className="mt-8" aria-labelledby="history-heading">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2
              id="history-heading"
              className="text-sm font-medium text-slate-300"
            >
              Evolução por execução
            </h2>
            <div className="flex gap-2">
              <select
                aria-label="Período do histórico"
                value={historyDays}
                onChange={(event) => setHistoryDays(Number(event.target.value))}
                className="rounded border border-slate-600 bg-slate-900 px-2 py-1 text-xs text-slate-100"
              >
                <option value={30}>30 dias</option>
                <option value={90}>90 dias</option>
                <option value={365}>365 dias</option>
              </select>
              <select
                aria-label="Granularidade do histórico"
                value={historyGranularity}
                onChange={(event) =>
                  setHistoryGranularity(
                    event.target.value as "day" | "week" | "month",
                  )
                }
                className="rounded border border-slate-600 bg-slate-900 px-2 py-1 text-xs text-slate-100"
              >
                <option value="day">Diária</option>
                <option value="week">Semanal</option>
                <option value="month">Mensal</option>
              </select>
            </div>
          </div>
          <div className="mt-3 grid gap-4 lg:grid-cols-2">
            <Card title="Armazenamento ao longo do tempo">
              <TrendBars
                points={storage.series ?? []}
                metric="size_bytes"
                granularity={historyGranularity}
              />
            </Card>
            <Card title="Achados observados por execução">
              <TrendBars
                points={trends.series ?? []}
                metric="findings"
                granularity={historyGranularity}
              />
            </Card>
            <Card
              title="Score ao longo do tempo"
              subtitle="A nota só aparece quando a coleta e a análise têm cobertura suficiente."
            >
              <TrendBars
                points={trends.series ?? []}
                metric="score"
                granularity={historyGranularity}
              />
            </Card>
            <Card
              title="Cobertura do score"
              subtitle="Percentual dos coletores necessários que concluíram com sucesso."
            >
              <TrendBars
                points={trends.series ?? []}
                metric="score_confidence"
                granularity={historyGranularity}
              />
            </Card>
          </div>
        </section>
      ) : null}

      {!loading && jobs ? (
        <section className="mt-8" aria-labelledby="jobs-heading">
          <h2 id="jobs-heading" className="text-sm font-medium text-slate-300">
            Saúde de tarefas e políticas
          </h2>
          {jobs.items.length === 0 ? (
            <p className="mt-2 text-sm text-slate-400">
              Nenhum ambiente com dados de jobs. Dispare uma coleta em
              Execuções.
            </p>
          ) : (
            <div className="mt-3">
              <Table
                dense
                headers={["Ambiente", "Tarefas", "Agendadas", "Políticas"]}
              >
                {jobs.items.map((item) => (
                  <tr key={item.environment_id}>
                    <td className="px-3 py-2">
                      {item.environment_name || item.environment_id.slice(0, 8)}
                    </td>
                    <td className="px-3 py-2 font-mono text-xs">
                      {item.jobs_total}
                    </td>
                    <td className="px-3 py-2 font-mono text-xs">
                      {item.jobs_scheduled}
                    </td>
                    <td className="px-3 py-2 font-mono text-xs">
                      {item.policies_total}
                    </td>
                  </tr>
                ))}
              </Table>
            </div>
          )}
        </section>
      ) : null}
    </>
  );
}
