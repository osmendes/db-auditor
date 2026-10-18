import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => vi.unstubAllGlobals());

describe("api.search (UI-07)", () => {
  it("calls the snapshot search endpoint with encoded params and the signal", async () => {
    const fetch = vi.fn(async (_url: string, _init?: RequestInit) =>
      Response.json({
        query: "a b",
        exact: false,
        groups: {},
        truncated: {},
        items: [],
        limits: {},
      }),
    );
    vi.stubGlobal("fetch", fetch);
    const { api } = await import("./api");
    const controller = new AbortController();
    const res = await api.search(
      { q: "a b&c", environment_id: "env-1", limit: 5 },
      controller.signal,
    );
    expect(res.items).toEqual([]);
    const [url, init] = fetch.mock.calls[0];
    expect(url).toContain("/api/v1/search?");
    const params = new URL(url, "http://x").searchParams;
    expect(params.get("q")).toBe("a b&c");
    expect(params.get("environment_id")).toBe("env-1");
    expect(params.get("limit")).toBe("5");
    expect(init?.signal).toBe(controller.signal);
  });

  it("omits an empty environment", async () => {
    const fetch = vi.fn(async (_url: string) =>
      Response.json({
        query: "ab",
        exact: false,
        groups: {},
        truncated: {},
        items: [],
        limits: {},
      }),
    );
    vi.stubGlobal("fetch", fetch);
    const { api } = await import("./api");
    await api.search({ q: "ab", environment_id: undefined });
    expect(fetch.mock.calls[0][0]).not.toContain("environment_id");
  });
});
