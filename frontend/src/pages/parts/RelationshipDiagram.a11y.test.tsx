// @vitest-environment happy-dom
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { RelationshipGraph, TableSnapshot } from "../../types";
import { RelationshipDiagram } from "./RelationshipDiagram";

const node = (table: string) => ({ database: "db", schema: "public", table });
const graph: RelationshipGraph = {
  nodes: [node("orders"), node("customers"), node("items")],
  edges: [
    {
      from: node("orders"),
      to: node("customers"),
      constraint_name: "orders_customer_fk",
      columns: ["customer_id"],
      referenced_columns: ["id"],
    },
    {
      from: node("items"),
      to: node("orders"),
      constraint_name: "items_order_fk",
      columns: ["order_id"],
      referenced_columns: ["id"],
    },
  ],
  truncated: false,
};
const root = {
  audit_run_id: "run-1",
  database_name: "db",
  schema_name: "public",
  table_name: "orders",
} as TableSnapshot;

const focusable =
  'a[href],button,input,select,textarea,[contenteditable],[tabindex]:not([tabindex="-1"])';
const scroller = /\boverflow(-[xy])?-(auto|scroll)\b/;

/** Nodes under aria-hidden that keyboard users could still reach, including
 * scroll containers, which browsers focus when they hold nothing focusable. */
function hiddenFocusable(html: string): string[] {
  const doc = new DOMParser().parseFromString(html, "text/html");
  const out: string[] = [];
  for (const hidden of doc.querySelectorAll('[aria-hidden="true"]')) {
    if (hidden.closest("[inert]")) continue;
    for (const el of [hidden, ...hidden.querySelectorAll("*")]) {
      const tabindex = el.getAttribute("tabindex");
      const isScroller = scroller.test(el.getAttribute("class") ?? "");
      if (
        (el !== hidden || tabindex !== null) &&
        el.matches(focusable) &&
        tabindex !== "-1"
      ) {
        out.push(el.outerHTML.slice(0, 80));
      } else if (isScroller && tabindex !== "-1") {
        out.push(`scroller: ${el.outerHTML.slice(0, 80)}`);
      }
    }
  }
  return out;
}

describe("RelationshipDiagram a11y", () => {
  it("never puts the hidden diagram box in the tab order", () => {
    const html = renderToStaticMarkup(
      <RelationshipDiagram
        graph={graph}
        root={root}
        env="env-1"
        run="run-1"
        database="db"
        onNavigate={() => {}}
      />,
    );
    expect(html).toContain('aria-hidden="true" tabindex="-1"');
    expect(hiddenFocusable(html)).toEqual([]);
    // The accessible FK list stays reachable.
    expect(html).toContain("Abrir tabela");
  });

  it("flags a hidden scroller or a hidden link", () => {
    expect(
      hiddenFocusable('<div class="overflow-auto" aria-hidden="true"></div>'),
    ).toHaveLength(1);
    expect(
      hiddenFocusable('<div aria-hidden="true"><a href="#x">x</a></div>'),
    ).toHaveLength(1);
    expect(
      hiddenFocusable(
        '<div aria-hidden="true" inert=""><a href="#x">x</a></div>',
      ),
    ).toEqual([]);
  });
});
