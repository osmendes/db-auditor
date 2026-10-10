import { Button, Card } from "../../components/ui";
import { formatBytes } from "../../lib/format";
import type { PageMeta, TrackedAction } from "../../types";
import { metricLabel, statusLabel } from "./assistedActionLabels";

export function AssistedActionList({
  actions,
  exportActions,
  openFinding,
  chooseAction,
  actionPage,
  actionOffset,
  setActionOffset,
}: {
  actions: TrackedAction[];
  exportActions: (format: "csv" | "jsonl") => void;
  openFinding: (id: string | null) => void;
  chooseAction: (item: TrackedAction) => void;
  actionPage: PageMeta | null;
  actionOffset: number;
  setActionOffset: (value: number) => void;
}) {
  return (
    <Card
      title="Ações de achados"
      subtitle="Acompanhe responsável, prazo, recorrência e observações antes/depois. Correlação não prova causalidade."
    >
      {actions.length ? (
        <>
          <div className="mt-3">
            <div className="flex gap-2">
              <Button
                variant="secondary"
                onClick={() => exportActions("jsonl")}
              >
                Baixar histórico JSONL
              </Button>
              <Button variant="secondary" onClick={() => exportActions("csv")}>
                Baixar histórico CSV
              </Button>
            </div>
          </div>
          <ul className="mt-3 space-y-2">
            {actions.map((item) => (
              <li
                key={item.finding_id}
                className="rounded border border-slate-700 p-3 text-sm"
              >
                <p className="font-medium">
                  {item.title} · {statusLabel(item.status)}
                </p>
                <p>
                  Responsável: {item.owner || "não definido"} · recorrências:{" "}
                  {item.recurrences} · prazo:{" "}
                  {item.due_at
                    ? new Date(item.due_at).toLocaleDateString("pt-BR")
                    : "não definido"}
                </p>
                {item.latest_measurement ? (
                  <p className="text-slate-400">
                    {metricLabel(item.latest_measurement.metric)}:{" "}
                    {item.latest_measurement.before_value ?? "sem dado"} →{" "}
                    {item.latest_measurement.after_value ?? "sem dado"} ·{" "}
                    {item.latest_measurement.comparison_note}
                  </p>
                ) : null}
                <p className="text-slate-400">
                  {item.potential_reclaim_bytes != null
                    ? `Indicador de espaço: ${formatBytes(item.potential_reclaim_bytes)}. `
                    : ""}
                  {item.estimate_note}
                </p>
                <div className="mt-2 flex gap-2">
                  <Button
                    variant="secondary"
                    onClick={() => openFinding(item.finding_id)}
                  >
                    Abrir achado
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={() => void chooseAction(item)}
                  >
                    Medir resultado
                  </Button>
                </div>
              </li>
            ))}
          </ul>
          {actionPage ? (
            <div className="mt-3 flex items-center gap-3 text-xs text-slate-300">
              <Button
                variant="secondary"
                disabled={actionOffset === 0}
                onClick={() => setActionOffset(Math.max(0, actionOffset - 50))}
              >
                Anterior
              </Button>
              <span>
                {actionOffset + 1}–
                {Math.min(actionOffset + 50, actionPage.total)} de{" "}
                {actionPage.total} ações
              </span>
              <Button
                variant="secondary"
                disabled={!actionPage.has_more}
                onClick={() => setActionOffset(actionOffset + 50)}
              >
                Próxima
              </Button>
            </div>
          ) : null}
        </>
      ) : (
        <p className="mt-3 text-sm text-slate-400">
          Nenhuma ação planejada ou registrada neste ambiente.
        </p>
      )}
    </Card>
  );
}
