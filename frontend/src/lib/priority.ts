import type {
  GainStatement,
  ImpactEstimate,
  MaintenanceCost,
  PriorityRanked,
} from "../types";

/** Unit of an impact estimate; "não informada" when there is no metric. */
export function impactUnit(estimate?: ImpactEstimate | null): string {
  if (estimate?.status !== "estimated" || !estimate.unit) {
    return "não informada";
  }
  if (
    estimate.denominator_unit &&
    estimate.denominator_unit !== estimate.unit
  ) {
    return `${estimate.unit} / ${estimate.denominator_unit}`;
  }
  return estimate.unit;
}

/** Period of an impact estimate, never empty. */
export function impactPeriod(estimate?: ImpactEstimate | null): string {
  return estimate?.period?.trim() || "não informado";
}

/** Label of an impact estimate; a missing estimate reads as not estimated. */
export function impactLabel(estimate?: ImpactEstimate | null): string {
  return estimate?.label?.trim() || "impacto não estimado";
}

export function formatScore(p: PriorityRanked): string {
  return `${p.score.toFixed(2)} / ${p.max_score.toFixed(1)}`;
}

/** One-line, screen-reader friendly description of a ranked item. */
export function priorityDescription(p: PriorityRanked): string {
  return [
    `Prioridade ${p.rank}`,
    `pontuação ${formatScore(p)}`,
    `impacto ${p.impact_level}`,
    `esforço ${p.effort}`,
    `confiança ${p.confidence_label} (${Math.round(p.confidence * 100)}%)`,
  ].join(" · ");
}

export function measuredGainLabel(gain?: GainStatement | null): string {
  if (!gain) {
    return "não medido: exige nova coleta completa e comparável depois da ação externa.";
  }
  return gain.measured;
}

const kindLabels: Record<string, string> = {
  partitioning: "particionamento",
  compression: "compressão",
  retention: "retenção",
  autovacuum: "autovacuum",
  analyze: "ANALYZE/estatísticas",
  reorder: "reordenação",
};

export function maintenanceKindLabel(cost: MaintenanceCost): string {
  return kindLabels[cost.kind] ?? cost.kind;
}
