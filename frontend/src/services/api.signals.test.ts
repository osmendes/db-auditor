import { describe, expect, it } from "vitest";
import auditRunsSource from "../pages/AuditRunsPage.tsx?raw";
import monitoringSource from "../pages/MonitoringPage.tsx?raw";
import tableSource from "../pages/TableAssessmentPanel.tsx?raw";
import apiSource from "./api.ts?raw";

/**
 * PF-02: every GET wrapper takes an optional AbortSignal and hands it to
 * fetch, and the screens that load per environment/object cancel requests.
 */
function apiProperties(): Array<{ name: string; text: string }> {
  const start = apiSource.indexOf("export const api = {");
  const end = apiSource.indexOf("\n};", start);
  const body = apiSource.slice(start, end);
  const marks = [...body.matchAll(/^ {2}([A-Za-z_]\w*):/gm)];
  return marks.map((mark, index) => ({
    name: mark[1],
    text: body.slice(mark.index, marks[index + 1]?.index ?? body.length),
  }));
}

const callers = import.meta.glob<string>(
  [
    "../pages/parts/useInventoryExplorer.tsx",
    "../pages/parts/useInventoryObjectLists.tsx",
    "../pages/parts/use*Assessment.tsx",
    "../pages/parts/assessmentCollection.tsx",
    "../pages/parts/useReportsPage.tsx",
    "../pages/parts/useSelectedRunContext.ts",
    "../components/InventoryDetailData.tsx",
  ],
  { query: "?raw", import: "default", eager: true },
);

describe("API GET cancellation contract (PF-02)", () => {
  const gets = apiProperties().filter(
    (p) =>
      (p.text.includes("getJSON<") ||
        /reportRequest<[\s\S]*?"GET"/.test(p.text)) &&
      p.name !== "restoreSession",
  );

  it("finds the GET wrappers", () => {
    expect(gets.length).toBeGreaterThan(90);
  });

  it.each(gets.map((p) => [p.name, p.text]))(
    "%s accepts and forwards a signal",
    (_name, text) => {
      expect(text).toMatch(/signal\?: AbortSignal/);
      expect(text).toMatch(/,\s*signal,?\s*\)/);
    },
  );
});

describe("screens cancel stale requests (PF-02)", () => {
  const sources = {
    ...callers,
    "../pages/AuditRunsPage.tsx": auditRunsSource,
    "../pages/MonitoringPage.tsx": monitoringSource,
    "../pages/TableAssessmentPanel.tsx": tableSource,
  };

  it("covers inventory, the six assessment panels, Reports, Runs and Monitoring", () => {
    expect(Object.keys(sources).length).toBeGreaterThanOrEqual(14);
  });

  it.each(Object.entries(sources))(
    "%s aborts what it starts",
    (_path, source) => {
      expect(source).toMatch(/new AbortController\(\)|useLatestSignal\(\)/);
      const guards = (
        source.match(/let (cancelled|active) = (false|true);/g) ?? []
      ).length;
      const controllers = (source.match(/new AbortController\(\)/g) ?? [])
        .length;
      // Every "ignore late answers" flag now comes with a real abort.
      expect(controllers).toBeGreaterThanOrEqual(guards);
    },
  );
});
