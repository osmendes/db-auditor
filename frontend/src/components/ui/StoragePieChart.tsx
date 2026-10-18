/** Pie chart — shadcn Chart + Recharts (Custom Label); cores --chart-1…5. */

import { useMemo } from "react";
import { Pie, PieChart } from "recharts";
import {
  type ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "./chart";

export interface StoragePieSlice {
  label: string;
  size_bytes: number;
}

/** Cores oficiais shadcn charts (CSS vars themáveis light/dark). */
const CHART_KEYS = [
  "chart-1",
  "chart-2",
  "chart-3",
  "chart-4",
  "chart-5",
] as const;

function formatBytes(n: number): string {
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

function slugKey(label: string, idx: number): string {
  const base = label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
  return base || `env-${idx}`;
}

export function StoragePieChart({ items }: { items: StoragePieSlice[] }) {
  const total = items.reduce((s, i) => s + Math.max(0, i.size_bytes), 0);

  const { chartData, chartConfig } = useMemo(() => {
    const config: ChartConfig = {
      size_bytes: { label: "Storage" },
    };
    const data = items.map((item, idx) => {
      const key = slugKey(item.label, idx);
      const chartToken = CHART_KEYS[idx % CHART_KEYS.length];
      const color = `var(--${chartToken})`;
      config[key] = { label: item.label, color };
      return {
        key,
        label: item.label,
        size_bytes: Math.max(0, item.size_bytes),
        fill: `var(--color-${key})`,
      };
    });
    return { chartData: data, chartConfig: config };
  }, [items]);

  if (items.length === 0 || total <= 0) {
    return <p className="text-sm text-slate-400">Sem dados de storage.</p>;
  }

  return (
    <div className="flex w-full flex-col gap-4 sm:flex-row sm:items-center">
      <ul className="flex min-w-0 w-full flex-col justify-center space-y-3 sm:w-[42%]">
        {chartData.map((s, idx) => {
          const chartToken = CHART_KEYS[idx % CHART_KEYS.length];
          return (
            <li
              key={s.key}
              className="flex items-center gap-2.5 text-sm text-slate-300"
            >
              <span
                className="h-2.5 w-2.5 shrink-0 rounded-sm"
                style={{ backgroundColor: `var(--${chartToken})` }}
                aria-hidden
              />
              <span
                className="min-w-0 flex-1 truncate font-medium text-slate-100"
                title={s.label}
              >
                {s.label}
              </span>
              <span className="shrink-0 font-mono text-xs text-slate-300">
                {formatBytes(s.size_bytes)}
              </span>
            </li>
          );
        })}
        <li className="flex items-center justify-between gap-2 border-t border-slate-800 pt-2 text-xs text-slate-400">
          <span>Total</span>
          <span className="font-mono text-slate-300">{formatBytes(total)}</span>
        </li>
      </ul>

      {/* The SVG is decorative for assistive tech: the sr-only table below is
          the text equivalent (UI-05). Hidden content must not take keyboard
          focus either (#122): Recharts gives the Pie layer tabindex=0 by
          default, so it is set to -1 and the wrapper is inert. */}
      <div
        className="flex w-full items-center justify-center sm:w-[58%]"
        aria-hidden="true"
        inert
      >
        <ChartContainer
          config={chartConfig}
          className="aspect-square h-[280px] w-full max-w-[300px]"
          initialDimension={{ width: 300, height: 280 }}
        >
          <PieChart
            accessibilityLayer={false}
            margin={{ top: 12, right: 12, bottom: 12, left: 12 }}
          >
            <ChartTooltip
              content={
                <ChartTooltipContent
                  nameKey="key"
                  hideLabel
                  formatter={(value) => (
                    <span className="font-mono tabular-nums">
                      {formatBytes(Number(value))}
                    </span>
                  )}
                />
              }
            />
            <Pie
              rootTabIndex={-1}
              data={chartData}
              dataKey="size_bytes"
              nameKey="key"
              cx="50%"
              cy="50%"
              outerRadius="78%"
              labelLine={false}
              label={({ payload, ...props }) => {
                const pct =
                  total > 0
                    ? ((Number(payload.size_bytes) / total) * 100).toFixed(0)
                    : "0";
                return (
                  <text
                    cx={props.cx}
                    cy={props.cy}
                    x={props.x}
                    y={props.y}
                    textAnchor={props.textAnchor}
                    dominantBaseline={props.dominantBaseline}
                    fill="currentColor"
                    className="fill-slate-100 text-[12px] font-medium"
                  >
                    {pct}%
                  </text>
                );
              }}
            />
          </PieChart>
        </ChartContainer>
      </div>

      <table className="sr-only">
        <caption>Storage por ambiente</caption>
        <thead>
          <tr>
            <th>Ambiente</th>
            <th>Tamanho</th>
            <th>Percentual</th>
          </tr>
        </thead>
        <tbody>
          {chartData.map((s) => {
            const pct = total > 0 ? (s.size_bytes / total) * 100 : 0;
            return (
              <tr key={s.key}>
                <td>{s.label}</td>
                <td>{formatBytes(s.size_bytes)}</td>
                <td>{pct.toFixed(1)}%</td>
              </tr>
            );
          })}
          <tr>
            <td>Total</td>
            <td>{formatBytes(total)}</td>
            <td>100%</td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
