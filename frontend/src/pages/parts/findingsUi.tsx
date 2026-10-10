export function severityTone(
  severity: string,
): "success" | "warning" | "danger" | "neutral" {
  if (severity === "critical" || severity === "high") {
    return "danger";
  }
  if (severity === "medium") {
    return "warning";
  }
  return "neutral";
}

export function statusTone(
  status: string,
): "success" | "warning" | "danger" | "neutral" {
  if (status === "open") {
    return "danger";
  }
  if (status === "acknowledged") {
    return "warning";
  }
  if (status === "resolved" || status === "suppressed") {
    return "success";
  }
  return "neutral";
}

export const SEVERITY_OPTIONS = [
  { value: "", label: "Todas as severidades" },
  { value: "critical", label: "Crítica" },
  { value: "high", label: "Alta" },
  { value: "medium", label: "Média" },
  { value: "low", label: "Baixa" },
  { value: "info", label: "Informativo" },
];

export const STATUS_OPTIONS = [
  { value: "", label: "Todos os status" },
  { value: "open", label: "Aberto" },
  { value: "acknowledged", label: "Reconhecido" },
  { value: "resolved", label: "Resolvido" },
  { value: "suppressed", label: "Suprimido" },
];

export function FilterChip({
  label,
  onClear,
}: {
  label: string;
  onClear: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClear}
      className="inline-flex items-center gap-1 rounded-full border border-slate-600 bg-slate-800/80 px-2.5 py-0.5 text-[11px] text-slate-200 hover:border-slate-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50"
    >
      {label}
      <span className="text-slate-400" aria-hidden>
        ×
      </span>
    </button>
  );
}
