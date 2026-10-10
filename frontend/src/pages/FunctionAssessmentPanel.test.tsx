import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { FunctionSnapshot } from "../types";
import { FunctionAssessmentPanel } from "./FunctionAssessmentPanel";

const fn: FunctionSnapshot = {
  id: "one",
  database_name: "db",
  schema_name: "public",
  function_name: "calc",
  identity_arguments: "integer",
  owner_name: "owner",
  language_name: "sql",
  is_security_definer: true,
  kind: "f",
  collected_at: "2026-10-09T12:00:00Z",
};

describe("function sheet identity", () => {
  it("separates overloads in links and shared navigation", () => {
    const integer = renderToStaticMarkup(
      <FunctionAssessmentPanel
        fn={fn}
        environment="env"
        run="run"
        snapshot={null}
      />,
    );
    const text = renderToStaticMarkup(
      <FunctionAssessmentPanel
        fn={{ ...fn, id: "two", identity_arguments: "text" }}
        environment="env"
        run="run"
        snapshot={null}
      />,
    );
    expect(integer).toContain("signature=integer");
    expect(text).toContain("signature=text");
    expect(integer).toContain("privilégios do proprietário");
    expect(integer.match(/role="tab"/g)).toHaveLength(6);
    expect(integer).not.toContain("function_definition");
  });

  it("shows incomplete collection explicitly", () => {
    const html = renderToStaticMarkup(
      <FunctionAssessmentPanel
        fn={fn}
        environment="env"
        run="run"
        snapshot={{
          audit_run_id: "run",
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
});
