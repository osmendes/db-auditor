import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ViewSnapshot } from "../types";
import { ViewAssessmentPanel } from "./ViewAssessmentPanel";

const base: ViewSnapshot = {
  id: "view-1",
  database_name: "db",
  schema_name: "public",
  view_name: "v_orders",
  owner_name: "reader",
  relkind: "v",
  size_bytes: 0,
  collected_at: "2026-10-01T12:00:00Z",
};

describe("view sheet initial navigation", () => {
  it("identifies a regular view without attributing its own storage", () => {
    const html = renderToStaticMarkup(
      <ViewAssessmentPanel
        view={base}
        environment="env-1"
        run="run-1"
        snapshot={null}
      />,
    );
    expect(html).toContain("Visão comum");
    expect(html).not.toContain("Espaço ocupado");
    expect(html).toContain("não possui tamanho de dados próprio");
    expect(html.match(/role="tab"/g)).toHaveLength(6);
    expect(html).toContain("run=run-1");
  });

  it("distinguishes materialized storage and partial collection", () => {
    const html = renderToStaticMarkup(
      <ViewAssessmentPanel
        view={{ ...base, relkind: "m", size_bytes: 4096 }}
        environment="env-1"
        run="run-1"
        snapshot={{
          audit_run_id: "run-1",
          run_status: "partial_success",
          completeness: "partial",
          failed_databases: 1,
          failed_collectors: 1,
          coverage_rows: 1,
          analysis_findings_produced: 0,
          analysis_findings_saved: 0,
        }}
      />,
    );
    expect(html).toContain("Visão materializada");
    expect(html).toContain("Espaço ocupado");
    expect(html).toContain("A coleta foi parcial");
    expect(html).toContain("não informa quando ocorreu o último REFRESH");
  });
});
