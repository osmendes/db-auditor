import { Button, EmptyState } from "../../components/ui";
import type { Finding, NavigationSection } from "../../types";

export function DashboardPriorities({
  priorities,
  setSection,
  openFinding,
}: {
  priorities: Finding[];
  setSection: (section: NavigationSection) => void;
  openFinding: (id: string | null) => void;
}) {
  return (
    <section className="mt-8" aria-labelledby="priority-heading">
      <h2 id="priority-heading" className="text-sm font-medium text-slate-300">
        Cinco prioridades
      </h2>
      {priorities.length === 0 ? (
        <div className="mt-3">
          <EmptyState
            title="Nenhuma prioridade nesta semana"
            description="Não há achado aberto com evidência suficiente. Isso não significa ausência de risco se a coleta estiver parcial."
            action={
              <Button type="button" onClick={() => setSection("Findings")}>
                Abrir achados
              </Button>
            }
          />
        </div>
      ) : (
        <ul className="mt-3 grid gap-3">
          {priorities.map((item) => (
            <li key={item.id}>
              <button
                type="button"
                className="w-full rounded-xl border border-slate-800 bg-slate-900/60 px-4 py-3 text-left"
                onClick={() => openFinding(item.id)}
              >
                <span className="text-sm font-medium text-slate-50">
                  {item.title}
                </span>
                <span className="mt-1 block text-xs text-slate-400">
                  {item.severity} · confiança{" "}
                  {item.confidence != null
                    ? `${Math.round(item.confidence * 100)}%`
                    : "baixa"}{" "}
                  · {item.summary || "impacto não estimado"}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
