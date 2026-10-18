// @vitest-environment happy-dom
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, click, press, render, type } from "../../test/dom";
import type { SearchHit, SearchResult } from "../../types/openapi";
import { GlobalSearch } from "./GlobalSearch";

afterEach(cleanup);

const uuid = "0f8fad5b-d9cb-469f-a165-70867728950e";

function result(items: SearchHit[], query = "ord"): SearchResult {
  return {
    query,
    exact: items.some((i) => i.exact),
    groups: {},
    truncated: {},
    items,
    limits: {},
  };
}

const hits: SearchHit[] = [
  {
    kind: "finding",
    id: uuid,
    title: "Seq scan em orders",
    subtitle: "performance.x",
    severity: "high",
    status: "open",
    environment_id: "e1",
    exact: false,
  },
  {
    kind: "run",
    id: "r1",
    audit_run_id: "r1",
    title: "Execução r1",
    subtitle: "manual",
    exact: false,
  },
  {
    kind: "inventory",
    id: "i1",
    object_kind: "tables",
    database: "app",
    schema: "public",
    name: "orders",
    audit_run_id: "r1",
    title: "public.orders",
    subtitle: "Tabela",
    exact: false,
  },
  {
    kind: "rule",
    id: "performance.x",
    rule_id: "performance.x",
    title: "performance.x",
    subtitle: "",
    exact: false,
  },
];

const wait = (ms: number) =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });

async function setup(
  search: ReturnType<typeof vi.fn>,
  environmentId: string | null = "e1",
) {
  const onNavigate = vi.fn();
  const view = await render(
    <GlobalSearch
      environmentId={environmentId}
      onNavigate={onNavigate}
      search={search as never}
      debounceMs={20}
    />,
  );
  const input = view.container.querySelector<HTMLInputElement>(
    "input[role=combobox]",
  );
  input?.focus();
  return { ...view, input, onNavigate };
}

describe("GlobalSearch (UI-07)", () => {
  it("debounces, searches the current environment and lists grouped results", async () => {
    const search = vi.fn(async (_p: unknown, _s?: AbortSignal) => result(hits));
    const { container, input } = await setup(search);
    type(input, "o");
    type(input, "or");
    type(input, "ord");
    await wait(5);
    expect(search).not.toHaveBeenCalled();
    await wait(40);
    expect(search).toHaveBeenCalledTimes(1);
    expect(search.mock.calls[0][0]).toEqual({ q: "ord", environment_id: "e1" });
    const options = container.querySelectorAll("[role=option]");
    expect(options).toHaveLength(4);
    expect(input?.getAttribute("aria-expanded")).toBe("true");
    expect(container.textContent).toContain("Inventário");
    expect(container.textContent).toContain("Regras");
    expect(options[0].getAttribute("aria-selected")).toBe("true");
  });

  it("does not search below two characters", async () => {
    const search = vi.fn(async () => result([]));
    const { input } = await setup(search);
    type(input, " a ");
    await wait(40);
    expect(search).not.toHaveBeenCalled();
  });

  it("aborts the previous request when the query changes", async () => {
    const signals: AbortSignal[] = [];
    const search = vi.fn(
      (_p: unknown, signal?: AbortSignal) =>
        new Promise<SearchResult>((resolve) => {
          if (signal) signals.push(signal);
          setTimeout(() => resolve(result(hits)), 30);
        }),
    );
    const { input } = await setup(search);
    type(input, "ord");
    await wait(25);
    type(input, "orde");
    await wait(5);
    expect(signals[0]?.aborted).toBe(true);
    await wait(60);
    expect(search).toHaveBeenCalledTimes(2);
    expect(signals[1]?.aborted).toBe(false);
  });

  it("navigates with the keyboard and resets", async () => {
    const search = vi.fn(async (_p: unknown, _s?: AbortSignal) => result(hits));
    const { input, onNavigate } = await setup(search);
    type(input, "ord");
    await wait(40);
    press("ArrowDown", {}, input as HTMLInputElement);
    press("ArrowDown", {}, input as HTMLInputElement);
    expect(input?.getAttribute("aria-activedescendant")).toMatch(/opt-2$/);
    press("Enter", {}, input as HTMLInputElement);
    expect(onNavigate).toHaveBeenCalledWith({
      type: "inventory",
      target: {
        kind: "tables",
        database: "app",
        schema: "public",
        name: "orders",
      },
      run: "r1",
      environmentId: undefined,
    });
    expect(input?.value).toBe("");
  });

  it("opens a result on click", async () => {
    const search = vi.fn(async (_p: unknown, _s?: AbortSignal) => result(hits));
    const { container, input, onNavigate } = await setup(search);
    type(input, "ord");
    await wait(40);
    click(container.querySelectorAll("[role=option]")[3]);
    expect(onNavigate).toHaveBeenCalledWith({
      type: "rule",
      ruleId: "performance.x",
    });
  });

  it("opens the finding directly for a pasted UUID", async () => {
    const search = vi.fn(async () =>
      result([{ ...hits[0], exact: true }], uuid),
    );
    const { input, onNavigate } = await setup(search);
    type(input, ` ${uuid} `);
    await wait(10);
    expect(search).toHaveBeenCalledTimes(1);
    expect(onNavigate).toHaveBeenCalledWith({
      type: "finding",
      id: uuid,
      environmentId: "e1",
    });
  });

  it("Enter on a UUID opens the finding even before the lookup answers", async () => {
    const search = vi.fn(() => new Promise<SearchResult>(() => undefined));
    const { input, onNavigate } = await setup(search);
    type(input, uuid.toUpperCase());
    press("Enter", {}, input as HTMLInputElement);
    expect(onNavigate).toHaveBeenCalledWith({
      type: "finding",
      id: uuid,
      environmentId: undefined,
    });
  });

  it("reports empty results and errors", async () => {
    const search = vi.fn(async (_p: unknown, _s?: AbortSignal) =>
      result([], "zzz"),
    );
    const { container, input } = await setup(search, null);
    type(input, "zzz");
    await wait(40);
    expect(search.mock.calls[0][0]).toEqual({
      q: "zzz",
      environment_id: undefined,
    });
    expect(container.textContent).toContain("Nada encontrado");
    search.mockRejectedValueOnce(new Error("boom"));
    type(input, "zzzz");
    await wait(40);
    expect(container.querySelector("[role=alert]")).not.toBeNull();
  });

  it("Ctrl+K focuses the search", async () => {
    const search = vi.fn(async () => result([]));
    const { input } = await setup(search);
    input?.blur();
    const event = press("k", { ctrlKey: true }, window);
    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(input);
  });
});
