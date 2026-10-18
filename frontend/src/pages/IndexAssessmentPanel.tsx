import { InventoryDetailAvailability } from "../components/InventoryDetailTabs";
import { inventoryPermalink } from "../lib/inventoryTarget";
import type { IndexSnapshot, SnapshotCompleteness } from "../types";
import { IndexAssessmentTabs } from "./parts/IndexAssessmentTabs";
import { useIndexAssessment } from "./parts/useIndexAssessment";

export { comparableIndexScans } from "./parts/useIndexAssessment";

export function IndexAssessmentPanel({
  index,
  environment,
  run,
  snapshot,
}: {
  index: IndexSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const s = useIndexAssessment({ index, environment, run, snapshot });
  const { target } = s;
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para este índice e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <IndexAssessmentTabs s={s} />
    </div>
  );
}
