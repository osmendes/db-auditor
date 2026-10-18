// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, flush, render } from "../test/dom";

/** One deferred response per (method, env), resolved by the test. */
const pending = new Map<
  string,
  { resolve: (value: unknown) => void; signal?: AbortSignal }
>();
function deferred(key: string, signal?: AbortSignal) {
  return new Promise((resolve, reject) => {
    pending.set(key, { resolve, signal });
    signal?.addEventListener("abort", () =>
      reject(new DOMException("aborted", "AbortError")),
    );
  });
}
const run = (id: string) => ({
  id,
  status: "success",
  started_at: "2026-10-01T00:00:00Z",
});

vi.mock("../services/api", () => ({
  api: {
    auditRuns: (p: { environment_id: string }, signal?: AbortSignal) =>
      deferred(`runs:${p.environment_id}`, signal),
    annotations: (env: string, signal?: AbortSignal) =>
      deferred(`annotations:${env}`, signal),
    regressions: (env: string, signal?: AbortSignal) =>
      deferred(`regressions:${env}`, signal),
    auditBaseline: async () => ({ audit_run_id: "" }),
    compareScopeScore: async () => ({ status: "comparable", reason: "" }),
    connectionStatus: async () => ({ items: [] }),
    hasRole: () => false,
  },
}));

const app = {
  environmentId: "env-a" as string | null,
  environments: [
    { id: "env-a", name: "A" },
    { id: "env-b", name: "B" },
  ],
  setEnvironmentId: vi.fn(),
  openRun: vi.fn(),
  setSection: vi.fn(),
};
vi.mock("../context/AppContext", () => ({ useApp: () => app }));

import { MonitoringPage } from "./MonitoringPage";

afterEach(async () => {
  await cleanup();
  pending.clear();
  app.environmentId = "env-a";
});

async function answer(env: string, runs: unknown[]) {
  pending.get(`runs:${env}`)?.resolve({ items: runs });
  pending.get(`annotations:${env}`)?.resolve({ items: [] });
  pending.get(`regressions:${env}`)?.resolve({ items: [] });
  await flush();
  await flush();
}

describe("MonitoringPage request cancellation (PF-02)", () => {
  it("a quick environment switch aborts the old load and never applies it", async () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    const view = await render(<MonitoringPage />);
    const stale = pending.get("runs:env-a");
    expect(stale?.signal?.aborted).toBe(false);

    app.environmentId = "env-b";
    await view.rerender(<MonitoringPage />);
    expect(stale?.signal?.aborted).toBe(true);

    await answer("env-b", [run("bbbbbbbb-new")]);
    // The old environment answers late: ignored.
    await answer("env-a", [run("aaaaaaaa-old")]);

    const text = view.container.textContent ?? "";
    expect(text).toContain("bbbbbbbb");
    expect(text).not.toContain("aaaaaaaa");
    // Aborts are silent: no error banner and nothing on the console.
    expect(view.container.querySelector('[role="alert"]')).toBeNull();
    expect(errors).not.toHaveBeenCalled();
    errors.mockRestore();
  });

  it("unmount aborts the in-flight load", async () => {
    const view = await render(<MonitoringPage />);
    const inflight = pending.get("regressions:env-a");
    await view.unmount();
    expect(inflight?.signal?.aborted).toBe(true);
  });
});
