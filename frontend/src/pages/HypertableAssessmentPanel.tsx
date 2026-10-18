import { InventoryDetailAvailability } from "../components/InventoryDetailTabs";
import { inventoryPermalink } from "../lib/inventoryTarget";
import type { HypertableSnapshot, SnapshotCompleteness } from "../types";
import { HypertableAssessmentTabs } from "./parts/HypertableAssessmentTabs";
import { useHypertableAssessment } from "./parts/useHypertableAssessment";

export function HypertableAssessmentPanel({
  hypertable,
  environment,
  run,
  snapshot,
}: {
  hypertable: HypertableSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const s = useHypertableAssessment({ hypertable, environment, run, snapshot });
  const { target } = s;
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para esta tabela temporal e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <HypertableAssessmentTabs s={s} />
    </div>
  );
}
