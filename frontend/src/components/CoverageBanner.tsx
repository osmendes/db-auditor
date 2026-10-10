export function CoverageBanner({
  kind,
}: {
  kind: "partial" | "gap" | "permission" | null;
}) {
  if (!kind) return null;
  const text = {
    partial:
      "A coleta está parcial. O score e a ausência de achados não cobrem o que não foi lido.",
    gap: "Há uma lacuna entre coletas. Não trate o intervalo vazio como zero.",
    permission:
      "A conta de leitura não tem permissão para parte do catálogo. O que falta não foi auditado.",
  }[kind];
  return (
    <p
      role="status"
      className="rounded-lg border border-amber-700/60 bg-amber-950/40 px-3 py-2 text-sm text-amber-100"
    >
      {text}
    </p>
  );
}
