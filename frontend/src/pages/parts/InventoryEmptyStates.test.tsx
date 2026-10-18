// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { EMPTY_CAUSE_ACTION } from "../../lib/emptyCause";
import { cleanup, click, flush, render } from "../../test/dom";
import {
  InventoryListEmpty,
  InventorySnapshotEmpty,
  inventoryItemsFor,
} from "./InventoryEmptyStates";

const model = {
  envId: "env-no-dsn" as string | null,
  kind: "tables" as string,
  isMongo: false,
  q: "",
  selectedDb: null as string | null,
  selectedSchema: null as string | null,
  setQ: vi.fn(),
  setSelectedDb: vi.fn(),
  setSelectedSchema: vi.fn(),
  setOffset: vi.fn(),
  setSection: vi.fn(),
};

vi.mock("./useInventoryExplorer", () => ({
  useInventoryModel: () => model,
}));

vi.mock("../../services/api", () => ({
  api: {
    auditRuns: async (params?: { environment_id?: string }) => ({
      items:
        params?.environment_id === "env-collected"
          ? [{ id: "r1", status: "success" }]
          : [],
    }),
    connectionStatus: async () => ({
      items: [
        { environment_id: "env-no-dsn", dsn_configured: false },
        { environment_id: "env-new", dsn_configured: true },
        { environment_id: "env-collected", dsn_configured: true },
      ],
    }),
  },
}));

afterEach(async () => {
  await cleanup();
  vi.clearAllMocks();
  Object.assign(model, {
    envId: "env-no-dsn",
    kind: "tables",
    q: "",
    selectedDb: null,
    selectedSchema: null,
  });
});

const causeOf = (root: ParentNode) =>
  root.querySelector("[data-empty-cause]")?.getAttribute("data-empty-cause");

describe("Inventory empty states (UI-09)", () => {
  it.each([
    ["env-no-dsn", "dsn", "Ambientes"],
    ["env-new", "never", "Execuções"],
    ["env-collected", "zero", "Execuções"],
  ])("snapshot of %s is empty because of %s", async (envId, cause, section) => {
    model.envId = envId;
    const { container } = await render(<InventorySnapshotEmpty />);
    await flush();
    await flush();
    expect(causeOf(container)).toBe(cause);
    const button = container.querySelector("button");
    expect(button?.textContent).toBe(
      EMPTY_CAUSE_ACTION[cause as keyof typeof EMPTY_CAUSE_ACTION],
    );
    click(button);
    expect(model.setSection).toHaveBeenCalledWith(section);
  });

  it("an empty list under a filter offers to clear it", async () => {
    model.q = "orders";
    model.selectedSchema = "public";
    const { container } = await render(<InventoryListEmpty />);
    expect(causeOf(container)).toBe("filter");
    expect(container.textContent).toContain("Nenhum item em tabelas");
    click(container.querySelector("button"));
    expect(model.setQ).toHaveBeenCalledWith("");
    expect(model.setSelectedDb).toHaveBeenCalledWith(null);
    expect(model.setSelectedSchema).toHaveBeenCalledWith(null);
    expect(model.setOffset).toHaveBeenCalledWith(0);
  });

  it("an empty list without filters is a zero result of the collection", async () => {
    model.kind = "functions";
    const { container } = await render(<InventoryListEmpty />);
    expect(causeOf(container)).toBe("zero");
    expect(container.textContent).toContain("funções");
    click(container.querySelector("button"));
    expect(model.setSection).toHaveBeenCalledWith("Execuções");
  });

  it("views and continuous aggregates share the same rows", () => {
    const lists = {
      sortedTables: [1],
      sortedIndexes: [1, 2],
      sortedViews: [1, 2, 3],
      sortedFunctions: [],
      sortedHypertables: [1, 2, 3, 4],
    };
    const count = (kind: string) =>
      // biome-ignore lint/suspicious/noExplicitAny: minimal model stub
      inventoryItemsFor({ ...lists, kind } as any).length;
    expect(count("tables")).toBe(1);
    expect(count("indexes")).toBe(2);
    expect(count("views")).toBe(3);
    expect(count("caggs")).toBe(3);
    expect(count("functions")).toBe(0);
    expect(count("hypertables")).toBe(4);
  });
});
