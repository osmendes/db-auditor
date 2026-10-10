import { useEffect, useRef, useState } from "react";
import {
  InventoryDetailAvailability,
  type InventoryDetailTab,
  InventoryDetailTabs,
} from "../components/InventoryDetailTabs";
import { PaginationControls } from "../components/ui/PaginationControls";
import { DetailGrid, DetailSection } from "../components/ui/Sheet";
import { ApiError, formatError } from "../lib/errors";
import { formatBytes } from "../lib/format";
import { inventoryPermalink } from "../lib/inventoryTarget";
import { api } from "../services/api";
import type {
  DependencySnapshot,
  Finding,
  GrantSnapshot,
  PagedResponse,
  SnapshotCompleteness,
  ViewDetailSnapshot,
  ViewSnapshot,
} from "../types";
import { ViewDetail } from "./InventoryDetails";

function useViewCollection<T>(
  active: boolean,
  scope: string,
  load: (offset: number) => Promise<PagedResponse<T>>,
) {
  const loader = useRef(load);
  loader.current = load;
  const [offset, setOffset] = useState(0);
  const [data, setData] = useState<PagedResponse<T> | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    setOffset(0);
    setData(null);
    setError(null);
  }, [scope]);
  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    void loader
      .current(offset)
      .then((result) => {
        if (!cancelled) setData(result);
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setData(null);
          setError(cause);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [active, scope, offset]);
  return { data, error, loading, offset, setOffset };
}

function collectionState(error: unknown, loading: boolean, hasData: boolean) {
  if (loading || (!hasData && !error)) return <p role="status">Carregando…</p>;
  if (
    error instanceof ApiError &&
    (error.status === 401 || error.status === 403)
  )
    return <InventoryDetailAvailability state="forbidden" />;
  if (error)
    return (
      <InventoryDetailAvailability state="error" detail={formatError(error)} />
    );
  return null;
}

function pageControls(
  total: number,
  offset: number,
  setOffset: (value: number) => void,
) {
  if (total <= 50) return null;
  return (
    <PaginationControls
      total={total}
      offset={offset}
      size={50}
      allowAll={false}
      onOffsetChange={setOffset}
      onSizeChange={() => setOffset(0)}
      label="Detalhe da visão"
    />
  );
}

export function ViewAssessmentPanel({
  view,
  environment,
  run,
  snapshot,
}: {
  view: ViewSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<ViewDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const scope = JSON.stringify([
    environment,
    run,
    view.database_name,
    view.schema_name,
    view.view_name,
  ]);
  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .viewDetail(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
      )
      .then((item) => {
        if (!cancelled) setDetail(item);
      })
      .catch((cause: unknown) => {
        if (!cancelled) setDetailError(cause);
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [scope]);
  const dependencies = useViewCollection<DependencySnapshot>(
    tab === "relationships",
    scope,
    (offset) =>
      api.viewDependencies(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
        50,
        offset,
      ),
  );
  const grants = useViewCollection<GrantSnapshot>(
    tab === "security",
    scope,
    (offset) =>
      api.viewGrants(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
        50,
        offset,
      ),
  );
  const findings = useViewCollection<Finding>(
    tab === "recommendations",
    scope,
    (offset) =>
      api.viewFindings(
        environment,
        run,
        view.database_name,
        view.schema_name,
        view.view_name,
        50,
        offset,
      ),
  );
  const target = {
    kind: "views" as const,
    database: view.database_name,
    schema: view.schema_name,
    name: view.view_name,
  };
  const materialized = view.relkind === "m";
  const detailState = collectionState(
    detailError,
    detailLoading,
    detail !== null,
  );
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para esta visão e execução
      </a>
      {snapshot?.completeness === "partial" ? (
        <InventoryDetailAvailability state="partial" />
      ) : null}
      {snapshot?.completeness === "empty" ? (
        <InventoryDetailAvailability state="empty" />
      ) : null}
      <InventoryDetailTabs tab={tab} onTabChange={setTab}>
        {tab === "overview" && (
          <>
            <ViewDetail v={view} />
            <DetailSection title="Interpretação">
              <p className="text-sm text-slate-300">
                {materialized
                  ? "Visão materializada: ocupa espaço próprio. A coleta não informa quando ocorreu o último REFRESH."
                  : "Visão comum: consulta dados de outros objetos. Ela não possui tamanho de dados próprio."}
              </p>
            </DetailSection>
          </>
        )}
        {tab === "structure" &&
          (detailState || (
            <>
              <DetailSection title="Colunas">
                {detail?.columns === null ? (
                  <InventoryDetailAvailability state="not_collected" />
                ) : detail?.columns?.length ? (
                  <ul className="space-y-1 text-sm text-slate-300">
                    {detail.columns.map((column) => (
                      <li key={column.position}>
                        {column.position}. {column.name} — {column.type}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability state="empty" />
                )}
              </DetailSection>
              <DetailSection title="Definição protegida">
                <p className="text-sm text-slate-300">
                  O SQL da visão pode conter literais sensíveis e não é exibido.{" "}
                  {detail?.definition_fingerprint
                    ? `Impressão digital SHA-256: ${detail.definition_fingerprint}`
                    : "A definição não foi coletada."}
                </p>
              </DetailSection>
            </>
          ))}
        {tab === "relationships" &&
          (collectionState(
            dependencies.error,
            dependencies.loading,
            dependencies.data !== null,
          ) || (
            <DetailSection title="Dependências observadas">
              {dependencies.data?.items.length ? (
                <ul className="space-y-1 text-sm text-slate-300">
                  {dependencies.data.items.map((item) => (
                    <li
                      key={`${item.target_schema}.${item.target_name}.${item.target_kind}`}
                    >
                      <a
                        className="text-cyan-300 underline"
                        href={inventoryPermalink(environment, run, {
                          kind:
                            item.target_kind === "view" ||
                            item.target_kind === "materialized_view"
                              ? "views"
                              : "tables",
                          database: view.database_name,
                          schema: item.target_schema,
                          name: item.target_name,
                        })}
                      >
                        {item.target_schema}.{item.target_name}
                      </a>{" "}
                      ({item.target_kind})
                    </li>
                  ))}
                </ul>
              ) : (
                <InventoryDetailAvailability
                  state="empty"
                  detail="Nenhuma dependência visível foi registrada nesta execução."
                />
              )}
              {pageControls(
                dependencies.data?.page.total ?? 0,
                dependencies.offset,
                dependencies.setOffset,
              )}
            </DetailSection>
          ))}
        {tab === "performance" && (
          <DetailSection title="Performance observável">
            {materialized ? (
              <>
                <DetailGrid
                  items={[
                    {
                      label: "Espaço ocupado",
                      value: formatBytes(view.size_bytes),
                    },
                  ]}
                />
                {detailState || (
                  <DetailGrid
                    items={[
                      {
                        label: "Populada na coleta",
                        value:
                          detail?.is_populated === null ||
                          detail?.is_populated === undefined
                            ? "Não coletado"
                            : detail.is_populated
                              ? "Sim"
                              : "Não",
                      },
                    ]}
                  />
                )}
              </>
            ) : (
              <InventoryDetailAvailability
                state="not_applicable"
                detail="Visões comuns não armazenam dados próprios; esta coleta não mede seu custo de execução."
              />
            )}
            {materialized && (
              <InventoryDetailAvailability
                state="not_collected"
                detail="Último REFRESH e custo das consultas não são coletados para esta visão."
              />
            )}
          </DetailSection>
        )}
        {tab === "security" &&
          (detailState || (
            <>
              <DetailSection title="Opções de segurança">
                <DetailGrid
                  items={[
                    {
                      label: "Responsável",
                      value: detail?.owner_name ?? "Não coletado",
                    },
                    {
                      label: "Usa permissões do invocador",
                      value:
                        detail?.security_invoker === null ||
                        detail?.security_invoker === undefined
                          ? "Não aplicável ou não coletado"
                          : detail.security_invoker
                            ? "Sim"
                            : "Não",
                    },
                    {
                      label: "Barreira de segurança",
                      value:
                        detail?.security_barrier === null ||
                        detail?.security_barrier === undefined
                          ? "Não aplicável ou não coletado"
                          : detail.security_barrier
                            ? "Sim"
                            : "Não",
                    },
                  ]}
                />
              </DetailSection>
              <DetailSection title="Acessos efetivos observados">
                {collectionState(
                  grants.error,
                  grants.loading,
                  grants.data !== null,
                ) ||
                  (grants.data?.items.length ? (
                    <ul className="space-y-1 text-sm text-slate-300">
                      {grants.data.items.map((item) => (
                        <li key={item.grantee}>
                          {item.grantee}: {item.privileges.join(", ")}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <InventoryDetailAvailability
                      state="empty"
                      detail="Nenhum acesso de papel com login foi registrado nesta execução."
                    />
                  ))}
                {pageControls(
                  grants.data?.page.total ?? 0,
                  grants.offset,
                  grants.setOffset,
                )}
              </DetailSection>
            </>
          ))}
        {tab === "recommendations" &&
          (collectionState(
            findings.error,
            findings.loading,
            findings.data !== null,
          ) || (
            <DetailSection title="Achados deste objeto e execução">
              {findings.data?.items.length ? (
                <ul className="space-y-3">
                  {findings.data.items.map((item) => (
                    <li
                      key={item.id}
                      className="rounded border border-slate-700 p-3 text-sm"
                    >
                      <strong>{item.title}</strong>
                      <p>{item.friendly_meaning || item.summary}</p>
                      {item.friendly_next && (
                        <p>Próximo passo: {item.friendly_next}</p>
                      )}
                    </li>
                  ))}
                </ul>
              ) : (
                <InventoryDetailAvailability
                  state="empty"
                  detail="Nenhum achado desta visão foi observado nesta execução. Isso não comprova ausência de risco."
                />
              )}
              {pageControls(
                findings.data?.page.total ?? 0,
                findings.offset,
                findings.setOffset,
              )}
            </DetailSection>
          ))}
      </InventoryDetailTabs>
    </div>
  );
}
