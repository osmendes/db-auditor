import { InventoryDetailAvailability } from "../components/InventoryDetailTabs";
import { inventoryPermalink } from "../lib/inventoryTarget";
import type { FunctionSnapshot, SnapshotCompleteness } from "../types";
import { FunctionAssessmentTabs } from "./parts/FunctionAssessmentTabs";
import { useFunctionAssessment } from "./parts/useFunctionAssessment";

export function FunctionAssessmentPanel({
  fn,
  environment,
  run,
  snapshot,
}: {
  fn: FunctionSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const s = useFunctionAssessment({ fn, environment, run, snapshot });
  const { target } = s;
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para esta sobrecarga e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <FunctionAssessmentTabs s={s} />
    </div>
  );
}
