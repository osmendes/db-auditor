import {
  InventoryDetailAvailability,
  InventoryDetailTabs,
} from "../../components/InventoryDetailTabs";
import { DetailGrid, DetailSection } from "../../components/ui/Sheet";
import { formatBytes } from "../../lib/format";
import { inventoryPermalink } from "../../lib/inventoryTarget";
import {
  controls,
  type IndexAssessmentState,
  state,
} from "./useIndexAssessment";

/** Tab bodies of IndexAssessmentPanel; same markup, moved out (UI-06). */
export function IndexAssessmentTabs({ s }: { s: IndexAssessmentState }) {
  const {
    delta,
    detail,
    detailState,
    environment,
    findings,
    grants,
    history,
    index,
    run,
    setTab,
    tab,
    tableTarget,
  } = s;
  return (
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
              mantido quando a tabela muda. Os contadores acumulados não medem o
              benefício de cada consulta.
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
                    Diferença de varreduras entre as duas últimas coletas desta
                    página:{" "}
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
                        {new Date(point.collected_at).toLocaleString("pt-BR")}:{" "}
                        {formatBytes(point.size_bytes)};{" "}
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
          {state(findings.error, findings.loading, findings.data !== null) || (
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
  );
}
