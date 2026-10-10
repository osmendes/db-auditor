import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  type PageSize,
  PaginationControls,
} from "../../components/ui/PaginationControls";
import { useApp } from "../../context/AppContext";
import { formatError } from "../../lib/errors";
import {
  inventoryRunForSelection,
  inventoryTargetKey,
  matchesInventorySelection,
  timescaleUnavailableForRun,
} from "../../lib/inventoryTarget";
import type { SortState } from "../../lib/sort";
import { api } from "../../services/api";
import type {
  CAGGSnapshot,
  ColumnStatSnapshot,
  DatabaseSnapshot,
  EnvironmentCapabilities,
  FunctionSnapshot,
  HypertableSnapshot,
  IndexSnapshot,
  InventoryObjectKind,
  PageMeta,
  SchemaSnapshot,
  SnapshotCompleteness,
  TableHistoryPoint,
  TableSnapshot,
  ViewSnapshot,
  WorkloadSnapshot,
} from "../../types";
import {
  KIND_LABELS,
  type SelectedInventoryItem,
  targetFor,
} from "./inventoryUi";

export function useInventoryExplorer() {
  const {
    environmentId,
    environments,
    setSection,
    inventory,
    search,
    openInventory,
    closeInventory,
  } = useApp();
  const envId = environmentId;
  const isMongo =
    environments.find((item) => item.id === envId)?.engine === "mongodb";

  const [error, setError] = useState<string | null>(null);
  const [databases, setDatabases] = useState<DatabaseSnapshot[] | null>(null);
  const [schemas, setSchemas] = useState<SchemaSnapshot[] | null>(null);
  const [selectedDb, setSelectedDb] = useState<string | null>(null);
  const [selectedSchema, setSelectedSchema] = useState<string | null>(null);
  const [kind, setKind] = useState<InventoryObjectKind>("tables");
  useEffect(() => {
    if (
      inventory &&
      (!isMongo || inventory.kind === "tables" || inventory.kind === "indexes")
    ) {
      setKind(inventory.kind);
    }
  }, [inventory?.kind, isMongo]);
  useEffect(() => {
    if (isMongo && kind !== "tables" && kind !== "indexes") setKind("tables");
  }, [isMongo, kind]);
  const [q, setQ] = useState("");
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState<PageSize>(20);
  const [page, setPage] = useState<PageMeta | null>(null);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [tables, setTables] = useState<TableSnapshot[]>([]);
  const [indexes, setIndexes] = useState<IndexSnapshot[]>([]);
  const [views, setViews] = useState<ViewSnapshot[]>([]);
  const [caggs, setCaggs] = useState<CAGGSnapshot[]>([]);
  const [functions, setFunctions] = useState<FunctionSnapshot[]>([]);
  const [hypertables, setHypertables] = useState<HypertableSnapshot[]>([]);
  const [selectedItem, setSelectedItem] =
    useState<SelectedInventoryItem | null>(null);
  const [selectedRun, setSelectedRun] = useState<string | null>(null);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [sort, setSort] = useState<SortState | null>(null);
  const [snapshotStatus, setSnapshotStatus] =
    useState<SnapshotCompleteness | null>(null);
  const [capabilities, setCapabilities] =
    useState<EnvironmentCapabilities | null>(null);
  const [detailSnapshot, setDetailSnapshot] =
    useState<SnapshotCompleteness | null>(null);
  const [detailHistory, setDetailHistory] = useState<TableHistoryPoint[]>([]);
  const [detailStats, setDetailStats] = useState<ColumnStatSnapshot[]>([]);
  const [detailWorkload, setDetailWorkload] = useState<WorkloadSnapshot[]>([]);
  const selectedKeyRef = useRef<string | null>(null);

  useEffect(() => {
    if (!envId) {
      setDatabases([]);
      setSchemas([]);
      setSelectedDb(null);
      setSelectedSchema(null);
      setOffset(0);
      setError(null);
      setSnapshotStatus(null);
      setCapabilities(null);
      return;
    }
    let cancelled = false;
    setDatabases(null);
    setSchemas(null);
    setSelectedDb(null);
    setSelectedSchema(null);
    setOffset(0);
    setError(null);
    setSnapshotStatus(null);
    setCapabilities(null);
    Promise.all([
      api.databases(envId),
      api.schemas(envId),
      api.snapshotStatus(envId).catch(() => null),
      api.environmentCapabilities(envId).catch(() => null),
    ])
      .then(([dbRes, scRes, status, capability]) => {
        if (!cancelled) {
          setDatabases(dbRes.items);
          setSchemas(scRes.items);
          setSnapshotStatus(status);
          setCapabilities(capability);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setListError(formatError(err, "Falha ao carregar topologia"));
          setDatabases([]);
          setSchemas([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [envId]);

  const schemasForDb = useMemo(() => {
    if (!schemas) return [];
    if (!selectedDb) return schemas;
    return schemas.filter((s) => s.database_name === selectedDb);
  }, [schemas, selectedDb]);

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
    setLoading(true);
    setListError(null);
    const params = {
      audit_run_id: search.run || undefined,
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
        .tables(envId, params)
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
        .indexes(envId, params)
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
        .views(envId, params)
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
        .functions(envId, params)
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
        .hypertablesPage(envId, params)
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
        .continuousAggregatesPage(envId, params)
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
    };
  }, [
    envId,
    kind,
    offset,
    pageSize,
    q,
    search.run,
    selectedDb,
    selectedSchema,
    sort,
  ]);

  useEffect(() => loadObjects(), [loadObjects]);
  useEffect(() => setSort(null), [kind]);

  // The server sorts the filtered result before applying LIMIT/OFFSET.
  const sortedTables = tables;
  const sortedIndexes = indexes;
  const sortedViews = views;
  const sortedFunctions = functions;
  const sortedHypertables = hypertables;
  const currentRun = inventoryRunForSelection(
    search.run,
    snapshotStatus?.audit_run_id,
  );
  const timescaleNotApplicable = timescaleUnavailableForRun(
    kind,
    currentRun,
    capabilities,
  );

  const openRow = (
    selection: SelectedInventoryItem,
    run: string | null,
    updateLocation = true,
  ) => {
    const target = targetFor(selection);
    const key = inventoryTargetKey(envId ?? "", run ?? "", target);
    selectedKeyRef.current = key;
    setSelectedItem(selection);
    setSelectedRun(run);
    setDetailError(null);
    setSheetOpen(true);
    setDetailHistory([]);
    setDetailStats([]);
    setDetailWorkload([]);
    if (updateLocation) openInventory(target, run ?? undefined);
  };

  const closeSheet = () => {
    selectedKeyRef.current = null;
    setSheetOpen(false);
    setSelectedItem(null);
    setSelectedRun(null);
    setDetailError(null);
    closeInventory();
  };

  const openTable = (t: TableSnapshot, updateLocation = true) => {
    const target = targetFor({ kind: "tables", item: t });
    const key = inventoryTargetKey(envId ?? "", t.audit_run_id, target);
    openRow({ kind: "tables", item: t }, t.audit_run_id, updateLocation);
    if (!envId || isMongo) return;
    const scope = {
      database: t.database_name,
      schema: t.schema_name,
      table: t.table_name,
    };
    Promise.allSettled([
      api.tableHistory(envId, { ...scope, granularity: "day" }),
      api.tableColumnStats(envId, scope),
      api.tableWorkload(envId, scope),
    ]).then(([history, stats, workload]) => {
      if (selectedKeyRef.current !== key) return;
      setDetailHistory(
        history.status === "fulfilled" ? history.value.items : [],
      );
      setDetailStats(stats.status === "fulfilled" ? stats.value.items : []);
      setDetailWorkload(
        workload.status === "fulfilled" ? workload.value.items : [],
      );
    });
  };

  useEffect(() => {
    selectedKeyRef.current = null;
    setSelectedItem(null);
    setSelectedRun(null);
    setSheetOpen(false);
  }, [envId]);

  useEffect(() => {
    if (inventory) return;
    selectedKeyRef.current = null;
    setSelectedItem(null);
    setSelectedRun(null);
    setSheetOpen(false);
  }, [inventory]);

  useEffect(() => {
    if (!envId || !inventory) return;
    const run = search.run ?? snapshotStatus?.audit_run_id;
    if (!run) return;
    const key = inventoryTargetKey(envId, run, inventory);
    if (selectedKeyRef.current === key) return;
    let cancelled = false;
    selectedKeyRef.current = key;
    setSelectedItem(null);
    setSelectedRun(run);
    setDetailError(null);
    setSheetOpen(true);
    const params = {
      audit_run_id: run,
      database: inventory.database,
      schema: inventory.schema,
      q: inventory.name,
    };
    const findPage = async <T,>(
      load: (offset: number) => Promise<{ items: T[]; page: PageMeta }>,
      matches: (item: T) => boolean,
    ): Promise<T | null> => {
      let offset = 0;
      for (let attempt = 0; attempt < 20; attempt++) {
        const result = await load(offset);
        const found = result.items.find(matches);
        if (found) return found;
        if (!result.page.has_more) return null;
        offset += result.items.length;
      }
      throw new Error("Busca limitada a 2.000 objetos. Refine o link.");
    };
    const resolve = async (): Promise<SelectedInventoryItem | null> => {
      switch (inventory.kind) {
        case "tables": {
          const result = await api.tableAssessment(
            envId,
            run,
            inventory.database,
            inventory.schema,
            inventory.name,
          );
          return { kind: "tables", item: result.table };
        }
        case "indexes": {
          const item = await findPage(
            (offset) => api.indexes(envId, { ...params, limit: 100, offset }),
            (value) =>
              value.database_name === inventory.database &&
              value.schema_name === inventory.schema &&
              value.index_name === inventory.name,
          );
          return item ? { kind: "indexes", item } : null;
        }
        case "views": {
          const item = await findPage(
            (offset) => api.views(envId, { ...params, limit: 100, offset }),
            (value) =>
              value.database_name === inventory.database &&
              value.schema_name === inventory.schema &&
              value.view_name === inventory.name,
          );
          return item ? { kind: "views", item } : null;
        }
        case "caggs": {
          const item = await findPage(
            (offset) =>
              api.continuousAggregatesPage(envId, {
                ...params,
                limit: 100,
                offset,
              }),
            (value) =>
              value.database_name === inventory.database &&
              value.schema_name === inventory.schema &&
              value.view_name === inventory.name,
          );
          if (!item) return null;
          return { kind: "caggs", item };
        }
        case "functions": {
          const item = await findPage(
            (offset) => api.functions(envId, { ...params, limit: 100, offset }),
            (value) =>
              value.database_name === inventory.database &&
              value.schema_name === inventory.schema &&
              value.function_name === inventory.name &&
              value.identity_arguments === (inventory.signature ?? ""),
          );
          return item ? { kind: "functions", item } : null;
        }
        case "hypertables": {
          const item = await findPage(
            (offset) =>
              api.hypertablesPage(envId, { ...params, limit: 100, offset }),
            (value) =>
              value.database_name === inventory.database &&
              value.schema_name === inventory.schema &&
              value.hypertable_name === inventory.name,
          );
          return item ? { kind: "hypertables", item } : null;
        }
      }
    };
    void resolve()
      .then((selection) => {
        if (cancelled || selectedKeyRef.current !== key) return;
        if (!selection) {
          setDetailError(
            "Objeto não encontrado nesta execução ou fora da cobertura da coleta.",
          );
          return;
        }
        if (!search.run) openInventory(inventory, run);
        if (selection.kind === "tables") openTable(selection.item, false);
        else {
          setSelectedItem(selection);
          setSheetOpen(true);
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled && selectedKeyRef.current === key) {
          setDetailError(
            formatError(cause, "Não foi possível abrir este objeto"),
          );
        }
      });
    return () => {
      cancelled = true;
    };
    // The canonical key captures every part of the selected object and run.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [envId, inventory, search.run, snapshotStatus?.audit_run_id]);

  const visibleSelectedItem =
    selectedItem &&
    selectedRun === currentRun &&
    matchesInventorySelection(
      envId,
      selectedRun,
      inventory,
      targetFor(selectedItem),
      selectedKeyRef.current,
    )
      ? selectedItem
      : null;
  const visibleSnapshot =
    detailSnapshot?.audit_run_id === selectedRun ? detailSnapshot : null;
  const selectedTable =
    visibleSelectedItem?.kind === "tables"
      ? visibleSelectedItem.item
      : undefined;

  const navigateToRelatedTable = (schema: string, table: string) => {
    if (!envId || !selectedTable) return;
    api
      .tableAssessment(
        envId,
        selectedTable.audit_run_id,
        selectedTable.database_name,
        schema,
        table,
      )
      .then((result) => openTable(result.table))
      .catch((cause: unknown) =>
        setListError(
          formatError(cause, "Não foi possível abrir a tabela relacionada"),
        ),
      );
  };
  const selectedIndex =
    visibleSelectedItem?.kind === "indexes"
      ? visibleSelectedItem.item
      : undefined;
  const selectedView =
    visibleSelectedItem?.kind === "views"
      ? visibleSelectedItem.item
      : undefined;
  const selectedCagg =
    visibleSelectedItem?.kind === "caggs"
      ? visibleSelectedItem.item
      : undefined;
  const selectedFn =
    visibleSelectedItem?.kind === "functions"
      ? visibleSelectedItem.item
      : undefined;
  const selectedHt =
    visibleSelectedItem?.kind === "hypertables"
      ? visibleSelectedItem.item
      : undefined;

  useEffect(() => {
    if (!envId || !selectedRun) {
      setDetailSnapshot(null);
      return;
    }
    if (snapshotStatus?.audit_run_id === selectedRun) {
      setDetailSnapshot(snapshotStatus);
      return;
    }
    let cancelled = false;
    setDetailSnapshot(null);
    void api
      .snapshotStatus(envId, selectedRun)
      .then((status) => {
        if (!cancelled) setDetailSnapshot(status);
      })
      .catch(() => {
        if (!cancelled) setDetailSnapshot(null);
      });
    return () => {
      cancelled = true;
    };
  }, [envId, selectedRun, snapshotStatus]);

  const envName =
    environments.find((e) => e.id === envId)?.name ??
    (envId ? envId.slice(0, 8) : null);

  const pagination = page ? (
    <PaginationControls
      total={page.total}
      offset={pageSize === "all" ? 0 : offset}
      size={pageSize}
      allowAll={false}
      onSizeChange={(next) => {
        setPageSize(next);
        setOffset(0);
      }}
      onOffsetChange={setOffset}
      label={KIND_LABELS[kind]}
    />
  ) : null;

  return {
    envId,
    isMongo,
    error,
    setError,
    databases,
    selectedDb,
    setSelectedDb,
    selectedSchema,
    setSelectedSchema,
    kind,
    setKind,
    q,
    setQ,
    setOffset,
    loading,
    listError,
    caggs,
    selectedItem,
    selectedRun,
    detailError,
    sheetOpen,
    sort,
    setSort,
    snapshotStatus,
    detailHistory,
    detailStats,
    detailWorkload,
    schemasForDb,
    loadObjects,
    sortedTables,
    sortedIndexes,
    sortedViews,
    sortedFunctions,
    sortedHypertables,
    currentRun,
    timescaleNotApplicable,
    openRow,
    closeSheet,
    openTable,
    visibleSelectedItem,
    visibleSnapshot,
    selectedTable,
    navigateToRelatedTable,
    selectedIndex,
    selectedView,
    selectedCagg,
    selectedFn,
    selectedHt,
    envName,
    pagination,
    setSection,
    inventory,
  };
}

type InventoryModel = ReturnType<typeof useInventoryExplorer>;

const InventoryModelContext = createContext<InventoryModel | null>(null);

export function InventoryModelProvider({ children }: { children: ReactNode }) {
  const model = useInventoryExplorer();
  return (
    <InventoryModelContext.Provider value={model}>
      {children}
    </InventoryModelContext.Provider>
  );
}

export function useInventoryModel(): InventoryModel {
  const value = useContext(InventoryModelContext);
  if (!value) {
    throw new Error("Inventário indisponível");
  }
  return value;
}
