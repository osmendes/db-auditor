import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { IndexHistoryPoint, IndexSnapshot } from "../types";
import {
  comparableIndexScans,
  IndexAssessmentPanel,
} from "./IndexAssessmentPanel";

const index: IndexSnapshot = {
  id: "index-1",
  database_name: "db",
  schema_name: "public",
  table_name: "orders",
  index_name: "orders_idx",
  access_method: "btree",
  is_unique: false,
  is_primary: false,
  size_bytes: 4096,
  idx_scan: 0,
  collected_at: "2026-10-09T11:00:00Z",
};

const point: IndexHistoryPoint = {
  audit_run_id: "run-2",
  run_status: "success",
  definition_fingerprint: "stable",
  size_bytes: 4096,
  idx_scan: 12,
  idx_tup_read: 20,
  idx_tup_fetch: 15,
  stats_reset: "2026-10-01T00:00:00Z",
  usage_observed: true,
  collected_at: "2026-10-09T11:00:00Z",
};

describe("index sheet", () => {
  it("shows a direct link, table context and six accessible sections without exposing SQL", () => {
    const html = renderToStaticMarkup(
      <IndexAssessmentPanel
        index={index}
        environment="env-1"
        run="run-2"
        snapshot={null}
      />,
    );
    expect(html).toContain("Link direto para este índice e execução");
    expect(html).toContain("Analisar a tabela relacionada");
    expect(html).toContain("run=run-2");
    expect(html.match(/role="tab"/g)).toHaveLength(6);
    expect(html).not.toContain("CREATE INDEX");
  });

  it("marks partial collection", () => {
    const html = renderToStaticMarkup(
      <IndexAssessmentPanel
        index={index}
        environment="env-1"
        run="run-2"
        snapshot={{
          audit_run_id: "run-2",
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
    expect(html).toContain("A coleta foi parcial");
  });

  it("compares counters only within the same observed statistics window", () => {
    expect(
      comparableIndexScans([
        point,
        { ...point, audit_run_id: "run-1", idx_scan: 7 },
      ]),
    ).toBe(5);
    expect(
      comparableIndexScans([
        point,
        { ...point, stats_reset: "2026-10-02T00:00:00Z" },
      ]),
    ).toBeNull();
    expect(
      comparableIndexScans([point, { ...point, usage_observed: false }]),
    ).toBeNull();
    expect(
      comparableIndexScans([
        point,
        { ...point, definition_fingerprint: "changed" },
      ]),
    ).toBeNull();
    expect(
      comparableIndexScans([point, { ...point, idx_scan: 20 }]),
    ).toBeNull();
    expect(comparableIndexScans([point])).toBeNull();
  });
});
