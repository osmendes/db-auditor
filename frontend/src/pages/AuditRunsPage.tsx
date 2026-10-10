import { useCallback, useEffect, useMemo, useState } from "react";
import type { PageSize } from "../components/ui/PaginationControls";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { labels } from "../lib/labels";
import { fetchAllPages } from "../lib/pagination";
import { api } from "../services/api";
import type {
  AnalysisRun,
  AuditBaseline,
  AuditRun,
  AuditRunCoverage,
  BaselineComparison,
  CollectorRun,
  Finding,
  ScopeScore,
} from "../types";
import { AuditRunsView } from "./parts/AuditRunsView";
import { type AuditScheduleRow, POLL_MS } from "./parts/auditRunUi";

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
  const [baseline, setBaseline] = useState<AuditBaseline | null>(null);
  const [baselineToken, setBaselineToken] = useState("");
  const [comparison, setComparison] = useState<BaselineComparison | null>(null);
  const [scopeScore, setScopeScore] = useState<ScopeScore | null>(null);
  const [runFindings, setRunFindings] = useState<Finding[] | null>(null);
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
    void api
      .schedules(triggerEnv)
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
    };
  }, [triggerEnv]);

  const loadRuns = useCallback(() => {
    setError(null);
    const filters = {
      status: statusFilter || undefined,
      profile: profileFilter || undefined,
      environment_id: environmentId || undefined,
    };
    const request =
      runPageSize === "all"
        ? fetchAllPages((offset, limit) =>
            api.auditRunsPage({ ...filters, offset, limit }),
          ).then((items) => ({ items, total: items.length }))
        : api
            .auditRunsPage({
              ...filters,
              offset: runOffset,
              limit: runPageSize,
            })
            .then((result) => ({
              items: result.items,
              total: result.page.total,
            }));
    return request
      .then((res) => {
        setRuns(res.items);
        setRunTotal(res.total);
        return res.items;
      })
      .catch((err: unknown) => {
        setError(
          formatError(
            err,
            "Falha ao listar execuções. Verifique a API e tente novamente.",
          ),
        );
        setRuns([]);
        return [] as AuditRun[];
      });
  }, [statusFilter, profileFilter, environmentId, runOffset, runPageSize]);

  useEffect(() => {
    setRuns(null);
    void loadRuns();
  }, [loadRuns]);

  const loadCollectors = useCallback(async (runId: string) => {
    try {
      const res = await api.auditRunCollectors(runId);
      setCollectors(res.items);
      return res.items;
    } catch {
      setCollectors([]);
      return [] as CollectorRun[];
    }
  }, []);

  const loadRunDiagnostics = useCallback(async (runId: string) => {
    const [coverageResult, analysisResult] = await Promise.allSettled([
      api.auditRunCoverage(runId),
      api.auditRunAnalysis(runId),
    ]);
    setCoverage(
      coverageResult.status === "fulfilled" ? coverageResult.value.items : [],
    );
    setAnalysis(
      analysisResult.status === "fulfilled" ? analysisResult.value : null,
    );
  }, []);

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

  useEffect(() => {
    if (!selected || selected.status === "running") {
      setBaseline(null);
      setComparison(null);
      setScopeScore(null);
      return;
    }
    let active = true;
    void Promise.allSettled([
      api.auditBaseline(selected.environment_id),
      api.baselineComparisons(selected.environment_id, selected.id),
      api.scopeScore(selected.environment_id, selected.id),
    ]).then(([base, comparisons, score]) => {
      if (!active) return;
      setBaseline(base.status === "fulfilled" ? base.value : null);
      setComparison(
        comparisons.status === "fulfilled"
          ? (comparisons.value.items[0] ?? null)
          : null,
      );
      setScopeScore(score.status === "fulfilled" ? score.value : null);
    });
    return () => {
      active = false;
    };
  }, [selected]);

  useEffect(() => {
    if (!selectedId) {
      setRunFindings(null);
      return;
    }
    let active = true;
    setRunFindings(null);
    void api
      .runFindings(selectedId)
      .then((result) => {
        if (active) setRunFindings(result.items ?? []);
      })
      .catch((cause: unknown) => {
        if (active) {
          setRunFindings([]);
          setError(
            formatError(
              cause,
              "Não foi possível ler os findings desta execução",
            ),
          );
        }
      });
    return () => {
      active = false;
    };
  }, [selectedId]);

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
