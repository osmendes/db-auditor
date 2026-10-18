import { InventoryDetailAvailability } from "../../components/InventoryDetailTabs";
import { PageHeader } from "../../components/PageHeader";
import {
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Skeleton,
  Table,
} from "../../components/ui";
import { formatBytes } from "../../lib/format";
import { nextSort } from "../../lib/sort";
import { InventoryDetailSheet } from "./InventoryDetailSheet";
import {
  InventoryListEmpty,
  InventorySnapshotEmpty,
  inventoryItemsFor,
} from "./InventoryEmptyStates";
import { InventoryFilters } from "./InventoryFilters";
import { InventoryFunctionsTable } from "./InventoryFunctionsTable";
import { InventoryHypertablesTable } from "./InventoryHypertablesTable";
import { InventoryTablesTable } from "./InventoryTablesTable";
import { InventoryViewsTable } from "./InventoryViewsTable";
import {
  boolLabel,
  detailButtonClass,
  fmtNum,
  KIND_LABELS,
  rowClass,
  runStatusLabel,
} from "./inventoryUi";
import { useInventoryModel } from "./useInventoryExplorer";

export function InventoryExplorer() {
  const model = useInventoryModel();
  const {
    envId,
    isMongo,
    error,
    setError,
    databases,
    kind,
    setOffset,
    loading,
    selectedItem,
    sort,
    setSort,
    snapshotStatus,
    sortedIndexes,
    currentRun,
    timescaleNotApplicable,
    openRow,
    envName,
    pagination,
    setSection,
  } = model;
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
            <InventorySnapshotEmpty />
          ) : (
            <>
              <InventoryFilters databases={databases} />

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
                ) : inventoryItemsFor(model).length === 0 ? (
                  <InventoryListEmpty />
                ) : kind === "tables" ? (
                  <InventoryTablesTable />
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
                  <InventoryViewsTable />
                ) : kind === "functions" ? (
                  <InventoryFunctionsTable />
                ) : (
                  <InventoryHypertablesTable />
                )}
              </Card>
            </>
          )}

          <InventoryDetailSheet />
        </div>
      )}
    </>
  );
}
