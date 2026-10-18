import type { RegressionAlert } from "../../types";

const labels: Record<RegressionAlert["category"], string> = {
  findings: "Achados altos ou críticos",
  structure: "Tabelas ou índices removidos ou alterados",
  workload: "Consultas lentas ou pesadas novas",
};

export function regressionCategoryLabel(category: string): string {
  return labels[category as RegressionAlert["category"]] ?? category;
}
