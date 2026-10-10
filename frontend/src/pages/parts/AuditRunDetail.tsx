import {
  Badge,
  Button,
  Card,
  EmptyState,
  Skeleton,
  Table,
} from "../../components/ui";
import { formatError } from "../../lib/errors";
import { labels } from "../../lib/labels";
import { api } from "../../services/api";
import type {
  AnalysisRun,
  AuditBaseline,
  AuditRun,
  AuditRunCoverage,
  BaselineComparison,
  CollectorRun,
  Finding,
  ScopeScore,
} from "../../types";
import {
  CollectorProgress,
  durationLabel,
  envLabel,
  statusTone,
} from "./auditRunUi";

export function AuditRunDetail({
  selected,
  loadRuns,
  setError,
  baseline,
  baselineToken,
  setBaselineToken,
  chooseBaseline,
  comparison,
  scopeScore,
  coverage,
  analysis,
  loadRunDiagnostics,
  runFindings,
  openFinding,
  collectors,
  loadCollectors,
}: {
  selected: AuditRun;
  loadRuns: () => void;
  setError: (value: string | null) => void;
  baseline: AuditBaseline | null;
  baselineToken: string;
  setBaselineToken: (value: string) => void;
  chooseBaseline: () => void;
  comparison: BaselineComparison | null;
  scopeScore: ScopeScore | null;
  coverage: AuditRunCoverage[] | null;
  analysis: AnalysisRun | null;
  loadRunDiagnostics: (runId: string) => void;
  runFindings: Finding[] | null;
  openFinding: (id: string | null) => void;
  collectors: CollectorRun[] | null;
  loadCollectors: (runId: string) => void;
}) {
  return (
    <section className="space-y-4">
      <h2 className="text-lg font-semibold text-slate-100">Detalhe</h2>
      <Card
        title={`${selected.profile} · ${labels.runStatus(selected.status)}`}
        subtitle={`${envLabel(selected)} · ${selected.id.slice(0, 8)}…`}
      >
        {selected.status === "running" ? (
          <Button
            variant="secondary"
            onClick={() =>
              void api
                .cancelAuditRun(selected.id)
                .then(() => loadRuns())
                .catch((cause: unknown) =>
                  setError(
                    formatError(cause, "Não foi possível cancelar a execução"),
                  ),
                )
            }
          >
            Cancelar execução
          </Button>
        ) : null}
        <ul className="mt-1 space-y-1 text-sm text-slate-300">
          <li>Início: {new Date(selected.started_at).toLocaleString()}</li>
          <li>
            Fim:{" "}
            {selected.finished_at
              ? new Date(selected.finished_at).toLocaleString()
              : "—"}
          </li>
          <li>
            Duração: {durationLabel(selected.started_at, selected.finished_at)}
          </li>
          <li>Avisos: {selected.warnings.length}</li>
          <li>Erros: {selected.errors.length}</li>
        </ul>
        {selected.errors.length > 0 ? (
          <ul className="mt-3 list-inside list-disc text-xs text-rose-300">
            {selected.errors.slice(0, 8).map((e) => (
              <li key={e}>{e}</li>
            ))}
          </ul>
        ) : null}
      </Card>

      <div className="grid gap-3 sm:grid-cols-2">
        <Card
          title="Baseline aprovado"
          subtitle={
            baseline
              ? `Execução ${baseline.audit_run_id.slice(0, 8)}…`
              : "Ainda não definido"
          }
        >
          <p className="mt-2 text-xs text-slate-400">
            A seleção é registrada no histórico e exige cobertura completa.
          </p>
          {!api.hasSession() && (
            <input
              aria-label="Token para aprovar baseline"
              type="password"
              autoComplete="off"
              placeholder="Token de operação protegida"
              value={baselineToken}
              onChange={(event) => setBaselineToken(event.target.value)}
              className="mt-2 w-full rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
            />
          )}
          {selected.status === "success" ? (
            <div className="mt-3">
              <Button
                disabled={!api.hasRole("operator") && !baselineToken}
                onClick={() => void chooseBaseline()}
              >
                Aprovar esta execução
              </Button>
            </div>
          ) : null}
          {comparison ? (
            <p className="mt-2 text-sm text-slate-300">
              Comparação {comparison.status}: +{comparison.added_tables} / −
              {comparison.removed_tables} tabelas; {comparison.changed_tables}{" "}
              alteradas.
            </p>
          ) : null}
        </Card>
        <Card
          title="Score por escopo"
          subtitle={scopeScore?.version ?? "Aguardando"}
        >
          <p className="mt-2 text-sm text-slate-300">
            {scopeScore?.score == null
              ? "Indisponível por cobertura insuficiente"
              : `${scopeScore.score}/100`}{" "}
            · confiança {Math.round((scopeScore?.confidence ?? 0) * 100)}%
          </p>
          {scopeScore?.categories.map((category) => (
            <p key={category.category} className="text-xs text-slate-400">
              {category.category}: {category.score}/100 ({category.findings}{" "}
              findings)
            </p>
          ))}
        </Card>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <Card
          title="Análise automática"
          subtitle={analysis ? labels.runStatus(analysis.status) : "Aguardando"}
        >
          <p className="mt-2 text-sm text-slate-300">
            {analysis
              ? `${analysis.findings_saved}/${analysis.findings_produced} findings persistidos`
              : "A análise será executada após a coleta."}
          </p>
          {analysis?.error ? (
            <p className="mt-2 text-xs text-rose-300">{analysis.error}</p>
          ) : null}
        </Card>
        <Card
          title="Cobertura"
          subtitle={
            coverage === null
              ? "Carregando…"
              : `${coverage.filter((item) => item.database_name).length} combinações collector/database`
          }
        >
          <p className="mt-2 text-sm text-slate-300">
            {coverage
              ? `${coverage.filter((item) => item.status === "failed").length} falha(s) registradas`
              : "—"}
          </p>
        </Card>
      </div>

      {coverage?.some((item) => item.database_name) ? (
        <Card title="Progresso por database">
          <Table dense headers={["Banco", "Coletor", "Estado", "Ação"]}>
            {coverage
              .filter((item) => item.database_name)
              .map((item) => (
                <tr key={`${item.collector_name}:${item.database_name}`}>
                  <td className="px-3 py-2">{item.database_name}</td>
                  <td className="px-3 py-2">{item.collector_name}</td>
                  <td className="px-3 py-2">{item.status}</td>
                  <td className="px-3 py-2">
                    {selected.status === "running" &&
                    api.hasRole("operator") &&
                    item.database_name &&
                    item.status === "attempted" ? (
                      <button
                        type="button"
                        onClick={() =>
                          void api
                            .cancelAuditDatabase(
                              selected.id,
                              item.database_name ?? "",
                            )
                            .then(() => loadRunDiagnostics(selected.id))
                            .catch((cause: unknown) =>
                              setError(
                                formatError(
                                  cause,
                                  "Falha ao cancelar database",
                                ),
                              ),
                            )
                        }
                        className="text-rose-300 hover:underline"
                      >
                        Cancelar database
                      </button>
                    ) : (
                      "—"
                    )}
                  </td>
                </tr>
              ))}
          </Table>
        </Card>
      ) : null}

      {coverage?.some((item) => item.status === "failed") ? (
        <Card title="Falhas de cobertura">
          <ul className="mt-2 space-y-1 text-xs text-rose-300">
            {coverage
              .filter((item) => item.status === "failed")
              .slice(0, 12)
              .map((item) => (
                <li
                  key={`${item.collector_name}:${item.database_name ?? "all"}`}
                >
                  {item.collector_name}
                  {item.database_name ? ` · ${item.database_name}` : ""}
                  {item.error ? ` — ${item.error}` : ""}
                </li>
              ))}
          </ul>
        </Card>
      ) : null}

      <Card
        title="Achados desta execução"
        subtitle="O conjunto observado neste run, não a fila de hoje"
      >
        {runFindings === null ? (
          <Skeleton className="mt-2 h-16 w-full" />
        ) : runFindings.length === 0 ? (
          <p className="mt-2 text-sm text-slate-400">
            Nenhum finding foi observado nesta execução.
          </p>
        ) : (
          <ul className="mt-2 space-y-1 text-sm text-slate-300">
            {runFindings.slice(0, 30).map((finding) => (
              <li key={finding.id}>
                <button
                  type="button"
                  className="text-left hover:underline"
                  onClick={() => openFinding(finding.id)}
                >
                  {finding.severity} · {finding.title}
                </button>
              </li>
            ))}
            {runFindings.length > 30 ? (
              <li className="text-xs text-slate-500">
                {runFindings.length - 30} a mais nesta execução.
              </li>
            ) : null}
          </ul>
        )}
      </Card>

      <div className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-medium text-slate-200">Collectors</h3>
          <Button
            variant="ghost"
            onClick={() => void loadCollectors(selected.id)}
          >
            Atualizar
          </Button>
        </div>

        {collectors === null ? (
          <Skeleton className="h-24 w-full" />
        ) : collectors.length === 0 ? (
          <EmptyState
            title="Sem collectors ainda"
            description={
              selected.status === "running"
                ? "A execução ainda está iniciando os collectors…"
                : "Nenhum registro de collector para esta execução."
            }
          />
        ) : (
          <>
            <CollectorProgress
              collectors={collectors}
              runStatus={selected.status}
            />
            <Table dense headers={["Nome", "Status", "Rows", "Duração"]}>
              {collectors.map((c) => (
                <tr key={c.id} className="border-t border-slate-800">
                  <td className="px-3 py-1.5 text-xs text-slate-100">
                    {c.collector_name}
                    {c.error ? (
                      <span className="mt-0.5 block truncate text-[10px] text-rose-300">
                        {c.error}
                      </span>
                    ) : c.warning ? (
                      <span className="mt-0.5 block truncate text-[10px] text-amber-300">
                        {c.warning}
                      </span>
                    ) : null}
                  </td>
                  <td className="px-3 py-1.5">
                    <Badge tone={statusTone(c.status)}>
                      {labels.runStatus(c.status)}
                    </Badge>
                  </td>
                  <td className="px-3 py-1.5 font-mono text-xs text-slate-300">
                    {c.rows_collected}
                  </td>
                  <td className="px-3 py-1.5 font-mono text-xs text-slate-400">
                    {durationLabel(c.started_at, c.finished_at)}
                  </td>
                </tr>
              ))}
            </Table>
          </>
        )}
      </div>
    </section>
  );
}
