import { useEffect, useState } from "react";
import { CoverageBanner } from "../components/CoverageBanner";
import { PageHeader } from "../components/PageHeader";
import { Button, ErrorBanner, Skeleton } from "../components/ui";
import { useApp } from "../context/AppContext";
import { useRunCoverage } from "../hooks/useRunCoverage";
import { scoreIsPartial } from "../lib/coverage";
import { formatError } from "../lib/errors";
import { api } from "../services/api";
import type {
  ConnectionStatus,
  DashboardKPIs,
  FindingsTrendResponse,
  JobHealthResponse,
  PriorityItem,
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
  const [priorities, setPriorities] = useState<PriorityItem[]>([]);
  const [priorityFormula, setPriorityFormula] = useState<string>();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);
  const [historyDays, setHistoryDays] = useState(90);
  const [historyGranularity, setHistoryGranularity] = useState<
    "day" | "week" | "month"
  >("day");
  const coverage = useRunCoverage(environmentId, null, reloadKey);
  const scoreRunId =
    environmentId && coverage.runs[0]?.environment_id === environmentId
      ? coverage.runs[0].id
      : null;

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();
    const { signal } = controller;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const params = environmentId
          ? { environment_id: environmentId }
          : undefined;
        // One clock reading for both ends: the API rejects windows over 90 days.
        const to = new Date();
        const historyParams = {
          ...params,
          from: new Date(to.getTime() - historyDays * 86400000).toISOString(),
          to: to.toISOString(),
          granularity: historyGranularity,
        };
        const [k, s, t, j, c] = await Promise.all([
          api.analyticsKpis(params, signal),
          api.analyticsStorage(historyParams, signal),
          api.analyticsFindingsTrends(historyParams, signal),
          api.analyticsJobHealth(params, signal),
          api.connectionStatus(signal),
        ]);
        if (!cancelled) {
          setKpis(k);
          setStorage(s);
          setTrends(t);
          setJobs(j);
          setConnections(c.items);
          const queue = await api.findingPriorities(
            { environment_id: environmentId ?? undefined, limit: 5 },
            signal,
          );
          if (!cancelled) {
            setPriorities(queue.items.slice(0, 5));
            setPriorityFormula(queue.formula);
          }
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
      controller.abort();
    };
  }, [environmentId, reloadKey, historyDays, historyGranularity]);

  // Scores come from the same run the coverage banner describes (the latest
  // completed one, partial included), never from an older complete run that
  // would hide a newer partial collection (UI-08).
  useEffect(() => {
    if (!environmentId || !scoreRunId) {
      setScores([]);
      return;
    }
    const controller = new AbortController();
    let active = true;
    void api
      .scopeScores(environmentId, scoreRunId, controller.signal)
      .then((agg) => {
        if (active) setScores(agg.items);
      })
      .catch(() => {
        if (active) setScores([]);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [environmentId, scoreRunId]);

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

      {coverage.signals.length > 0 ? (
        <div className="mt-4">
          <CoverageBanner signals={coverage.signals} />
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
            formula={priorityFormula}
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
        <DashboardKpis
          kpis={kpis}
          scores={scores}
          scorePartial={scoreIsPartial(coverage.signals)}
          setSection={setSection}
        />
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
