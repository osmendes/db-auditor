export const statuses = [
  { value: "suggested", label: "Sugerida" },
  { value: "in_review", label: "Em análise" },
  { value: "planned", label: "Planejada" },
  { value: "executed_externally", label: "Executada externamente" },
  { value: "validated", label: "Validada" },
  { value: "discarded", label: "Descartada" },
];

export const qualityLabels: Record<string, string> = {
  null: "Valores ausentes",
  duplicate: "Possível duplicidade",
  orphan: "Referência sem origem",
  date_range: "Data fora do intervalo",
  distribution: "Concentração de valores",
};

export const capabilityLabels: Record<string, string> = {
  catalog: "Catálogo",
  constraints: "Restrições",
  indexes: "Índices",
  workload: "Carga de trabalho",
  security: "Segurança",
  timescale: "TimescaleDB",
  data_quality: "Qualidade de dados",
};

export function statusLabel(value: string): string {
  return statuses.find((item) => item.value === value)?.label ?? value;
}

export function metricLabel(value: string): string {
  switch (value) {
    case "finding_observed":
      return "Achado observado";
    case "table_size_bytes":
      return "Tamanho da tabela";
    case "query_mean_latency_us":
      return "Latência média da consulta (µs)";
    case "query_reads_per_1000_calls":
      return "Blocos lidos por 1.000 chamadas";
    default:
      return value;
  }
}

export function words(raw: string): string[] {
  return raw
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean);
}
