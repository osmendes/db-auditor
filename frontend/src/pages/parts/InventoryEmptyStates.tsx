import { EmptyState } from "../../components/ui";
import { useEmptyContext } from "../../hooks/useEmptyContext";
import { type EmptySubject, resolveEmptyCause } from "../../lib/emptyCause";
import { KIND_LABELS } from "./inventoryUi";
import { type InventoryModel, useInventoryModel } from "./useInventoryExplorer";

export const INVENTORY_SUBJECT: EmptySubject = {
  none: "Nenhum objeto de inventário",
  plural: "objetos de inventário",
};

/** Rows of the list shown for the current kind (views and CAGGs share one). */
export function inventoryItemsFor(
  model: Pick<
    InventoryModel,
    | "kind"
    | "sortedTables"
    | "sortedIndexes"
    | "sortedViews"
    | "sortedFunctions"
    | "sortedHypertables"
  >,
): readonly unknown[] {
  switch (model.kind) {
    case "tables":
      return model.sortedTables;
    case "indexes":
      return model.sortedIndexes;
    case "views":
    case "caggs":
      return model.sortedViews;
    case "functions":
      return model.sortedFunctions;
    case "hypertables":
      return model.sortedHypertables;
  }
}

/** The environment has no inventory snapshot: never collected, no DSN or zero databases (UI-09). */
export function InventorySnapshotEmpty() {
  const { envId, setSection } = useInventoryModel();
  const context = useEmptyContext(envId ?? null, true);
  const cause = resolveEmptyCause({
    filtered: false,
    dsnMissing: context.dsnMissing,
    collected: context.collected,
  });
  return (
    <EmptyState
      cause={cause}
      subject={INVENTORY_SUBJECT}
      onAction={() => setSection(cause === "dsn" ? "Ambientes" : "Execuções")}
      actionDisabled={context.loading}
    />
  );
}

/** The snapshot exists but the selected list is empty: filter vs. zero (UI-09). */
export function InventoryListEmpty() {
  const {
    kind,
    isMongo,
    q,
    selectedDb,
    selectedSchema,
    setQ,
    setSelectedDb,
    setSelectedSchema,
    setOffset,
    setSection,
  } = useInventoryModel();
  const filtered = Boolean(q.trim() || selectedDb || selectedSchema);
  const label = (
    isMongo && kind === "tables" ? "Coleções" : KIND_LABELS[kind]
  ).toLowerCase();
  const cause = filtered ? "filter" : "zero";
  return (
    <EmptyState
      cause={cause}
      subject={{ none: `Nenhum item em ${label}`, plural: label }}
      onAction={() => {
        if (cause === "filter") {
          setQ("");
          setSelectedDb(null);
          setSelectedSchema(null);
          setOffset(0);
        } else {
          setSection("Execuções");
        }
      }}
    />
  );
}
