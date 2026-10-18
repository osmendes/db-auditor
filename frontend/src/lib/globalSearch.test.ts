import { describe, expect, it, vi } from "vitest";
import type { SearchHit } from "../types/openapi";
import {
  actionForHit,
  applySearchAction,
  autoOpenHit,
  groupHits,
  isUUID,
  type SearchNavigator,
  searchableQuery,
} from "./globalSearch";

const uuid = "0f8fad5b-d9cb-469f-a165-70867728950e";
const hit = (over: Partial<SearchHit>): SearchHit => ({
  kind: "finding",
  id: "id",
  title: "t",
  subtitle: "s",
  exact: false,
  ...over,
});

describe("global search helpers (UI-07)", () => {
  it("detects UUIDs and bounds the query", () => {
    expect(isUUID(` ${uuid.toUpperCase()} `)).toBe(true);
    expect(isUUID("0f8fad5b-d9cb")).toBe(false);
    expect(searchableQuery(" a ")).toBeNull();
    expect(searchableQuery(" pedidos ")).toBe("pedidos");
    expect(searchableQuery("x".repeat(129))).toBeNull();
    expect(searchableQuery("é".repeat(128))).toBe("é".repeat(128));
  });

  it("maps each hit kind to a navigation action", () => {
    expect(actionForHit(hit({ id: uuid, environment_id: "e1" }))).toEqual({
      type: "finding",
      id: uuid,
      environmentId: "e1",
    });
    expect(
      actionForHit(hit({ kind: "run", id: "r1", audit_run_id: "r1" })),
    ).toEqual({ type: "run", id: "r1", environmentId: undefined });
    expect(
      actionForHit(
        hit({ kind: "rule", id: "performance.x", rule_id: "performance.x" }),
      ),
    ).toEqual({ type: "rule", ruleId: "performance.x" });
    expect(
      actionForHit(
        hit({
          kind: "inventory",
          object_kind: "functions",
          database: "app",
          schema: "public",
          name: "fn",
          signature: "integer",
          audit_run_id: "r2",
          environment_id: "e2",
        }),
      ),
    ).toEqual({
      type: "inventory",
      target: {
        kind: "functions",
        database: "app",
        schema: "public",
        name: "fn",
        signature: "integer",
      },
      run: "r2",
      environmentId: "e2",
    });
    const table = actionForHit(
      hit({
        kind: "inventory",
        object_kind: "tables",
        database: "app",
        schema: "public",
        name: "orders",
      }),
    );
    expect(table).toMatchObject({ target: { kind: "tables", name: "orders" } });
    expect(table?.type === "inventory" && "signature" in table.target).toBe(
      false,
    );
    expect(
      actionForHit(
        hit({
          kind: "inventory",
          object_kind: "chunks",
          database: "a",
          schema: "b",
          name: "c",
        }),
      ),
    ).toBeNull();
    expect(
      actionForHit(hit({ kind: "inventory", object_kind: "tables" })),
    ).toBeNull();
    expect(actionForHit(hit({ kind: "other" }))).toBeNull();
  });

  it("groups in a stable order with exact matches first", () => {
    const groups = groupHits([
      hit({ kind: "rule", id: "r" }),
      hit({ kind: "finding", id: "a" }),
      hit({ kind: "finding", id: "b", exact: true }),
      hit({ kind: "inventory", id: "i" }),
      hit({ kind: "run", id: "x" }),
    ]);
    expect(groups.map((g) => g.label)).toEqual([
      "Findings",
      "Execuções",
      "Inventário",
      "Regras",
    ]);
    expect(groups[0].hits.map((h) => h.id)).toEqual(["b", "a"]);
  });

  it("auto-opens only a single exact finding or run", () => {
    expect(autoOpenHit([hit({ id: uuid, exact: true })])?.id).toBe(uuid);
    expect(autoOpenHit([hit({ kind: "run", exact: true })])?.kind).toBe("run");
    expect(autoOpenHit([hit({ exact: false })])).toBeNull();
    expect(
      autoOpenHit([hit({ exact: true }), hit({ kind: "run", exact: true })]),
    ).toBeNull();
  });

  it("routes actions through the app context", () => {
    const nav: SearchNavigator = {
      environmentId: "e1",
      setEnvironmentId: vi.fn(),
      openFinding: vi.fn(),
      openRun: vi.fn(),
      openInventory: vi.fn(),
      setSection: vi.fn(),
      setSearch: vi.fn(),
    };
    applySearchAction({ type: "finding", id: uuid, environmentId: "e1" }, nav);
    expect(nav.setEnvironmentId).not.toHaveBeenCalled();
    expect(nav.openFinding).toHaveBeenCalledWith(uuid);
    applySearchAction({ type: "run", id: "r", environmentId: "e2" }, nav);
    expect(nav.setEnvironmentId).toHaveBeenCalledWith("e2");
    expect(nav.openRun).toHaveBeenCalledWith("r");
    const target = {
      kind: "tables" as const,
      database: "app",
      schema: "public",
      name: "orders",
    };
    applySearchAction({ type: "inventory", target, run: "r9" }, nav);
    expect(nav.openInventory).toHaveBeenCalledWith(target, "r9");
    applySearchAction({ type: "rule", ruleId: "performance.x" }, nav);
    expect(nav.setSection).toHaveBeenCalledWith("Findings");
    expect(nav.setSearch).toHaveBeenCalledWith({
      view: "rules",
      rule: "performance.x",
    });
  });
});
