import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => vi.unstubAllGlobals());

describe("history series API (#106)", () => {
  it("sends the page and returns the page metadata", async () => {
    const fetch = vi.fn().mockResolvedValue(
      Response.json({
        items: [],
        granularity: "day",
        page: { total: 120, limit: 500, offset: 50, has_more: false },
      }),
    );
    vi.stubGlobal("fetch", fetch);
    const { api } = await import("./api");
    const res = await api.tableHistory("env", {
      database: "db",
      schema: "public",
      table: "orders",
      limit: 500,
      offset: 50,
    });
    expect(fetch.mock.calls[0][0]).toContain(
      "/environments/env/tables/history?database=db&schema=public&table=orders&limit=500&offset=50",
    );
    expect(res.page.total).toBe(120);
  });
});
