import { useEffect, useState } from "react";
import { api } from "../services/api";

export interface EmptyContext {
  /** Every connection in scope lacks a DSN. */
  dsnMissing: boolean;
  /** At least one run finished (success or partial) in scope. */
  collected: boolean;
  loading: boolean;
}

/** Inputs of `resolveEmptyCause`; fetched only while a list is empty. */
export async function fetchEmptyContext(
  environmentId: string | null,
  signal?: AbortSignal,
): Promise<Omit<EmptyContext, "loading">> {
  const [runs, connections] = await Promise.all([
    api.auditRuns({ environment_id: environmentId || undefined }, signal),
    api.connectionStatus(signal),
  ]);
  const scoped = environmentId
    ? connections.items.filter((item) => item.environment_id === environmentId)
    : connections.items;
  return {
    dsnMissing: scoped.length > 0 && scoped.every((i) => !i.dsn_configured),
    collected: runs.items.some(
      (run) => run.status === "success" || run.status === "partial_success",
    ),
  };
}

export function useEmptyContext(
  environmentId: string | null,
  enabled: boolean,
): EmptyContext {
  const [state, setState] = useState<EmptyContext>({
    dsnMissing: false,
    collected: true,
    loading: false,
  });
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    let active = true;
    setState((prev) => ({ ...prev, loading: true }));
    fetchEmptyContext(environmentId, controller.signal)
      .then((next) => {
        if (active) setState({ ...next, loading: false });
      })
      .catch(() => {
        if (active) setState((prev) => ({ ...prev, loading: false }));
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [environmentId, enabled]);
  return state;
}
