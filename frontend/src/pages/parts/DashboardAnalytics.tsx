import { Suspense } from "react";
import { Card, Table } from "../../components/ui";
import { labels } from "../../lib/labels";
import type {
  FindingsTrendResponse,
  JobHealthResponse,
  StorageGrowthResponse,
} from "../../types";
import {
  formatBytes,
  MiniBar,
  StoragePieChart,
  TrendBars,
} from "./DashboardCharts";

export function DashboardAnalytics({
  loading,
  storage,
  trends,
  jobs,
  consumersMax,
  severityMax,
  statusMax,
  typeMax,
  historyDays,
  setHistoryDays,
  historyGranularity,
  setHistoryGranularity,
}: {
  loading: boolean;
  storage: StorageGrowthResponse | null;
  trends: FindingsTrendResponse | null;
  jobs: JobHealthResponse | null;
  consumersMax: number;
  severityMax: number;
  statusMax: number;
  typeMax: number;
  historyDays: number;
  setHistoryDays: (value: number) => void;
  historyGranularity: "day" | "week" | "month";
  setHistoryGranularity: (value: "day" | "week" | "month") => void;
}) {
  return (
    <>
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
