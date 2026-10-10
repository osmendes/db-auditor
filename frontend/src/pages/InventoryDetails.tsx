import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { DetailGrid, DetailSection } from "../components/ui/Sheet";
import { Table } from "../components/ui/Table";
import { formatBytes } from "../lib/format";
import {
  relationClassBadgeClass,
  relationClassLabel,
} from "../lib/relationClass";
import type {
  ColumnSnapshot,
  ColumnStatSnapshot,
  FunctionSnapshot,
  HypertableSnapshot,
  IndexSnapshot,
  TableHistoryPoint,
  TableSnapshot,
  ViewSnapshot,
  WorkloadSnapshot,
} from "../types";

function formatDate(iso: string | null | undefined): string {
  if (!iso) {
    return "—";
  }
  try {
    return new Date(iso).toLocaleString("pt-BR");
  } catch {
    return iso;
  }
}

function formatNumber(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(n)) {
    return "—";
  }
  return n.toLocaleString("pt-BR");
}

function boolLabel(v: boolean | null | undefined): string {
  if (v == null) {
    return "—";
  }
  return v ? "Sim" : "Não";
}

export function TableDetail({
  t,
  columns = [],
  history = [],
  columnStats = [],
  workload = [],
}: {
  t: TableSnapshot;
  columns?: ColumnSnapshot[];
  history?: TableHistoryPoint[];
  columnStats?: ColumnStatSnapshot[];
  workload?: WorkloadSnapshot[];
}) {
  const classLabel = relationClassLabel(t.relation_class, t.relkind);
  return (
    <>
      <DetailSection title="Identificação">
        <DetailGrid
          items={[
            { label: "Banco", value: t.database_name },
            { label: "Esquema", value: t.schema_name },
            { label: "Tabela", value: t.table_name },
            { label: "Responsável", value: t.owner_name ?? "—" },
            { label: "Tipo interno (relkind)", value: t.relkind },
            {
              label: "Classe de relação",
              value: (
                <span
                  className={`inline-flex rounded border px-1.5 py-0.5 text-[11px] font-medium ${relationClassBadgeClass(
                    t.relation_class,
                  )}`}
                >
                  {classLabel}
                </span>
              ),
            },
            { label: "Partição", value: boolLabel(t.is_partition) },
            { label: "Chave primária", value: boolLabel(t.has_primary_key) },
            {
              label: "Colunas (contagem)",
              value: formatNumber(t.column_count),
            },
            { label: "Coletado em", value: formatDate(t.collected_at) },
          ]}
        />
      </DetailSection>
      <DetailSection title="Tamanho">
        <DetailGrid
          items={[
            { label: "Total", value: formatBytes(t.total_size_bytes) },
            { label: "Dados", value: formatBytes(t.data_size_bytes) },
            { label: "Índices", value: formatBytes(t.index_size_bytes) },
            {
              label: "Estimativa de linhas",
              value: formatNumber(t.row_estimate),
            },
          ]}
        />
      </DetailSection>
      <DetailSection title="Colunas">
        {columns.length === 0 ? (
          <p className="text-sm text-slate-400">
            Contagem no snapshot:{" "}
            <span className="font-medium text-slate-200">
              {formatNumber(t.column_count)}
            </span>
            . Lista detalhada indisponível para este objeto.
          </p>
        ) : (
          <Table dense headers={["#", "Nome", "Tipo", "Nullable", "Default"]}>
            {columns.map((c) => (
              <tr key={c.id} className="border-t border-slate-800">
                <td className="px-2 py-1.5">{c.ordinal_position}</td>
                <td className="px-2 py-1.5 font-medium text-slate-100">
                  {c.column_name}
                </td>
                <td className="px-2 py-1.5">{c.data_type}</td>
                <td className="px-2 py-1.5">{boolLabel(c.is_nullable)}</td>
                <td className="px-2 py-1.5 font-mono text-[11px] text-slate-400">
                  {c.column_default ?? "—"}
                </td>
              </tr>
            ))}
          </Table>
        )}
      </DetailSection>
      <DetailSection title="Histórico (últimos 30 dias)">
        {history.length < 2 ? (
          <p className="text-sm text-slate-400">
            Ainda não há snapshots compatíveis suficientes para calcular
            tendência.
          </p>
        ) : (
          <div
            className="h-56 w-full"
            role="img"
            aria-label="Gráfico temporal de tamanho e linhas"
          >
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
        )}
        {history.some((p) => p.counters_reset || !p.complete) ? (
          <p className="mt-2 text-xs text-amber-300">
            Deltas de atividade são omitidos em resets de estatísticas e runs
            parciais permanecem sinalizados.
          </p>
        ) : null}
      </DetailSection>
      <DetailSection title="Estatísticas agregadas de colunas">
        {columnStats.length === 0 ? (
          <p className="text-sm text-slate-400">
            Sem estatísticas agregadas. Nenhum valor bruto é coletado.
          </p>
        ) : (
          <Table
            dense
            headers={[
              "Coluna",
              "Nulos",
              "Distintos (est.)",
              "Largura média",
              "Qualidade",
            ]}
          >
            {columnStats.map((s) => (
              <tr key={s.column_name} className="border-t border-slate-800">
                <td className="px-2 py-1.5 font-medium">{s.column_name}</td>
                <td className="px-2 py-1.5">
                  {(s.null_fraction * 100).toFixed(1)}%
                </td>
                <td className="px-2 py-1.5">
                  {formatNumber(s.distinct_estimate)}
                </td>
                <td className="px-2 py-1.5">
                  {formatNumber(s.average_width)} B
                </td>
                <td className="px-2 py-1.5">{s.quality}</td>
              </tr>
            ))}
          </Table>
        )}
      </DetailSection>
      <DetailSection title="Carga de consultas relacionada">
        {workload.length === 0 ? (
          <p className="text-sm text-slate-400">
            Sem evidência confiável de workload para esta tabela na janela
            observada.
          </p>
        ) : (
          <div className="space-y-2">
            {workload.slice(0, 10).map((w) => (
              <div
                key={`${w.query_fingerprint}-${w.collected_at}`}
                className="rounded border border-slate-700 p-2 text-xs"
              >
                <div className="font-mono text-slate-300">
                  {w.query_fingerprint.slice(0, 24)}…
                </div>
                <div className="mt-1 text-slate-400">
                  {formatNumber(w.calls)} chamadas ·{" "}
                  {w.total_exec_time_ms.toFixed(1)} ms total · evidência{" "}
                  {w.evidence_quality}
                </div>
                <div className="mt-1 text-slate-500">
                  {(w.query_kind || "other").toUpperCase()} · pg_stat_statements{" "}
                  {w.extension_version || "versão desconhecida"} · leituras{" "}
                  {formatNumber(w.shared_blocks_read || 0)} blocos · janela
                  desde{" "}
                  {w.stats_reset
                    ? new Date(w.stats_reset).toLocaleString()
                    : "início desconhecido"}
                </div>
              </div>
            ))}
          </div>
        )}
        <p className="mt-2 text-xs text-slate-500">
          Somente fingerprints normalizados e métricas agregadas são
          persistidos; textos e literais das queries não são armazenados.
        </p>
      </DetailSection>
      <DetailSection title="Referência">
        <DetailGrid
          items={[
            { label: "ID do inventário", value: t.id },
            { label: "Execução", value: t.audit_run_id },
            { label: "Ambiente", value: t.environment_id },
          ]}
        />
      </DetailSection>
    </>
  );
}

export function IndexDetail({ i }: { i: IndexSnapshot }) {
  return (
    <>
      <DetailSection title="Identificação">
        <DetailGrid
          items={[
            { label: "Banco", value: i.database_name },
            { label: "Esquema", value: i.schema_name },
            { label: "Tabela", value: i.table_name },
            { label: "Índice", value: i.index_name },
            { label: "Método", value: i.access_method ?? "—" },
            { label: "Único", value: boolLabel(i.is_unique) },
            { label: "Primário", value: boolLabel(i.is_primary) },
            { label: "Coletado em", value: formatDate(i.collected_at) },
          ]}
        />
      </DetailSection>
      <DetailSection title="Métricas">
        <DetailGrid
          items={[
            { label: "Tamanho", value: formatBytes(i.size_bytes) },
            { label: "idx_scan", value: formatNumber(i.idx_scan) },
          ]}
        />
      </DetailSection>
      <DetailSection title="Definição protegida">
        <p className="text-sm text-slate-300">
          A definição SQL pode conter dados sensíveis e não é exibida nesta
          tela.
        </p>
      </DetailSection>
    </>
  );
}

export function ViewDetail({ v }: { v: ViewSnapshot }) {
  const kindLabel =
    v.relkind === "m"
      ? "Visão materializada"
      : v.relkind === "v"
        ? "Visão comum"
        : v.relkind === "cagg"
          ? "Agregado contínuo"
          : "Tipo não identificado";
  return (
    <DetailSection title="Identificação">
      <DetailGrid
        items={[
          { label: "Banco", value: v.database_name },
          { label: "Esquema", value: v.schema_name },
          { label: "Visão", value: v.view_name },
          { label: "Responsável", value: v.owner_name ?? "—" },
          { label: "Tipo", value: kindLabel },
          ...(v.relkind === "m"
            ? [{ label: "Espaço ocupado", value: formatBytes(v.size_bytes) }]
            : []),
          { label: "Coletado em", value: formatDate(v.collected_at) },
        ]}
      />
    </DetailSection>
  );
}

export function FunctionDetail({ f }: { f: FunctionSnapshot }) {
  return (
    <DetailSection title="Identificação">
      <DetailGrid
        items={[
          { label: "Banco", value: f.database_name },
          { label: "Esquema", value: f.schema_name },
          { label: "Função", value: f.function_name },
          { label: "Argumentos", value: f.identity_arguments || "—" },
          { label: "Responsável", value: f.owner_name ?? "—" },
          { label: "Linguagem", value: f.language_name ?? "—" },
          { label: "Tipo", value: f.kind ?? "—" },
          {
            label: "Executa com privilégios do proprietário",
            value: boolLabel(f.is_security_definer),
          },
          { label: "Coletado em", value: formatDate(f.collected_at) },
        ]}
      />
    </DetailSection>
  );
}

export function HypertableDetail({ h }: { h: HypertableSnapshot }) {
  return (
    <>
      <DetailSection title="Identificação">
        <DetailGrid
          items={[
            { label: "Banco", value: h.database_name },
            { label: "Esquema", value: h.schema_name },
            { label: "Tabela temporal", value: h.hypertable_name },
            { label: "Responsável", value: h.owner_name ?? "—" },
            { label: "Dimensões", value: formatNumber(h.num_dimensions) },
            { label: "Chunks", value: formatNumber(h.num_chunks) },
            {
              label: "Compressão",
              value: boolLabel(h.compression_enabled),
            },
            { label: "Distribuída", value: boolLabel(h.is_distributed) },
            { label: "Coletado em", value: formatDate(h.collected_at) },
          ]}
        />
      </DetailSection>
      <DetailSection title="Tamanho">
        <DetailGrid
          items={[
            { label: "Total", value: formatBytes(h.total_size_bytes) },
            { label: "Dados", value: formatBytes(h.data_size_bytes) },
            { label: "Índices", value: formatBytes(h.index_size_bytes) },
          ]}
        />
      </DetailSection>
    </>
  );
}
