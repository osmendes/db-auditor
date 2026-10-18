import { Table } from "../../components/ui";
import { formatBytes } from "../../lib/format";
import { nextSort } from "../../lib/sort";
import { boolLabel, detailButtonClass, fmtNum, rowClass } from "./inventoryUi";
import { useInventoryModel } from "./useInventoryExplorer";

/** Hypertables list; markup moved verbatim from InventoryExplorer (UI-06). */
export function InventoryHypertablesTable() {
  const {
    setOffset,
    selectedItem,
    sort,
    setSort,
    sortedHypertables,
    openRow,
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
                openRow({ kind: "hypertables", item: h }, h.audit_run_id)
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
                    openRow({ kind: "hypertables", item: h }, h.audit_run_id);
                  }}
                >
                  {h.hypertable_name}
                </button>
              </td>
              <td className="px-3 py-2">{formatBytes(h.total_size_bytes)}</td>
              <td className="px-3 py-2">{fmtNum(h.num_chunks)}</td>
              <td className="px-3 py-2">{boolLabel(h.compression_enabled)}</td>
            </tr>
          );
        })}
      </Table>
      {pagination}
    </>
  );
}
