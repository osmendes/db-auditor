// @vitest-environment happy-dom
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchEmptyContext } from "../../hooks/useEmptyContext";
import {
  EMPTY_CAUSE_ACTION,
  type EmptyCause,
  emptyCauseCopy,
  resolveEmptyCause,
} from "../../lib/emptyCause";
import { cleanup, click, render } from "../../test/dom";
import { EmptyState } from "./EmptyState";

const CAUSES: EmptyCause[] = ["never", "zero", "filter", "dsn"];
const subject = { none: "Nenhum achado", plural: "achados" };

vi.mock("../../services/api", () => ({
  api: {
    auditRuns: async (params?: { environment_id?: string }) => ({
      items:
        params?.environment_id === "env-collected"
          ? [{ id: "r1", status: "partial_success" }]
          : [{ id: "r0", status: "failed" }],
    }),
    connectionStatus: async () => ({
      items: [
        { environment_id: "env-no-dsn", dsn_configured: false },
        { environment_id: "env-collected", dsn_configured: true },
        { environment_id: "env-new", dsn_configured: true },
      ],
    }),
  },
}));

afterEach(cleanup);

describe("EmptyState variants (UI-09)", () => {
  it("gives each cause its own title, text and CTA", () => {
    const copies = CAUSES.map((cause) => emptyCauseCopy(cause, subject));
    expect(new Set(copies.map((c) => c.title)).size).toBe(4);
    expect(new Set(copies.map((c) => c.description)).size).toBe(4);
    expect(new Set(Object.values(EMPTY_CAUSE_ACTION)).size).toBe(4);
    expect(EMPTY_CAUSE_ACTION).toEqual({
      never: "Iniciar uma execução",
      zero: "Ver cobertura da execução",
      filter: "Limpar filtros",
      dsn: "Ir para Ambientes",
    });
    expect(copies[1].title).toBe("Nenhum achado na última coleta");
    expect(copies[1].description).toContain("não prova ausência de risco");
    expect(copies[2].title).toBe("Nenhum achado neste filtro");
  });

  it("renders the CTA and calls it", async () => {
    for (const cause of CAUSES) {
      const onAction = vi.fn();
      const { container } = await render(
        <EmptyState cause={cause} subject={subject} onAction={onAction} />,
      );
      const root = container.querySelector(`[data-empty-cause="${cause}"]`);
      expect(root?.textContent).toContain(emptyCauseCopy(cause, subject).title);
      const button = container.querySelector("button");
      expect(button?.textContent).toBe(EMPTY_CAUSE_ACTION[cause]);
      click(button);
      expect(onAction).toHaveBeenCalledOnce();
      await cleanup();
    }
  });

  it("keeps the plain title/description form and allows overrides", () => {
    const plain = renderToStaticMarkup(
      <EmptyState title="Selecione um ambiente" description="Escolha um." />,
    );
    expect(plain).toContain("Selecione um ambiente");
    expect(plain).not.toContain("<button");
    const override = renderToStaticMarkup(
      <EmptyState cause="never" description="Texto próprio." />,
    );
    expect(override).toContain("Nenhuma coleta concluída");
    expect(override).toContain("Texto próprio.");
  });
});

describe("resolveEmptyCause", () => {
  it("tells the four causes apart", () => {
    const base = { filtered: false, dsnMissing: false, collected: false };
    expect(resolveEmptyCause(base)).toBe("never");
    expect(resolveEmptyCause({ ...base, collected: true })).toBe("zero");
    expect(resolveEmptyCause({ ...base, dsnMissing: true })).toBe("dsn");
    expect(resolveEmptyCause({ ...base, filtered: true })).toBe("filter");
    // A filter over an empty data set is not the reason the list is empty.
    expect(
      resolveEmptyCause({ ...base, filtered: true, unfilteredTotal: 0 }),
    ).toBe("never");
    expect(
      resolveEmptyCause({
        filtered: true,
        unfilteredTotal: 0,
        dsnMissing: true,
        collected: false,
      }),
    ).toBe("dsn");
  });

  it("reads DSN and collection state from the API", async () => {
    expect(await fetchEmptyContext("env-no-dsn")).toEqual({
      dsnMissing: true,
      collected: false,
    });
    expect(await fetchEmptyContext("env-collected")).toEqual({
      dsnMissing: false,
      collected: true,
    });
    expect(await fetchEmptyContext("env-new")).toEqual({
      dsnMissing: false,
      collected: false,
    });
  });
});
