import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { InventoryDetailAvailability } from "../components/InventoryDetailTabs";
import { PageHeader } from "../components/PageHeader";
import {
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Input,
  Select,
  Sheet,
  Skeleton,
  Table,
} from "../components/ui";
import {
  type PageSize,
  PaginationControls,
} from "../components/ui/PaginationControls";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { formatBytes } from "../lib/format";
import {
  type InventoryTarget,
  inventoryRunForSelection,
  inventoryTargetKey,
  matchesInventorySelection,
  timescaleUnavailableForRun,
} from "../lib/inventoryTarget";
import {
  relationClassBadgeClass,
  relationClassLabel,
} from "../lib/relationClass";
import { nextSort, type SortState } from "../lib/sort";
import { api } from "../services/api";
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
} from "../types";
import { CAGGAssessmentPanel } from "./CAGGAssessmentPanel";
import { FunctionAssessmentPanel } from "./FunctionAssessmentPanel";
import { HypertableAssessmentPanel } from "./HypertableAssessmentPanel";
import { IndexAssessmentPanel } from "./IndexAssessmentPanel";
import {
  FunctionDetail,
  HypertableDetail,
  IndexDetail,
  ViewDetail,
} from "./InventoryDetails";
import { InventoryObjectPanel } from "./InventoryObjectPanel";
import { TableAssessmentPanel } from "./TableAssessmentPanel";
import { ViewAssessmentPanel } from "./ViewAssessmentPanel";

type SelectedInventoryItem =
  | { kind: "tables"; item: TableSnapshot }
  | { kind: "indexes"; item: IndexSnapshot }
  | { kind: "views"; item: ViewSnapshot }
  | { kind: "caggs"; item: CAGGSnapshot }
  | { kind: "functions"; item: FunctionSnapshot }
  | { kind: "hypertables"; item: HypertableSnapshot };

function targetFor(item: SelectedInventoryItem): InventoryTarget {
  const value = item.item;
  const shared = {
    database: value.database_name,
    schema: value.schema_name,
  };
  switch (item.kind) {
    case "tables":
      return { ...shared, kind: item.kind, name: item.item.table_name };
    case "indexes":
      return { ...shared, kind: item.kind, name: item.item.index_name };
    case "views":
      return { ...shared, kind: item.kind, name: item.item.view_name };
    case "caggs":
      return { ...shared, kind: item.kind, name: item.item.view_name };
    case "functions":
      return {
        ...shared,
        kind: item.kind,
        name: item.item.function_name,
        signature: item.item.identity_arguments,
      };
    case "hypertables":
      return { ...shared, kind: item.kind, name: item.item.hypertable_name };
  }
}

const KINDS: InventoryObjectKind[] = [
  "tables",
  "hypertables",
  "indexes",
  "views",
  "functions",
  "caggs",
];

const KIND_LABELS: Record<InventoryObjectKind, string> = {
  tables: "Tabelas",
  hypertables: "Tabelas temporais",
  indexes: "Índices",
  views: "Visões",
  functions: "Funções",
  caggs: "Agregados contínuos",
};

const detailButtonClass =
  "rounded text-left font-medium text-slate-100 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400";

function rowClass(selected: boolean): string {
  return `cursor-pointer border-t border-slate-800/80 transition-colors ${
    selected ? "bg-slate-800/70" : "hover:bg-slate-900/50"
  }`;
}

function fmtNum(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(n)) return "—";
  return n.toLocaleString("pt-BR");
}

function boolLabel(v: boolean | null | undefined): string {
  if (v == null) return "—";
  return v ? "Sim" : "Não";
}

function runStatusLabel(status: string): string {
  return (
    (
      {
        success: "concluída",
        partial_success: "parcial",
        failed: "falhou",
        running: "em andamento",
        queued: "na fila",
        cancelled: "cancelada",
      } as Record<string, string>
    )[status] ?? "desconhecido"
  );
}

export function InventoryPage() {
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

  return (
    <>
      <PageHeader
        eyebrow="INVENTÁRIO"
        title="Explorer de inventário"
        description={
          isMongo
            ? "Ambiente → banco → coleção. A coleta mostra somente metadados e índices; documentos não são lidos."
            : "Ambiente → banco → esquema → objeto. Clique em uma linha para abrir o painel de detalhes."
        }
        actions={
          envName ? (
            <span className="rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs text-slate-300">
              {envName}
            </span>
          ) : null
        }
      />

      {!envId ? (
        <div className="mt-8">
          <EmptyState
            title="Selecione um ambiente"
            description="Escolha um ambiente na barra lateral para explorar o inventário coletado."
            action={
              <Button onClick={() => setSection("Ambientes")}>
                Ir para Ambientes
              </Button>
            }
          />
        </div>
      ) : (
        <div className="mt-6 flex min-h-0 flex-col gap-4">
          {error ? (
            <ErrorBanner message={error} onRetry={() => setError(null)} />
          ) : null}

          {snapshotStatus ? (
            <div
              className={
                snapshotStatus.completeness === "complete"
                  ? "rounded-md border border-emerald-800/60 bg-emerald-950/40 px-3 py-2 text-sm text-emerald-200"
                  : snapshotStatus.completeness === "partial"
                    ? "rounded-md border border-amber-800/60 bg-amber-950/40 px-3 py-2 text-sm text-amber-200"
                    : "rounded-md border border-slate-700 bg-slate-900/60 px-3 py-2 text-sm text-slate-300"
              }
            >
              <div className="font-medium">
                Inventário{" "}
                {snapshotStatus.completeness === "complete"
                  ? "completo"
                  : snapshotStatus.completeness === "partial"
                    ? "parcial"
                    : snapshotStatus.completeness === "empty"
                      ? "vazio"
                      : "desconhecido"}
              </div>
              <div className="mt-0.5 text-xs opacity-90">
                Execução {snapshotStatus.audit_run_id.slice(0, 8)}… ·{" "}
                {runStatusLabel(snapshotStatus.run_status)}
                {snapshotStatus.analysis_status
                  ? ` · análise ${runStatusLabel(snapshotStatus.analysis_status)}`
                  : ""}
              </div>
            </div>
          ) : null}

          {databases === null ? (
            <Skeleton className="h-[28rem] w-full" />
          ) : databases.length === 0 ? (
            <EmptyState
              title="Sem snapshot de inventário"
              description="Rode uma auditoria neste ambiente para popular databases/schemas/objetos."
              action={
                <Button onClick={() => setSection("Execuções")}>
                  Ir para Execuções
                </Button>
              }
            />
          ) : (
            <>
              <Card title="Filtros">
                <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  <Select
                    label="Banco"
                    value={selectedDb ?? ""}
                    onChange={(e) => {
                      setSelectedDb(e.target.value || null);
                      setSelectedSchema(null);
                      setOffset(0);
                    }}
                    options={[
                      { value: "", label: "(todos)" },
                      ...databases.map((db) => ({
                        value: db.database_name,
                        label: db.database_name,
                      })),
                    ]}
                  />
                  <Select
                    label="Esquema"
                    value={selectedSchema ?? ""}
                    onChange={(e) => {
                      setSelectedSchema(e.target.value || null);
                      setOffset(0);
                    }}
                    options={[
                      { value: "", label: "(todos)" },
                      ...schemasForDb.map((sc) => ({
                        value: sc.schema_name,
                        label: sc.schema_name,
                      })),
                    ]}
                  />
                  <Input
                    label="Buscar"
                    placeholder="Buscar nome…"
                    value={q}
                    onChange={(e) => {
                      setQ(e.target.value);
                      setOffset(0);
                    }}
                  />
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {KINDS.filter(
                    (k) => !isMongo || k === "tables" || k === "indexes",
                  ).map((k) => (
                    <button
                      key={k}
                      type="button"
                      onClick={() => {
                        setKind(k);
                        setOffset(0);
                        closeSheet();
                      }}
                      className={`rounded px-2.5 py-1 text-xs ${
                        kind === k
                          ? "bg-emerald-800 text-white"
                          : "bg-slate-800/80 text-slate-300 hover:bg-slate-800"
                      }`}
                    >
                      {isMongo && k === "tables" ? "Coleções" : KIND_LABELS[k]}
                    </button>
                  ))}
                </div>
                {listError ? (
                  <div className="mt-3">
                    <ErrorBanner
                      message={listError}
                      onRetry={() => loadObjects()}
                    />
                  </div>
                ) : null}
              </Card>

              <Card
                title={
                  isMongo && kind === "tables" ? "Coleções" : KIND_LABELS[kind]
                }
              >
                {loading ? (
                  <Skeleton className="h-64 w-full" />
                ) : timescaleNotApplicable ? (
                  <InventoryDetailAvailability
                    state="not_applicable"
                    detail="TimescaleDB não foi observado nesta execução. Tabelas temporais e agregados contínuos não se aplicam a esta coleta."
                  />
                ) : kind === "tables" ? (
                  <>
                    <Table
                      pagination={false}
                      dense
                      virtualize
                      sortKey={sort?.key}
                      sortDir={sort?.dir}
                      onSort={(id) => {
                        setOffset(0);
                        setSort((s) => nextSort(s, id));
                      }}
                      headers={[
                        { id: "database", label: "Banco", sortable: true },
                        {
                          id: "schema",
                          label: isMongo ? "Grupo" : "Esquema",
                          sortable: true,
                        },
                        {
                          id: "name",
                          label: isMongo ? "Coleção" : "Tabela",
                          sortable: true,
                        },
                        { id: "type", label: "Tipo", sortable: true },
                        {
                          id: "cols",
                          label: isMongo ? "Campos" : "Colunas",
                          sortable: true,
                        },
                        {
                          id: "rows",
                          label: isMongo ? "Documentos estimados" : "Linhas",
                          sortable: true,
                        },
                        { id: "size", label: "Tamanho", sortable: true },
                        {
                          id: "pk",
                          label: isMongo ? "ID único" : "PK",
                          sortable: true,
                        },
                      ]}
                    >
                      {sortedTables.map((t) => {
                        return (
                          <tr
                            key={t.id}
                            className={rowClass(
                              selectedItem?.kind === "tables" &&
                                selectedItem.item.id === t.id,
                            )}
                            onClick={() => openTable(t)}
                          >
                            <td className="px-3 py-2">{t.database_name}</td>
                            <td className="px-3 py-2">{t.schema_name}</td>
                            <td className="px-3 py-2">
                              <button
                                type="button"
                                className={detailButtonClass}
                                aria-label={`Abrir detalhes de ${isMongo ? "coleção" : "tabela"} ${t.table_name}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openTable(t);
                                }}
                              >
                                {t.table_name}
                              </button>
                            </td>
                            <td className="px-3 py-2">
                              <span
                                className={`inline-flex rounded border px-1.5 py-0.5 text-[11px] font-medium ${relationClassBadgeClass(
                                  t.relation_class,
                                )}`}
                              >
                                {relationClassLabel(
                                  t.relation_class,
                                  t.relkind,
                                )}
                              </span>
                            </td>
                            <td className="px-3 py-2">
                              {isMongo
                                ? "não coletado"
                                : fmtNum(t.column_count)}
                            </td>
                            <td className="px-3 py-2">
                              {fmtNum(t.row_estimate)}
                            </td>
                            <td className="px-3 py-2">
                              {formatBytes(t.total_size_bytes)}
                            </td>
                            <td className="px-3 py-2">
                              {boolLabel(t.has_primary_key)}
                            </td>
                          </tr>
                        );
                      })}
                    </Table>
                    {pagination}
                  </>
                ) : kind === "indexes" ? (
                  <>
                    <Table
                      pagination={false}
                      dense
                      sortKey={sort?.key}
                      sortDir={sort?.dir}
                      onSort={(id) => {
                        setOffset(0);
                        setSort((s) => nextSort(s, id));
                      }}
                      headers={[
                        { id: "schema", label: "Esquema", sortable: true },
                        { id: "name", label: "Índice", sortable: true },
                        { id: "table", label: "Tabela", sortable: true },
                        { id: "method", label: "Método", sortable: true },
                        { id: "size", label: "Tamanho", sortable: true },
                        { id: "scans", label: "Leituras", sortable: true },
                        { id: "unique", label: "Único", sortable: true },
                      ]}
                    >
                      {sortedIndexes.map((i) => {
                        return (
                          <tr
                            key={i.id}
                            className={rowClass(
                              selectedItem?.kind === "indexes" &&
                                selectedItem.item.id === i.id,
                            )}
                            onClick={() =>
                              openRow({ kind: "indexes", item: i }, currentRun)
                            }
                          >
                            <td className="px-3 py-2">{i.schema_name}</td>
                            <td className="px-3 py-2">
                              <button
                                type="button"
                                className={detailButtonClass}
                                aria-label={`Abrir detalhes do índice ${i.index_name}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openRow(
                                    { kind: "indexes", item: i },
                                    currentRun,
                                  );
                                }}
                              >
                                {i.index_name}
                              </button>
                            </td>
                            <td className="px-3 py-2">{i.table_name}</td>
                            <td className="px-3 py-2">
                              {i.access_method ?? "—"}
                            </td>
                            <td className="px-3 py-2">
                              {formatBytes(i.size_bytes)}
                            </td>
                            <td className="px-3 py-2">{fmtNum(i.idx_scan)}</td>
                            <td className="px-3 py-2">
                              {boolLabel(i.is_unique)}
                            </td>
                          </tr>
                        );
                      })}
                    </Table>
                    {pagination}
                  </>
                ) : kind === "views" || kind === "caggs" ? (
                  <>
                    <Table
                      pagination={false}
                      dense
                      sortKey={sort?.key}
                      sortDir={sort?.dir}
                      onSort={(id) => {
                        setOffset(0);
                        setSort((s) => nextSort(s, id));
                      }}
                      headers={[
                        { id: "schema", label: "Esquema", sortable: true },
                        { id: "name", label: "Nome", sortable: true },
                        { id: "owner", label: "Responsável", sortable: true },
                        {
                          id: "size",
                          label: "Tamanho",
                          sortable: kind === "views",
                        },
                        {
                          id: "kind",
                          label: "Tipo",
                          sortable: kind === "views",
                        },
                      ]}
                    >
                      {sortedViews.map((v) => {
                        const viewKind = kind === "caggs" ? "caggs" : "views";
                        const selectedCaggRow =
                          viewKind === "caggs"
                            ? caggs.find((item) => item.id === v.id)
                            : undefined;
                        if (viewKind === "caggs" && !selectedCaggRow)
                          return null;
                        const selection: SelectedInventoryItem = selectedCaggRow
                          ? { kind: "caggs", item: selectedCaggRow }
                          : { kind: "views", item: v };
                        return (
                          <tr
                            key={v.id}
                            className={rowClass(
                              selectedItem?.kind === viewKind &&
                                selectedItem.item.id === v.id,
                            )}
                            onClick={() => openRow(selection, currentRun)}
                          >
                            <td className="px-3 py-2">{v.schema_name}</td>
                            <td className="px-3 py-2">
                              <button
                                type="button"
                                className={detailButtonClass}
                                aria-label={`Abrir detalhes ${viewKind === "caggs" ? "do agregado contínuo" : "da visão"} ${v.view_name}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openRow(selection, currentRun);
                                }}
                              >
                                {v.view_name}
                              </button>
                            </td>
                            <td className="px-3 py-2">{v.owner_name ?? "—"}</td>
                            <td className="px-3 py-2">
                              {kind === "caggs"
                                ? "Não coletado"
                                : formatBytes(v.size_bytes)}
                            </td>
                            <td className="px-3 py-2">
                              {kind === "caggs"
                                ? "Agregado contínuo"
                                : v.relkind}
                            </td>
                          </tr>
                        );
                      })}
                    </Table>
                    {pagination}
                  </>
                ) : kind === "functions" ? (
                  <>
                    <Table
                      pagination={false}
                      dense
                      sortKey={sort?.key}
                      sortDir={sort?.dir}
                      onSort={(id) => {
                        setOffset(0);
                        setSort((s) => nextSort(s, id));
                      }}
                      headers={[
                        { id: "schema", label: "Esquema", sortable: true },
                        { id: "name", label: "Função", sortable: true },
                        { id: "kind", label: "Tipo", sortable: true },
                        { id: "lang", label: "Linguagem", sortable: true },
                        {
                          id: "secdef",
                          label: "Segurança definida",
                          sortable: true,
                        },
                      ]}
                    >
                      {sortedFunctions.map((f) => {
                        return (
                          <tr
                            key={f.id}
                            className={rowClass(
                              selectedItem?.kind === "functions" &&
                                selectedItem.item.id === f.id,
                            )}
                            onClick={() =>
                              openRow(
                                { kind: "functions", item: f },
                                currentRun,
                              )
                            }
                          >
                            <td className="px-3 py-2">{f.schema_name}</td>
                            <td className="px-3 py-2">
                              <button
                                type="button"
                                className={detailButtonClass}
                                aria-label={`Abrir detalhes da função ${f.function_name}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openRow(
                                    { kind: "functions", item: f },
                                    currentRun,
                                  );
                                }}
                              >
                                {f.function_name}
                              </button>
                            </td>
                            <td className="px-3 py-2">{f.kind ?? "—"}</td>
                            <td className="px-3 py-2">
                              {f.language_name ?? "—"}
                            </td>
                            <td className="px-3 py-2">
                              {boolLabel(f.is_security_definer)}
                            </td>
                          </tr>
                        );
                      })}
                    </Table>
                    {pagination}
                  </>
                ) : (
                  <>
                    <Table
                      pagination={false}
                      dense
                      virtualize
                      sortKey={sort?.key}
                      sortDir={sort?.dir}
                      onSort={(id) => {
                        setOffset(0);
                        setSort((s) => nextSort(s, id));
                      }}
                      headers={[
                        { id: "schema", label: "Esquema", sortable: true },
                        {
                          id: "name",
                          label: "Tabela temporal",
                          sortable: true,
                        },
                        { id: "size", label: "Tamanho", sortable: true },
                        { id: "chunks", label: "Chunks", sortable: true },
                        {
                          id: "compressed",
                          label: "Compressão",
                          sortable: true,
                        },
                      ]}
                    >
                      {sortedHypertables.map((h) => {
                        return (
                          <tr
                            key={h.id}
                            className={rowClass(
                              selectedItem?.kind === "hypertables" &&
                                selectedItem.item.id === h.id,
                            )}
                            onClick={() =>
                              openRow(
                                { kind: "hypertables", item: h },
                                h.audit_run_id,
                              )
                            }
                          >
                            <td className="px-3 py-2">{h.schema_name}</td>
                            <td className="px-3 py-2">
                              <button
                                type="button"
                                className={detailButtonClass}
                                aria-label={`Abrir detalhes da tabela temporal ${h.hypertable_name}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  openRow(
                                    { kind: "hypertables", item: h },
                                    h.audit_run_id,
                                  );
                                }}
                              >
                                {h.hypertable_name}
                              </button>
                            </td>
                            <td className="px-3 py-2">
                              {formatBytes(h.total_size_bytes)}
                            </td>
                            <td className="px-3 py-2">
                              {fmtNum(h.num_chunks)}
                            </td>
                            <td className="px-3 py-2">
                              {boolLabel(h.compression_enabled)}
                            </td>
                          </tr>
                        );
                      })}
                    </Table>
                    {pagination}
                  </>
                )}
              </Card>
            </>
          )}

          <Sheet
            open={sheetOpen}
            onOpenChange={(open) => {
              if (!open) closeSheet();
            }}
            title={
              visibleSelectedItem
                ? targetFor(visibleSelectedItem).name
                : (inventory?.name ?? "Detalhe")
            }
            wide
          >
            {detailError ? (
              <p role="alert" className="mb-4 text-sm text-red-300">
                {detailError}
              </p>
            ) : null}
            {!visibleSelectedItem && !detailError && inventory ? (
              <p role="status" className="text-sm text-slate-400">
                Carregando detalhes…
              </p>
            ) : null}
            {selectedTable && isMongo ? (
              <div className="space-y-2 text-sm text-slate-200">
                <p>
                  Coleção {selectedTable.database_name}.
                  {selectedTable.table_name}
                </p>
                <p>
                  Documentos estimados: {fmtNum(selectedTable.row_estimate)}
                </p>
                <p>
                  Armazenamento e índices:{" "}
                  {formatBytes(selectedTable.total_size_bytes)}
                </p>
                <p>
                  O auditor leu apenas metadados. Regras específicas do
                  PostgreSQL não são aplicadas a esta coleção.
                </p>
              </div>
            ) : selectedTable ? (
              <TableAssessmentPanel
                key={inventoryTargetKey(
                  envId ?? selectedTable.environment_id,
                  selectedTable.audit_run_id,
                  targetFor({ kind: "tables", item: selectedTable }),
                )}
                t={selectedTable}
                env={envId ?? selectedTable.environment_id}
                history={detailHistory}
                columnStats={detailStats}
                workload={detailWorkload}
                onNavigate={navigateToRelatedTable}
              />
            ) : null}
            {visibleSelectedItem?.kind === "views" && envId && selectedRun ? (
              <ViewAssessmentPanel
                key={inventoryTargetKey(
                  envId,
                  selectedRun,
                  targetFor(visibleSelectedItem),
                )}
                view={visibleSelectedItem.item}
                environment={envId}
                run={selectedRun}
                snapshot={visibleSnapshot}
              />
            ) : null}
            {visibleSelectedItem?.kind === "indexes" && envId && selectedRun ? (
              <IndexAssessmentPanel
                key={inventoryTargetKey(
                  envId,
                  selectedRun,
                  targetFor(visibleSelectedItem),
                )}
                index={visibleSelectedItem.item}
                environment={envId}
                run={selectedRun}
                snapshot={visibleSnapshot}
              />
            ) : null}
            {visibleSelectedItem?.kind === "functions" &&
            envId &&
            selectedRun ? (
              <FunctionAssessmentPanel
                key={inventoryTargetKey(
                  envId,
                  selectedRun,
                  targetFor(visibleSelectedItem),
                )}
                fn={visibleSelectedItem.item}
                environment={envId}
                run={selectedRun}
                snapshot={visibleSnapshot}
              />
            ) : null}
            {visibleSelectedItem?.kind === "hypertables" &&
            envId &&
            selectedRun ? (
              <HypertableAssessmentPanel
                key={inventoryTargetKey(
                  envId,
                  selectedRun,
                  targetFor(visibleSelectedItem),
                )}
                hypertable={visibleSelectedItem.item}
                environment={envId}
                run={selectedRun}
                snapshot={visibleSnapshot}
              />
            ) : null}
            {selectedCagg && envId && selectedRun ? (
              <CAGGAssessmentPanel
                key={inventoryTargetKey(
                  envId,
                  selectedRun,
                  targetFor({ kind: "caggs", item: selectedCagg }),
                )}
                cagg={selectedCagg}
                environment={envId}
                run={selectedRun}
                snapshot={visibleSnapshot}
              />
            ) : null}
            {visibleSelectedItem &&
            visibleSelectedItem.kind !== "tables" &&
            (visibleSelectedItem.kind !== "views" || !selectedRun) &&
            (visibleSelectedItem.kind !== "indexes" || !selectedRun) &&
            (visibleSelectedItem.kind !== "functions" || !selectedRun) &&
            (visibleSelectedItem.kind !== "hypertables" || !selectedRun) &&
            (visibleSelectedItem.kind !== "caggs" || !selectedRun) &&
            envId ? (
              <InventoryObjectPanel
                key={inventoryTargetKey(
                  envId,
                  selectedRun ?? "",
                  targetFor(visibleSelectedItem),
                )}
                target={targetFor(visibleSelectedItem)}
                environment={envId}
                run={selectedRun}
                snapshot={visibleSnapshot}
                overview={
                  <>
                    {selectedIndex ? <IndexDetail i={selectedIndex} /> : null}
                    {selectedView ? <ViewDetail v={selectedView} /> : null}
                    {selectedFn ? <FunctionDetail f={selectedFn} /> : null}
                    {selectedHt ? <HypertableDetail h={selectedHt} /> : null}
                  </>
                }
              />
            ) : null}
            {!selectedTable &&
            !selectedIndex &&
            !selectedView &&
            !selectedFn &&
            !selectedHt &&
            !selectedCagg &&
            !inventory &&
            !detailError ? (
              <p className="text-sm text-slate-400">Nenhum detalhe.</p>
            ) : null}
          </Sheet>
        </div>
      )}
    </>
  );
}
