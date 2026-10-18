import { useEffect, useState } from "react";
import { formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type {
  AuditBaseline,
  AuditRun,
  BaselineComparison,
  Finding,
  ScopeScore,
} from "../../types";

/**
 * Baseline, comparison, score and findings of the selected run. Every request
 * is aborted when the selection changes or the page unmounts, so a slow answer
 * for the previous run never lands on the new one (PF-02).
 */
export function useSelectedRunContext(
  selected: AuditRun | null,
  selectedId: string | null,
  setError: (message: string | null) => void,
) {
  const [baseline, setBaseline] = useState<AuditBaseline | null>(null);
  const [comparison, setComparison] = useState<BaselineComparison | null>(null);
  const [scopeScore, setScopeScore] = useState<ScopeScore | null>(null);
  const [runFindings, setRunFindings] = useState<Finding[] | null>(null);

  useEffect(() => {
    if (!selected || selected.status === "running") {
      setBaseline(null);
      setComparison(null);
      setScopeScore(null);
      return;
    }
    let active = true;
    const controller = new AbortController();
    const { signal } = controller;
    void Promise.allSettled([
      api.auditBaseline(selected.environment_id, "", "", "", signal),
      api.baselineComparisons(selected.environment_id, selected.id, signal),
      api.scopeScore(
        selected.environment_id,
        selected.id,
        undefined,
        undefined,
        undefined,
        signal,
      ),
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
      controller.abort();
    };
  }, [selected]);

  useEffect(() => {
    if (!selectedId) {
      setRunFindings(null);
      return;
    }
    let active = true;
    const controller = new AbortController();
    setRunFindings(null);
    void api
      .runFindings(selectedId, controller.signal)
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
      controller.abort();
    };
  }, [selectedId]);

  return { baseline, setBaseline, comparison, scopeScore, runFindings };
}
