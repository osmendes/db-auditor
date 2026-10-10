import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CAGGSnapshot, SnapshotCompleteness } from "../types";
import { CAGGAssessmentPanel } from "./CAGGAssessmentPanel";

const cagg: CAGGSnapshot = {
  id: "c1",
  database_name: "db",
  schema_name: "public",
  view_name: "hourly",
  owner_name: "db_owner",
  materialization_schema: "_timescaledb_internal",
  materialization_hypertable: "hourly_mat",
  materialized_only: true,
  compression_enabled: false,
  collected_at: "2026-10-01T12:00:00Z",
};

const partial: SnapshotCompleteness = {
  audit_run_id: "run-1",
  run_status: "partial_success",
  completeness: "partial",
  failed_databases: 1,
  failed_collectors: 1,
  coverage_rows: 1,
  analysis_findings_produced: 0,
  analysis_findings_saved: 0,
};

describe("continuous aggregate sheet initial navigation", () => {
  it("uses its own identity and all common sections", () => {
    const html = renderToStaticMarkup(
      <CAGGAssessmentPanel
        cagg={cagg}
        environment="env-1"
        run="run-1"
        snapshot={null}
      />,
    );
    expect(html).toContain("Agregado contínuo selecionado");
    expect(html).toContain("kind=caggs");
    expect(html).toContain("run=run-1");
    expect(html.match(/role="tab"/g)).toHaveLength(6);
    expect(html).not.toContain("Visão comum");
  });

  it("explains partial coverage without implying an observed refresh", () => {
    const html = renderToStaticMarkup(
      <CAGGAssessmentPanel
        cagg={{ ...cagg, materialized_only: false }}
        environment="env-1"
        run="run-1"
        snapshot={partial}
      />,
    );
    expect(html).toContain("A coleta foi parcial");
    expect(html).toContain("Apenas dados materializados");
    expect(html).not.toContain("Última atualização concluída");
  });
});
