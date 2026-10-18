import { Table } from "../../components/ui";
import { formatBytes } from "../../lib/format";
import {
  relationClassBadgeClass,
  relationClassLabel,
} from "../../lib/relationClass";
import { nextSort } from "../../lib/sort";
import { boolLabel, detailButtonClass, fmtNum, rowClass } from "./inventoryUi";
import { useInventoryModel } from "./useInventoryExplorer";

/** Tables/collections list; markup moved verbatim from InventoryExplorer (UI-06). */
export function InventoryTablesTable() {
  const {
    isMongo,
    setOffset,
    selectedItem,
    sort,
    setSort,
    sortedTables,
    openTable,
    pagination,
  } = useInventoryModel();
  return (
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
                  {relationClassLabel(t.relation_class, t.relkind)}
                </span>
              </td>
              <td className="px-3 py-2">
                {isMongo ? "não coletado" : fmtNum(t.column_count)}
              </td>
              <td className="px-3 py-2">{fmtNum(t.row_estimate)}</td>
              <td className="px-3 py-2">{formatBytes(t.total_size_bytes)}</td>
              <td className="px-3 py-2">{boolLabel(t.has_primary_key)}</td>
            </tr>
          );
        })}
      </Table>
      {pagination}
    </>
  );
}
