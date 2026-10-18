import { Table } from "../../components/ui";
import { nextSort } from "../../lib/sort";
import { boolLabel, detailButtonClass, rowClass } from "./inventoryUi";
import { useInventoryModel } from "./useInventoryExplorer";

/** Functions list; markup moved verbatim from InventoryExplorer (UI-06). */
export function InventoryFunctionsTable() {
  const {
    setOffset,
    selectedItem,
    sort,
    setSort,
    sortedFunctions,
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
                openRow({ kind: "functions", item: f }, currentRun)
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
                    openRow({ kind: "functions", item: f }, currentRun);
                  }}
                >
                  {f.function_name}
                </button>
              </td>
              <td className="px-3 py-2">{f.kind ?? "—"}</td>
              <td className="px-3 py-2">{f.language_name ?? "—"}</td>
              <td className="px-3 py-2">{boolLabel(f.is_security_definer)}</td>
            </tr>
          );
        })}
      </Table>
      {pagination}
    </>
  );
}
