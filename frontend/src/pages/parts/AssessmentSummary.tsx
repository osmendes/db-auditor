import type { TableAssessment } from "../../types";

/** Score and run notice shared by the table assessment (UI-06 extraction). */
export function AssessmentScoreSummary({
  summary,
}: {
  summary: TableAssessment["summary"];
}) {
  if (summary.score_status !== "available" || summary.score === null) {
    return (
      <p role="status" className="text-sm text-amber-300">
        Score indisponível: cobertura insuficiente neste run
        {summary.score_missing_collectors.length > 0
          ? ` (${summary.score_missing_collectors.join(", ")})`
          : ""}
        .
      </p>
    );
  }
  return (
    <div className="space-y-2 text-sm">
      <p>
        <strong>{summary.score}/100</strong> · {summary.score_version} ·
        confiança {Math.round((summary.score_confidence ?? 0) * 100)}%
      </p>
      <p className="text-xs text-slate-400">
        Base estrutural 100; os fatores abaixo reduzem a pontuação. Findings
        aparecem separadamente porque seu histórico ainda não é imutável.
      </p>
      {summary.score_factors.length === 0 ? (
        <p className="text-xs text-slate-400">
          Nenhuma penalidade estrutural identificada no snapshot.
        </p>
      ) : (
        <ul className="space-y-1" aria-label="Fatores do score">
          {summary.score_factors.map((factor) => (
            <li key={factor.code}>
              {factor.description}: −{factor.penalty}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function AssessmentRunNotice({ partial }: { partial: boolean }) {
  if (!partial) return null;
  return (
    <p className="rounded border border-amber-600 p-2 text-xs text-amber-300">
      Run parcial: a ausência de dados não confirma ausência de problemas.
    </p>
  );
}
