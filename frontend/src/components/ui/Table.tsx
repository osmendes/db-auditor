import { Children, type ReactNode, useMemo, useState } from "react";
import type { SortDir } from "../../lib/sort";
import { type PageSize, PaginationControls } from "./PaginationControls";

export type TableHeader =
  | string
  | {
      id: string;
      label: string;
      sortable?: boolean;
    };

export interface TableProps {
  headers: TableHeader[];
  children: ReactNode;
  className?: string;
  /** Compact row padding for dense inventories. */
  dense?: boolean;
  sortKey?: string;
  sortDir?: SortDir;
  onSort?: (id: string) => void;
  pagination?: boolean;
  /**
   * When set, only this many rows are mounted. The page is still the server
   * page already passed as children — virtualization does not fetch more.
   */
  virtualize?: boolean;
  rowHeight?: number;
  /** Accessible name of the table (rendered as an sr-only caption). */
  caption?: string;
}

const DEFAULT_WINDOW = 30;

function headerId(header: TableHeader, index: number): string {
  if (typeof header === "string") {
    return `h-${index}-${header}`;
  }
  return header.id;
}

function headerLabel(header: TableHeader): string {
  return typeof header === "string" ? header : header.label;
}

function headerSortable(header: TableHeader): boolean {
  return typeof header !== "string" && Boolean(header.sortable);
}

export function Table({
  headers,
  children,
  className = "",
  dense = false,
  sortKey,
  sortDir,
  onSort,
  pagination = true,
  virtualize = false,
  rowHeight = 36,
  caption,
}: TableProps) {
  const thPad = dense ? "px-3 py-2" : "px-4 py-3";
  const [size, setSize] = useState<PageSize>(20);
  const [offset, setOffset] = useState(0);
  const [windowStart, setWindowStart] = useState(0);
  const rows = Children.toArray(children);
  const safeOffset =
    size === "all"
      ? 0
      : Math.min(offset, Math.max(0, Math.ceil(rows.length / size) - 1) * size);
  const pageRows =
    !pagination || size === "all"
      ? rows
      : rows.slice(safeOffset, safeOffset + size);
  const mounted = useMemo(() => {
    if (!virtualize || pageRows.length <= DEFAULT_WINDOW) {
      return { rows: pageRows, padBefore: 0, padAfter: 0 };
    }
    const start = Math.min(
      windowStart,
      Math.max(0, pageRows.length - DEFAULT_WINDOW),
    );
    const end = Math.min(pageRows.length, start + DEFAULT_WINDOW);
    return {
      rows: pageRows.slice(start, end),
      padBefore: start * rowHeight,
      padAfter: (pageRows.length - end) * rowHeight,
    };
  }, [pageRows, virtualize, windowStart, rowHeight]);

  return (
    <div className={className}>
      {/* Focusable scroll region so keyboard users can scroll wide tables
          (axe: scrollable-region-focusable). */}
      <section
        className="overflow-x-auto rounded-lg border border-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
        aria-label={caption ?? `Tabela: ${headers.map(headerLabel).join(", ")}`}
        // biome-ignore lint/a11y/noNoninteractiveTabindex: scrollable region must be keyboard-reachable
        tabIndex={0}
        style={virtualize ? { maxHeight: 640, overflowY: "auto" } : undefined}
        onScroll={
          virtualize
            ? (event) => {
                const top = event.currentTarget.scrollTop;
                setWindowStart(Math.max(0, Math.floor(top / rowHeight) - 5));
              }
            : undefined
        }
      >
        <table className="min-w-full text-left text-sm text-slate-200">
          {caption ? <caption className="sr-only">{caption}</caption> : null}
          <thead className="sticky top-0 z-10 bg-slate-900/95 text-slate-400 backdrop-blur-sm">
            <tr>
              {headers.map((header, index) => {
                const id = headerId(header, index);
                const label = headerLabel(header);
                const sortable = headerSortable(header);
                const active = sortable && sortKey === id;
                if (sortable && onSort) {
                  return (
                    <th
                      key={id}
                      scope="col"
                      className={`${thPad} font-medium`}
                      aria-sort={
                        active
                          ? sortDir === "asc"
                            ? "ascending"
                            : "descending"
                          : "none"
                      }
                    >
                      <button
                        type="button"
                        onClick={() => onSort(id)}
                        className="inline-flex items-center gap-1 text-left text-slate-300 transition hover:text-slate-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
                      >
                        <span>{label}</span>
                        <span
                          className="font-mono text-[10px] text-slate-500"
                          aria-hidden
                        >
                          {active ? (sortDir === "asc" ? "↑" : "↓") : "↕"}
                        </span>
                      </button>
                    </th>
                  );
                }
                return (
                  <th key={id} scope="col" className={`${thPad} font-medium`}>
                    {label}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800">
            {mounted.padBefore > 0 ? (
              <tr aria-hidden>
                <td style={{ height: mounted.padBefore }} />
              </tr>
            ) : null}
            {mounted.rows}
            {mounted.padAfter > 0 ? (
              <tr aria-hidden>
                <td style={{ height: mounted.padAfter }} />
              </tr>
            ) : null}
          </tbody>
        </table>
      </section>
      {pagination ? (
        <PaginationControls
          total={rows.length}
          offset={safeOffset}
          size={size}
          onSizeChange={(next) => {
            setSize(next);
            setOffset(0);
            setWindowStart(0);
          }}
          onOffsetChange={(next) => {
            setOffset(next);
            setWindowStart(0);
          }}
          label={headers.map(headerLabel).join(", ")}
        />
      ) : null}
    </div>
  );
}
