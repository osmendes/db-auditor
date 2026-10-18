import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { formatBytes } from "../../lib/format";
import type { TableHistoryPoint } from "../../types";

function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString("pt-BR");
}

function formatNumber(n: number | null | undefined): string {
  return n == null ? "—" : n.toLocaleString("pt-BR");
}

/**
 * Size/rows history of one table. The SVG is hidden from assistive tech and
 * an sr-only table carries the same points as text (UI-05).
 */
export function TableHistoryChart({
  history,
}: {
  history: TableHistoryPoint[];
}) {
  return (
    <>
      <div className="h-56 w-full" aria-hidden="true">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={history}>
            <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
            <XAxis
              dataKey="bucket"
              tickFormatter={(v) =>
                new Date(String(v)).toLocaleDateString("pt-BR")
              }
              stroke="#94a3b8"
            />
            <YAxis
              yAxisId="bytes"
              tickFormatter={(v) => formatBytes(Number(v))}
              stroke="#94a3b8"
              width={72}
            />
            <YAxis
              yAxisId="rows"
              orientation="right"
              stroke="#94a3b8"
              width={52}
            />
            <Tooltip
              labelFormatter={(v) => formatDate(String(v))}
              formatter={(value, name) =>
                name === "Storage"
                  ? formatBytes(Number(value))
                  : formatNumber(Number(value))
              }
            />
            <Line
              yAxisId="bytes"
              type="monotone"
              dataKey="total_size_bytes"
              name="Storage"
              stroke="#38bdf8"
              dot={false}
            />
            <Line
              yAxisId="rows"
              type="monotone"
              dataKey="row_estimate"
              name="Linhas"
              stroke="#a78bfa"
              dot={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
      <table className="sr-only">
        <caption>Histórico de tamanho e linhas da tabela</caption>
        <thead>
          <tr>
            <th scope="col">Período</th>
            <th scope="col">Tamanho</th>
            <th scope="col">Linhas estimadas</th>
            <th scope="col">Coleta</th>
          </tr>
        </thead>
        <tbody>
          {history.map((point) => (
            <tr key={`${point.bucket}-${point.audit_run_id}`}>
              <td>{formatDate(point.bucket)}</td>
              <td>{formatBytes(point.total_size_bytes)}</td>
              <td>{formatNumber(point.row_estimate)}</td>
              <td>
                {point.complete ? "completa" : "parcial"}
                {point.counters_reset ? ", contadores reiniciados" : ""}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}
