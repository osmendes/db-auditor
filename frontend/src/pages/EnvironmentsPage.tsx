import {
  Badge,
  Button,
  EmptyState,
  ErrorBanner,
  Input,
  Skeleton,
  Table,
} from "../components/ui";
import { connectionPresentation } from "../lib/connectionStatus";
import { formatError } from "../lib/errors";
import { formatBytes } from "../lib/format";
import { labels } from "../lib/labels";
import { nextSort } from "../lib/sort";
import { api } from "../services/api";
import {
  type EnvironmentsPageProps,
  useEnvironmentsPage,
} from "./parts/useEnvironmentsPage";

export type { EnvironmentsPageProps };

export function EnvironmentsPage({ onNavigate }: EnvironmentsPageProps) {
  const s = useEnvironmentsPage({ onNavigate });
  const {
    connectionById,
    connections,
    dbSort,
    detailError,
    envSort,
    error,
    filter,
    filteredDatabases,
    filteredSchemas,
    labelError,
    labelName,
    schemaSort,
    selectedId,
    setDbSort,
    setEnvSort,
    setFilter,
    setLabelError,
    setLabelName,
    setReloadKey,
    setSchemaSort,
    setSelectedId,
    sortedEnvironments,
  } = s;
  return (
    <>
      <p className="text-xs font-medium text-slate-400">Ambientes</p>
      <h1 className="mt-2 text-3xl font-semibold text-slate-50 md:text-4xl">
        Ambientes
      </h1>
      <p className="mt-3 max-w-3xl text-sm leading-relaxed text-slate-400">
        O rótulo nasce aqui. A conexão continua no segredo do servidor e não é
        digitada nem devolvida por esta tela. A coleta só ocorre quando o slot
        do ambiente corresponde a esse rótulo.
      </p>
      {api.hasRole("operator") ? (
        <form
          className="mt-4 flex max-w-xl flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            const name = labelName.trim();
            if (!name) return;
            setLabelError(null);
            void api
              .createEnvironmentLabel(name)
              .then(() => {
                setLabelName("");
                setReloadKey((k) => k + 1);
              })
              .catch((err: unknown) =>
                setLabelError(
                  formatError(err, "Não foi possível criar o rótulo"),
                ),
              );
          }}
        >
          <Input
            label="Novo ambiente"
            value={labelName}
            placeholder="Nome do ambiente"
            onChange={(event) => setLabelName(event.target.value)}
          />
          <Button type="submit">Criar rótulo</Button>
        </form>
      ) : null}
      {labelError ? (
        <p className="mt-2 text-sm text-rose-300">{labelError}</p>
      ) : null}

      <div className="mt-6 space-y-6">
        {error ? (
          <ErrorBanner
            message={error}
            onRetry={() => setReloadKey((k) => k + 1)}
          />
        ) : sortedEnvironments === null ? (
          <Skeleton className="h-32 w-full" />
        ) : sortedEnvironments.length === 0 ? (
          <EmptyState
            title="Nenhum ambiente registrado"
            description="Configure as connection strings no .env do backend, reinicie a API e execute a discovery. O guia passo a passo está em Documentação."
            action={
              onNavigate ? (
                <Button
                  type="button"
                  onClick={() => onNavigate("Documentação")}
                >
                  Abrir documentação
                </Button>
              ) : null
            }
          />
        ) : (
          <Table
            headers={[
              { id: "name", label: "Nome", sortable: true },
              { id: "type", label: "Tipo", sortable: true },
              { id: "discovery", label: "Discovery", sortable: true },
              { id: "status", label: "Status", sortable: true },
            ]}
            sortKey={envSort?.key}
            sortDir={envSort?.dir}
            onSort={(id) => setEnvSort((prev) => nextSort(prev, id))}
          >
            {sortedEnvironments.map((env) => {
              const status = connectionPresentation(
                connections === null
                  ? null
                  : (connectionById.get(env.id) ?? null),
              );
              const label =
                connections === null ? "Verificando…" : status.label;
              return (
                <tr
                  key={env.id}
                  className={`cursor-pointer border-t border-slate-800 ${
                    selectedId === env.id ? "bg-slate-900/80" : ""
                  }`}
                  onClick={() => setSelectedId(env.id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      setSelectedId(env.id);
                    }
                  }}
                  tabIndex={0}
                >
                  <td className="px-4 py-3 text-slate-100">{env.name}</td>
                  <td className="px-4 py-3 text-slate-300">
                    {labels.envType(env.type)}
                  </td>
                  <td className="px-4 py-3 text-slate-300">
                    {labels.discoveryMode(env.discovery_mode)}
                  </td>
                  <td className="px-4 py-3">
                    <Badge
                      tone={connections === null ? "neutral" : status.tone}
                    >
                      {label}
                    </Badge>
                  </td>
                </tr>
              );
            })}
          </Table>
        )}

        {selectedId ? (
          <section className="space-y-4">
            <h2 className="text-xl font-semibold text-slate-100">Topologia</h2>
            <div className="w-full max-w-md">
              <Input
                label="Filtrar databases e schemas"
                placeholder="Buscar…"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
              />
            </div>

            {detailError ? <ErrorBanner message={detailError} /> : null}

            <div className="grid items-stretch gap-6 lg:grid-cols-2">
              <div className="flex h-full flex-col">
                <h3 className="mb-2 text-sm font-medium text-slate-300">
                  Databases
                </h3>
                <div className="flex flex-1 flex-col">
                  {filteredDatabases === null ? (
                    <Skeleton className="h-40 w-full" />
                  ) : filteredDatabases.length === 0 ? (
                    <EmptyState
                      className="flex h-full flex-1 flex-col justify-center"
                      title="Nenhum database"
                      description="Sem snapshots ainda ou nenhum resultado para o filtro. Execute uma coleta em Execuções."
                    />
                  ) : (
                    <Table
                      headers={[
                        { id: "name", label: "Nome", sortable: true },
                        { id: "size", label: "Tamanho", sortable: true },
                        {
                          id: "connections",
                          label: "Conexões",
                          sortable: true,
                        },
                      ]}
                      sortKey={dbSort?.key}
                      sortDir={dbSort?.dir}
                      onSort={(id) => setDbSort((prev) => nextSort(prev, id))}
                    >
                      {filteredDatabases.map((db) => (
                        <tr key={db.id} className="border-t border-slate-800">
                          <td className="px-4 py-3 text-slate-100">
                            {db.database_name}
                          </td>
                          <td className="px-4 py-3 text-slate-300">
                            {formatBytes(db.size_bytes)}
                          </td>
                          <td className="px-4 py-3 text-slate-300">
                            {db.connection_count}
                          </td>
                        </tr>
                      ))}
                    </Table>
                  )}
                </div>
              </div>

              <div className="flex h-full flex-col">
                <h3 className="mb-2 text-sm font-medium text-slate-300">
                  Schemas
                </h3>
                <div className="flex flex-1 flex-col">
                  {filteredSchemas === null ? (
                    <Skeleton className="h-40 w-full" />
                  ) : filteredSchemas.length === 0 ? (
                    <EmptyState
                      className="flex h-full flex-1 flex-col justify-center"
                      title="Nenhum schema"
                      description="Sem snapshots ou nenhum resultado para o filtro."
                    />
                  ) : (
                    <Table
                      headers={[
                        { id: "database", label: "Database", sortable: true },
                        { id: "schema", label: "Schema", sortable: true },
                        { id: "tables", label: "Tables", sortable: true },
                        { id: "views", label: "Views", sortable: true },
                        { id: "size", label: "Tamanho", sortable: true },
                      ]}
                      sortKey={schemaSort?.key}
                      sortDir={schemaSort?.dir}
                      onSort={(id) =>
                        setSchemaSort((prev) => nextSort(prev, id))
                      }
                    >
                      {filteredSchemas.map((sc) => (
                        <tr key={sc.id} className="border-t border-slate-800">
                          <td className="px-4 py-3 text-slate-300">
                            {sc.database_name}
                          </td>
                          <td className="px-4 py-3 text-slate-100">
                            {sc.schema_name}
                          </td>
                          <td className="px-4 py-3 text-slate-300">
                            {sc.table_count}
                          </td>
                          <td className="px-4 py-3 text-slate-300">
                            {sc.view_count + sc.materialized_view_count}
                          </td>
                          <td className="px-4 py-3 text-slate-300">
                            {formatBytes(sc.size_bytes)}
                          </td>
                        </tr>
                      ))}
                    </Table>
                  )}
                </div>
              </div>
            </div>
          </section>
        ) : null}
      </div>
    </>
  );
}
