import {
  COVERAGE_MESSAGES,
  type CoverageKind,
  type CoverageSignal,
} from "../lib/coverage";

const ORDER: CoverageKind[] = [
  "partial",
  "permission",
  "gap",
  "not_applicable",
];

/**
 * The one coverage notice of the product (UI-08). Every screen passes the
 * signals from `useRunCoverage`, so the same run yields the same text on
 * Dashboard, Findings and Reports.
 */
export function CoverageBanner({ signals }: { signals: CoverageSignal[] }) {
  if (signals.length === 0) return null;
  const multiEnv =
    new Set(signals.map((signal) => signal.environmentName ?? "")).size > 1;
  const grouped = ORDER.map((kind) => ({
    kind,
    items: signals.filter((signal) => signal.kind === kind),
  })).filter((group) => group.items.length > 0);
  return (
    <section
      role="status"
      aria-label="Cobertura da coleta"
      data-coverage={grouped.map((group) => group.kind).join(" ")}
      className="space-y-2 rounded-lg border border-amber-700/60 bg-amber-950/40 px-3 py-2 text-sm text-amber-100"
    >
      {grouped.map((group) => (
        <div key={group.kind}>
          <p>{COVERAGE_MESSAGES[group.kind]}</p>
          {group.items.some(
            (item) =>
              item.detail.length > 0 || (multiEnv && item.environmentName),
          ) ? (
            <ul className="mt-1 list-disc space-y-0.5 pl-5 text-xs text-amber-200">
              {group.items.map((item) => (
                <li key={`${item.environmentName ?? ""}-${item.detail.join()}`}>
                  {multiEnv && item.environmentName
                    ? `${item.environmentName}: `
                    : ""}
                  {item.detail.length > 0
                    ? item.detail.join(", ")
                    : "execução marcada como parcial"}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ))}
    </section>
  );
}
