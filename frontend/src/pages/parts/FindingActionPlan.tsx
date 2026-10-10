import { Button } from "../../components/ui";
import { api } from "../../services/api";
import type { ActionEvent, FindingAction } from "../../types";

export function FindingActionPlan({
  action,
  actionStatus,
  setActionStatus,
  actionOwner,
  setActionOwner,
  actionJustification,
  setActionJustification,
  actionResult,
  setActionResult,
  saveAction,
  actionEvents,
}: {
  action: FindingAction | null;
  actionStatus: string;
  setActionStatus: (value: string) => void;
  actionOwner: string;
  setActionOwner: (value: string) => void;
  actionJustification: string;
  setActionJustification: (value: string) => void;
  actionResult: string;
  setActionResult: (value: string) => void;
  saveAction: () => void;
  actionEvents: ActionEvent[];
}) {
  return (
    <>
      {action ? (
        <section
          className="mt-5 rounded border border-slate-700 bg-slate-950/50 p-4"
          aria-labelledby="action-heading"
        >
          <h3
            id="action-heading"
            className="text-sm font-semibold text-slate-100"
          >
            Plano de ação sugerido
          </h3>
          <p className="mt-2 text-sm text-slate-200">{action.suggestion}</p>
          <p className="mt-1 text-xs text-amber-300">
            Cobertura: {action.coverage}.{" "}
            {action.coverage !== "complete"
              ? "Confirme com nova coleta antes de decidir."
              : "A sugestão ainda exige validação humana."}
          </p>
          <dl className="mt-3 grid gap-2 text-xs text-slate-300 sm:grid-cols-2">
            <div>
              <dt className="font-semibold">Benefício esperado</dt>
              <dd>{action.plan.expected_benefit}</dd>
            </div>
            <div>
              <dt className="font-semibold">Risco</dt>
              <dd>{action.plan.risk}</dd>
            </div>
            <div>
              <dt className="font-semibold">Pré-requisitos</dt>
              <dd>{action.plan.prerequisites}</dd>
            </div>
            <div>
              <dt className="font-semibold">Como confirmar</dt>
              <dd>{action.plan.confirmation}</dd>
            </div>
            <div>
              <dt className="font-semibold">Como validar depois</dt>
              <dd>{action.plan.validation}</dd>
            </div>
            <div>
              <dt className="font-semibold">Possível falso positivo</dt>
              <dd>{action.plan.false_positive_risk}</dd>
            </div>
          </dl>
          {action.plan.read_only_query ? (
            <details className="mt-3 text-xs text-slate-400">
              <summary>Consulta de confirmação para revisão externa</summary>
              <pre className="mt-2 overflow-auto">
                {action.plan.read_only_query}
              </pre>
            </details>
          ) : null}
          <p className="mt-3 text-xs text-slate-400">
            O auditor não executa alterações no banco analisado. Registre o
            resultado após a ação externa. Para marcar como validada, registre
            em Ações assistidas uma medição comparável de uma coleta completa
            posterior à mudança.
          </p>
          {api.hasRole("auditor") ? (
            <div className="mt-3 grid gap-2 sm:grid-cols-2">
              <label className="text-xs text-slate-300">
                Estado
                <select
                  value={actionStatus}
                  onChange={(e) => setActionStatus(e.target.value)}
                  className="mt-1 block w-full rounded border border-slate-600 bg-slate-900 p-2"
                >
                  <option value="suggested">Sugerida</option>
                  <option value="in_review">Em análise</option>
                  <option value="planned">Planejada</option>
                  <option value="executed_externally">
                    Executada externamente
                  </option>
                  <option value="validated">Validada</option>
                  <option value="discarded">Descartada</option>
                </select>
              </label>
              <label className="text-xs text-slate-300">
                Responsável
                <input
                  value={actionOwner}
                  onChange={(e) => setActionOwner(e.target.value)}
                  className="mt-1 block w-full rounded border border-slate-600 bg-slate-900 p-2"
                />
              </label>
              <label className="text-xs text-slate-300">
                Justificativa
                <textarea
                  value={actionJustification}
                  onChange={(e) => setActionJustification(e.target.value)}
                  className="mt-1 block w-full rounded border border-slate-600 bg-slate-900 p-2"
                />
              </label>
              <label className="text-xs text-slate-300">
                Resultado ou evidência posterior
                <textarea
                  value={actionResult}
                  onChange={(e) => setActionResult(e.target.value)}
                  className="mt-1 block w-full rounded border border-slate-600 bg-slate-900 p-2"
                />
              </label>
              <Button onClick={() => void saveAction()}>Salvar decisão</Button>
            </div>
          ) : null}
          {actionEvents.length ? (
            <details className="mt-3 text-xs text-slate-400">
              <summary>Histórico da ação ({actionEvents.length})</summary>
              <ul className="mt-2 space-y-1">
                {actionEvents.map((event, index) => (
                  <li key={`${event.recorded_at}-${index}`}>
                    {new Date(event.recorded_at).toLocaleString()} ·{" "}
                    {event.actor} · {event.status}{" "}
                    {event.result ? `· ${event.result}` : ""}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </section>
      ) : null}
    </>
  );
}
