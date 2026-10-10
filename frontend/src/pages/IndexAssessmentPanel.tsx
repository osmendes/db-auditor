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
  Finding,
  GrantSnapshot,
  IndexHistoryPoint,
  IndexSnapshot,
  PagedResponse,
  SnapshotCompleteness,
} from "../types";
import type { IndexDetailSnapshot } from "../types/openapi";

function usePage<T>(
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
      .then((value) => {
        if (!cancelled) setData(value);
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
  return { offset, setOffset, data, error, loading };
}

function state(error: unknown, loading: boolean, hasData: boolean) {
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

function controls(
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
      label="Detalhe do índice"
    />
  );
}

export function comparableIndexScans(
  points: IndexHistoryPoint[],
): number | null {
  if (points.length < 2) return null;
  const [current, previous] = points;
  if (
    !current.usage_observed ||
    !previous.usage_observed ||
    !current.stats_reset ||
    current.stats_reset !== previous.stats_reset ||
    current.definition_fingerprint !== previous.definition_fingerprint ||
    current.idx_scan < previous.idx_scan
  )
    return null;
  return current.idx_scan - previous.idx_scan;
}

export function IndexAssessmentPanel({
  index,
  environment,
  run,
  snapshot,
}: {
  index: IndexSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<IndexDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const scope = JSON.stringify([
    environment,
    run,
    index.database_name,
    index.schema_name,
    index.index_name,
  ]);
  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .indexDetail(
        environment,
        run,
        index.database_name,
        index.schema_name,
        index.index_name,
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
  const history = usePage<IndexHistoryPoint>(
    tab === "performance",
    scope,
    (offset) =>
      api.indexHistory(
        environment,
        run,
        index.database_name,
        index.schema_name,
        index.index_name,
        50,
        offset,
      ),
  );
  const grants = usePage<GrantSnapshot>(tab === "security", scope, (offset) =>
    api.tableGrants(
      environment,
      run,
      index.database_name,
      index.schema_name,
      index.table_name,
      50,
      offset,
    ),
  );
  const findings = usePage<Finding>(
    tab === "recommendations",
    scope,
    (offset) =>
      api.indexFindings(
        environment,
        run,
        index.database_name,
        index.schema_name,
        index.index_name,
        50,
        offset,
      ),
  );
  const detailState = state(detailError, detailLoading, detail !== null);
  const target = {
    kind: "indexes" as const,
    database: index.database_name,
    schema: index.schema_name,
    name: index.index_name,
  };
  const tableTarget = {
    kind: "tables" as const,
    database: index.database_name,
    schema: index.schema_name,
    name: detail?.table_name ?? index.table_name,
  };
  const delta = comparableIndexScans(history.data?.items ?? []);
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para este índice e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <InventoryDetailTabs tab={tab} onTabChange={setTab}>
        {tab === "overview" && (
          <>
            <DetailSection title="Índice e tabela relacionada">
              <DetailGrid
                items={[
                  { label: "Banco", value: index.database_name },
                  { label: "Esquema", value: index.schema_name },
                  { label: "Índice", value: index.index_name },
                  { label: "Tabela", value: index.table_name },
                  {
                    label: "Método",
                    value: index.access_method ?? "Não coletado",
                  },
                  { label: "Tamanho", value: formatBytes(index.size_bytes) },
                ]}
              />
              <a
                href={inventoryPermalink(environment, run, tableTarget)}
                className="text-sm text-cyan-300 underline"
              >
                Analisar a tabela relacionada
              </a>
            </DetailSection>
            <DetailSection title="Como interpretar">
              <p className="text-sm text-slate-300">
                Um índice pode acelerar leituras, mas ocupa espaço e precisa ser
                mantido quando a tabela muda. Os contadores acumulados não medem
                o benefício de cada consulta.
              </p>
            </DetailSection>
          </>
        )}
        {tab === "structure" &&
          (detailState || (
            <>
              <DetailSection title="Estrutura observada">
                <DetailGrid
                  items={[
                    {
                      label: "Método",
                      value: detail?.access_method ?? "Não coletado",
                    },
                    {
                      label: "Único",
                      value: detail?.is_unique ? "Sim" : "Não",
                    },
                    {
                      label: "Chave primária",
                      value: detail?.is_primary ? "Sim" : "Não",
                    },
                    {
                      label: "Válido",
                      value: detail?.is_valid ? "Sim" : "Não",
                    },
                    {
                      label: "Pronto para escrita",
                      value: detail?.is_ready ? "Sim" : "Não",
                    },
                    {
                      label: "Parcial",
                      value: detail?.is_partial ? "Sim" : "Não",
                    },
                  ]}
                />
                {detail?.key_columns === null ? (
                  <InventoryDetailAvailability
                    state="not_collected"
                    detail="Colunas da chave não foram coletadas nesta execução."
                  />
                ) : (
                  <p className="text-sm text-slate-300">
                    Colunas ou expressões da chave:{" "}
                    {detail?.key_columns?.length
                      ? detail.key_columns.join(", ")
                      : "Nenhuma registrada"}
                  </p>
                )}
                {detail?.include_columns === null ? (
                  <InventoryDetailAvailability
                    state="not_collected"
                    detail="Colunas incluídas não foram coletadas nesta execução."
                  />
                ) : (
                  <p className="text-sm text-slate-300">
                    Colunas incluídas:{" "}
                    {detail?.include_columns?.length
                      ? detail.include_columns.join(", ")
                      : "Nenhuma"}
                  </p>
                )}
              </DetailSection>
              <DetailSection title="Definição protegida">
                <p className="text-sm text-slate-300">
                  O SQL e o predicado podem conter valores sensíveis e não são
                  exibidos. Impressão digital da definição:{" "}
                  {detail?.definition_fingerprint || "Não coletada"}.{" "}
                  {detail?.is_partial
                    ? `Impressão digital do predicado: ${detail.predicate_fingerprint || "Não coletada"}.`
                    : ""}
                </p>
              </DetailSection>
            </>
          ))}
        {tab === "relationships" && (
          <DetailSection title="Vínculo com a tabela">
            <p className="text-sm text-slate-300">
              Este índice pertence à tabela {index.schema_name}.
              {detail?.table_name ?? index.table_name}.
            </p>
            <a
              href={inventoryPermalink(environment, run, tableTarget)}
              className="text-sm text-cyan-300 underline"
            >
              Abrir análise da tabela
            </a>
            <InventoryDetailAvailability
              state="not_collected"
              detail="Outras dependências do índice não são coletadas nesta execução."
            />
          </DetailSection>
        )}
        {tab === "performance" && (
          <>
            {detailState || (
              <DetailSection title="Uso e espaço nesta execução">
                <DetailGrid
                  items={[
                    {
                      label: "Tamanho",
                      value: formatBytes(detail?.size_bytes ?? 0),
                    },
                    {
                      label: "Varreduras do índice",
                      value:
                        detail?.usage_observed === true
                          ? (detail.idx_scan ?? 0).toLocaleString("pt-BR")
                          : "Estatística indisponível",
                    },
                    {
                      label: "Tuplas lidas",
                      value:
                        detail?.usage_observed === true
                          ? (detail.idx_tup_read ?? 0).toLocaleString("pt-BR")
                          : "Estatística indisponível",
                    },
                    {
                      label: "Reinício das estatísticas",
                      value: detail?.stats_reset
                        ? new Date(detail.stats_reset).toLocaleString("pt-BR")
                        : "Não informado",
                    },
                  ]}
                />
              </DetailSection>
            )}
            {state(history.error, history.loading, history.data !== null) || (
              <DetailSection title="Histórico de coletas">
                {history.data?.items.length ? (
                  <>
                    <p className="text-sm text-slate-300">
                      Diferença de varreduras entre as duas últimas coletas
                      desta página:{" "}
                      {delta === null
                        ? "Não comparável: confira disponibilidade, reinício dos contadores e paginação."
                        : delta.toLocaleString("pt-BR")}
                    </p>
                    <ul className="space-y-2 text-sm text-slate-300">
                      {history.data.items.map((point) => (
                        <li
                          key={point.audit_run_id}
                          className="rounded border border-slate-700 p-2"
                        >
                          {new Date(point.collected_at).toLocaleString("pt-BR")}
                          : {formatBytes(point.size_bytes)};{" "}
                          {point.usage_observed
                            ? `${point.idx_scan.toLocaleString("pt-BR")} varreduras acumuladas`
                            : "estatística indisponível"}
                        </li>
                      ))}
                    </ul>
                  </>
                ) : (
                  <InventoryDetailAvailability
                    state="empty"
                    detail="Não há histórico comparável para este índice."
                  />
                )}
                {controls(
                  history.data?.page.total ?? 0,
                  history.offset,
                  history.setOffset,
                )}
              </DetailSection>
            )}
            <InventoryDetailAvailability
              state="not_collected"
              detail="O custo de escrita e o benefício por consulta não são medidos diretamente. Revise a carga de trabalho antes de alterar o índice."
            />
          </>
        )}
        {tab === "security" &&
          (detailState || (
            <>
              <DetailSection title="Responsável e acesso">
                <DetailGrid
                  items={[
                    {
                      label: "Responsável pela tabela",
                      value: detail?.table_owner_name ?? "Não coletado",
                    },
                  ]}
                />
                <p className="text-sm text-slate-300">
                  O PostgreSQL controla o acesso aos dados pela tabela. Esta
                  coleta não registra permissões próprias para índices.
                </p>
              </DetailSection>
              <DetailSection title="Acessos observados à tabela">
                {state(grants.error, grants.loading, grants.data !== null) ||
                  (grants.data?.items.length ? (
                    <ul className="space-y-1 text-sm text-slate-300">
                      {grants.data.items.map((grant) => (
                        <li key={grant.grantee}>
                          {grant.grantee}: {grant.privileges.join(", ")}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <InventoryDetailAvailability
                      state="empty"
                      detail="Nenhum acesso de papel com login foi registrado para esta tabela nesta execução."
                    />
                  ))}
                {controls(
                  grants.data?.page.total ?? 0,
                  grants.offset,
                  grants.setOffset,
                )}
              </DetailSection>
            </>
          ))}
        {tab === "recommendations" && (
          <>
            {detail && !detail.is_valid && (
              <InventoryDetailAvailability
                state="partial"
                detail="Índice inválido: revise a causa e planeje a correção com um administrador. O auditor não executa mudanças."
              />
            )}
            {detail?.usage_observed && detail.idx_scan === 0 && (
              <InventoryDetailAvailability
                state="partial"
                detail="Nenhuma varredura foi registrada desde o reinício das estatísticas. Isso, sozinho, não indica que o índice pode ser removido."
              />
            )}
            {state(
              findings.error,
              findings.loading,
              findings.data !== null,
            ) || (
              <DetailSection title="Achados deste índice e execução">
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
                    detail="Nenhum achado deste índice foi observado nesta execução. Isso não comprova ausência de risco."
                  />
                )}
                {controls(
                  findings.data?.page.total ?? 0,
                  findings.offset,
                  findings.setOffset,
                )}
              </DetailSection>
            )}
          </>
        )}
      </InventoryDetailTabs>
    </div>
  );
}
