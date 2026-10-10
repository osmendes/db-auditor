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
  Finding,
  GrantSnapshot,
  HypertableChunkDetail,
  HypertableDimensionDetail,
  HypertableHistoryPoint,
  HypertableJobDetail,
  HypertablePolicyDetail,
  HypertableSnapshot,
  IndexSnapshot,
  RLSPolicySnapshot,
  SnapshotCompleteness,
} from "../types";
import type { HypertableDetailSnapshot } from "../types/openapi";

function date(value: string | null | undefined): string {
  return value ? new Date(value).toLocaleString("pt-BR") : "Não informado";
}

function jobStatus(value: string | null): string {
  if (!value) return "Sem resultado coletado";
  if (value.toLowerCase() === "success") return "Concluído";
  if (value.toLowerCase() === "failed") return "Falhou";
  return value;
}

export function HypertableAssessmentPanel({
  hypertable,
  environment,
  run,
  snapshot,
}: {
  hypertable: HypertableSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<HypertableDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const {
    database_name: database,
    schema_name: schema,
    hypertable_name: name,
  } = hypertable;
  const scope = JSON.stringify([environment, run, database, schema, name]);
  const args = [environment, run, database, schema, name] as const;
  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .hypertableDetail(...args)
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
  const dimensions = useInventoryDetailPage<HypertableDimensionDetail>(
    tab === "structure",
    scope,
    (offset) => api.hypertableDimensions(...args, 50, offset),
  );
  const chunks = useInventoryDetailPage<HypertableChunkDetail>(
    tab === "structure",
    scope,
    (offset) => api.hypertableChunks(...args, 50, offset),
  );
  const policies = useInventoryDetailPage<HypertablePolicyDetail>(
    tab === "relationships",
    scope,
    (offset) => api.hypertablePolicies(...args, 50, offset),
  );
  const jobs = useInventoryDetailPage<HypertableJobDetail>(
    tab === "performance",
    scope,
    (offset) => api.hypertableJobs(...args, 50, offset),
  );
  const history = useInventoryDetailPage<HypertableHistoryPoint>(
    tab === "performance",
    scope,
    (offset) => api.hypertableHistory(...args, 50, offset),
  );
  const indexes = useInventoryDetailPage<IndexSnapshot>(
    tab === "relationships",
    scope,
    (offset) => api.hypertableIndexes(...args, 50, offset),
  );
  const grants = useInventoryDetailPage<GrantSnapshot>(
    tab === "security" && !!detail?.base_table_observed,
    scope,
    (offset) => api.hypertableGrants(...args, 50, offset),
  );
  const rls = useInventoryDetailPage<RLSPolicySnapshot>(
    tab === "security" && !!detail?.base_table_observed,
    scope,
    (offset) => api.hypertableRLSPolicies(...args, 50, offset),
  );
  const findings = useInventoryDetailPage<Finding>(
    tab === "recommendations",
    scope,
    (offset) => api.hypertableFindings(...args, 50, offset),
  );
  const detailState = inventoryDetailState(
    detailError,
    detailLoading,
    detail !== null,
  );
  const target = { kind: "hypertables" as const, database, schema, name };
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
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para esta tabela temporal e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <InventoryDetailTabs tab={tab} onTabChange={setTab}>
        {tab === "overview" && (
          <DetailSection title="Tabela temporal selecionada">
            <DetailGrid
              items={[
                { label: "Banco", value: database },
                { label: "Esquema", value: schema },
                { label: "Nome", value: name },
                {
                  label: "Responsável",
                  value: hypertable.owner_name ?? "Não coletado",
                },
                {
                  label: "Dimensões",
                  value: hypertable.num_dimensions.toLocaleString("pt-BR"),
                },
                {
                  label: "Chunks",
                  value: hypertable.num_chunks.toLocaleString("pt-BR"),
                },
                {
                  label: "Compressão ou columnstore",
                  value: hypertable.compression_enabled
                    ? "Habilitada"
                    : "Não habilitada",
                },
                {
                  label: "Distribuída",
                  value: hypertable.is_distributed ? "Sim" : "Não",
                },
                { label: "Coletado em", value: date(hypertable.collected_at) },
              ]}
            />
            <p className="text-sm text-slate-300">
              Os números representam o estado observado na execução selecionada.
            </p>
          </DetailSection>
        )}
        {tab === "structure" && (
          <>
            <DetailSection title="Dimensões e particionamento">
              {sectionState(dimensions) ||
                (dimensions.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {dimensions.data.items.map((item) => (
                      <li key={item.dimension_number}>
                        <strong>{item.column_name}</strong> (
                        {item.column_type ?? "tipo não coletado"});{" "}
                        {item.dimension_type ?? "tipo de dimensão não coletado"}
                        ; intervalo{" "}
                        {item.time_interval ??
                          item.integer_interval ??
                          "não coletado"}
                        ; {item.num_slices ?? "—"} fatias
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability
                    state="empty"
                    detail="Nenhuma dimensão foi encontrada nesta execução."
                  />
                ))}
              {controls(dimensions, "Dimensões")}
            </DetailSection>
            <DetailSection title="Chunks observados">
              {sectionState(chunks) ||
                (chunks.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {chunks.data.items.map((item) => (
                      <li key={`${item.chunk_schema}.${item.chunk_name}`}>
                        {item.chunk_schema}.{item.chunk_name}:{" "}
                        {formatBytes(item.total_size_bytes)};{" "}
                        {item.is_compressed ? "comprimido" : "não comprimido"}
                        {"; período "}
                        {date(item.range_start)} a {date(item.range_end)}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability
                    state="empty"
                    detail="Nenhum chunk foi encontrado nesta execução."
                  />
                ))}
              {controls(chunks, "Chunks")}
            </DetailSection>
          </>
        )}
        {tab === "relationships" && (
          <>
            <DetailSection title="Tabela base e índices">
              {detailState ||
                (detail?.base_table_observed ? (
                  <a
                    className="text-cyan-300 underline"
                    href={inventoryPermalink(environment, run, {
                      kind: "tables",
                      database,
                      schema,
                      name,
                    })}
                  >
                    Abrir a análise da tabela base nesta execução
                  </a>
                ) : (
                  <InventoryDetailAvailability
                    state="not_collected"
                    detail="A tabela base não foi observada nesta execução. Seus dados e permissões não são presumidos."
                  />
                ))}
              {sectionState(indexes) ||
                (indexes.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {indexes.data.items.map((item) => (
                      <li key={item.id}>
                        <a
                          className="text-cyan-300 underline"
                          href={inventoryPermalink(environment, run, {
                            kind: "indexes",
                            database,
                            schema,
                            name: item.index_name,
                          })}
                        >
                          {item.index_name}
                        </a>{" "}
                        — {formatBytes(item.size_bytes)}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability
                    state="empty"
                    detail="Nenhum índice foi observado para esta tabela temporal."
                  />
                ))}
              {controls(indexes, "Índices")}
            </DetailSection>
            <DetailSection title="Políticas agendadas">
              {sectionState(policies) ||
                (policies.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {policies.data.items.map((item) => (
                      <li key={item.job_id}>
                        Job {item.job_id}: {item.policy_type};{" "}
                        {item.scheduled ? "agendado" : "desativado"}; último
                        resultado {jobStatus(item.last_run_status)}; falhas{" "}
                        {item.total_failures ?? "não coletadas"}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability
                    state="empty"
                    detail="Nenhuma política foi encontrada nesta execução."
                  />
                ))}
              {controls(policies, "Políticas")}
            </DetailSection>
          </>
        )}
        {tab === "performance" && (
          <>
            <DetailSection title="Armazenamento observado">
              <DetailGrid
                items={[
                  {
                    label: "Total",
                    value: formatBytes(hypertable.total_size_bytes),
                  },
                  {
                    label: "Dados",
                    value: formatBytes(hypertable.data_size_bytes),
                  },
                  {
                    label: "Índices",
                    value: formatBytes(hypertable.index_size_bytes),
                  },
                ]}
              />
              <p className="text-sm text-slate-300">
                Estes tamanhos não medem tempo de consulta. Compare apenas
                execuções com cobertura semelhante.
              </p>
            </DetailSection>
            <DetailSection title="Jobs desta tabela temporal">
              {sectionState(jobs) ||
                (jobs.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {jobs.data.items.map((item) => (
                      <li key={item.job_id}>
                        Job {item.job_id}:{" "}
                        {item.application_name ?? item.proc_name ?? "sem nome"};{" "}
                        {item.scheduled ? "agendado" : "desativado"}; último
                        resultado {jobStatus(item.last_run_status)};{" "}
                        {item.total_failures} falhas; próxima execução{" "}
                        {date(item.next_start)}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <InventoryDetailAvailability
                    state="empty"
                    detail="Nenhum job específico desta tabela temporal foi observado."
                  />
                ))}
              {controls(jobs, "Jobs")}
            </DetailSection>
            <DetailSection title="Evolução por execução">
              {sectionState(history) ||
                (history.data?.items.length ? (
                  <ul className="space-y-2 text-sm text-slate-300">
                    {history.data.items.map((item) => (
                      <li key={item.audit_run_id}>
                        {date(item.collected_at)}:{" "}
                        {formatBytes(item.total_size_bytes)}, {item.num_chunks}{" "}
                        chunks
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
          <>
            <DetailSection title="Responsável e permissões">
              <DetailGrid
                items={[
                  {
                    label: "Responsável",
                    value: hypertable.owner_name ?? "Não coletado",
                  },
                ]}
              />
              {detailState ||
                (!detail?.base_table_observed ? (
                  <InventoryDetailAvailability
                    state="not_collected"
                    detail="Permissões da tabela base não foram observadas nesta execução."
                  />
                ) : (
                  <>
                    <p className="text-sm text-slate-300">
                      Acessos observados na tabela base. Valide privilégios
                      efetivos no banco antes de alterar permissões.
                    </p>
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
                          state="empty"
                          detail="Nenhum acesso explícito observado."
                        />
                      ))}
                    {controls(grants, "Acessos")}
                  </>
                ))}
            </DetailSection>
            <DetailSection title="Políticas de segurança por linha">
              {detailState ||
                (!detail?.base_table_observed ? (
                  <InventoryDetailAvailability state="not_collected" />
                ) : (
                  sectionState(rls) ||
                  (rls.data?.items.length ? (
                    <ul className="text-sm text-slate-300">
                      {rls.data.items.map((item) => (
                        <li key={item.name}>
                          {item.name}: {item.command ?? "comando não coletado"}
                          {"; papéis "}
                          {item.roles.join(", ")}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <InventoryDetailAvailability
                      state="empty"
                      detail="Nenhuma política de linha foi observada; isso não confirma que RLS esteja desabilitado."
                    />
                  ))
                ))}
              {controls(rls, "Políticas de linha")}
            </DetailSection>
          </>
        )}
        {tab === "recommendations" && (
          <DetailSection title="Achados desta tabela temporal e execução">
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
                  detail="Nenhum achado específico desta tabela temporal foi registrado nesta execução."
                />
              ))}
            {controls(findings, "Achados")}
            <p className="text-sm text-slate-300">
              Confirme qualquer recomendação com consultas somente leitura e um
              plano de retorno antes de alterar o banco.
            </p>
          </DetailSection>
        )}
      </InventoryDetailTabs>
    </div>
  );
}
