/**
 * One source for "what this screen does not cover" (UI-08). Dashboard,
 * Findings and Reports feed the same inputs (run status, run coverage and
 * engine capabilities) and therefore show the same sentences.
 */
import type {
  AuditRun,
  AuditRunCoverage,
  EnvironmentCapabilities,
} from "../types";

export type CoverageKind = "partial" | "permission" | "gap" | "not_applicable";

export interface CoverageSignal {
  kind: CoverageKind;
  /** Environment the signal belongs to (shown when several are in scope). */
  environmentName?: string;
  /** Collectors, capabilities or dates behind the signal. */
  detail: string[];
}

export const COVERAGE_MESSAGES: Record<CoverageKind, string> = {
  partial:
    "A coleta está parcial. O score e a ausência de achados não cobrem o que não foi lido.",
  permission:
    "A conta de leitura não tem permissão para parte do catálogo. O que falta não foi auditado.",
  gap: "Há uma lacuna entre coletas. Não trate o intervalo vazio como zero.",
  not_applicable:
    "Parte das verificações não se aplica a este mecanismo. Sem achado nessas áreas não significa resultado saudável.",
};

const COMPLETED = new Set(["success", "partial_success"]);
const ATTEMPT_FAILED = new Set(["failed", "cancelled"]);
// Optional features: off by configuration, not a coverage gap.
const OPTIONAL_CAPABILITIES = new Set(["timescale", "data_quality"]);

export const CAPABILITY_LABELS: Record<string, string> = {
  catalog: "catálogo",
  constraints: "restrições",
  indexes: "índices",
  workload: "carga",
  security: "segurança",
  timescale: "Timescale",
  data_quality: "qualidade de dados",
};

/** The collector runner prefixes failures with `<collector>: <kind>: …`. */
export function isPermissionFailure(item: AuditRunCoverage): boolean {
  const text = `${item.error ?? ""} ${item.warning ?? ""}`.toLowerCase();
  return (
    /:\s*permission:/.test(text) ||
    text.includes("permission denied") ||
    text.includes("42501") ||
    text.includes("sem permissão")
  );
}

function byNewest(a: AuditRun, b: AuditRun) {
  return b.started_at.localeCompare(a.started_at);
}

/** Latest completed run per environment, newest first. */
export function latestCompletedRuns(runs: AuditRun[]): AuditRun[] {
  const seen = new Set<string>();
  const out: AuditRun[] = [];
  for (const run of [...runs].sort(byNewest)) {
    if (!COMPLETED.has(run.status) || seen.has(run.environment_id)) continue;
    seen.add(run.environment_id);
    out.push(run);
  }
  return out;
}

function shortDate(iso: string) {
  const date = new Date(iso);
  return Number.isNaN(date.getTime())
    ? iso
    : date.toLocaleString("pt-BR", {
        day: "2-digit",
        month: "2-digit",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
}

function collectorLabel(item: AuditRunCoverage) {
  return item.database_name
    ? `${item.collector_name} (${item.database_name})`
    : item.collector_name;
}

export interface CoverageInput {
  /** Run the screen reads from (latest completed, or the one selected). */
  run: AuditRun | null;
  /** Every run of the same environment, used to spot a newer failed attempt. */
  runs: AuditRun[];
  coverage: AuditRunCoverage[];
  capabilities: EnvironmentCapabilities | null;
}

export function deriveCoverageSignals({
  run,
  runs,
  coverage,
  capabilities,
}: CoverageInput): CoverageSignal[] {
  const signals: CoverageSignal[] = [];
  const environmentName = run?.environment_name;
  const failed = coverage.filter((item) => item.status === "failed");
  const permission = failed.filter(isPermissionFailure);
  const otherFailures = failed.filter((item) => !isPermissionFailure(item));
  if (
    run &&
    (run.status === "partial_success" ||
      otherFailures.length > 0 ||
      (permission.length === 0 && run.errors.length > 0))
  ) {
    signals.push({
      kind: "partial",
      environmentName,
      detail: otherFailures.map(collectorLabel),
    });
  }
  if (permission.length > 0) {
    signals.push({
      kind: "permission",
      environmentName,
      detail: permission.map(collectorLabel),
    });
  }
  if (run) {
    const newerFailed = runs
      .filter(
        (other) =>
          other.environment_id === run.environment_id &&
          ATTEMPT_FAILED.has(other.status) &&
          other.started_at > run.started_at,
      )
      .sort(byNewest);
    if (newerFailed.length > 0) {
      signals.push({
        kind: "gap",
        environmentName,
        detail: [
          `Última tentativa sem coleta: ${shortDate(newerFailed[0].started_at)} (${newerFailed.length === 1 ? "1 tentativa" : `${newerFailed.length} tentativas`}). Dados da coleta de ${shortDate(run.started_at)}.`,
        ],
      });
    }
  }
  const notApplicable = (capabilities?.items ?? []).filter(
    (item) => !item.applicable && !OPTIONAL_CAPABILITIES.has(item.name),
  );
  if (notApplicable.length > 0) {
    signals.push({
      kind: "not_applicable",
      environmentName,
      detail: notApplicable.map(
        (item) => CAPABILITY_LABELS[item.name] ?? item.name,
      ),
    });
  }
  return signals;
}

/** True when a score from this run must not be read as a full measurement. */
export function scoreIsPartial(signals: CoverageSignal[]): boolean {
  return signals.some(
    (signal) => signal.kind === "partial" || signal.kind === "permission",
  );
}
