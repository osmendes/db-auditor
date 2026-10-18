import { afterEach, describe, expect, it, vi } from "vitest";
import { formatError } from "../lib/errors";

afterEach(() => vi.unstubAllGlobals());

function abortableFetch() {
  return vi.fn((_url: string, init?: RequestInit) => {
    const signal = init?.signal;
    return new Promise<Response>((resolve, reject) => {
      if (signal?.aborted) {
        reject(new DOMException("The operation was aborted.", "AbortError"));
        return;
      }
      signal?.addEventListener("abort", () =>
        reject(new DOMException("The operation was aborted.", "AbortError")),
      );
      setTimeout(
        () =>
          resolve(
            Response.json({
              items: [],
              page: { total: 0, limit: 1, offset: 0 },
            }),
          ),
        50,
      );
    });
  });
}

describe("GET cancellation (PF-02)", () => {
  it("forwards the caller signal to fetch", async () => {
    const fetch = vi.fn(async (_url: string, _init?: RequestInit) =>
      Response.json({ items: [], page: { total: 0, limit: 1, offset: 0 } }),
    );
    vi.stubGlobal("fetch", fetch);
    const { api } = await import("./api");
    const controller = new AbortController();
    await api.findingsPage(
      { environment_id: "env", limit: 1, offset: 0 },
      controller.signal,
    );
    await api.analyticsKpis({ environment_id: "env" }, controller.signal);
    await api.auditRuns({ environment_id: "env" }, controller.signal);
    for (const call of fetch.mock.calls) {
      expect(call[1]?.signal).toBe(controller.signal);
    }
  });

  it("rejects a superseded request quietly so its payload is never applied", async () => {
    vi.stubGlobal("fetch", abortableFetch());
    const { api } = await import("./api");
    const stale = new AbortController();
    const pending = api.findingsPage(
      { environment_id: "old-env", limit: 1, offset: 0 },
      stale.signal,
    );
    stale.abort();
    const error = await pending.then(
      () => null,
      (cause: unknown) => cause,
    );
    expect(error).not.toBeNull();
    expect(formatError(error, "fallback")).toBe("");
  });

  it("fetchFindingsList passes the signal to every request", async () => {
    const fetch = vi.fn(async (_url: string, _init?: RequestInit) =>
      Response.json({ items: [], page: { total: 0, limit: 1, offset: 0 } }),
    );
    vi.stubGlobal("fetch", fetch);
    const { fetchFindingsList } = await import(
      "../pages/parts/fetchFindingsList"
    );
    const controller = new AbortController();
    await fetchFindingsList({
      severityFilter: "",
      statusFilter: "open",
      environmentId: "env",
      mine: false,
      overdueOnly: false,
      pageSize: 20,
      offset: 0,
      signal: controller.signal,
    });
    expect(fetch.mock.calls.length).toBeGreaterThan(1);
    for (const call of fetch.mock.calls) {
      expect(call[1]?.signal).toBe(controller.signal);
    }
  });
});
