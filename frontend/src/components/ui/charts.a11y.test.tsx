// @vitest-environment happy-dom
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { TableHistoryChart } from "../../pages/parts/TableHistoryChart";
import { StoragePieChart } from "./StoragePieChart";
import { Table } from "./Table";

describe("charts have a text equivalent (UI-05)", () => {
  it("StoragePieChart hides the SVG and exposes an sr-only table", () => {
    const html = renderToStaticMarkup(
      <StoragePieChart
        items={[
          { label: "prod", size_bytes: 3 * 1024 },
          { label: "homolog", size_bytes: 1024 },
        ]}
      />,
    );
    expect(html).toContain('aria-hidden="true"');
    expect(html).toContain('<table class="sr-only"><caption>Storage por');
    expect(html).toContain("<td>prod</td><td>3.0 KiB</td><td>75.0%</td>");
    expect(html).toContain("<td>homolog</td><td>1.0 KiB</td><td>25.0%</td>");
  });

  it("TableHistoryChart lists every point as text, flagging partial runs", () => {
    const html = renderToStaticMarkup(
      <TableHistoryChart
        history={[
          {
            bucket: "2026-10-01T00:00:00Z",
            audit_run_id: "r1",
            run_status: "success",
            total_size_bytes: 2048,
            row_estimate: 1200,
            counters_reset: false,
            complete: true,
          },
          {
            bucket: "2026-10-02T00:00:00Z",
            audit_run_id: "r2",
            run_status: "partial_success",
            total_size_bytes: 4096,
            row_estimate: 1500,
            counters_reset: true,
            complete: false,
          },
        ]}
      />,
    );
    expect(html).toMatch(
      /<div class="h-56 w-full" aria-hidden="true" inert="">/,
    );
    expect(html).toContain(
      "<caption>Histórico de tamanho e linhas da tabela</caption>",
    );
    expect(html).toContain("<td>2 KB</td><td>1.200</td>");
    expect(html).toContain("<td>completa</td>");
    expect(html).toContain("<td>parcial, contadores reiniciados</td>");
  });
});

describe("Table semantics (UI-05)", () => {
  it("marks column headers, sort state and a keyboard-scrollable region", () => {
    const html = renderToStaticMarkup(
      <Table
        caption="Tabelas do esquema public"
        headers={[{ id: "name", label: "Nome", sortable: true }, "Tamanho"]}
        sortKey="name"
        sortDir="asc"
        onSort={() => {}}
      >
        <tr>
          <td>orders</td>
          <td>1 MiB</td>
        </tr>
      </Table>,
    );
    expect(html).toContain('aria-label="Tabelas do esquema public"');
    expect(html).toContain('tabindex="0"');
    expect(html).toContain('<caption class="sr-only">Tabelas do esquema');
    expect(html).toContain('scope="col"');
    expect(html).toContain('aria-sort="ascending"');
  });
});

// #122: content under aria-hidden must not be reachable with Tab either.
function focusAudit(html: string) {
  const doc = new DOMParser().parseFromString(html, "text/html");
  const hidden = [...doc.querySelectorAll('[aria-hidden="true"]')];
  const focusable = hidden.flatMap((el) =>
    [
      ...el.querySelectorAll(
        "[tabindex], a[href], button, input, select, textarea",
      ),
    ]
      .filter((node) => node.getAttribute("tabindex") !== "-1")
      .map((node) => `${node.tagName}.${node.getAttribute("class")}`),
  );
  const chart = hidden.filter((el) => el.querySelector(".recharts-surface"));
  return {
    focusable,
    charts: chart.length,
    inert: chart.every((el) => el.hasAttribute("inert")),
  };
}

describe("decorative charts take no keyboard focus (#122)", () => {
  it("StoragePieChart renders the pie without a focusable layer", () => {
    const html = renderToStaticMarkup(
      <StoragePieChart
        items={[
          { label: "prod", size_bytes: 3 * 1024 },
          { label: "homolog", size_bytes: 1024 },
        ]}
      />,
    );
    expect(html).toContain("recharts-pie");
    expect(focusAudit(html)).toEqual({ focusable: [], charts: 1, inert: true });
  });

  it("TableHistoryChart renders the lines without a focusable layer", () => {
    const html = renderToStaticMarkup(
      <TableHistoryChart
        history={[
          {
            bucket: "2026-10-01T00:00:00Z",
            audit_run_id: "r1",
            run_status: "success",
            total_size_bytes: 2048,
            row_estimate: 1200,
            counters_reset: false,
            complete: true,
          },
          {
            bucket: "2026-10-02T00:00:00Z",
            audit_run_id: "r2",
            run_status: "success",
            total_size_bytes: 4096,
            row_estimate: 1500,
            counters_reset: false,
            complete: true,
          },
        ]}
      />,
    );
    expect(html).toContain("recharts-line");
    expect(focusAudit(html)).toEqual({ focusable: [], charts: 1, inert: true });
  });
});
