import { InventoryDetailAvailability } from "../components/InventoryDetailTabs";
import { inventoryPermalink } from "../lib/inventoryTarget";
import type { SnapshotCompleteness, ViewSnapshot } from "../types";
import { useViewAssessment } from "./parts/useViewAssessment";
import { ViewAssessmentTabs } from "./parts/ViewAssessmentTabs";

export function ViewAssessmentPanel({
  view,
  environment,
  run,
  snapshot,
}: {
  view: ViewSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const s = useViewAssessment({ view, environment, run, snapshot });
  const { target } = s;
  return (
    <div className="space-y-4">
      <a
        href={inventoryPermalink(environment, run, target)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para esta visão e execução
      </a>
      {snapshot?.completeness === "partial" ? (
        <InventoryDetailAvailability state="partial" />
      ) : null}
      {snapshot?.completeness === "empty" ? (
        <InventoryDetailAvailability state="empty" />
      ) : null}
      <ViewAssessmentTabs s={s} />
    </div>
  );
}
