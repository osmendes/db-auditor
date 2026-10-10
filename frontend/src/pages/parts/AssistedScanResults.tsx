import { Button, Card, Select } from "../../components/ui";
import { api } from "../../services/api";
import type { QualityIssue, QualityScan } from "../../types";
import { qualityLabels, statuses, statusLabel } from "./assistedActionLabels";

export function AssistedScanResults({
  scans,
  chooseIssue,
  selectedIssue,
  issueStatus,
  setIssueStatus,
  owner,
  setOwner,
  justification,
  setJustification,
  result,
  setResult,
  busy,
  saveIssue,
}: {
  scans: QualityScan[];
  chooseIssue: (item: QualityIssue) => void;
  selectedIssue: QualityIssue | null;
  issueStatus: string;
  setIssueStatus: (value: string) => void;
  owner: string;
  setOwner: (value: string) => void;
  justification: string;
  setJustification: (value: string) => void;
  result: string;
  setResult: (value: string) => void;
  busy: boolean;
  saveIssue: () => void;
}) {
  return (
    <>
      <Card
        title="Diagnósticos recentes"
        subtitle="Contagens e decisões, sem valores pessoais ou linhas copiadas."
      >
        {scans.length === 0 ? (
          <p className="mt-3 text-sm text-slate-400">
            Nenhum diagnóstico registrado.
          </p>
        ) : (
          <div className="mt-3 space-y-4">
            {scans.map((scan) => (
              <div
                key={scan.id}
                className="rounded border border-slate-700 p-3 text-sm"
              >
                <p className="font-medium">
                  {scan.database}.{scan.schema}.{scan.table} ·{" "}
                  {new Date(scan.created_at).toLocaleString("pt-BR")}
                </p>
                <p className="text-slate-400">
                  {scan.sampled_rows}/{scan.sample_limit} linhas · amostra
                  {scan.sample_method === "paginas_aleatorias_sistema"
                    ? "de páginas selecionadas pelo PostgreSQL"
                    : "limitada por ordem física"}
                  ; mudanças na amostra podem afetar as contagens.
                </p>
                <p className="text-slate-400">{scan.sampling_note}</p>
                <ul className="mt-2 space-y-2">
                  {scan.issues.map((issue) => (
                    <li key={issue.id} className="rounded bg-slate-800/60 p-2">
                      <p>
                        {qualityLabels[issue.check_kind] ?? issue.check_kind} ·{" "}
                        {issue.column_name}: {issue.affected_rows}/
                        {issue.sampled_rows} · {statusLabel(issue.status)}
                      </p>
                      <p className="text-slate-300">{issue.plan.meaning}</p>
                      <p className="text-slate-400">
                        {issue.comparison_note}
                        {issue.comparable &&
                        issue.previous_affected_rows != null
                          ? ` Antes: ${issue.previous_affected_rows}; agora: ${issue.affected_rows}.`
                          : ""}
                      </p>
                      {issue.validation_scan_id ? (
                        <p className="text-slate-400">
                          Validação vinculada ao diagnóstico{" "}
                          {issue.validation_scan_id}.
                        </p>
                      ) : null}
                      <details className="mt-1">
                        <summary className="cursor-pointer">
                          Roteiro de confirmação e sanitização
                        </summary>
                        <p>Confirmar: {issue.plan.confirmation}</p>
                        <p>Etapas externas: {issue.plan.external_steps}</p>
                        <p>Validar: {issue.plan.validation}</p>
                        <p>Risco: {issue.plan.risk}</p>
                      </details>
                      {api.hasRole("auditor") ? (
                        <Button
                          variant="secondary"
                          onClick={() => chooseIssue(issue)}
                        >
                          Registrar decisão
                        </Button>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}
      </Card>
      {selectedIssue ? (
        <Card
          title={`Decisão sobre ${qualityLabels[selectedIssue.check_kind] ?? selectedIssue.check_kind} em ${selectedIssue.column_name}`}
          subtitle="Registre responsável e evidência. Nenhuma correção é executada aqui."
        >
          <div className="mt-3 space-y-2">
            <Select
              label="Estado"
              value={issueStatus}
              onChange={(e) => setIssueStatus(e.target.value)}
              options={statuses}
            />
            {issueStatus === "validated" ? (
              <p className="text-xs text-amber-200">
                Para validar, repita o diagnóstico após a ação com método,
                limite e quantidade de linhas comparáveis. A nova coleta ficará
                vinculada à decisão.
              </p>
            ) : null}
            <input
              aria-label="Responsável"
              value={owner}
              onChange={(e) => setOwner(e.target.value)}
              placeholder="Responsável"
              className="w-full rounded border border-slate-600 bg-slate-900 p-2"
            />
            <textarea
              aria-label="Justificativa"
              value={justification}
              onChange={(e) => setJustification(e.target.value)}
              placeholder="Justificativa"
              className="w-full rounded border border-slate-600 bg-slate-900 p-2"
            />
            <textarea
              aria-label="Resultado ou evidência externa"
              value={result}
              onChange={(e) => setResult(e.target.value)}
              placeholder="Resultado ou evidência externa"
              className="w-full rounded border border-slate-600 bg-slate-900 p-2"
            />
            <Button disabled={busy} onClick={() => void saveIssue()}>
              Salvar decisão
            </Button>
          </div>
        </Card>
      ) : null}
    </>
  );
}
