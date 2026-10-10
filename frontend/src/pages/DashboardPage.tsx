import { useEffect, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import { Button, ErrorBanner, Skeleton } from "../components/ui";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { api } from "../services/api";
import type {
  ConnectionStatus,
  DashboardKPIs,
  Finding,
  FindingsTrendResponse,
  JobHealthResponse,
  ScopeAggregate,
  StorageGrowthResponse,
} from "../types";
import { DashboardAnalytics } from "./parts/DashboardAnalytics";
import { DashboardConnections } from "./parts/DashboardConnections";
import { DashboardKpis } from "./parts/DashboardKpis";
import { DashboardPriorities } from "./parts/DashboardPriorities";

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
          <DashboardPriorities
            priorities={priorities}
            setSection={setSection}
            openFinding={openFinding}
          />
          <DashboardConnections
            connections={connections}
            setSection={setSection}
          />
        </>
      ) : null}

      {!loading && kpis ? (
        <DashboardKpis kpis={kpis} scores={scores} setSection={setSection} />
      ) : null}

      <DashboardAnalytics
        loading={loading}
        storage={storage}
        trends={trends}
        jobs={jobs}
        consumersMax={consumersMax}
        severityMax={severityMax}
        statusMax={statusMax}
        typeMax={typeMax}
        historyDays={historyDays}
        setHistoryDays={setHistoryDays}
        historyGranularity={historyGranularity}
        setHistoryGranularity={setHistoryGranularity}
      />
    </>
  );
}
