// @vitest-environment happy-dom
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import docsGuide from "../../../docs/guia-do-usuario.md?raw";
import publicGuide from "../../public/guia-do-usuario.md?raw";
import { navigationSections } from "../components/layout/Shell";
import { parseGuideTopics } from "../lib/guideTopics";
import { byText, cleanup, click, flush, render } from "../test/dom";
import { DocsPage } from "./DocsPage";
import docsPageSource from "./DocsPage.tsx?raw";

const topics = parseGuideTopics(docsGuide, navigationSections);

afterEach(async () => {
  await cleanup();
  vi.unstubAllGlobals();
});

describe("guide topics come from docs/guia-do-usuario.md (UI-10)", () => {
  it("serves the same guide that lives in docs/", () => {
    // public/ is what the UI fetches; it must be a byte copy of docs/.
    // Fix with: bun run sync:guide
    expect(publicGuide).toBe(docsGuide);
  });

  it("parses one topic per screen with intro, steps and terms", () => {
    const titles = topics.map((topic) => topic.title);
    expect(new Set(titles).size).toBe(titles.length);
    expect(new Set(topics.map((t) => t.id)).size).toBe(topics.length);
    expect(topics.length).toBeGreaterThan(10);
    for (const topic of topics) {
      expect(topic.intro.length).toBeGreaterThan(40);
      expect(topic.steps.length).toBeGreaterThan(2);
    }
    expect(topics[0].id).toBe("comecar");
    expect(topics[0].title).toBe("Por onde começar");
    expect(topics[0].terms[0]).toEqual({
      name: "Ambiente",
      meaning:
        "Um banco, ou um grupo de bancos, que o auditor acompanha. Cada ambiente tem um nome na tela.",
    });
    // Every "Abrir" target is a real section of the app.
    const targets = topics.flatMap((t) => (t.goTo ? [t.goTo] : []));
    expect(targets.length).toBeGreaterThan(10);
    for (const target of targets) {
      expect(navigationSections).toContain(target);
    }
  });

  it("does not bundle the guide text in the Docs chunk", () => {
    expect(docsPageSource).not.toMatch(/\?raw|docsTopics|\.md["']/);
    expect(docsPageSource).toContain("fetch(guideURL()");
    expect(docsPageSource).not.toContain(topics[0].intro.slice(0, 40));
  });
});

describe("DocsPage", () => {
  it("fetches the guide and opens topics in place", async () => {
    const fetchMock = vi.fn(async () => new Response(docsGuide));
    vi.stubGlobal("fetch", fetchMock);
    const onNavigate = vi.fn();
    const { container } = await render(<DocsPage onNavigate={onNavigate} />);
    await flush();
    expect(fetchMock).toHaveBeenCalledWith(
      "/guia-do-usuario.md",
      expect.anything(),
    );
    expect(container.textContent).toContain("Documentação");
    expect(container.textContent).toContain("Por onde começar");
    expect(container.textContent).toContain(
      "O DB Auditor mostra um retrato do banco",
    );
    expect(container.textContent).toContain("Como usar");
    click(byText(container, "Dashboard", "button"));
    click(byText(container, "Abrir Dashboard", "button"));
    expect(onNavigate).toHaveBeenCalledWith("Dashboard");
  });

  it("explains a failed load and links the raw guide", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 404 })),
    );
    const { container } = await render(<DocsPage />);
    await flush();
    const alert = container.querySelector('[role="alert"]');
    expect(alert?.textContent).toContain("HTTP 404");
    expect(alert?.querySelector("a")?.getAttribute("href")).toBe(
      "/guia-do-usuario.md",
    );
  });

  it("renders parsed topics without fetching (SSR)", () => {
    const html = renderToStaticMarkup(<DocsPage initialTopics={topics} />);
    expect(html).toContain("Por onde começar");
    expect(html).toContain("O DB Auditor mostra um retrato do banco");
  });
});
