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
import { formatBytes } from "../lib/format";
import { inventoryPermalink } from "../lib/inventoryTarget";
import { api } from "../services/api";
import type {
  CAGGHistoryPoint,
  CAGGRefreshPolicy,
  CAGGSnapshot,
  DependencySnapshot,
  Finding,
  GrantSnapshot,
  SnapshotCompleteness,
} from "../types";
import type { CAGGDetailSnapshot } from "../types/openapi";

function date(value: string | null | undefined): string {
  return value ? new Date(value).toLocaleString("pt-BR") : "Não informado";
}

function refreshStatus(value: string | null): string {
  if (!value) return "Não coletado";
  if (value.toLowerCase() === "success") return "Concluído";
  if (value.toLowerCase() === "failed") return "Falhou";
  return value;
}

export function CAGGAssessmentPanel({
  cagg,
  environment,
  run,
  snapshot,
}: {
  cagg: CAGGSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<CAGGDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const {
    database_name: database,
    schema_name: schema,
    view_name: name,
  } = cagg;
  const scope = JSON.stringify([environment, run, database, schema, name]);
  const args = [environment, run, database, schema, name] as const;
  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .caggDetail(...args)
      .then((item) => {
        if (!cancelled) setDetail(item);
      })
      .catch((error: unknown) => {
        if (!cancelled) setDetailError(error);
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [scope]);
  const policies = useInventoryDetailPage<CAGGRefreshPolicy>(
    tab === "performance",
    scope,
    (offset) => api.caggRefreshPolicies(...args, 50, offset),
  );
  const history = useInventoryDetailPage<CAGGHistoryPoint>(
    tab === "performance",
    scope,
    (offset) => api.caggHistory(...args, 50, offset),
  );
  const dependencies = useInventoryDetailPage<DependencySnapshot>(
    tab === "relationships",
    scope,
    (offset) => api.caggDependencies(...args, 50, offset),
  );
  const grants = useInventoryDetailPage<GrantSnapshot>(
    tab === "security",
    scope,
    (offset) => api.caggGrants(...args, 50, offset),
  );
  const findings = useInventoryDetailPage<Finding>(
    tab === "recommendations",
    scope,
    (offset) => api.caggFindings(...args, 50, offset),
  );
  const detailState = inventoryDetailState(
    detailError,
    detailLoading,
    detail !== null,
  );
  const sectionState = (page: {
    error: unknown;
    loading: boolean;
    data: unknown;
  }) => inventoryDetailState(page.error, page.loading, page.data !== null);
  const controls = (
    page: {
      data: { page: { total: number } } | null;
      offset: number;
      setOffset: (value: number) => void;
    },
    label: string,
  ) =>
    inventoryDetailPageControls(
      page.data?.page.total ?? 0,
      page.offset,
      page.setOffset,
      label,
    );
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, {
          kind: "caggs",
          database,
          schema,
          name,
        })}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para este agregado e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <InventoryDetailTabs tab={tab} onTabChange={setTab}>
        {tab === "overview" && (
          <DetailSection title="Agregado contínuo selecionado">
            <DetailGrid
              items={[
                { label: "Banco", value: database },
                { label: "Esquema", value: schema },
                { label: "Nome", value: name },
                {
                  label: "Responsável",
                  value: cagg.owner_name ?? "Não coletado",
                },
                {
                  label: "Apenas dados materializados",
                  value: cagg.materialized_only ? "Sim" : "Não",
                },
                {
                  label: "Compressão ou columnstore",
                  value: cagg.compression_enabled
                    ? "Habilitada"
                    : "Não habilitada",
                },
                { label: "Coletado em", value: date(cagg.collected_at) },
              ]}
            />
            <p className="text-sm text-slate-300">
              O agregado é um objeto próprio do TimescaleDB. A cobertura de sua
              origem e da materialização é mostrada separadamente.
            </p>
          </DetailSection>
        )}
        {tab === "structure" &&
          (detailState || (
            <>
              <DetailSection title="Definição protegida">
                <p className="text-sm text-slate-300">
                  O SQL completo não é exibido porque pode conter valores
                  sensíveis. Impressão digital:{" "}
                  {detail?.definition_fingerprint || "Não coletada"}.
                </p>
                <DetailGrid
                  items={[
                    {
                      label: "Esquema da origem",
                      value: detail?.source_hypertable_schema ?? "Não coletado",
                    },
                    {
                      label: "Tabela temporal de origem",
                      value: detail?.source_hypertable_name ?? "Não coletada",
                    },
                    {
                      label: "Intervalo do bucket",
                      value: detail?.bucket_interval ?? "Não coletado",
                    },
                  ]}
                />
                {detail?.source_hypertable_schema &&
                  detail.source_hypertable_name && (
                    <a
                      className="text-cyan-300 underline"
                      href={inventoryPermalink(environment, run, {
                        kind: "hypertables",
                        database,
                        schema: detail.source_hypertable_schema,
                        name: detail.source_hypertable_name,
                      })}
                    >
                      Abrir a tabela temporal de origem nesta execução
                    </a>
                  )}
                <p className="text-sm text-slate-300">
                  O intervalo do bucket só aparece quando o catálogo o fornece;
                  a coluna temporal usada no SQL deve ser confirmada no banco
                  com acesso autorizado.
                </p>
              </DetailSection>
              <DetailSection title="Materialização">
                <DetailGrid
                  items={[
                    {
                      label: "Esquema da materialização",
                      value: detail?.materialization_schema ?? "Não coletado",
                    },
                    {
                      label: "Tabela de materialização",
                      value:
                        detail?.materialization_hypertable ?? "Não coletada",
                    },
                    {
                      label: "Finalizado",
                      value:
                        detail?.finalized == null
                          ? "Não coletado"
                          : detail.finalized
                            ? "Sim"
                            : "Não",
                    },
                  ]}
                />
                {detail?.materialization_schema &&
                  detail.materialization_hypertable && (
                    <a
                      className="text-cyan-300 underline"
                      href={inventoryPermalink(environment, run, {
                        kind: "hypertables",
                        database,
                        schema: detail.materialization_schema,
                        name: detail.materialization_hypertable,
                      })}
                    >
                      Abrir a tabela de materialização nesta execução
                    </a>
                  )}
              </DetailSection>
            </>
          ))}
        {tab === "relationships" && (
          <DetailSection title="Dependências observadas">
            {sectionState(dependencies) ||
              (dependencies.data?.items.length ? (
                <ul className="space-y-2 text-sm text-slate-300">
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
                  detail="Nenhuma dependência foi catalogada. A origem pode aparecer apenas na definição SQL protegida."
                />
              ))}
            {controls(dependencies, "Dependências")}
          </DetailSection>
        )}
        {tab === "performance" && (
          <>
            <DetailSection title="Atualização e atraso">
              {detailState || (
                <DetailGrid
                  items={[
                    {
                      label: "Atraso observado",
                      value: detail?.lag_interval || "Não coletado",
                    },
                    {
                      label: "Espaço da materialização",
                      value:
                        detail?.materialization_size_bytes == null
                          ? "Não coletado"
                          : formatBytes(detail.materialization_size_bytes),
                    },
                  ]}
                />
              )}
              <p className="text-sm text-slate-300">
                Atraso é uma estimativa da posição dos dados materializados na
                coleta; não mede o tempo de uma consulta nem confirma a última
                atualização.
              </p>
            </DetailSection>
            <DetailSection title="Políticas de atualização">
              {sectionState(policies) ||
                (policies.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {policies.data.items.map((item) => (
                      <li key={item.job_id}>
                        Job {item.job_id}:{" "}
                        {item.scheduled ? "agendado" : "desativado"}; intervalo{" "}
                        {item.schedule_interval ?? "não coletado"}; último
                        resultado {refreshStatus(item.last_run_status)}; falhas{" "}
                        {item.total_failures ?? "não coletadas"}; próxima
                        execução {date(item.next_start)}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability
                    state="empty"
                    detail="Nenhuma política de atualização foi observada. Confirme se o agregado é atualizado manualmente antes de propor uma política."
                  />
                ))}
              {controls(policies, "Políticas de atualização")}
            </DetailSection>
            <DetailSection title="Evolução por execução">
              {sectionState(history) ||
                (history.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {history.data.items.map((item) => (
                      <li key={item.audit_run_id}>
                        {date(item.collected_at)}: materialização{" "}
                        {item.materialization_size_bytes == null
                          ? "não coletada"
                          : formatBytes(item.materialization_size_bytes)}
                        ; atraso {item.lag_interval || "não coletado"}
                        {item.run_status === "partial_success"
                          ? " — coleta parcial, comparação limitada"
                          : ""}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability state="empty" />
                ))}
              {controls(history, "Histórico")}
            </DetailSection>
          </>
        )}
        {tab === "security" && (
          <DetailSection title="Proprietário e acessos à visão do agregado">
            <DetailGrid
              items={[
                {
                  label: "Responsável",
                  value: cagg.owner_name ?? "Não coletado",
                },
              ]}
            />
            {sectionState(grants) ||
              (grants.data?.items.length ? (
                <ul className="text-sm text-slate-300">
                  {grants.data.items.map((item) => (
                    <li key={item.grantee}>
                      {item.grantee}: {item.privileges.join(", ")}
                    </li>
                  ))}
                </ul>
              ) : (
                <InventoryDetailAvailability
                  state="not_collected"
                  detail="Nenhum acesso da visão foi registrado nesta execução. Isso não confirma ausência de acesso efetivo."
                />
              ))}
            {controls(grants, "Acessos")}
            <p className="text-sm text-slate-300">
              Estes acessos dizem respeito à visão do agregado; permissões da
              hypertable de origem e da materialização exigem análise separada.
            </p>
          </DetailSection>
        )}
        {tab === "recommendations" && (
          <DetailSection title="Achados deste agregado e execução">
            {sectionState(findings) ||
              (findings.data?.items.length ? (
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
                      {item.confidence !== undefined && (
                        <p>Confiança: {Math.round(item.confidence * 100)}%</p>
                      )}
                    </li>
                  ))}
                </ul>
              ) : (
                <InventoryDetailAvailability
                  state="empty"
                  detail="Nenhum achado específico deste agregado foi registrado nesta execução."
                />
              ))}
            {controls(findings, "Achados")}
            <p className="text-sm text-slate-300">
              Confirme atraso, política e carga real com consultas somente
              leitura antes de alterar o banco. Uma hipótese não representa uma
              falha comprovada.
            </p>
          </DetailSection>
        )}
      </InventoryDetailTabs>
    </div>
  );
}
