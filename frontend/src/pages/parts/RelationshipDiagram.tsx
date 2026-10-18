import { useState } from "react";
import { inventoryPermalink } from "../../lib/inventoryTarget";
import type {
  RelationshipEdge,
  RelationshipGraph,
  TableSnapshot,
} from "../../types";

/** Foreign-key diagram of the table assessment (UI-06 extraction). */

/** Direct link to a table of one run (shared with TableAssessmentPanel). */
export function tablePermalink(env: string, t: TableSnapshot): string {
  return inventoryPermalink(env, t.audit_run_id, {
    kind: "tables",
    database: t.database_name,
    schema: t.schema_name,
    name: t.table_name,
  });
}

function nodeKey(schema: string, table: string) {
  return `${schema}.${table}`;
}

export function visibleRelationshipEdges(
  graph: RelationshipGraph,
  root: TableSnapshot,
  direction: "all" | "incoming" | "outgoing",
) {
  const key = nodeKey(root.schema_name, root.table_name);
  return graph.edges.filter(
    (edge) =>
      direction === "all" ||
      (direction === "incoming"
        ? nodeKey(edge.to.schema, edge.to.table) === key
        : nodeKey(edge.from.schema, edge.from.table) === key),
  );
}

export function RelationshipDiagram({
  graph,
  root,
  env,
  run,
  database,
  onNavigate,
}: {
  graph: RelationshipGraph;
  root: TableSnapshot;
  env: string;
  run: string;
  database: string;
  onNavigate: (schema: string, table: string) => void;
}) {
  const [direction, setDirection] = useState<"all" | "incoming" | "outgoing">(
    "all",
  );
  const [schemaFilter, setSchemaFilter] = useState("all");
  const [zoom, setZoom] = useState(1);
  const rootKey = nodeKey(root.schema_name, root.table_name);
  const schemas = [...new Set(graph.nodes.map((n) => n.schema))].sort();
  const edges = visibleRelationshipEdges(graph, root, direction).filter(
    (edge) =>
      schemaFilter === "all" ||
      (nodeKey(edge.from.schema, edge.from.table) === rootKey
        ? edge.to.schema
        : edge.from.schema) === schemaFilter,
  );
  const nodes = graph.nodes.filter(
    (n) =>
      nodeKey(n.schema, n.table) === rootKey ||
      edges.some(
        (e) =>
          nodeKey(e.from.schema, e.from.table) === nodeKey(n.schema, n.table) ||
          nodeKey(e.to.schema, e.to.table) === nodeKey(n.schema, n.table),
      ),
  );
  const positions = new Map(
    nodes.map((n, i) => [
      nodeKey(n.schema, n.table),
      i === 0
        ? { x: 300, y: 180 }
        : {
            x:
              300 +
              220 *
                Math.cos(
                  (2 * Math.PI * (i - 1)) / Math.max(1, nodes.length - 1),
                ),
            y:
              180 +
              130 *
                Math.sin(
                  (2 * Math.PI * (i - 1)) / Math.max(1, nodes.length - 1),
                ),
          },
    ]),
  );
  const link = (schema: string, table: string) =>
    tablePermalink(env, {
      ...root,
      audit_run_id: run,
      database_name: database,
      schema_name: schema,
      table_name: table,
    });
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3 text-xs">
        <label>
          Direção{" "}
          <select
            aria-label="Direção dos relacionamentos"
            value={direction}
            onChange={(e) => setDirection(e.target.value as typeof direction)}
            className="rounded bg-slate-800 p-1"
          >
            <option value="all">Todas</option>
            <option value="incoming">Entrada</option>
            <option value="outgoing">Saída</option>
          </select>
        </label>
        <label>
          Schema relacionado{" "}
          <select
            aria-label="Filtrar schema relacionado"
            value={schemaFilter}
            onChange={(e) => setSchemaFilter(e.target.value)}
            className="rounded bg-slate-800 p-1"
          >
            <option value="all">Todos</option>
            {schemas.map((schema) => (
              <option key={schema} value={schema}>
                {schema}
              </option>
            ))}
          </select>
        </label>
        <label>
          Zoom{" "}
          <input
            aria-label="Zoom do diagrama"
            type="range"
            min="0.7"
            max="1.8"
            step="0.1"
            value={zoom}
            onChange={(e) => setZoom(Number(e.target.value))}
          />
        </label>
      </div>
      {edges.length === 0 ? (
        <p className="text-sm text-slate-400">Nenhuma FK nesta direção.</p>
      ) : (
        <div
          className="overflow-auto rounded border border-slate-700"
          aria-hidden="true"
        >
          <svg
            viewBox="0 0 600 360"
            width={600 * zoom}
            height={360 * zoom}
            role="img"
          >
            <title>{`Diagrama ER focado em ${rootKey}`}</title>
            {edges.map((edge: RelationshipEdge) => {
              const from = positions.get(
                nodeKey(edge.from.schema, edge.from.table),
              );
              const to = positions.get(nodeKey(edge.to.schema, edge.to.table));
              return from && to ? (
                <line
                  key={edge.constraint_name}
                  x1={from.x}
                  y1={from.y}
                  x2={to.x}
                  y2={to.y}
                  stroke="#38bdf8"
                  strokeWidth="2"
                />
              ) : null;
            })}
            {nodes.map((n) => {
              const p = positions.get(nodeKey(n.schema, n.table));
              if (!p) return null;
              const isRoot = nodeKey(n.schema, n.table) === rootKey;
              return (
                <g key={nodeKey(n.schema, n.table)}>
                  <rect
                    x={p.x - 67}
                    y={p.y - 17}
                    width="134"
                    height="34"
                    rx="6"
                    fill={isRoot ? "#0e7490" : "#334155"}
                  />
                  <text
                    x={p.x}
                    y={p.y + 5}
                    textAnchor="middle"
                    fill="white"
                    fontSize="11"
                  >
                    {n.table.slice(0, 19)}
                  </text>
                </g>
              );
            })}
          </svg>
        </div>
      )}
      <p className="text-xs text-slate-400">
        Legenda: bloco azul = tabela selecionada; linha = chave estrangeira. Use
        a lista abaixo para navegação acessível.
      </p>
      <ul className="space-y-2 text-sm" aria-label="Relacionamentos acessíveis">
        {edges.map((e) => {
          const target =
            nodeKey(e.from.schema, e.from.table) === rootKey ? e.to : e.from;
          return (
            <li
              key={e.constraint_name}
              className="rounded border border-slate-700 p-2"
            >
              <span>
                {e.from.schema}.{e.from.table} ({e.columns.join(", ")}) →{" "}
                {e.to.schema}.{e.to.table} ({e.referenced_columns.join(", ")})
              </span>
              <span className="ml-2 text-slate-400">{e.constraint_name}</span>{" "}
              <a
                href={link(target.schema, target.table)}
                onClick={(event) => {
                  event.preventDefault();
                  onNavigate(target.schema, target.table);
                }}
                className="text-cyan-300 underline"
              >
                Abrir tabela
              </a>
            </li>
          );
        })}
      </ul>
      {graph.truncated ? (
        <p className="text-xs text-amber-300">
          Grafo limitado a 50 relacionamentos. Refine o escopo.
        </p>
      ) : null}
    </div>
  );
}
