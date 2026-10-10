import { Button, Card, Select } from "../../components/ui";
import { formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type {
  ActionMeasurement,
  AuditRun,
  PageMeta,
  TrackedAction,
} from "../../types";
import { metricLabel } from "./assistedActionLabels";

export function AssistedMeasurementCard({
  selectedAction,
  before,
  setBefore,
  after,
  setAfter,
  runs,
  metric,
  setMetric,
  hypothesis,
  setHypothesis,
  windowNote,
  setWindowNote,
  workloadComparable,
  setWorkloadComparable,
  busy,
  measure,
  measurements,
  measurementPage,
  measurementOffset,
  setMeasurements,
  setMeasurementPage,
  setMeasurementOffset,
  setError,
}: {
  selectedAction: TrackedAction | null;
  before: string;
  setBefore: (value: string) => void;
  after: string;
  setAfter: (value: string) => void;
  runs: AuditRun[];
  metric: ActionMeasurement["metric"];
  setMetric: (value: ActionMeasurement["metric"]) => void;
  hypothesis: string;
  setHypothesis: (value: string) => void;
  windowNote: string;
  setWindowNote: (value: string) => void;
  workloadComparable: boolean;
  setWorkloadComparable: (value: boolean) => void;
  busy: boolean;
  measure: () => void;
  measurements: ActionMeasurement[];
  measurementPage: PageMeta | null;
  measurementOffset: number;
  setMeasurements: (value: ActionMeasurement[]) => void;
  setMeasurementPage: (value: PageMeta | null) => void;
  setMeasurementOffset: (value: number) => void;
  setError: (value: string) => void;
}) {
  return (
    <>
      {selectedAction ? (
        <Card
          title={`Medir ação: ${selectedAction.title}`}
          subtitle="Escolha dois runs da mesma cobertura, perfil e versão; informe a hipótese e a janela de observação."
        >
          <div className="mt-3 space-y-2">
            <Select
              label="Antes"
              value={before}
              onChange={(e) => setBefore(e.target.value)}
              options={[
                { value: "", label: "Selecione" },
                ...runs.map((run) => ({
                  value: run.id,
                  label: `${new Date(run.started_at).toLocaleString("pt-BR")} · ${run.id.slice(0, 8)}`,
                })),
              ]}
            />
            <Select
              label="Depois"
              value={after}
              onChange={(e) => setAfter(e.target.value)}
              options={[
                { value: "", label: "Selecione" },
                ...runs.map((run) => ({
                  value: run.id,
                  label: `${new Date(run.started_at).toLocaleString("pt-BR")} · ${run.id.slice(0, 8)}`,
                })),
              ]}
            />
            <Select
              label="Métrica"
              value={metric}
              onChange={(e) =>
                setMetric(e.target.value as ActionMeasurement["metric"])
              }
              options={[
                {
                  value: "finding_observed",
                  label: "Achado observado (0/1)",
                },
                {
                  value: "table_size_bytes",
                  label: "Tamanho da tabela (bytes)",
                },
                {
                  value: "query_mean_latency_us",
                  label: "Latência média da consulta (µs)",
                },
                {
                  value: "query_reads_per_1000_calls",
                  label: "Blocos lidos por 1.000 chamadas",
                },
              ]}
            />
            <textarea
              aria-label="Hipótese"
              placeholder="Hipótese de melhora, sem afirmar causalidade"
              value={hypothesis}
              onChange={(e) => setHypothesis(e.target.value)}
              className="w-full rounded border border-slate-600 bg-slate-900 p-2"
            />
            <textarea
              aria-label="Janela e carga"
              placeholder="Janela, carga e mudanças concorrentes"
              value={windowNote}
              onChange={(e) => setWindowNote(e.target.value)}
              className="w-full rounded border border-slate-600 bg-slate-900 p-2"
            />
            <label className="flex items-start gap-2 text-sm text-slate-300">
              <input
                type="checkbox"
                checked={workloadComparable}
                onChange={(event) =>
                  setWorkloadComparable(event.target.checked)
                }
              />
              Confirmo que as cargas de trabalho nas duas janelas são
              suficientemente semelhantes, com base em evidência registrada na
              nota acima.
            </label>
            {api.hasRole("auditor") ? (
              <Button
                disabled={
                  busy ||
                  !before ||
                  !after ||
                  hypothesis.trim().length < 8 ||
                  windowNote.trim().length < 4
                }
                onClick={() => void measure()}
              >
                Registrar medição
              </Button>
            ) : null}
            <p className="text-xs text-slate-400">
              Histórico de medições. A carga informada pelo operador é uma
              ressalva, não uma confirmação automática de condições iguais.
            </p>
            <ul
              className="space-y-2 text-sm"
              aria-label="Série de medições da ação"
            >
              {measurements.map((item) => (
                <li
                  key={item.id}
                  className="rounded border border-slate-700 p-2"
                >
                  {new Date(item.recorded_at).toLocaleString("pt-BR")} ·{" "}
                  {metricLabel(item.metric)}: {item.before_value ?? "sem dado"}{" "}
                  → {item.after_value ?? "sem dado"}. {item.comparison_note}{" "}
                  Hipótese: {item.hypothesis}. Janela e carga:{" "}
                  {item.window_note}.
                </li>
              ))}
            </ul>
            {measurementPage && measurementPage.total > 50 ? (
              <div className="mt-3 flex items-center gap-3 text-xs text-slate-300">
                <Button
                  variant="secondary"
                  disabled={measurementOffset === 0}
                  onClick={() => {
                    const next = Math.max(0, measurementOffset - 50);
                    void api
                      .actionMeasurements(selectedAction.finding_id, next)
                      .then((page) => {
                        setMeasurements(page.items);
                        setMeasurementPage(page.page);
                        setMeasurementOffset(next);
                      })
                      .catch((cause) =>
                        setError(
                          formatError(cause, "Falha ao carregar medições"),
                        ),
                      );
                  }}
                >
                  Anterior
                </Button>
                <span>
                  {measurementOffset + 1}–
                  {Math.min(measurementOffset + 50, measurementPage.total)} de{" "}
                  {measurementPage.total} medições
                </span>
                <Button
                  variant="secondary"
                  disabled={!measurementPage.has_more}
                  onClick={() => {
                    const next = measurementOffset + 50;
                    void api
                      .actionMeasurements(selectedAction.finding_id, next)
                      .then((page) => {
                        setMeasurements(page.items);
                        setMeasurementPage(page.page);
                        setMeasurementOffset(next);
                      })
                      .catch((cause) =>
                        setError(
                          formatError(cause, "Falha ao carregar medições"),
                        ),
                      );
                  }}
                >
                  Próxima
                </Button>
              </div>
            ) : null}
          </div>
        </Card>
      ) : null}
    </>
  );
}
