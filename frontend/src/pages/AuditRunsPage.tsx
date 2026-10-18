import { useCallback, useEffect, useMemo, useState } from "react";
import type { PageSize } from "../components/ui/PaginationControls";
import { useApp } from "../context/AppContext";
import { useLatestSignal } from "../hooks/useLatestSignal";
import { formatError, isAbortError } from "../lib/errors";
import { labels } from "../lib/labels";
import { fetchAllPages } from "../lib/pagination";
import { api } from "../services/api";
import type {
  AnalysisRun,
  AuditRun,
  AuditRunCoverage,
  CollectorRun,
} from "../types";
import { AuditRunsView } from "./parts/AuditRunsView";
import { type AuditScheduleRow, POLL_MS } from "./parts/auditRunUi";
import { useSelectedRunContext } from "./parts/useSelectedRunContext";

export function AuditRunsPage() {
  const { environments, environmentId, runId, openRun, openFinding } = useApp();
  const [runs, setRuns] = useState<AuditRun[] | null>(null);
  const [runTotal, setRunTotal] = useState(0);
  const [runOffset, setRunOffset] = useState(0);
  const [runPageSize, setRunPageSize] = useState<PageSize>(20);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState("");
  const [profileFilter, setProfileFilter] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(runId);

  useEffect(() => {
    if (runId) setSelectedId(runId);
  }, [runId]);
  const [collectors, setCollectors] = useState<CollectorRun[] | null>(null);
  const [coverage, setCoverage] = useState<AuditRunCoverage[] | null>(null);
  const [analysis, setAnalysis] = useState<AnalysisRun | null>(null);
  const [baselineToken, setBaselineToken] = useState("");
  const [triggerEnv, setTriggerEnv] = useState("");
  const [triggerMsg, setTriggerMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [polling, setPolling] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [schedules, setSchedules] = useState<AuditScheduleRow[]>([]);
  const [scheduleMsg, setScheduleMsg] = useState<string | null>(null);

  useEffect(() => {
    if (environmentId) {
      setTriggerEnv(environmentId);
    } else if (environments.length > 0 && !triggerEnv) {
      setTriggerEnv(environments[0].id);
    }
  }, [environmentId, environments, triggerEnv]);

  useEffect(() => {
    if (!triggerEnv) {
      setSchedules([]);
      return;
    }
    let cancelled = false;
    const controller = new AbortController();
    void api
      .schedules(triggerEnv, controller.signal)
      .then((result) => {
        if (!cancelled) {
          setSchedules(result.items);
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setScheduleMsg(formatError(cause));
        }
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [triggerEnv]);

  const nextRunsSignal = useLatestSignal();
  const nextCollectorsSignal = useLatestSignal();
  const nextDiagnosticsSignal = useLatestSignal();

  // Each call aborts the previous one: a slow answer for the old environment
  // or page can no longer overwrite the current list (PF-02).
  const loadRuns = useCallback(() => {
    const signal = nextRunsSignal();
    setError(null);
    const filters = {
      status: statusFilter || undefined,
      profile: profileFilter || undefined,
      environment_id: environmentId || undefined,
    };
    const request =
      runPageSize === "all"
        ? fetchAllPages(
            (offset, limit) =>
              api.auditRunsPage({ ...filters, offset, limit }, signal),
            signal,
          ).then((items) => ({ items, total: items.length }))
        : api
            .auditRunsPage(
              {
                ...filters,
                offset: runOffset,
                limit: runPageSize,
              },
              signal,
            )
            .then((result) => ({
              items: result.items,
              total: result.page.total,
            }));
    return request
      .then((res) => {
        if (signal.aborted) return res.items;
        setRuns(res.items);
        setRunTotal(res.total);
        return res.items;
      })
      .catch((err: unknown) => {
        if (signal.aborted || isAbortError(err)) return [] as AuditRun[];
        setError(
          formatError(
            err,
            "Falha ao listar execuções. Verifique a API e tente novamente.",
          ),
        );
        setRuns([]);
        return [] as AuditRun[];
      });
  }, [
    statusFilter,
    profileFilter,
    environmentId,
    runOffset,
    runPageSize,
    nextRunsSignal,
  ]);

  useEffect(() => {
    setRuns(null);
    void loadRuns();
  }, [loadRuns]);

  const loadCollectors = useCallback(
    async (runId: string) => {
      const signal = nextCollectorsSignal();
      try {
        const res = await api.auditRunCollectors(runId, signal);
        if (!signal.aborted) setCollectors(res.items);
        return res.items;
      } catch {
        if (!signal.aborted) setCollectors([]);
        return [] as CollectorRun[];
      }
    },
    [nextCollectorsSignal],
  );

  const loadRunDiagnostics = useCallback(
    async (runId: string) => {
      const signal = nextDiagnosticsSignal();
      const [coverageResult, analysisResult] = await Promise.allSettled([
        api.auditRunCoverage(runId, signal),
        api.auditRunAnalysis(runId, signal),
      ]);
      if (signal.aborted) return;
      setCoverage(
        coverageResult.status === "fulfilled" ? coverageResult.value.items : [],
      );
      setAnalysis(
        analysisResult.status === "fulfilled" ? analysisResult.value : null,
      );
    },
    [nextDiagnosticsSignal],
  );

  useEffect(() => {
    if (!selectedId) {
      setCollectors(null);
      setCoverage(null);
      setAnalysis(null);
      return;
    }
    setCollectors(null);
    setCoverage(null);
    void loadCollectors(selectedId);
    void loadRunDiagnostics(selectedId);
  }, [selectedId, loadCollectors, loadRunDiagnostics]);

  const selected = useMemo(
    () => runs?.find((r) => r.id === selectedId) ?? null,
    [runs, selectedId],
  );

  const { baseline, setBaseline, comparison, scopeScore, runFindings } =
    useSelectedRunContext(selected, selectedId, setError);

  const chooseBaseline = async () => {
    if (
      !selected ||
      !window.confirm(
        `Usar a execução ${selected.id} como baseline aprovado para todo o ambiente?`,
      )
    )
      return;
    try {
      api.setReportToken(baselineToken);
      setBaseline(
        await api.selectAuditBaseline(selected.environment_id, selected.id),
      );
      setError(null);
    } catch (err: unknown) {
      setError(formatError(err, "Não foi possível aprovar o baseline"));
    }
  };

  useEffect(() => {
    const listRunning = runs?.some((r) => r.status === "running") ?? false;
    const selectedRunning = selected?.status === "running";
    if ((!listRunning && !selectedRunning) || runPageSize === "all") {
      setPolling(false);
      return;
    }
    setPolling(true);
    const id = window.setInterval(() => {
      void loadRuns().then((items) => {
        const still = items.find((r) => r.id === selectedId);
        if (still && selectedId) {
          void loadCollectors(selectedId);
          void loadRunDiagnostics(selectedId);
        }
      });
    }, POLL_MS);
    return () => window.clearInterval(id);
  }, [
    runs,
    selected,
    selectedId,
    loadRuns,
    loadCollectors,
    loadRunDiagnostics,
    runPageSize,
  ]);

  const triggerEnvName =
    environments.find((e) => e.id === triggerEnv)?.name ??
    (triggerEnv ? `${triggerEnv.slice(0, 8)}…` : "—");

  const executeTrigger = async () => {
    if (!triggerEnv) {
      return;
    }
    setBusy(true);
    setTriggerMsg(null);
    try {
      const res = await api.triggerAuditRun(triggerEnv, "manual");
      setTriggerMsg(
        `Execução iniciada (${res.audit_run_id.slice(0, 8)}…) — ${labels.runStatus(res.status)}`,
      );
      setSelectedId(res.audit_run_id);
      openRun(res.audit_run_id);
      await loadRuns();
      await loadCollectors(res.audit_run_id);
    } catch (err: unknown) {
      setTriggerMsg(
        formatError(
          err,
          "Não foi possível iniciar a execução. Verifique o ambiente e a API.",
        ),
      );
    } finally {
      setBusy(false);
      setConfirmOpen(false);
    }
  };

  const onTriggerClick = () => {
    if (!triggerEnv) {
      return;
    }
    setConfirmOpen(true);
  };

  return (
    <AuditRunsView
      polling={polling}
      confirmOpen={confirmOpen}
      setConfirmOpen={setConfirmOpen}
      triggerEnvName={triggerEnvName}
      busy={busy}
      executeTrigger={executeTrigger}
      environments={environments}
      triggerEnv={triggerEnv}
      setTriggerEnv={setTriggerEnv}
      onTriggerClick={onTriggerClick}
      triggerMsg={triggerMsg}
      schedules={schedules}
      setBusy={setBusy}
      setScheduleMsg={setScheduleMsg}
      setSchedules={setSchedules}
      scheduleMsg={scheduleMsg}
      statusFilter={statusFilter}
      setStatusFilter={setStatusFilter}
      profileFilter={profileFilter}
      setProfileFilter={setProfileFilter}
      setRunOffset={setRunOffset}
      error={error}
      loadRuns={loadRuns}
      runs={runs}
      selectedId={selectedId}
      setSelectedId={setSelectedId}
      openRun={openRun}
      runTotal={runTotal}
      runPageSize={runPageSize}
      runOffset={runOffset}
      setRunPageSize={setRunPageSize}
      selected={selected}
      setError={setError}
      baseline={baseline}
      baselineToken={baselineToken}
      setBaselineToken={setBaselineToken}
      chooseBaseline={chooseBaseline}
      comparison={comparison}
      scopeScore={scopeScore}
      coverage={coverage}
      analysis={analysis}
      loadRunDiagnostics={loadRunDiagnostics}
      runFindings={runFindings}
      openFinding={openFinding}
      collectors={collectors}
      loadCollectors={loadCollectors}
    />
  );
}
