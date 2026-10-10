import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => vi.unstubAllGlobals());

describe("function detail API", () => {
  it("preserves an encoded overload signature in every request", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        Response.json({ items: [], page: { total: 0, limit: 2, offset: 4 } }),
      );
    vi.stubGlobal("fetch", fetch);
    const { api } = await import("./api");
    await api.functionFindings(
      "env",
      "run",
      "db",
      "public",
      "calc",
      "numeric(10, 2)",
      2,
      4,
    );
    expect(fetch.mock.calls[0][0]).toContain(
      "/functions/calc/findings?signature=numeric%2810%2C+2%29&limit=2&offset=4",
    );
  });
});
