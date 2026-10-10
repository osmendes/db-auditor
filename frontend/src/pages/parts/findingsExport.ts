import { downloadCSV, downloadJSON } from "../../lib/export";
import type { Finding } from "../../types";

export function exportFindingsCSV(sortedItems: Finding[]) {
  const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-");
  const rows = sortedItems.map((f) => ({
    id: f.id,
    finding_type: f.finding_type,
    severity: f.severity,
    status: f.status,
    title: f.title,
    summary: f.summary,
    object_key: f.object_key ?? "",
    environment_id: f.environment_id ?? "",
    last_seen_at: f.last_seen_at ?? "",
  }));
  downloadCSV(
    `findings-${stamp}.csv`,
    [
      "id",
      "finding_type",
      "severity",
      "status",
      "title",
      "summary",
      "object_key",
      "environment_id",
      "last_seen_at",
    ],
    rows,
  );
}

export function exportFindingsJSON(
  sortedItems: Finding[],
  filters: {
    environment_id: string | null;
    severity: string | null;
    status: string | null;
  },
) {
  const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-");
  downloadJSON(`findings-${stamp}.json`, {
    exported_at: new Date().toISOString(),
    filters,
    total: sortedItems.length,
    items: sortedItems,
  });
}
