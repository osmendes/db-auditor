import { type KeyboardEvent, type ReactNode, useId } from "react";

export type InventoryDetailTab =
  | "overview"
  | "structure"
  | "relationships"
  | "performance"
  | "security"
  | "recommendations";

export const inventoryDetailTabs: Array<{
  id: InventoryDetailTab;
  label: string;
}> = [
  { id: "overview", label: "Visão geral" },
  { id: "structure", label: "Estrutura" },
  { id: "relationships", label: "Relacionamentos" },
  { id: "performance", label: "Performance" },
  { id: "security", label: "Segurança" },
  { id: "recommendations", label: "Recomendações" },
];

export function nextInventoryTab(
  current: InventoryDetailTab,
  key: string,
): InventoryDetailTab | null {
  const index = inventoryDetailTabs.findIndex((item) => item.id === current);
  let next = index;
  if (key === "ArrowRight") next = (index + 1) % inventoryDetailTabs.length;
  else if (key === "ArrowLeft")
    next =
      (index - 1 + inventoryDetailTabs.length) % inventoryDetailTabs.length;
  else if (key === "Home") next = 0;
  else if (key === "End") next = inventoryDetailTabs.length - 1;
  else return null;
  return inventoryDetailTabs[next].id;
}

export type DetailAvailability =
  | "empty"
  | "partial"
  | "not_applicable"
  | "not_collected"
  | "forbidden"
  | "error";

const availabilityText: Record<DetailAvailability, string> = {
  empty: "Nenhuma informação encontrada nesta coleta.",
  partial: "A coleta foi parcial. Estas informações podem estar incompletas.",
  not_applicable:
    "Esta análise não se aplica a este tipo de objeto ou mecanismo.",
  not_collected:
    "Esta informação ainda não é coletada para este tipo de objeto.",
  forbidden: "Sua conta não tem permissão para consultar estas informações.",
  error: "Não foi possível carregar estas informações. Tente novamente.",
};

export function InventoryDetailAvailability({
  state,
  detail,
}: {
  state: DetailAvailability;
  detail?: string;
}) {
  return (
    <p
      role={state === "error" ? "alert" : "status"}
      className="rounded border border-slate-700 bg-slate-900/60 p-3 text-sm text-slate-300"
    >
      {detail ?? availabilityText[state]}
    </p>
  );
}

export function InventoryDetailTabs({
  tab,
  onTabChange,
  children,
}: {
  tab: InventoryDetailTab;
  onTabChange: (tab: InventoryDetailTab) => void;
  children: ReactNode;
}) {
  const id = useId();
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const nextTab = nextInventoryTab(tab, event.key);
    if (!nextTab) return;
    const next = inventoryDetailTabs.findIndex((item) => item.id === nextTab);
    event.preventDefault();
    onTabChange(nextTab);
    const buttons =
      event.currentTarget.querySelectorAll<HTMLButtonElement>("[role=tab]");
    buttons[next]?.focus();
  };

  return (
    <>
      <div
        role="tablist"
        aria-label="Seções do inventário"
        className="flex flex-wrap gap-1 border-b border-slate-700 pb-2"
        onKeyDown={onKeyDown}
      >
        {inventoryDetailTabs.map((item) => (
          <button
            key={item.id}
            id={`${id}-tab-${item.id}`}
            aria-controls={`${id}-panel-${item.id}`}
            type="button"
            role="tab"
            tabIndex={tab === item.id ? 0 : -1}
            aria-selected={tab === item.id}
            onClick={() => onTabChange(item.id)}
            className={`rounded px-2 py-1 text-xs ${tab === item.id ? "bg-cyan-800 text-white" : "text-slate-300 hover:bg-slate-800"}`}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div
        role="tabpanel"
        id={`${id}-panel-${tab}`}
        aria-labelledby={`${id}-tab-${tab}`}
      >
        {children}
      </div>
    </>
  );
}
