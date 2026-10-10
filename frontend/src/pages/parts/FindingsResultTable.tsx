import type { Dispatch, SetStateAction } from "react";
import { Badge, Table } from "../../components/ui";
import {
  type PageSize,
  PaginationControls,
} from "../../components/ui/PaginationControls";
import { labels } from "../../lib/labels";
import { nextSort, type SortState } from "../../lib/sort";
import { api } from "../../services/api";
import type { Finding } from "../../types";
import { severityTone, statusTone } from "./findingsUi";

export function FindingsResultTable({
  busy,
  sortedItems,
  sort,
  setSort,
  allSelected,
  toggleAll,
  selectedIds,
  toggleOne,
  setSelected,
  openFinding,
  total,
  pageSize,
  offset,
  setPageSize,
  setOffset,
}: {
  busy: boolean;
  sortedItems: Finding[];
  sort: SortState | null;
  setSort: Dispatch<SetStateAction<SortState | null>>;
  allSelected: boolean;
  toggleAll: () => void;
  selectedIds: Set<string>;
  toggleOne: (id: string) => void;
  setSelected: (finding: Finding) => void;
  openFinding: (id: string | null) => void;
  total: number;
  pageSize: PageSize;
  offset: number;
  setPageSize: (value: PageSize) => void;
  setOffset: (value: number) => void;
}) {
  return (
    <>
      {!busy && sortedItems.length > 0 ? (
        <>
          <Table
            dense
            pagination={false}
            virtualize
            headers={[
              { id: "sel", label: "Sel." },
              { id: "type", label: "Tipo", sortable: true },
              { id: "severity", label: "Severidade", sortable: true },
              { id: "status", label: "Status", sortable: true },
              { id: "title", label: "Título", sortable: true },
              { id: "object", label: "Objeto", sortable: true },
              { id: "last_seen", label: "Última vez", sortable: true },
            ]}
            sortKey={sort?.key}
            sortDir={sort?.dir}
            onSort={(id) => {
              if (id === "sel") {
                return;
              }
              setSort((prev) => nextSort(prev, id));
            }}
          >
            <tr className="border-t border-slate-800 bg-slate-900/40">
              <td className="px-3 py-1.5">
                <input
                  type="checkbox"
                  disabled={!api.hasRole("auditor")}
                  checked={allSelected}
                  onChange={toggleAll}
                  aria-label="Selecionar todos"
                  className="rounded border-slate-600 bg-slate-900 text-emerald-500 focus:ring-emerald-400/50"
                />
              </td>
              <td className="px-3 py-1.5 text-xs text-slate-500" colSpan={6}>
                Selecionar todos na página ({sortedItems.length})
              </td>
            </tr>
            {sortedItems.map((f) => (
              <tr
                key={f.id}
                tabIndex={0}
                aria-label={`Abrir achado: ${f.friendly_meaning ?? f.title}`}
                className="cursor-pointer border-t border-slate-800 hover:bg-slate-900/50"
                onClick={() => {
                  setSelected(f);
                  openFinding(f.id);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setSelected(f);
                    openFinding(f.id);
                  }
                }}
              >
                <td className="px-3 py-1.5">
                  <input
                    type="checkbox"
                    disabled={!api.hasRole("auditor")}
                    checked={selectedIds.has(f.id)}
                    onClick={(e) => {
                      e.stopPropagation();
                    }}
                    onChange={() => {
                      toggleOne(f.id);
                    }}
                    aria-label={`Selecionar achado em ${f.object_key || "objeto não informado"}`}
                    className="rounded border-slate-600 bg-slate-900 text-emerald-500 focus:ring-emerald-400/50"
                  />
                </td>
                <td className="px-3 py-1.5 font-mono text-xs text-slate-300">
                  {labels.findingCategory(
                    f.category || f.finding_type.split(".")[0],
                  )}
                </td>
                <td className="px-3 py-1.5">
                  <Badge tone={severityTone(f.severity)}>
                    {labels.severity(f.severity)}
                  </Badge>
                </td>
                <td className="px-3 py-1.5">
                  <Badge tone={statusTone(f.status)}>
                    {labels.findingStatus(f.status)}
                  </Badge>
                </td>
                <td className="px-3 py-1.5 text-slate-100">{f.title}</td>
                <td className="px-3 py-1.5 font-mono text-xs text-slate-400">
                  {f.object_key || "—"}
                </td>
                <td className="px-3 py-1.5 text-xs text-slate-400">
                  {f.last_seen_at
                    ? new Date(f.last_seen_at).toLocaleString()
                    : "—"}
                </td>
              </tr>
            ))}
          </Table>
          <PaginationControls
            total={total}
            offset={pageSize === "all" ? 0 : offset}
            size={pageSize}
            onSizeChange={(next) => {
              setPageSize(next);
              setOffset(0);
            }}
            onOffsetChange={setOffset}
            label="Achados"
          />
        </>
      ) : null}
    </>
  );
}
