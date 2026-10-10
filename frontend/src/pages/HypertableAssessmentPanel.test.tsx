import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { HypertableSnapshot, SnapshotCompleteness } from "../types";
import { HypertableAssessmentPanel } from "./HypertableAssessmentPanel";

const hypertable: HypertableSnapshot = {
  id: "h1",
  audit_run_id: "run-1",
  environment_id: "env-1",
  database_name: "db",
  schema_name: "public",
  hypertable_name: "metrics",
  owner_name: "db_owner",
  num_dimensions: 1,
  num_chunks: 3,
  compression_enabled: false,
  is_distributed: false,
  total_size_bytes: 1024,
  data_size_bytes: 768,
  index_size_bytes: 256,
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

describe("hypertable sheet initial navigation", () => {
  it("shows the selected execution, identity and six common sections", () => {
    const html = renderToStaticMarkup(
      <HypertableAssessmentPanel
        hypertable={hypertable}
        environment="env-1"
        run="run-1"
        snapshot={null}
      />,
    );
    expect(html).toContain("Tabela temporal selecionada");
    expect(html).toContain("metrics");
    expect(html).toContain("run=run-1");
    expect(html).toContain("Não habilitada");
    expect(html.match(/role="tab"/g)).toHaveLength(6);
  });

  it("explains partial coverage and compression observed", () => {
    const html = renderToStaticMarkup(
      <HypertableAssessmentPanel
        hypertable={{ ...hypertable, compression_enabled: true }}
        environment="env-1"
        run="run-1"
        snapshot={partial}
      />,
    );
    expect(html).toContain("A coleta foi parcial");
    expect(html).toContain("Habilitada");
  });
});
