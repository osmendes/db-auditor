import type { InventoryTarget } from "../../lib/inventoryTarget";
import type {
  CAGGSnapshot,
  FunctionSnapshot,
  HypertableSnapshot,
  IndexSnapshot,
  InventoryObjectKind,
  TableSnapshot,
  ViewSnapshot,
} from "../../types";

export type SelectedInventoryItem =
  | { kind: "tables"; item: TableSnapshot }
  | { kind: "indexes"; item: IndexSnapshot }
  | { kind: "views"; item: ViewSnapshot }
  | { kind: "caggs"; item: CAGGSnapshot }
  | { kind: "functions"; item: FunctionSnapshot }
  | { kind: "hypertables"; item: HypertableSnapshot };

export function targetFor(item: SelectedInventoryItem): InventoryTarget {
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

export const KINDS: InventoryObjectKind[] = [
  "tables",
  "hypertables",
  "indexes",
  "views",
  "functions",
  "caggs",
];

export const KIND_LABELS: Record<InventoryObjectKind, string> = {
  tables: "Tabelas",
  hypertables: "Tabelas temporais",
  indexes: "Índices",
  views: "Visões",
  functions: "Funções",
  caggs: "Agregados contínuos",
};

export const detailButtonClass =
  "rounded text-left font-medium text-slate-100 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400";

export function rowClass(selected: boolean): string {
  return `cursor-pointer border-t border-slate-800/80 transition-colors ${
    selected ? "bg-slate-800/70" : "hover:bg-slate-900/50"
  }`;
}

export function fmtNum(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(n)) return "—";
  return n.toLocaleString("pt-BR");
}

export function boolLabel(v: boolean | null | undefined): string {
  if (v == null) return "—";
  return v ? "Sim" : "Não";
}

export function runStatusLabel(status: string): string {
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
