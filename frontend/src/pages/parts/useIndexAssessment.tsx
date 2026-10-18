import { useEffect, useRef, useState } from "react";
import {
  InventoryDetailAvailability,
  type InventoryDetailTab,
} from "../../components/InventoryDetailTabs";
import { PaginationControls } from "../../components/ui/PaginationControls";
import { ApiError, formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type {
  Finding,
  GrantSnapshot,
  IndexHistoryPoint,
  IndexSnapshot,
  PagedResponse,
  SnapshotCompleteness,
} from "../../types";
import type { IndexDetailSnapshot } from "../../types/openapi";

/** Data and state of IndexAssessmentPanel (UI-06 extraction). */
export function usePage<T>(
  active: boolean,
  scope: string,
  load: (offset: number, signal?: AbortSignal) => Promise<PagedResponse<T>>,
) {
  const loader = useRef(load);
  loader.current = load;
  const [offset, setOffset] = useState(0);
  const [data, setData] = useState<PagedResponse<T> | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    setOffset(0);
    setData(null);
    setError(null);
  }, [scope]);
  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void loader
      .current(offset, controller.signal)
      .then((value) => {
        if (!cancelled) setData(value);
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setData(null);
          setError(cause);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [active, scope, offset]);
  return { offset, setOffset, data, error, loading };
}

export function state(error: unknown, loading: boolean, hasData: boolean) {
  if (loading || (!hasData && !error)) return <p role="status">Carregando…</p>;
  if (
    error instanceof ApiError &&
    (error.status === 401 || error.status === 403)
  )
    return <InventoryDetailAvailability state="forbidden" />;
  if (error)
    return (
      <InventoryDetailAvailability state="error" detail={formatError(error)} />
    );
  return null;
}

export function controls(
  total: number,
  offset: number,
  setOffset: (value: number) => void,
) {
  if (total <= 50) return null;
  return (
    <PaginationControls
      total={total}
      offset={offset}
      size={50}
      allowAll={false}
      onOffsetChange={setOffset}
      onSizeChange={() => setOffset(0)}
      label="Detalhe do índice"
    />
  );
}

export function comparableIndexScans(
  points: IndexHistoryPoint[],
): number | null {
  if (points.length < 2) return null;
  const [current, previous] = points;
  if (
    !current.usage_observed ||
    !previous.usage_observed ||
    !current.stats_reset ||
    current.stats_reset !== previous.stats_reset ||
    current.definition_fingerprint !== previous.definition_fingerprint ||
    current.idx_scan < previous.idx_scan
  )
    return null;
  return current.idx_scan - previous.idx_scan;
}

export function useIndexAssessment({
  index,
  environment,
  run,
  snapshot,
}: {
  index: IndexSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<IndexDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const scope = JSON.stringify([
    environment,
    run,
    index.database_name,
    index.schema_name,
    index.index_name,
  ]);
  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .indexDetail(
        environment,
        run,
        index.database_name,
        index.schema_name,
        index.index_name,
        controller.signal,
      )
      .then((value) => {
        if (!cancelled) setDetail(value);
      })
      .catch((cause: unknown) => {
        if (!cancelled) setDetailError(cause);
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [scope]);
  const history = usePage<IndexHistoryPoint>(
    tab === "performance",
    scope,
    (offset, signal) =>
      api.indexHistory(
        environment,
        run,
        index.database_name,
        index.schema_name,
        index.index_name,
        50,
        offset,
        signal,
      ),
  );
  const grants = usePage<GrantSnapshot>(
    tab === "security",
    scope,
    (offset, signal) =>
      api.tableGrants(
        environment,
        run,
        index.database_name,
        index.schema_name,
        index.table_name,
        50,
        offset,
        signal,
      ),
  );
  const findings = usePage<Finding>(
    tab === "recommendations",
    scope,
    (offset, signal) =>
      api.indexFindings(
        environment,
        run,
        index.database_name,
        index.schema_name,
        index.index_name,
        50,
        offset,
        signal,
      ),
  );
  const detailState = state(detailError, detailLoading, detail !== null);
  const target = {
    kind: "indexes" as const,
    database: index.database_name,
    schema: index.schema_name,
    name: index.index_name,
  };
  const tableTarget = {
    kind: "tables" as const,
    database: index.database_name,
    schema: index.schema_name,
    name: detail?.table_name ?? index.table_name,
  };
  const delta = comparableIndexScans(history.data?.items ?? []);
  return {
    delta,
    detail,
    detailError,
    detailLoading,
    detailState,
    environment,
    findings,
    grants,
    history,
    index,
    run,
    scope,
    setDetail,
    setDetailError,
    setDetailLoading,
    setTab,
    snapshot,
    tab,
    tableTarget,
    target,
  };
}

export type IndexAssessmentState = ReturnType<typeof useIndexAssessment>;
