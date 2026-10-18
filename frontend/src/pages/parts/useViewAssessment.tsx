import { useEffect, useRef, useState } from "react";
import {
  InventoryDetailAvailability,
  type InventoryDetailTab,
} from "../../components/InventoryDetailTabs";
import { PaginationControls } from "../../components/ui/PaginationControls";
import { ApiError, formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type {
  DependencySnapshot,
  Finding,
  GrantSnapshot,
  PagedResponse,
  SnapshotCompleteness,
  ViewDetailSnapshot,
  ViewSnapshot,
} from "../../types";

/** Data and state of ViewAssessmentPanel (UI-06 extraction). */
export function useViewCollection<T>(
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
      .then((result) => {
        if (!cancelled) setData(result);
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
  return { data, error, loading, offset, setOffset };
}

export function collectionState(
  error: unknown,
  loading: boolean,
  hasData: boolean,
) {
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

export function pageControls(
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
      label="Detalhe da visão"
    />
  );
}

export function useViewAssessment({
  view,
  environment,
  run,
  snapshot,
}: {
  view: ViewSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<ViewDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const scope = JSON.stringify([
    environment,
    run,
    view.database_name,
    view.schema_name,
    view.view_name,
  ]);
  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .viewDetail(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
        controller.signal,
      )
      .then((item) => {
        if (!cancelled) setDetail(item);
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
  const dependencies = useViewCollection<DependencySnapshot>(
    tab === "relationships",
    scope,
    (offset, signal) =>
      api.viewDependencies(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
        50,
        offset,
        signal,
      ),
  );
  const grants = useViewCollection<GrantSnapshot>(
    tab === "security",
    scope,
    (offset, signal) =>
      api.viewGrants(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
        50,
        offset,
        signal,
      ),
  );
  const findings = useViewCollection<Finding>(
    tab === "recommendations",
    scope,
    (offset, signal) =>
      api.viewFindings(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
        50,
        offset,
        signal,
      ),
  );
  const target = {
    kind: "views" as const,
    database: view.database_name,
    schema: view.schema_name,
    name: view.view_name,
  };
  const materialized = view.relkind === "m";
  const detailState = collectionState(
    detailError,
    detailLoading,
    detail !== null,
  );
  return {
    dependencies,
    detail,
    detailError,
    detailLoading,
    detailState,
    environment,
    findings,
    grants,
    materialized,
    run,
    scope,
    setDetail,
    setDetailError,
    setDetailLoading,
    setTab,
    snapshot,
    tab,
    target,
    view,
  };
}

export type ViewAssessmentState = ReturnType<typeof useViewAssessment>;
