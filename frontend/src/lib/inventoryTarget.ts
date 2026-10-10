import type { EnvironmentCapabilities, InventoryObjectKind } from "../types";

export interface InventoryTarget {
  kind: InventoryObjectKind;
  database: string;
  schema: string;
  name: string;
  /** PostgreSQL functions can have the same name with different arguments. */
  signature?: string;
}

export type LegacyTableTarget = {
  database: string;
  schema: string;
  table: string;
};

export const inventoryKinds: InventoryObjectKind[] = [
  "tables",
  "hypertables",
  "indexes",
  "views",
  "functions",
  "caggs",
];

export function normalizeInventoryTarget(
  target: InventoryTarget | LegacyTableTarget,
): InventoryTarget {
  if ("table" in target) {
    return {
      kind: "tables",
      database: target.database,
      schema: target.schema,
      name: target.table,
    };
  }
  return target;
}

export function inventoryTargetKey(
  environment: string,
  run: string,
  target: InventoryTarget,
): string {
  return JSON.stringify([
    environment,
    run,
    target.kind,
    target.database,
    target.schema,
    target.name,
    target.signature ?? "",
  ]);
}

export function matchesInventorySelection(
  environment: string | null,
  run: string | null,
  requested: InventoryTarget | null,
  selected: InventoryTarget | null,
  pendingKey: string | null,
): boolean {
  if (!environment || !run || !requested || !selected || !pendingKey)
    return false;
  return (
    inventoryTargetKey(environment, run, requested) === pendingKey &&
    inventoryTargetKey(environment, run, selected) === pendingKey
  );
}

export function inventoryRunForSelection(
  requestedRun: string | undefined,
  latestRun: string | undefined,
): string | null {
  return requestedRun || latestRun || null;
}

export function timescaleUnavailableForRun(
  kind: InventoryObjectKind,
  run: string | null,
  capabilities: EnvironmentCapabilities | null,
): boolean {
  return (
    (kind === "hypertables" || kind === "caggs") &&
    !!run &&
    capabilities?.audit_run_id === run &&
    capabilities.items.some(
      (item) => item.name === "timescale" && !item.applicable,
    )
  );
}

export function inventoryTargetFromParams(
  params: URLSearchParams,
): InventoryTarget | null {
  const kind = params.get("kind");
  const database = params.get("database");
  const schema = params.get("schema");
  const name = params.get("name") ?? params.get("table");
  if (!kind || !inventoryKinds.includes(kind as InventoryObjectKind)) {
    const table = params.get("table");
    if (!database || !schema || !table) return null;
    return { kind: "tables", database, schema, name: table };
  }
  if (!database || !schema || !name) return null;
  return {
    kind: kind as InventoryObjectKind,
    database,
    schema,
    name,
    ...(kind === "functions" && params.has("signature")
      ? { signature: params.get("signature") ?? "" }
      : {}),
  };
}

export function inventoryTargetParams(
  target: InventoryTarget,
): URLSearchParams {
  const params = new URLSearchParams({
    kind: target.kind,
    database: target.database,
    schema: target.schema,
    name: target.name,
  });
  if (target.kind === "functions" && target.signature !== undefined) {
    params.set("signature", target.signature);
  }
  return params;
}

export function inventoryPermalink(
  environment: string,
  run: string,
  target: InventoryTarget,
  base = typeof window === "undefined"
    ? "http://localhost/"
    : `${window.location.origin}${window.location.pathname}`,
): string {
  const params = new URLSearchParams({ env: environment, run });
  inventoryTargetParams(target).forEach((value, key) => {
    params.set(key, value);
  });
  return `${base}#/inventory?${params}`;
}
