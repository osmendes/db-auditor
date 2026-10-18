import type { InventoryObjectKind } from "../types";
import type { SearchHit } from "../types/openapi";
import type { InventoryTarget } from "./inventoryTarget";

/** UI-07: global search helpers (pure, unit-tested). */

export const SEARCH_MIN_CHARS = 2;
export const SEARCH_MAX_CHARS = 128;
export const SEARCH_DEBOUNCE_MS = 250;

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUUID(value: string): boolean {
  return UUID_RE.test(value.trim());
}

/** Returns the trimmed query when it is searchable, otherwise null. */
export function searchableQuery(value: string): string | null {
  const q = value.trim();
  const length = [...q].length;
  return length >= SEARCH_MIN_CHARS && length <= SEARCH_MAX_CHARS ? q : null;
}

export type SearchAction =
  | { type: "finding"; id: string; environmentId?: string }
  | { type: "run"; id: string; environmentId?: string }
  | { type: "rule"; ruleId: string }
  | {
      type: "inventory";
      target: InventoryTarget;
      run?: string;
      environmentId?: string;
    };

const inventoryKinds: InventoryObjectKind[] = [
  "tables",
  "indexes",
  "views",
  "functions",
];

export function actionForHit(hit: SearchHit): SearchAction | null {
  const environmentId = hit.environment_id || undefined;
  switch (hit.kind) {
    case "finding":
      return { type: "finding", id: hit.id, environmentId };
    case "run":
      return { type: "run", id: hit.audit_run_id || hit.id, environmentId };
    case "rule":
      return { type: "rule", ruleId: hit.rule_id || hit.id };
    case "inventory": {
      const kind = hit.object_kind as InventoryObjectKind;
      if (
        !inventoryKinds.includes(kind) ||
        !hit.database ||
        !hit.schema ||
        !hit.name
      ) {
        return null;
      }
      const target: InventoryTarget = {
        kind,
        database: hit.database,
        schema: hit.schema,
        name: hit.name,
        ...(kind === "functions" ? { signature: hit.signature ?? "" } : {}),
      };
      return {
        type: "inventory",
        target,
        run: hit.audit_run_id || undefined,
        environmentId,
      };
    }
    default:
      return null;
  }
}

export const groupLabels: Record<string, string> = {
  finding: "Findings",
  run: "Execuções",
  inventory: "Inventário",
  rule: "Regras",
};

const groupOrder = ["finding", "run", "inventory", "rule"];

export interface SearchGroup {
  kind: string;
  label: string;
  hits: SearchHit[];
}

/** Groups hits in a stable order; exact matches stay first in each group. */
export function groupHits(hits: SearchHit[]): SearchGroup[] {
  return groupOrder
    .map((kind) => ({
      kind,
      label: groupLabels[kind] ?? kind,
      hits: hits
        .filter((hit) => hit.kind === kind)
        .sort((a, b) => Number(b.exact) - Number(a.exact)),
    }))
    .filter((group) => group.hits.length > 0);
}

/** The hit to open automatically: a single exact UUID match. */
export function autoOpenHit(hits: SearchHit[]): SearchHit | null {
  const exact = hits.filter((hit) => hit.exact);
  if (exact.length !== 1) return null;
  const [hit] = exact;
  return hit.kind === "finding" || hit.kind === "run" ? hit : null;
}

export interface SearchNavigator {
  environmentId: string | null;
  setEnvironmentId: (id: string | null) => void;
  openFinding: (id: string | null) => void;
  openRun: (id: string) => void;
  openInventory: (target: InventoryTarget, run?: string) => void;
  setSection: (section: "Findings") => void;
  setSearch: (patch: Record<string, string | null>) => void;
}

/** Routes a search action through the app context. */
export function applySearchAction(
  action: SearchAction,
  nav: SearchNavigator,
): void {
  if (
    action.type !== "rule" &&
    action.environmentId &&
    action.environmentId !== nav.environmentId
  ) {
    nav.setEnvironmentId(action.environmentId);
  }
  switch (action.type) {
    case "finding":
      nav.openFinding(action.id);
      return;
    case "run":
      nav.openRun(action.id);
      return;
    case "inventory":
      nav.openInventory(action.target, action.run);
      return;
    case "rule":
      nav.setSection("Findings");
      nav.setSearch({ view: "rules", rule: action.ruleId });
      return;
  }
}
