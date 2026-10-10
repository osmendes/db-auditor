import { useEffect, useState } from "react";
import {
  inventoryDetailPageControls,
  inventoryDetailState,
  useInventoryDetailPage,
} from "../components/InventoryDetailData";
import {
  InventoryDetailAvailability,
  type InventoryDetailTab,
  InventoryDetailTabs,
} from "../components/InventoryDetailTabs";
import { DetailGrid, DetailSection } from "../components/ui/Sheet";
import { inventoryPermalink } from "../lib/inventoryTarget";
import { api } from "../services/api";
import type {
  DependencySnapshot,
  Finding,
  FunctionSnapshot,
  GrantSnapshot,
  SnapshotCompleteness,
} from "../types";
import type { FunctionDetailSnapshot } from "../types/openapi";

function functionKind(kind: string | null | undefined): string {
  switch (kind) {
    case "f":
      return "Função";
    case "p":
      return "Procedimento";
    case "a":
      return "Agregação";
    case "w":
      return "Função de janela";
    default:
      return "Não coletado";
  }
}

function volatilityLabel(value: string | null | undefined): string {
  return value === "i"
    ? "Imutável"
    : value === "s"
      ? "Estável"
      : value === "v"
        ? "Volátil"
        : "Não coletado";
}

export function FunctionAssessmentPanel({
  fn,
  environment,
  run,
  snapshot,
}: {
  fn: FunctionSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<FunctionDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const scope = JSON.stringify([
    environment,
    run,
    fn.database_name,
    fn.schema_name,
    fn.function_name,
    fn.identity_arguments,
  ]);
  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .functionDetail(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
      )
      .then((value) => {
        if (!cancelled) setDetail(value);
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
  const dependencies = useInventoryDetailPage<DependencySnapshot>(
    tab === "relationships",
    scope,
    (offset) =>
      api.functionDependencies(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
        50,
        offset,
      ),
  );
  const grants = useInventoryDetailPage<GrantSnapshot>(
    tab === "security",
    scope,
    (offset) =>
      api.functionGrants(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
        50,
        offset,
      ),
  );
  const findings = useInventoryDetailPage<Finding>(
    tab === "recommendations",
    scope,
    (offset) =>
      api.functionFindings(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
        50,
        offset,
      ),
  );
  const detailState = inventoryDetailState(
    detailError,
    detailLoading,
    detail !== null,
  );
  const target = {
    kind: "functions" as const,
    database: fn.database_name,
    schema: fn.schema_name,
    name: fn.function_name,
    signature: fn.identity_arguments,
  };
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para esta sobrecarga e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <InventoryDetailTabs tab={tab} onTabChange={setTab}>
        {tab === "overview" && (
          <DetailSection title="Função selecionada">
            <DetailGrid
              items={[
                { label: "Banco", value: fn.database_name },
                { label: "Esquema", value: fn.schema_name },
                { label: "Nome", value: fn.function_name },
                {
                  label: "Assinatura",
                  value: fn.identity_arguments || "Sem argumentos",
                },
                { label: "Tipo", value: functionKind(fn.kind) },
                {
                  label: "Linguagem",
                  value: fn.language_name ?? "Não coletado",
                },
                {
                  label: "Responsável",
                  value: fn.owner_name ?? "Não coletado",
                },
                {
                  label: "Executa com privilégios do proprietário",
                  value: fn.is_security_definer ? "Sim" : "Não",
                },
              ]}
            />
            <p className="text-sm text-slate-300">
              Funções de mesmo nome podem ter argumentos diferentes. Esta
              análise usa a assinatura para separar as sobrecargas.
            </p>
          </DetailSection>
        )}
        {tab === "structure" &&
          (detailState || (
            <>
              <DetailSection title="Estrutura e comportamento">
                <DetailGrid
                  items={[
                    {
                      label: "Assinatura",
                      value: detail?.identity_arguments || "Sem argumentos",
                    },
                    {
                      label: "Retorno",
                      value:
                        detail?.return_type ?? "Não coletado ou não aplicável",
                    },
                    {
                      label: "Linguagem",
                      value: detail?.language_name ?? "Não coletado",
                    },
                    {
                      label: "Volatilidade",
                      value: volatilityLabel(detail?.volatility),
                    },
                    {
                      label: "Paralelismo",
                      value: detail?.parallel_safety ?? "Não coletado",
                    },
                  ]}
                />
              </DetailSection>
              <DetailSection title="Código protegido">
                <p className="text-sm text-slate-300">
                  O corpo SQL e as configurações brutas não são exibidos porque
                  podem conter dados sensíveis. Impressão digital da definição:{" "}
                  {detail?.definition_fingerprint || "Não coletada"}.
                </p>
              </DetailSection>
            </>
          ))}
        {tab === "relationships" &&
          (inventoryDetailState(
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
                      {item.target_schema}.{item.target_name} (
                      {item.target_kind})
                    </li>
                  ))}
                </ul>
              ) : (
                <InventoryDetailAvailability
                  state="empty"
                  detail="Nenhuma dependência catalogada nesta execução. Referências feitas por SQL dinâmico podem não aparecer."
                />
              )}
              {inventoryDetailPageControls(
                dependencies.data?.page.total ?? 0,
                dependencies.offset,
                dependencies.setOffset,
                "Dependências da função",
              )}
            </DetailSection>
          ))}
        {tab === "performance" &&
          (detailState || (
            <DetailSection title="Estatísticas agregadas">
              {detail?.stats_observed === null ||
              detail?.stats_observed === undefined ? (
                <InventoryDetailAvailability state="not_collected" />
              ) : detail.stats_observed ? (
                <>
                  <DetailGrid
                    items={[
                      {
                        label: "Chamadas acumuladas",
                        value: (detail.calls ?? 0).toLocaleString("pt-BR"),
                      },
                      {
                        label: "Tempo total acumulado",
                        value: `${(detail.total_time_ms ?? 0).toLocaleString("pt-BR")} ms`,
                      },
                      {
                        label: "Tempo interno acumulado",
                        value: `${(detail.self_time_ms ?? 0).toLocaleString("pt-BR")} ms`,
                      },
                      {
                        label: "Reinício das estatísticas",
                        value: detail.stats_reset
                          ? new Date(detail.stats_reset).toLocaleString("pt-BR")
                          : "Não informado",
                      },
                    ]}
                  />
                  <p className="text-sm text-slate-300">
                    São contadores acumulados até a coleta; não representam
                    duração de uma chamada específica.
                  </p>
                </>
              ) : (
                <InventoryDetailAvailability
                  state="not_collected"
                  detail="O PostgreSQL não forneceu estatísticas desta função. Verifique se o rastreamento de funções estava habilitado antes de comparar desempenho."
                />
              )}
            </DetailSection>
          ))}
        {tab === "security" &&
          (detailState || (
            <>
              <DetailSection title="Contexto de execução">
                <DetailGrid
                  items={[
                    {
                      label: "Responsável",
                      value: detail?.owner_name ?? "Não coletado",
                    },
                    {
                      label: "Privilégios do proprietário",
                      value: detail?.is_security_definer ? "Sim" : "Não",
                    },
                    {
                      label: "Caminho de busca fixado",
                      value:
                        detail?.search_path_pinned === null ||
                        detail?.search_path_pinned === undefined
                          ? "Não coletado"
                          : detail.search_path_pinned
                            ? "Sim"
                            : "Não",
                    },
                    {
                      label: "Papéis com EXECUTE observado",
                      value:
                        detail?.execute_role_count === null ||
                        detail?.execute_role_count === undefined
                          ? "Não coletado"
                          : detail.execute_role_count.toLocaleString("pt-BR"),
                    },
                  ]}
                />
                <p className="text-sm text-slate-300">
                  SECURITY DEFINER merece revisão, mas a opção sozinha não
                  comprova vulnerabilidade. Confirme o corpo, o caminho de busca
                  e a necessidade de EXECUTE com um administrador.
                </p>
              </DetailSection>
              <DetailSection title="Acessos efetivos observados">
                {detail?.execute_role_count === null ||
                detail?.execute_role_count === undefined ? (
                  <InventoryDetailAvailability state="not_collected" />
                ) : (
                  inventoryDetailState(
                    grants.error,
                    grants.loading,
                    grants.data !== null,
                  ) ||
                  (grants.data?.items.length ? (
                    <ul className="space-y-1 text-sm text-slate-300">
                      {grants.data.items.map((item) => (
                        <li key={item.grantee}>{item.grantee}: EXECUTE</li>
                      ))}
                    </ul>
                  ) : (
                    <InventoryDetailAvailability
                      state="empty"
                      detail="Nenhum papel com login e acesso ao esquema tinha EXECUTE observado nesta coleta."
                    />
                  ))
                )}
                {inventoryDetailPageControls(
                  grants.data?.page.total ?? 0,
                  grants.offset,
                  grants.setOffset,
                  "Acessos à função",
                )}
              </DetailSection>
            </>
          ))}
        {tab === "recommendations" && (
          <>
            {detail?.is_security_definer &&
              detail.search_path_pinned === false && (
                <InventoryDetailAvailability
                  state="partial"
                  detail="Revise o caminho de busca desta função com privilégios do proprietário. Não altere permissões sem validar dependências e plano de retorno."
                />
              )}
            {inventoryDetailState(
              findings.error,
              findings.loading,
              findings.data !== null,
            ) || (
              <DetailSection title="Achados desta sobrecarga e execução">
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
                    detail="Nenhum achado desta sobrecarga foi observado nesta execução. Isso não comprova ausência de risco."
                  />
                )}
                {inventoryDetailPageControls(
                  findings.data?.page.total ?? 0,
                  findings.offset,
                  findings.setOffset,
                  "Achados da função",
                )}
              </DetailSection>
            )}
          </>
        )}
      </InventoryDetailTabs>
    </div>
  );
}
