import { lazy, type ReactNode } from "react";
import { trendEntries } from "../../lib/trend-gaps";
import type { RunTrendPoint } from "../../types";

export const StoragePieChart = lazy(() =>
  import("../../components/ui/StoragePieChart").then((module) => ({
    default: module.StoragePieChart,
  })),
);

export function formatBytes(n: number): string {
  if (n <= 0) {
    return "0 B";
  }
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function MiniBar({
  label,
  value,
  max,
  tone = "emerald",
  valueLabel,
}: {
  label: string;
  value: number;
  max: number;
  tone?: "emerald" | "amber" | "rose" | "sky";
  valueLabel?: string;
}) {
  const pct = max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0;
  const colors = {
    emerald: "bg-emerald-500/80",
    amber: "bg-amber-500/80",
    rose: "bg-rose-500/80",
    sky: "bg-sky-500/80",
  };
  return (
    <div className="space-y-1">
      <div className="flex justify-between gap-2 text-xs">
        <span className="truncate text-slate-300" title={label}>
          {label}
        </span>
        <span className="shrink-0 font-mono text-slate-100">
          {valueLabel ?? value}
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
        <div
          className={`h-full rounded-full ${colors[tone]}`}
          style={{ width: `${pct}%` }}
          role="presentation"
        />
      </div>
    </div>
  );
}

export function KpiGroup({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-2">
      <p className="text-xs font-medium text-slate-400">{title}</p>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{children}</div>
    </div>
  );
}

export function TrendBars({
  points,
  metric,
  granularity,
}: {
  points: RunTrendPoint[];
  metric: "size_bytes" | "findings" | "score" | "score_confidence";
  granularity: "day" | "week" | "month";
}) {
  const visible = points.slice(-60);
  const entries = trendEntries(visible, granularity);
  const max = Math.max(1, ...visible.map((point) => point[metric] ?? 0));
  if (visible.length === 0) {
    return (
      <p className="text-sm text-slate-400">
        Sem medições comparáveis no período.
      </p>
    );
  }
  return (
    <div className="max-h-72 space-y-3 overflow-y-auto pr-2">
      {entries.map((entry) => {
        if (entry.kind === "gap") {
          return (
            <p
              key={entry.key}
              className="border-l-2 border-dashed border-amber-500 pl-2 text-xs text-amber-200"
            >
              {entry.environment}: {entry.periods}{" "}
              {entry.periods === 1
                ? "período sem coleta"
                : "períodos sem coleta"}
              .
            </p>
          );
        }
        const { point } = entry;
        if (point[metric] == null) {
          return (
            <p
              key={point.audit_run_id}
              className="border-l-2 border-dashed border-amber-500 pl-2 text-xs text-amber-200"
            >
              {new Date(point.at).toLocaleDateString("pt-BR")} ·{" "}
              {point.environment_name}: sem dado para esta métrica (
              {point.coverage === "partial"
                ? "coleta ou análise parcial"
                : "inventário ausente"}
              ).
            </p>
          );
        }
        const value = point[metric] ?? 0;
        const when = new Date(point.at).toLocaleDateString("pt-BR");
        return (
          <div
            key={point.audit_run_id}
            title={`Execução ${point.audit_run_id}`}
          >
            <MiniBar
              label={`${when} · ${point.environment_name}${point.coverage === "partial" ? " · coleta parcial" : ""}${point.counter_reset ? " · contadores reiniciados" : ""}${!point.comparable && point.comparison_note ? ` · ${point.comparison_note}` : ""}`}
              value={value}
              max={max}
              valueLabel={
                metric === "size_bytes"
                  ? formatBytes(value)
                  : metric === "score_confidence"
                    ? `${Math.round(value * 100)}%`
                    : metric === "score"
                      ? `${value}/100`
                      : String(value)
              }
              tone={!point.comparable ? "amber" : "sky"}
            />
          </div>
        );
      })}
      <p className="text-xs text-slate-400">
        Cada barra representa a última execução elegível do período. Períodos
        sem coleta não são tratados como zero. Mostrando {visible.length} de{" "}
        {points.length} pontos; selecione um ambiente ou aumente a granularidade
        para facilitar a leitura.
      </p>
    </div>
  );
}
