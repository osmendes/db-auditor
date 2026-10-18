import { useEffect, useRef, useState } from "react";
import {
  type PageSize,
  PaginationControls,
} from "../../components/ui/PaginationControls";
import { formatError } from "../../lib/errors";
import { fetchAllPages } from "../../lib/pagination";
import type { PagedResponse, PageMeta } from "../../types";

/** Paged, lazily loaded collection of one assessment tab (UI-06 extraction). */
export function useCollection<T>(
  enabled: boolean,
  key: string,
  load: (
    offset: number,
    limit: number,
    signal?: AbortSignal,
  ) => Promise<PagedResponse<T>>,
) {
  const loader = useRef(load);
  loader.current = load;
  const [offset, setOffset] = useState(0);
  const [size, setSize] = useState<PageSize>(20);
  const [items, setItems] = useState<T[]>([]);
  const [page, setPage] = useState<PageMeta | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    setOffset(0);
    setItems([]);
    setPage(null);
    setError(null);
  }, [key]);
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const controller = new AbortController();
    setLoading(true);
    const request =
      size === "all"
        ? fetchAllPages(
            (nextOffset, limit) =>
              loader.current(nextOffset, limit, controller.signal),
            controller.signal,
          ).then((allItems) => ({
            items: allItems,
            page: {
              total: allItems.length,
              limit: allItems.length,
              offset: 0,
              has_more: false,
            },
          }))
        : loader.current(offset, size, controller.signal);
    request
      .then((result) => {
        if (!cancelled) {
          setItems(result.items);
          setPage(result.page);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setItems([]);
          setPage(null);
          setError(formatError(cause, "Não foi possível carregar esta seção"));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [enabled, key, offset, size]);
  return { items, page, error, loading, offset, size, setOffset, setSize };
}

export function CollectionState({
  data,
  children,
}: {
  data: Collection<unknown>;
  children: React.ReactNode;
}) {
  if (data.loading)
    return (
      <p role="status" className="text-sm text-slate-400">
        Carregando…
      </p>
    );
  if (data.error)
    return (
      <p role="alert" className="text-sm text-red-300">
        {data.error}
      </p>
    );
  const page = data.page;
  return (
    <div className="space-y-2">
      {data.items.length === 0 ? (
        <p className="text-sm text-slate-400">Nenhum item neste run.</p>
      ) : (
        children
      )}
      {page ? (
        <PaginationControls
          total={page.total}
          offset={data.offset}
          size={data.size}
          onSizeChange={(next) => {
            data.setSize(next);
            data.setOffset(0);
          }}
          onOffsetChange={data.setOffset}
          label="Detalhe da tabela"
        />
      ) : null}
    </div>
  );
}

export type Collection<T> = ReturnType<typeof useCollection<T>>;
