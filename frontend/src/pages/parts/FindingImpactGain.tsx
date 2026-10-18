import {
  impactLabel,
  impactPeriod,
  impactUnit,
  maintenanceKindLabel,
  measuredGainLabel,
} from "../../lib/priority";
import type { FindingAction } from "../../types";

/**
 * Impact with unit and period (AN-03), hypothesis / predicted / measured gain
 * kept apart and maintenance costs (BA-21), and the read-only confirmation
 * query contract (AN-05). Nothing here runs against the audited database.
 */
export function FindingImpactGain({ action }: { action: FindingAction }) {
  const estimate = action.impact_estimate;
  const gain = action.gain;
  const query = action.plan.query;
  const cost = action.plan.maintenance;
  return (
    <div className="mt-3 space-y-3 text-xs text-slate-300">
      <section aria-labelledby="impact-heading">
        <h4 id="impact-heading" className="font-semibold text-slate-100">
          Impacto
        </h4>
        <p className="mt-1">{impactLabel(estimate)}</p>
        <dl className="mt-1 grid gap-1 sm:grid-cols-2">
          <div>
            <dt className="inline font-semibold">Unidade: </dt>
            <dd className="inline">{impactUnit(estimate)}</dd>
          </div>
          <div>
            <dt className="inline font-semibold">Período: </dt>
            <dd className="inline">{impactPeriod(estimate)}</dd>
          </div>
        </dl>
        {estimate?.basis ? (
          <p className="mt-1 text-slate-400">Base: {estimate.basis}</p>
        ) : null}
      </section>
      <section aria-labelledby="gain-heading">
        <h4 id="gain-heading" className="font-semibold text-slate-100">
          Hipótese e ganho
        </h4>
        <dl className="mt-1 space-y-1">
          <div>
            <dt className="font-semibold">Hipótese</dt>
            <dd>
              {gain?.hypothesis ||
                action.plan.meaning ||
                "Hipótese até confirmação humana."}
            </dd>
          </div>
          <div>
            <dt className="font-semibold">Ganho previsto</dt>
            <dd>
              {gain?.predicted ||
                "não estimado: teste a mudança em homologação antes de prever ganho."}
            </dd>
          </div>
          <div>
            <dt className="font-semibold">Ganho medido</dt>
            <dd data-status={gain?.measured_status ?? "not_measured"}>
              {measuredGainLabel(gain)}
            </dd>
          </div>
        </dl>
      </section>
      {cost ? (
        <section aria-labelledby="cost-heading">
          <h4 id="cost-heading" className="font-semibold text-slate-100">
            Custos de {maintenanceKindLabel(cost)} ({cost.capability})
          </h4>
          <dl className="mt-1 grid gap-2 sm:grid-cols-2">
            <div>
              <dt className="font-semibold">Pré-condições</dt>
              <dd>{cost.preconditions}</dd>
            </div>
            <div>
              <dt className="font-semibold">Janela</dt>
              <dd>{cost.window}</dd>
            </div>
            <div>
              <dt className="font-semibold">Leitura</dt>
              <dd>{cost.read_cost}</dd>
            </div>
            <div>
              <dt className="font-semibold">Escrita</dt>
              <dd>{cost.write_cost}</dd>
            </div>
            <div>
              <dt className="font-semibold">Armazenamento</dt>
              <dd>{cost.storage_cost}</dd>
            </div>
            <div>
              <dt className="font-semibold">Risco</dt>
              <dd>{cost.risk}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="font-semibold">Validação posterior</dt>
              <dd>{cost.post_validation}</dd>
            </div>
          </dl>
          {cost.business_approval ? (
            <p className="mt-1 text-amber-300">
              Exige aprovação de negócio/compliance e backup restaurável antes
              de qualquer execução. O auditor não aplica a mudança.
            </p>
          ) : null}
        </section>
      ) : null}
      {query ? (
        <details>
          <summary>Consulta de confirmação somente leitura</summary>
          <dl className="mt-2 grid gap-1 sm:grid-cols-2">
            <div>
              <dt className="inline font-semibold">Mecanismo: </dt>
              <dd className="inline">{query.engine}</dd>
            </div>
            <div>
              <dt className="inline font-semibold">Capacidade: </dt>
              <dd className="inline">{query.capability}</dd>
            </div>
            <div>
              <dt className="inline font-semibold">Permissão mínima: </dt>
              <dd className="inline">{query.min_permission}</dd>
            </div>
            <div>
              <dt className="inline font-semibold">Limites: </dt>
              <dd className="inline">
                até {query.row_limit} linhas · timeout {query.timeout_ms} ms
              </dd>
            </div>
            {query.params.length ? (
              <div className="sm:col-span-2">
                <dt className="inline font-semibold">Parâmetros: </dt>
                <dd className="inline">{query.params.join(", ")}</dd>
              </div>
            ) : null}
          </dl>
          <p className="mt-1 text-amber-300">{query.impact_warning}</p>
          <pre className="mt-2 overflow-auto">{query.sql}</pre>
        </details>
      ) : null}
    </div>
  );
}
