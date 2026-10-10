import { Button, Card } from "../../components/ui";
import { api } from "../../services/api";
import type { EnvironmentCapabilities } from "../../types";
import { capabilityLabels } from "./assistedActionLabels";

export function AssistedSetupCards({
  capabilities,
  enabled,
  database,
  setDatabase,
  schema,
  setSchema,
  table,
  setTable,
  limit,
  setLimit,
  nonNull,
  setNonNull,
  keys,
  setKeys,
  dateColumn,
  setDateColumn,
  dateFrom,
  setDateFrom,
  dateTo,
  setDateTo,
  busy,
  startScan,
}: {
  capabilities: EnvironmentCapabilities | null;
  enabled: boolean;
  database: string;
  setDatabase: (value: string) => void;
  schema: string;
  setSchema: (value: string) => void;
  table: string;
  setTable: (value: string) => void;
  limit: number;
  setLimit: (value: number) => void;
  nonNull: string;
  setNonNull: (value: string) => void;
  keys: string;
  setKeys: (value: string) => void;
  dateColumn: string;
  setDateColumn: (value: string) => void;
  dateFrom: string;
  setDateFrom: (value: string) => void;
  dateTo: string;
  setDateTo: (value: string) => void;
  busy: boolean;
  startScan: () => void;
}) {
  return (
    <>
      <Card
        title="Capacidades do mecanismo"
        subtitle={`Mecanismo: ${capabilities?.engine ?? "desconhecido"}. Uma capacidade ausente aparece como não aplicável, não como ausência de problema.`}
      >
        <ul className="mt-3 grid gap-2 text-sm sm:grid-cols-2">
          {capabilities?.items.map((item) => (
            <li key={item.name} className="rounded border border-slate-700 p-2">
              {capabilityLabels[item.name] ?? item.name}:{" "}
              {item.applicable
                ? "disponível"
                : `não aplicável — ${item.reason}`}
            </li>
          ))}
        </ul>
      </Card>
      <Card
        title="Diagnóstico opcional de dados"
        subtitle="Até 1.000 linhas por tabela. Tabelas maiores usam páginas selecionadas pelo PostgreSQL; a margem estatística não é calculável. Nenhum valor de linha é armazenado."
      >
        {!enabled ? (
          <p className="mt-3 text-sm text-amber-300">
            Desativado por padrão. Um operador deve habilitar
            AUDITOR_DATA_QUALITY_ENABLED=1 e fornecer uma conta de leitura sem
            privilégios de escrita.
          </p>
        ) : (
          <div className="mt-3 space-y-3 text-sm">
            <div className="grid gap-2 sm:grid-cols-3">
              <input
                aria-label="Banco"
                placeholder="Banco"
                value={database}
                onChange={(e) => setDatabase(e.target.value)}
                className="rounded border border-slate-600 bg-slate-900 p-2"
              />
              <input
                aria-label="Schema"
                placeholder="Schema"
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                className="rounded border border-slate-600 bg-slate-900 p-2"
              />
              <input
                aria-label="Tabela"
                placeholder="Tabela"
                value={table}
                onChange={(e) => setTable(e.target.value)}
                className="rounded border border-slate-600 bg-slate-900 p-2"
              />
            </div>
            <label className="block">
              Limite de linhas{" "}
              <input
                type="number"
                min={1}
                max={1000}
                value={limit}
                onChange={(e) => setLimit(Number(e.target.value))}
                className="ml-2 w-24 rounded border border-slate-600 bg-slate-900 p-2"
              />
            </label>
            <input
              aria-label="Colunas obrigatórias, separadas por vírgula"
              placeholder="Colunas obrigatórias (vírgula)"
              value={nonNull}
              onChange={(e) => setNonNull(e.target.value)}
              className="w-full rounded border border-slate-600 bg-slate-900 p-2"
            />
            <input
              aria-label="Chaves candidatas, separadas por vírgula"
              placeholder="Chaves candidatas (vírgula)"
              value={keys}
              onChange={(e) => setKeys(e.target.value)}
              className="w-full rounded border border-slate-600 bg-slate-900 p-2"
            />
            <div className="grid gap-2 sm:grid-cols-3">
              <input
                aria-label="Coluna de data opcional"
                placeholder="Coluna de data (opcional)"
                value={dateColumn}
                onChange={(e) => setDateColumn(e.target.value)}
                className="rounded border border-slate-600 bg-slate-900 p-2"
              />
              <input
                aria-label="Data inicial"
                type="datetime-local"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="rounded border border-slate-600 bg-slate-900 p-2"
              />
              <input
                aria-label="Data final"
                type="datetime-local"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="rounded border border-slate-600 bg-slate-900 p-2"
              />
            </div>
            <p className="text-slate-400">
              Órfãos são verificados em chaves estrangeiras simples visíveis ao
              papel de leitura. Resultados de amostra são hipóteses para
              confirmação na população.
            </p>
            {api.hasRole("auditor") ? (
              <Button
                disabled={
                  busy ||
                  !database.trim() ||
                  !schema.trim() ||
                  !table.trim() ||
                  limit < 1 ||
                  limit > 1000 ||
                  (Boolean(dateColumn) && (!dateFrom || !dateTo))
                }
                onClick={() => void startScan()}
              >
                Executar diagnóstico de leitura
              </Button>
            ) : (
              <p>
                Seu papel permite consultar resultados; somente auditores podem
                iniciar diagnóstico.
              </p>
            )}
          </div>
        )}
      </Card>
    </>
  );
}
