import { describe, expect, it } from "vitest";

/**
 * UI-06: no page module grows past ~400 lines without a justified extraction.
 * Hooks listed below hold one cohesive state machine; each entry carries a
 * ceiling so further growth still fails and forces another split.
 */
const LIMIT = 400;
const JUSTIFIED: Record<string, { max: number; why: string }> = {
  "./parts/useInventoryExplorer.tsx": {
    max: 500,
    why: "Single inventory state machine (selection, deep link, sheet, detail fetch) shared through InventoryModelProvider; lists and the deep-link resolver already live in useInventoryObjectLists/inventoryResolve.",
  },
  "./parts/useFindingsPage.tsx": {
    max: 430,
    why: "Findings state (filters, bulk triage, assisted action form) kept together because the bulk and single-item actions share selection and refresh; the markup is in FindingsBrowser/FindingDetailCard.",
  },
};

const sources = import.meta.glob<string>("./**/*.tsx", {
  query: "?raw",
  import: "default",
  eager: true,
});

describe("page module size (UI-06)", () => {
  it("finds the page modules", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(20);
  });

  it.each(Object.entries(sources).filter(([path]) => !path.includes(".test.")))(
    "%s stays within its line budget",
    (path, source) => {
      const lines = source.split("\n").length;
      const budget = JUSTIFIED[path]?.max ?? LIMIT;
      expect(
        lines,
        `${path} has ${lines} lines (budget ${budget})`,
      ).toBeLessThanOrEqual(budget);
    },
  );

  it("every justified exception is still needed and explained", () => {
    for (const [path, { why }] of Object.entries(JUSTIFIED)) {
      expect(sources[path], path).toBeDefined();
      expect(sources[path].split("\n").length).toBeGreaterThan(LIMIT);
      expect(why.length).toBeGreaterThan(40);
    }
  });
});
