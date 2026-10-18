// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { useLatestSignal } from "../hooks/useLatestSignal";
import { useCollection } from "../pages/parts/assessmentCollection";
import { cleanup, flush, render } from "../test/dom";
import { ApiError, formatError, isAbortError, networkApiError } from "./errors";
import { fetchAllPages } from "./pagination";

afterEach(cleanup);

describe("abort helpers (PF-02)", () => {
  it("recognises every shape of an aborted request", () => {
    const native = new DOMException("aborted", "AbortError");
    expect(isAbortError(native)).toBe(true);
    const wrapped = networkApiError("/api/v1/x", native);
    expect(isAbortError(wrapped)).toBe(true);
    expect(formatError(wrapped, "fallback")).toBe("");
    expect(isAbortError(new ApiError(0, "/x", "offline"))).toBe(false);
    expect(isAbortError(new Error("boom"))).toBe(false);
  });

  it("useLatestSignal aborts the previous request and the last one on unmount", async () => {
    const holder: { next?: () => AbortSignal } = {};
    function Probe() {
      holder.next = useLatestSignal();
      return null;
    }
    const view = await render(<Probe />);
    const next = () => {
      if (!holder.next) throw new Error("hook not mounted");
      return holder.next();
    };
    const first = next();
    const second = next();
    expect(first.aborted).toBe(true);
    expect(second.aborted).toBe(false);
    await view.unmount();
    expect(second.aborted).toBe(true);
  });

  it("fetchAllPages stops paging once the caller aborts", async () => {
    const controller = new AbortController();
    const load = vi.fn(async (offset: number) => {
      controller.abort();
      return {
        items: [offset],
        page: { total: 2, limit: 1, offset, has_more: true },
      };
    });
    await expect(fetchAllPages(load, controller.signal)).rejects.toMatchObject({
      name: "AbortError",
    });
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("assessment collections abort the previous tab request when the scope changes", async () => {
    const signals: AbortSignal[] = [];
    const load = (_offset: number, _limit: number, signal?: AbortSignal) => {
      if (signal) signals.push(signal);
      return new Promise<never>(() => {});
    };
    function Probe({ scope }: { scope: string }) {
      useCollection(true, scope, load);
      return null;
    }
    const view = await render(<Probe scope="table-a" />);
    await view.rerender(<Probe scope="table-b" />);
    await flush();
    expect(signals.length).toBeGreaterThanOrEqual(2);
    expect(signals[0].aborted).toBe(true);
    expect(signals.at(-1)?.aborted).toBe(false);
    await view.unmount();
    expect(signals.at(-1)?.aborted).toBe(true);
  });
});
