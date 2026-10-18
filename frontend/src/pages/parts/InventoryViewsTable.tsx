import { Table } from "../../components/ui";
import { formatBytes } from "../../lib/format";
import { nextSort } from "../../lib/sort";
import {
  detailButtonClass,
  rowClass,
  type SelectedInventoryItem,
} from "./inventoryUi";
import { useInventoryModel } from "./useInventoryExplorer";

/** Views and continuous aggregates list; markup moved verbatim from InventoryExplorer (UI-06). */
export function InventoryViewsTable() {
  const {
    kind,
    setOffset,
    caggs,
    selectedItem,
    sort,
    setSort,
    sortedViews,
    currentRun,
    openRow,
    pagination,
  } = useInventoryModel();
  return (
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
          if (viewKind === "caggs" && !selectedCaggRow) return null;
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
                {kind === "caggs" ? "Não coletado" : formatBytes(v.size_bytes)}
              </td>
              <td className="px-3 py-2">
                {kind === "caggs" ? "Agregado contínuo" : v.relkind}
              </td>
            </tr>
          );
        })}
      </Table>
      {pagination}
    </>
  );
}
