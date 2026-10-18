import { Card, ErrorBanner, Input, Select } from "../../components/ui";
import { KIND_LABELS, KINDS } from "./inventoryUi";
import { type InventoryModel, useInventoryModel } from "./useInventoryExplorer";

/** Filter card of the inventory explorer; markup moved verbatim from InventoryExplorer (UI-06). */
export function InventoryFilters({
  databases,
}: {
  /** Narrowed by the caller: the filters render only once databases load. */
  databases: NonNullable<InventoryModel["databases"]>;
}) {
  const {
    isMongo,
    selectedDb,
    setSelectedDb,
    selectedSchema,
    setSelectedSchema,
    kind,
    setKind,
    q,
    setQ,
    setOffset,
    listError,
    schemasForDb,
    loadObjects,
    closeSheet,
  } = useInventoryModel();
  return (
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
        {KINDS.filter((k) => !isMongo || k === "tables" || k === "indexes").map(
          (k) => (
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
          ),
        )}
      </div>
      {listError ? (
        <div className="mt-3">
          <ErrorBanner message={listError} onRetry={() => loadObjects()} />
        </div>
      ) : null}
    </Card>
  );
}
