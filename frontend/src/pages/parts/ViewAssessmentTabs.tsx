import {
  InventoryDetailAvailability,
  InventoryDetailTabs,
} from "../../components/InventoryDetailTabs";
import { DetailGrid, DetailSection } from "../../components/ui/Sheet";
import { formatBytes } from "../../lib/format";
import { inventoryPermalink } from "../../lib/inventoryTarget";
import { ViewDetail } from "../InventoryDetails";
import {
  collectionState,
  pageControls,
  type ViewAssessmentState,
} from "./useViewAssessment";

/** Tab bodies of ViewAssessmentPanel; same markup, moved out (UI-06). */
export function ViewAssessmentTabs({ s }: { s: ViewAssessmentState }) {
  const {
    dependencies,
    detail,
    detailState,
    environment,
    findings,
    grants,
    materialized,
    run,
    setTab,
    tab,
    view,
  } = s;
  return (
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
  );
}
