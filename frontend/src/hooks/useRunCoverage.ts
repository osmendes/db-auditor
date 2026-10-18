import { useEffect, useState } from "react";
import {
  type CoverageSignal,
  deriveCoverageSignals,
  latestCompletedRuns,
} from "../lib/coverage";
import { api } from "../services/api";
import type { AuditRun } from "../types";

export interface RunCoverageState {
  /** Run(s) the signals describe: one per environment in scope. */
  runs: AuditRun[];
  signals: CoverageSignal[];
  loading: boolean;
}

// "Todos os ambientes" can be large; the banner describes the first few.
const MAX_ENVIRONMENTS = 8;

/**
 * Coverage of what a screen is showing (UI-08). With `runId`, describes that
 * run (Reports); otherwise the latest completed run of the environment, or of
 * each visible environment when none is selected (Dashboard, Findings).
 */
export function useRunCoverage(
  environmentId: string | null,
  runId?: string | null,
  reloadKey?: unknown,
): RunCoverageState {
  const [state, setState] = useState<RunCoverageState>({
    runs: [],
    signals: [],
    loading: true,
  });
  useEffect(() => {
    const controller = new AbortController();
    const { signal } = controller;
    let active = true;
    setState((prev) => ({ ...prev, loading: true }));
    void (async () => {
      try {
        const all = (
          await api.auditRuns(
            environmentId ? { environment_id: environmentId } : undefined,
            signal,
          )
        ).items;
        const selected = runId
          ? all.filter((run) => run.id === runId)
          : latestCompletedRuns(all).slice(0, MAX_ENVIRONMENTS);
        const perRun = await Promise.all(
          selected.map(async (run) => {
            const [coverage, capabilities] = await Promise.all([
              api
                .auditRunCoverage(run.id, signal)
                .then((res) => res.items)
                .catch(() => []),
              api
                .environmentCapabilities(run.environment_id, signal)
                .catch(() => null),
            ]);
            return deriveCoverageSignals({
              run,
              runs: all,
              coverage,
              capabilities,
            });
          }),
        );
        if (!active) return;
        setState({
          runs: selected,
          signals: perRun.flat(),
          loading: false,
        });
      } catch {
        if (active) setState({ runs: [], signals: [], loading: false });
      }
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [environmentId, runId, reloadKey]);
  return state;
}
