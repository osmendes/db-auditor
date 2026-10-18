import { describe, expect, it, vi } from "vitest";

const indexes = vi.fn();
vi.mock("../../services/api", () => ({
  api: { indexes: (...args: unknown[]) => indexes(...args) },
}));

import { resolveInventorySelection } from "./inventoryResolve";

const target = {
  kind: "indexes" as const,
  database: "app",
  schema: "public",
  name: "orders_pkey",
};
const row = (name: string) => ({
  id: name,
  database_name: "app",
  schema_name: "public",
  index_name: name,
});

describe("resolveInventorySelection (UI-06 extraction)", () => {
  it("pages until the deep-linked object is found", async () => {
    indexes
      .mockResolvedValueOnce({
        items: [row("a"), row("b")],
        page: { has_more: true },
      })
      .mockResolvedValueOnce({
        items: [row("orders_pkey")],
        page: { has_more: false },
      });
    const found = await resolveInventorySelection("env-1", "run-1", target);
    expect(found).toMatchObject({
      kind: "indexes",
      item: { id: "orders_pkey" },
    });
    expect(indexes).toHaveBeenLastCalledWith(
      "env-1",
      expect.objectContaining({ audit_run_id: "run-1", offset: 2, limit: 100 }),
      undefined,
    );
  });

  it("returns null when the run does not have the object", async () => {
    indexes.mockResolvedValueOnce({
      items: [row("a")],
      page: { has_more: false },
    });
    await expect(
      resolveInventorySelection("env-1", "run-1", target),
    ).resolves.toBeNull();
  });

  it("forwards the signal and stops paging once aborted (PF-02)", async () => {
    const controller = new AbortController();
    indexes.mockImplementationOnce(async () => {
      controller.abort();
      return { items: [row("a")], page: { has_more: true } };
    });
    await expect(
      resolveInventorySelection("env-1", "run-1", target, controller.signal),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(indexes).toHaveBeenLastCalledWith(
      "env-1",
      expect.anything(),
      controller.signal,
    );
  });
});
