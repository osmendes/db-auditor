import { Button, Card } from "../../components/ui";
import type { LegacyTableTarget } from "../../lib/inventoryTarget";
import { labels } from "../../lib/labels";
import { api } from "../../services/api";
import type {
  ActionEvent,
  Finding,
  FindingAction,
  FindingEvent,
  NavigationSection,
} from "../../types";
import { FindingActionPlan } from "./FindingActionPlan";

export function FindingDetailCard({
  selected,
  action,
  openInventory,
  setSection,
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
  triage,
  assignee,
  setAssignee,
  dueAt,
  setDueAt,
  saveWorkflow,
  suppressionReason,
  setSuppressionReason,
  suppressedUntil,
  setSuppressedUntil,
  suppressSelected,
  timeline,
}: {
  selected: Finding | null;
  action: FindingAction | null;
  openInventory: (target: LegacyTableTarget) => void;
  setSection: (section: NavigationSection) => void;
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
  triage: (id: string, status: string) => void;
  assignee: string;
  setAssignee: (value: string) => void;
  dueAt: string;
  setDueAt: (value: string) => void;
  saveWorkflow: () => void;
  suppressionReason: string;
  setSuppressionReason: (value: string) => void;
  suppressedUntil: string;
  setSuppressedUntil: (value: string) => void;
  suppressSelected: () => void;
  timeline: FindingEvent[];
}) {
  return (
    <>
      {selected ? (
        <Card
          title={`Detalhe · ${labels.severity(selected.severity)}`}
          subtitle={action?.plan.meaning ?? selected.title}
        >
          <ul className="mt-3 space-y-1 text-sm text-slate-300">
            <li>Status: {labels.findingStatus(selected.status)}</li>
            <li>
              Primeira observação:{" "}
              {new Date(selected.first_seen_at).toLocaleString()}
            </li>
            <li>
              Última observação:{" "}
              {new Date(selected.last_seen_at).toLocaleString()}
            </li>
            <li>Recorrências: {selected.recurrence_count ?? 0}</li>
            {selected.resolved_at ? (
              <li>
                Resolução: {new Date(selected.resolved_at).toLocaleString()}
              </li>
            ) : null}
            {selected.superseded_by ? (
              <li>Substituído por: {selected.superseded_by}</li>
            ) : null}
            {selected.suppression_reason ? (
              <li>
                Supressão: {selected.suppression_reason} (até{" "}
                {selected.suppressed_until
                  ? new Date(selected.suppressed_until).toLocaleString()
                  : "—"}
                )
              </li>
            ) : null}
            <li>Objeto: {selected.object_key || "—"}</li>
            <li>
              Confiança:{" "}
              {selected.confidence != null
                ? `${Math.round(selected.confidence * 100)}%`
                : "não informada"}
            </li>
            <li>
              Próximo passo:{" "}
              {action?.plan.next ?? "Abra a ação para ver o plano desta regra."}
            </li>
            {action?.plan.impact ? (
              <li>Impacto: {action.plan.impact}</li>
            ) : null}
            {action?.plan.effort ? (
              <li>Esforço: {action.plan.effort}</li>
            ) : null}
            {selected.audit_run_id ? (
              <li>Execução de origem: {selected.audit_run_id}</li>
            ) : null}
          </ul>
          <div className="mt-3 flex flex-wrap gap-2">
            {selected.object_type === "table" &&
            selected.database_name &&
            selected.schema_name &&
            selected.object_name ? (
              <Button
                variant="secondary"
                onClick={() =>
                  openInventory({
                    database: selected.database_name ?? "",
                    schema: selected.schema_name ?? "",
                    table: selected.object_name ?? "",
                  })
                }
              >
                Abrir objeto no inventário
              </Button>
            ) : null}
            <Button variant="secondary" onClick={() => setSection("Regras")}>
              Ver catálogo de regras
            </Button>
          </div>
          <FindingActionPlan
            action={action}
            actionStatus={actionStatus}
            setActionStatus={setActionStatus}
            actionOwner={actionOwner}
            setActionOwner={setActionOwner}
            actionJustification={actionJustification}
            setActionJustification={setActionJustification}
            actionResult={actionResult}
            setActionResult={setActionResult}
            saveAction={saveAction}
            actionEvents={actionEvents}
          />
          <details className="mt-3 text-xs text-slate-400">
            <summary className="cursor-pointer">
              Detalhes técnicos da regra
            </summary>
            <p className="mt-2">Título original: {selected.title}</p>
            <p>
              Regra: {selected.rule_id || selected.finding_type}{" "}
              {selected.rule_version ? `(v${selected.rule_version})` : ""}
            </p>
            <p>Categoria: {selected.category || "—"}</p>
            <p>Impacto: {selected.impact || "—"}</p>
            <p>
              Risco/ressalva: {selected.risk || "revisão humana necessária"}
            </p>
            <p>Resumo original: {selected.summary}</p>
            <p>Recomendação original: {selected.recommendation || "—"}</p>
            <p>Validação original: {selected.validation || "—"}</p>
          </details>
          {selected.references?.length ? (
            <div className="mt-3 text-xs text-slate-400">
              Referências:{" "}
              {selected.references.map((url) => (
                <a
                  key={url}
                  className="mr-2 underline"
                  href={url}
                  target="_blank"
                  rel="noreferrer"
                >
                  PostgreSQL
                </a>
              ))}
            </div>
          ) : null}
          {selected.finding_type.startsWith("inactivity.") ? (
            <p className="mt-3 text-xs text-amber-300">
              Classificação POSSIBLY_INACTIVE — o auditor nunca recomenda DROP,
              TRUNCATE ou exclusão automática.
            </p>
          ) : null}
          {selected.evidence ? (
            <pre className="mt-3 overflow-auto rounded bg-slate-950 p-3 text-xs text-slate-400">
              {JSON.stringify(selected.evidence, null, 2)}
            </pre>
          ) : null}
          {api.hasRole("auditor") ? (
            <div className="mt-4 flex flex-wrap gap-2">
              <Button onClick={() => void triage(selected.id, "acknowledged")}>
                Reconhecer
              </Button>
              <Button onClick={() => void triage(selected.id, "resolved")}>
                Resolver
              </Button>
              <Button
                variant="secondary"
                onClick={() => void triage(selected.id, "open")}
              >
                Reabrir
              </Button>
            </div>
          ) : null}
          {api.hasRole("auditor") ? (
            <div className="mt-4 grid gap-2 sm:grid-cols-3">
              <input
                className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
                aria-label="Responsável"
                placeholder="Responsável"
                value={assignee}
                onChange={(event) => setAssignee(event.target.value)}
              />
              <input
                className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
                aria-label="Prazo"
                type="datetime-local"
                value={dueAt}
                onChange={(event) => setDueAt(event.target.value)}
              />
              <div className="flex gap-2">
                <Button
                  disabled={!assignee.trim() && !dueAt}
                  onClick={() => void saveWorkflow()}
                >
                  Salvar prazo
                </Button>
                {api.currentUser() ? (
                  <Button
                    variant="secondary"
                    onClick={() => setAssignee(api.currentUser())}
                  >
                    Eu
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null}
          {api.hasRole("auditor") ? (
            <div className="mt-4 grid gap-2 sm:grid-cols-3">
              <input
                className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
                aria-label="Motivo da supressão"
                placeholder="Motivo da supressão"
                value={suppressionReason}
                onChange={(event) => setSuppressionReason(event.target.value)}
              />
              <input
                className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
                aria-label="Validade da supressão"
                type="datetime-local"
                value={suppressedUntil}
                onChange={(event) => setSuppressedUntil(event.target.value)}
              />
              <Button
                disabled={!suppressionReason.trim() || !suppressedUntil}
                onClick={() => void suppressSelected()}
              >
                Suprimir com validade
              </Button>
            </div>
          ) : null}
          <h3 className="mt-5 text-sm font-semibold text-slate-200">
            Linha do tempo
          </h3>
          <ul className="mt-2 space-y-1 text-xs text-slate-400">
            {timeline.map((event) => (
              <li key={event.id}>
                {new Date(event.recorded_at).toLocaleString()} ·{" "}
                {event.event_type}
                {event.reason ? ` — ${event.reason}` : ""}
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
    </>
  );
}
