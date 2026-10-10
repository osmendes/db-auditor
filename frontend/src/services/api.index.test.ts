import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => vi.unstubAllGlobals());

describe("index detail API", () => {
  it("encodes the complete object scope and pagination", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        Response.json({ items: [], page: { total: 0, limit: 1, offset: 2 } }),
      );
    vi.stubGlobal("fetch", fetch);
    const { api } = await import("./api");
    await api.indexHistory(
      "env",
      "run",
      "db/name",
      "public",
      "orders idx",
      1,
      2,
    );
    expect(fetch.mock.calls[0][0]).toContain(
      "/environments/env/runs/run/databases/db%2Fname/schemas/public/indexes/orders%20idx/history?limit=1&offset=2",
    );
  });
});
