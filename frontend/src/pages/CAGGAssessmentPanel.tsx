import { InventoryDetailAvailability } from "../components/InventoryDetailTabs";
import { inventoryPermalink } from "../lib/inventoryTarget";
import type { CAGGSnapshot, SnapshotCompleteness } from "../types";
import { CAGGAssessmentTabs } from "./parts/CAGGAssessmentTabs";
import { useCAGGAssessment } from "./parts/useCAGGAssessment";

export function CAGGAssessmentPanel({
  cagg,
  environment,
  run,
  snapshot,
}: {
  cagg: CAGGSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const s = useCAGGAssessment({ cagg, environment, run, snapshot });
  const { database, name, schema } = s;
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, {
          kind: "caggs",
          database,
          schema,
          name,
        })}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para este agregado e execução
      </a>
      {snapshot?.completeness === "partial" && (
        <InventoryDetailAvailability state="partial" />
      )}
      {snapshot?.completeness === "empty" && (
        <InventoryDetailAvailability state="empty" />
      )}
      <CAGGAssessmentTabs s={s} />
    </div>
  );
}
