import { useCallback, useEffect, useState } from "react";
import type { PageSize } from "../../components/ui/PaginationControls";
import { formatError } from "../../lib/errors";
import type { SortState } from "../../lib/sort";
import { api } from "../../services/api";
import type {
  CAGGSnapshot,
  FunctionSnapshot,
  HypertableSnapshot,
  IndexSnapshot,
  InventoryObjectKind,
  PageMeta,
  TableSnapshot,
  ViewSnapshot,
} from "../../types";

export interface InventoryListQuery {
  envId: string | null | undefined;
  kind: InventoryObjectKind;
  offset: number;
  pageSize: PageSize;
  q: string;
  /** Audit run pinned by the URL (`?run=`), else the latest snapshot. */
  run: string | null | undefined;
  selectedDb: string | null;
  selectedSchema: string | null;
  sort: SortState | null;
}

/** Server-side paged lists of the inventory explorer (UI-06 extraction). */
export function useInventoryObjectLists({
  envId,
  kind,
  offset,
  pageSize,
  q,
  run,
  selectedDb,
  selectedSchema,
  sort,
}: InventoryListQuery) {
  const [page, setPage] = useState<PageMeta | null>(null);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [tables, setTables] = useState<TableSnapshot[]>([]);
  const [indexes, setIndexes] = useState<IndexSnapshot[]>([]);
  const [views, setViews] = useState<ViewSnapshot[]>([]);
  const [caggs, setCaggs] = useState<CAGGSnapshot[]>([]);
  const [functions, setFunctions] = useState<FunctionSnapshot[]>([]);
  const [hypertables, setHypertables] = useState<HypertableSnapshot[]>([]);
  const loadObjects = useCallback(() => {
    if (!envId) {
      setTables([]);
      setIndexes([]);
      setViews([]);
      setCaggs([]);
      setFunctions([]);
      setHypertables([]);
      setPage(null);
      setLoading(false);
      return () => {};
    }
    let cancelled = false;
    const controller = new AbortController();
    setLoading(true);
    setListError(null);
    const params = {
      audit_run_id: run || undefined,
      limit: pageSize === "all" ? 100 : pageSize,
      offset,
      q: q || undefined,
      database: selectedDb || undefined,
      schema: selectedSchema || undefined,
      order_by: sort ? `${sort.key}:${sort.dir}` : undefined,
    };
    const fail = (msg: string) => {
      if (!cancelled) {
        setListError(msg);
        setLoading(false);
      }
    };
    const ok = () => {
      if (!cancelled) setLoading(false);
    };

    if (kind === "tables") {
      api
        .tables(envId, params, controller.signal)
        .then((res) => {
          if (!cancelled) {
            setTables(res.items);
            setPage(res.page);
          }
        })
        .catch((e: unknown) =>
          fail(formatError(e, "Não foi possível listar as tabelas")),
        )
        .finally(ok);
    } else if (kind === "indexes") {
      api
        .indexes(envId, params, controller.signal)
        .then((res) => {
          if (!cancelled) {
            setIndexes(res.items);
            setPage(res.page);
          }
        })
        .catch((e: unknown) =>
          fail(formatError(e, "Não foi possível listar os índices")),
        )
        .finally(ok);
    } else if (kind === "views") {
      api
        .views(envId, params, controller.signal)
        .then((res) => {
          if (!cancelled) {
            setViews(res.items);
            setPage(res.page);
          }
        })
        .catch((e: unknown) =>
          fail(formatError(e, "Não foi possível listar as visualizações")),
        )
        .finally(ok);
    } else if (kind === "functions") {
      api
        .functions(envId, params, controller.signal)
        .then((res) => {
          if (!cancelled) {
            setFunctions(res.items);
            setPage(res.page);
          }
        })
        .catch((e: unknown) =>
          fail(formatError(e, "Não foi possível listar as funções")),
        )
        .finally(ok);
    } else if (kind === "hypertables") {
      api
        .hypertablesPage(envId, params, controller.signal)
        .then((res) => {
          if (!cancelled) {
            setHypertables(res.items);
            setPage(res.page);
          }
        })
        .catch((e: unknown) =>
          fail(formatError(e, "Não foi possível listar as hypertables")),
        )
        .finally(ok);
    } else {
      api
        .continuousAggregatesPage(envId, params, controller.signal)
        .then((res) => {
          if (!cancelled) {
            setPage(res.page);
            setCaggs(res.items);
            setViews(
              res.items.map((c) => ({
                id: c.id,
                database_name: c.database_name,
                schema_name: c.schema_name,
                view_name: c.view_name,
                owner_name: c.owner_name,
                relkind: "cagg",
                size_bytes: 0,
                collected_at: c.collected_at,
              })),
            );
          }
        })
        .catch((e: unknown) =>
          fail(
            formatError(e, "Não foi possível listar os agregados contínuos"),
          ),
        )
        .finally(ok);
    }
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [envId, kind, offset, pageSize, q, run, selectedDb, selectedSchema, sort]);

  useEffect(() => loadObjects(), [loadObjects]);

  return {
    page,
    loading,
    listError,
    setListError,
    tables,
    indexes,
    views,
    caggs,
    functions,
    hypertables,
    loadObjects,
  };
}
