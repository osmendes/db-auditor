// @vitest-environment happy-dom
/**
 * UI-01: the URL fragment is the routing contract (`#/section/:id?env=…`).
 * These tests drive the real AppProvider/App in a DOM: reload (F5),
 * back/forward, filter preservation and a direct link opened with an
 * expired session.
 */
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, flush, render } from "../test/dom";
import { type AppContextValue, AppProvider, useApp } from "./AppContext";

const FINDING = "11111111-1111-1111-1111-111111111111";
const ENV = "22222222-2222-2222-2222-222222222222";
const OTHER_ENV = "33333333-3333-3333-3333-333333333333";

const session = vi.hoisted(() => ({
  user: null as null | { username: string; role: string },
}));

vi.mock("../services/api", () => {
  const empty = { items: [], page: { total: 0, limit: 20, offset: 0 } };
  const handlers: Record<string, unknown> = {
    hasRole: () => true,
    hasSession: () => session.user !== null,
    currentUser: () => session.user?.username ?? "",
    restoreSession: async () => session.user,
    login: async (username: string) => {
      session.user = { username, role: "operator" };
      return session.user;
    },
    logout: async () => {
      session.user = null;
    },
    environments: async () => ({
      items: [
        { id: ENV, name: "prod" },
        { id: OTHER_ENV, name: "homolog" },
      ],
    }),
    analyticsKpis: async () => ({ open_findings: 0 }),
  };
  const api = new Proxy(handlers, {
    get: (target, key: string) => target[key] ?? (async () => empty),
  });
  return { api };
});

// Light pages: they only print what the router handed them.
function Probe({ name }: { name: string }) {
  const { findingId, environmentId, search } = useApp();
  return (
    <p data-testid="page">
      {name}|{findingId ?? "-"}|{environmentId ?? "-"}|{JSON.stringify(search)}
    </p>
  );
}
vi.mock("../pages/FindingsPage", () => ({
  FindingsPage: () => <Probe name="findings" />,
}));
vi.mock("../pages/DashboardPage", () => ({
  DashboardPage: () => <Probe name="dashboard" />,
}));

let ctx: AppContextValue | null = null;
function Capture() {
  ctx = useApp();
  return null;
}

async function mountProvider() {
  return render(
    <AppProvider>
      <Capture />
    </AppProvider>,
  );
}

function app(): AppContextValue {
  if (!ctx) throw new Error("AppProvider not mounted");
  return ctx;
}

async function popTo(hash: string) {
  // What the browser does on back/forward: the URL changes, then popstate.
  await act(async () => {
    window.history.replaceState(null, "", hash);
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
}

beforeEach(() => {
  session.user = { username: "maycon", role: "operator" };
  window.history.replaceState(null, "", "/");
  ctx = null;
});

afterEach(cleanup);

describe("routing contract (UI-01)", () => {
  it("F5 restores section, environment, finding id and filters from the hash", async () => {
    window.history.replaceState(
      null,
      "",
      `#/findings/${FINDING}?env=${ENV}&status=open&severity=high`,
    );
    await mountProvider();
    await flush();
    expect(app().section).toBe("Findings");
    expect(app().findingId).toBe(FINDING);
    expect(app().environmentId).toBe(ENV);
    expect(app().search).toEqual({ status: "open", severity: "high" });
    // Booting does not rewrite or duplicate the entry.
    expect(window.location.hash).toBe(
      `#/findings/${FINDING}?env=${ENV}&status=open&severity=high`,
    );
  });

  it("pushes one history entry per navigation and replays them on back/forward", async () => {
    window.history.replaceState(null, "", `#/dashboard?env=${ENV}`);
    await mountProvider();
    await flush();
    const before = window.history.length;

    await act(async () => app().setSection("Findings"));
    expect(window.location.hash).toBe(`#/findings?env=${ENV}`);
    await act(async () => app().setSearch({ status: "open" }));
    expect(window.location.hash).toBe(`#/findings?env=${ENV}&status=open`);
    await act(async () => app().openFinding(FINDING));
    expect(window.location.hash).toBe(
      `#/findings/${FINDING}?env=${ENV}&status=open`,
    );
    expect(window.history.length).toBe(before + 3);

    // Back: the list with its filter, not a reset page.
    await popTo(`#/findings?env=${ENV}&status=open`);
    expect(app().section).toBe("Findings");
    expect(app().findingId).toBeNull();
    expect(app().search).toEqual({ status: "open" });
    // A history replay must not push a new entry.
    expect(window.history.length).toBe(before + 3);

    // Back again to the dashboard.
    await popTo(`#/dashboard?env=${ENV}`);
    expect(app().section).toBe("Dashboard");
    expect(app().search).toEqual({});

    // Forward to the finding.
    await popTo(`#/findings/${FINDING}?env=${ENV}&status=open`);
    expect(app().findingId).toBe(FINDING);
    expect(app().environmentId).toBe(ENV);
    expect(app().search).toEqual({ status: "open" });
  });

  it("still records the next navigation after back fires popstate and hashchange", async () => {
    window.history.replaceState(null, "", `#/dashboard?env=${ENV}`);
    await mountProvider();
    await flush();
    await act(async () => app().setSection("Findings"));
    // Browsers fire popstate and then hashchange for one fragment back.
    // They are separate tasks, so React commits in between.
    await act(async () => {
      window.history.replaceState(null, "", `#/dashboard?env=${ENV}`);
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    await act(async () => {
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    expect(app().section).toBe("Dashboard");
    await act(async () => app().setSection("Execuções"));
    expect(window.location.hash).toBe(`#/runs?env=${ENV}`);
  });

  it("keeps list filters when the environment changes and after a reload", async () => {
    window.history.replaceState(
      null,
      "",
      `#/findings?env=${ENV}&severity=high&run=r1`,
    );
    await mountProvider();
    await flush();
    await act(async () => app().setEnvironmentId(OTHER_ENV));
    // The run belongs to the old environment; the other filters stay.
    expect(window.location.hash).toBe(
      `#/findings?env=${OTHER_ENV}&severity=high`,
    );
    await cleanup();
    ctx = null;
    await mountProvider();
    await flush();
    expect(app().environmentId).toBe(OTHER_ENV);
    expect(app().search).toEqual({ severity: "high" });
  });

  it("drops an environment from the link that the account cannot see", async () => {
    window.history.replaceState(null, "", "#/findings?env=not-mine");
    await mountProvider();
    await flush();
    await flush();
    expect(app().environmentId).toBeNull();
    expect(window.location.hash).toBe("#/findings");
  });
});

describe("direct link and session (UI-01, no auth regression)", () => {
  async function loadApp() {
    const { App } = await import("../app");
    return render(<App />);
  }

  function page() {
    return document.querySelector('[data-testid="page"]')?.textContent ?? "";
  }

  it("opens a finding link after logging in, without losing the fragment", async () => {
    session.user = null;
    const link = `#/findings/${FINDING}?env=${ENV}&status=open`;
    window.history.replaceState(null, "", link);
    await loadApp();
    await flush();
    expect(document.querySelector("form")?.textContent).toContain(
      "Entrar no DB Auditor",
    );
    expect(window.location.hash).toBe(link);

    const form = document.querySelector("form") as HTMLFormElement;
    const [user, pass] = Array.from(form.querySelectorAll("input"));
    const setValue = (el: HTMLInputElement, value: string) => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )?.set;
      setter?.call(el, value);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    };
    await act(async () => {
      setValue(user, "maycon");
      setValue(pass, "uma-senha-longa-local");
    });
    await act(async () => {
      form.requestSubmit();
    });
    await flush();
    await flush();
    expect(page()).toContain(`findings|${FINDING}|${ENV}|`);
    expect(page()).toContain('{"status":"open"}');
    expect(window.location.hash).toBe(link);
  });

  it("an expired session shows the login and returns to the same link", async () => {
    const link = `#/findings/${FINDING}?env=${ENV}`;
    window.history.replaceState(null, "", link);
    await loadApp();
    await flush();
    await flush();
    expect(page()).toContain(`findings|${FINDING}|${ENV}`);

    await act(async () => {
      session.user = null;
      window.dispatchEvent(new Event("auditor:session-expired"));
    });
    expect(page()).toBe("");
    expect(document.querySelector("form")?.textContent).toContain(
      "Entrar no DB Auditor",
    );
    // The protected content is gone but the address bar still has the link.
    expect(window.location.hash).toBe(link);
  });
});
