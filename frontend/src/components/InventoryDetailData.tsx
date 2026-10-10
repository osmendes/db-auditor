import { useEffect, useRef, useState } from "react";
import { ApiError, formatError } from "../lib/errors";
import type { PagedResponse } from "../types";
import { InventoryDetailAvailability } from "./InventoryDetailTabs";
import { PaginationControls } from "./ui/PaginationControls";

/** Loads only an active tab and ignores responses from a previous object/run. */
export function useInventoryDetailPage<T>(
  active: boolean,
  scope: string,
  load: (offset: number) => Promise<PagedResponse<T>>,
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
    setLoading(true);
    setError(null);
    void loader
      .current(offset)
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
    };
  }, [active, scope, offset]);
  return { offset, setOffset, data, error, loading };
}

export function inventoryDetailState(
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

export function inventoryDetailPageControls(
  total: number,
  offset: number,
  setOffset: (value: number) => void,
  label: string,
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
      label={label}
    />
  );
}
