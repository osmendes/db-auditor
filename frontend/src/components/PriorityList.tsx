import { labels } from "../lib/labels";
import { formatScore, priorityDescription } from "../lib/priority";
import type { PriorityItem } from "../types";

/**
 * Ranked priorities (AN-02): impact, effort, confidence and risk with the
 * finding id each item points to. Shared by the Dashboard and Findings.
 */
export function PriorityList({
  items,
  openFinding,
}: {
  items: PriorityItem[];
  openFinding: (id: string) => void;
}) {
  return (
    <ol className="mt-3 grid gap-3">
      {items.map(({ priority, finding }) => (
        <li key={priority.finding_id}>
          <button
            type="button"
            className="w-full rounded-xl border border-slate-800 bg-slate-900/60 px-4 py-3 text-left"
            onClick={() => openFinding(priority.finding_id)}
            aria-label={`${finding.friendly_meaning || finding.title}. ${priorityDescription(priority)}`}
          >
            <span className="flex items-baseline justify-between gap-3">
              <span className="text-sm font-medium text-slate-50">
                {priority.rank}. {finding.friendly_meaning || finding.title}
              </span>
              <span className="shrink-0 text-xs tabular-nums text-slate-300">
                {formatScore(priority)}
              </span>
            </span>
            <span className="mt-1 block text-xs text-slate-400">
              {labels.severity(priority.severity)} · impacto{" "}
              {priority.impact_level} · esforço {priority.effort} · confiança{" "}
              {priority.confidence_label} (
              {Math.round(priority.confidence * 100)}%)
            </span>
            <span className="mt-1 block text-xs text-slate-400">
              {priority.impact_label}
            </span>
            <span className="mt-1 block text-xs text-slate-500">
              Risco: {priority.risk} · achado {priority.finding_id}
            </span>
          </button>
        </li>
      ))}
    </ol>
  );
}
