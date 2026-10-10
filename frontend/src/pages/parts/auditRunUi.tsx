import type { AuditRun, CollectorRun } from "../../types";

export type AuditScheduleRow = {
  profile: string;
  enabled: boolean;
  next_run_at?: string;
  last_status?: string;
};

export const POLL_MS = 2500;

export const STATUS_OPTIONS = [
  { value: "", label: "Todos os status" },
  { value: "running", label: "Em execução" },
  { value: "success", label: "Sucesso" },
  { value: "partial_success", label: "Sucesso parcial" },
  { value: "failed", label: "Falhou" },
  { value: "cancelled", label: "Cancelado" },
  { value: "skipped", label: "Ignorado" },
];

export const PROFILE_OPTIONS = [
  { value: "", label: "Todos os perfis" },
  { value: "manual", label: "Manual" },
  { value: "fast", label: "Rápido" },
  { value: "daily", label: "Diário" },
  { value: "weekly", label: "Semanal" },
  { value: "monthly", label: "Mensal" },
];

export const SCHEDULE_PROFILES = [
  { value: "fast", label: "Rápido (15 min)" },
  { value: "daily", label: "Diário" },
  { value: "weekly", label: "Semanal" },
  { value: "monthly", label: "Mensal" },
];

export function nextRunLabel(value?: string): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }
  return date.toLocaleString("pt-BR");
}

export function statusTone(
  status: string,
): "success" | "warning" | "danger" | "neutral" {
  if (status === "success") {
    return "success";
  }
  if (status === "partial_success" || status === "running") {
    return "warning";
  }
  if (status === "failed") {
    return "danger";
  }
  return "neutral";
}

export function envLabel(run: AuditRun): string {
  if (run.environment_name?.trim()) {
    return run.environment_name;
  }
  return `${run.environment_id.slice(0, 8)}…`;
}

function isTerminal(status: string): boolean {
  return (
    status === "success" ||
    status === "partial_success" ||
    status === "failed" ||
    status === "cancelled" ||
    status === "skipped"
  );
}

export function durationLabel(start: string, end?: string | null): string {
  const a = new Date(start).getTime();
  const b = end ? new Date(end).getTime() : Date.now();
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) {
    return "—";
  }
  const sec = Math.round((b - a) / 1000);
  if (sec < 60) {
    return `${sec}s`;
  }
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}m ${s}s`;
}

export function CollectorProgress({
  collectors,
  runStatus,
}: {
  collectors: CollectorRun[];
  runStatus: string;
}) {
  const total = collectors.length;
  const done = collectors.filter((c) => isTerminal(c.status)).length;
  const running = collectors.filter((c) => c.status === "running").length;
  const failed = collectors.filter((c) => c.status === "failed").length;
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  const live = runStatus === "running" || running > 0;

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-slate-300">
        <span>
          {done}/{total || "…"} collectors
          {running > 0 ? ` · ${running} em execução` : ""}
          {failed > 0 ? ` · ${failed} falha(s)` : ""}
        </span>
        <span className="font-mono text-slate-400">
          {total > 0 ? `${pct}%` : live ? "…" : "—"}
          {live ? " · ao vivo" : ""}
        </span>
      </div>
      <div
        className="h-2 overflow-hidden rounded-full bg-slate-800"
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <div
          className={`h-full rounded-full transition-all duration-500 ${
            failed > 0
              ? "bg-rose-500/80"
              : live
                ? "bg-amber-400/90"
                : "bg-emerald-500/80"
          }`}
          style={{ width: `${total > 0 ? pct : live ? 8 : 0}%` }}
        />
      </div>
    </div>
  );
}
