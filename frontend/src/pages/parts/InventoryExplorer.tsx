import { InventoryDetailAvailability } from "../../components/InventoryDetailTabs";
import { PageHeader } from "../../components/PageHeader";
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
} from "../../components/ui";
import { formatBytes } from "../../lib/format";
import { inventoryTargetKey } from "../../lib/inventoryTarget";
import {
  relationClassBadgeClass,
  relationClassLabel,
} from "../../lib/relationClass";
import { nextSort } from "../../lib/sort";
import { CAGGAssessmentPanel } from "../CAGGAssessmentPanel";
import { FunctionAssessmentPanel } from "../FunctionAssessmentPanel";
import { HypertableAssessmentPanel } from "../HypertableAssessmentPanel";
import { IndexAssessmentPanel } from "../IndexAssessmentPanel";
import {
  FunctionDetail,
  HypertableDetail,
  IndexDetail,
  ViewDetail,
} from "../InventoryDetails";
import { InventoryObjectPanel } from "../InventoryObjectPanel";
import { TableAssessmentPanel } from "../TableAssessmentPanel";
import { ViewAssessmentPanel } from "../ViewAssessmentPanel";
import {
  boolLabel,
  detailButtonClass,
  fmtNum,
  KIND_LABELS,
  KINDS,
  rowClass,
  runStatusLabel,
  type SelectedInventoryItem,
  targetFor,
} from "./inventoryUi";
import { useInventoryModel } from "./useInventoryExplorer";

export function InventoryExplorer() {
  const {
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
  } = useInventoryModel();
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
