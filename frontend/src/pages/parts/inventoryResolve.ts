import type { InventoryTarget } from "../../lib/inventoryTarget";
import { api } from "../../services/api";
import type { PageMeta } from "../../types";
import type { SelectedInventoryItem } from "./inventoryUi";

/**
 * Resolves a deep-linked inventory object (`#/inventory/...`) to the row the
 * explorer opens. Pages through at most 2.000 objects (UI-06 extraction).
 */
export async function resolveInventorySelection(
  envId: string,
  run: string,
  inventory: InventoryTarget,
  signal?: AbortSignal,
): Promise<SelectedInventoryItem | null> {
  const params = {
    audit_run_id: run,
    database: inventory.database,
    schema: inventory.schema,
    q: inventory.name,
  };
  const findPage = async <T>(
    load: (offset: number) => Promise<{ items: T[]; page: PageMeta }>,
    matches: (item: T) => boolean,
  ): Promise<T | null> => {
    let offset = 0;
    for (let attempt = 0; attempt < 20; attempt++) {
      signal?.throwIfAborted();
      const result = await load(offset);
      const found = result.items.find(matches);
      if (found) return found;
      if (!result.page.has_more) return null;
      offset += result.items.length;
    }
    throw new Error("Busca limitada a 2.000 objetos. Refine o link.");
  };
  const resolve = async (): Promise<SelectedInventoryItem | null> => {
    switch (inventory.kind) {
      case "tables": {
        const result = await api.tableAssessment(
          envId,
          run,
          inventory.database,
          inventory.schema,
          inventory.name,
          signal,
        );
        return { kind: "tables", item: result.table };
      }
      case "indexes": {
        const item = await findPage(
          (offset) =>
            api.indexes(envId, { ...params, limit: 100, offset }, signal),
          (value) =>
            value.database_name === inventory.database &&
            value.schema_name === inventory.schema &&
            value.index_name === inventory.name,
        );
        return item ? { kind: "indexes", item } : null;
      }
      case "views": {
        const item = await findPage(
          (offset) =>
            api.views(envId, { ...params, limit: 100, offset }, signal),
          (value) =>
            value.database_name === inventory.database &&
            value.schema_name === inventory.schema &&
            value.view_name === inventory.name,
        );
        return item ? { kind: "views", item } : null;
      }
      case "caggs": {
        const item = await findPage(
          (offset) =>
            api.continuousAggregatesPage(
              envId,
              {
                ...params,
                limit: 100,
                offset,
              },
              signal,
            ),
          (value) =>
            value.database_name === inventory.database &&
            value.schema_name === inventory.schema &&
            value.view_name === inventory.name,
        );
        if (!item) return null;
        return { kind: "caggs", item };
      }
      case "functions": {
        const item = await findPage(
          (offset) =>
            api.functions(envId, { ...params, limit: 100, offset }, signal),
          (value) =>
            value.database_name === inventory.database &&
            value.schema_name === inventory.schema &&
            value.function_name === inventory.name &&
            value.identity_arguments === (inventory.signature ?? ""),
        );
        return item ? { kind: "functions", item } : null;
      }
      case "hypertables": {
        const item = await findPage(
          (offset) =>
            api.hypertablesPage(
              envId,
              { ...params, limit: 100, offset },
              signal,
            ),
          (value) =>
            value.database_name === inventory.database &&
            value.schema_name === inventory.schema &&
            value.hypertable_name === inventory.name,
        );
        return item ? { kind: "hypertables", item } : null;
      }
    }
  };
  return resolve();
}
