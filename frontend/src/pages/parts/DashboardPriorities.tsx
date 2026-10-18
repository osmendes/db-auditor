import { PriorityList } from "../../components/PriorityList";
import { Button, EmptyState } from "../../components/ui";
import type { NavigationSection, PriorityItem } from "../../types";

export function DashboardPriorities({
  priorities,
  formula,
  setSection,
  openFinding,
}: {
  priorities: PriorityItem[];
  formula?: string;
  setSection: (section: NavigationSection) => void;
  openFinding: (id: string | null) => void;
}) {
  return (
    <section className="mt-8" aria-labelledby="priority-heading">
      <h2 id="priority-heading" className="text-sm font-medium text-slate-300">
        Cinco prioridades
      </h2>
      <p className="mt-1 text-xs text-slate-500">
        Severidade × impacto medido × confiança ÷ esforço
        {formula ? ` (${formula})` : ""}. Impacto sem métrica não eleva a
        prioridade.
      </p>
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
        <PriorityList items={priorities} openFinding={openFinding} />
      )}
    </section>
  );
}
