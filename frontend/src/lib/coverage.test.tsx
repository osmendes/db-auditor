// @vitest-environment happy-dom
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CoverageBanner } from "../components/CoverageBanner";
import { useRunCoverage } from "../hooks/useRunCoverage";
import dashboardSource from "../pages/DashboardPage.tsx?raw";
import findingsSource from "../pages/FindingsPage.tsx?raw";
import { scoreSubtitle, scoreTitle } from "../pages/parts/DashboardKpis";
import reportsSource from "../pages/ReportsPage.tsx?raw";
import { cleanup, flush, render } from "../test/dom";
import type { AuditRun, AuditRunCoverage, ScopeAggregate } from "../types";
import {
  COVERAGE_MESSAGES,
  type CoverageSignal,
  deriveCoverageSignals,
  latestCompletedRuns,
  scoreIsPartial,
} from "./coverage";

function run(patch: Partial<AuditRun>): AuditRun {
  return {
    id: "run-1",
    environment_id: "env-1",
    environment_name: "prod",
    profile: "standard",
    status: "success",
    service_version: "1",
    collector_version: "1",
    started_at: "2026-10-10T10:00:00Z",
    warnings: [],
    errors: [],
    ...patch,
  };
}

function cov(patch: Partial<AuditRunCoverage>): AuditRunCoverage {
  return {
    collector_name: "postgres.tables",
    status: "success",
    rows_collected: 10,
    collected_at: "2026-10-10T10:01:00Z",
    ...patch,
  };
}

const capabilitiesPg = {
  environment_id: "env-1",
  engine: "postgresql",
  items: [
    { name: "catalog", applicable: true },
    { name: "timescale", applicable: false, reason: "x" },
    { name: "data_quality", applicable: false, reason: "x" },
  ],
};

describe("deriveCoverageSignals (UI-08)", () => {
  it("is silent for a complete run on PostgreSQL (optional features are not gaps)", () => {
    expect(
      deriveCoverageSignals({
        run: run({}),
        runs: [run({})],
        coverage: [cov({})],
        capabilities: capabilitiesPg,
      }),
    ).toEqual([]);
  });

  it("separates permission failures from other collector failures", () => {
    const signals = deriveCoverageSignals({
      run: run({ status: "partial_success" }),
      runs: [],
      coverage: [
        cov({
          collector_name: "postgres.security",
          status: "failed",
          error:
            "postgres.security: permission: ERROR: permission denied for pg_authid (SQLSTATE 42501)",
        }),
        cov({
          collector_name: "postgres.workload",
          database_name: "app",
          status: "failed",
          error: "postgres.workload: timeout: canceling statement",
        }),
      ],
      capabilities: null,
    });
    expect(signals.map((s) => s.kind)).toEqual(["partial", "permission"]);
    expect(signals[0].detail).toEqual(["postgres.workload (app)"]);
    expect(signals[1].detail).toEqual(["postgres.security"]);
    expect(scoreIsPartial(signals)).toBe(true);
  });

  it("flags a gap when a newer attempt failed after the run shown", () => {
    const shown = run({});
    const signals = deriveCoverageSignals({
      run: shown,
      runs: [
        shown,
        run({ id: "r2", status: "failed", started_at: "2026-10-12T10:00:00Z" }),
        run({
          id: "r3",
          status: "cancelled",
          started_at: "2026-10-13T10:00:00Z",
        }),
        run({
          id: "other-env",
          environment_id: "env-2",
          status: "failed",
          started_at: "2026-10-14T10:00:00Z",
        }),
      ],
      coverage: [],
      capabilities: null,
    });
    expect(signals).toHaveLength(1);
    expect(signals[0].kind).toBe("gap");
    expect(signals[0].detail[0]).toContain("2 tentativas");
    expect(scoreIsPartial(signals)).toBe(false);
  });

  it("uses capabilities to say what does not apply to the engine", () => {
    const signals = deriveCoverageSignals({
      run: run({}),
      runs: [],
      coverage: [],
      capabilities: {
        environment_id: "env-1",
        engine: "mongodb",
        items: [
          { name: "catalog", applicable: true },
          { name: "indexes", applicable: true },
          { name: "constraints", applicable: false },
          { name: "workload", applicable: false },
          { name: "security", applicable: false },
          { name: "timescale", applicable: false },
        ],
      },
    });
    expect(signals).toEqual([
      {
        kind: "not_applicable",
        environmentName: "prod",
        detail: ["restrições", "carga", "segurança"],
      },
    ]);
  });

  it("picks the latest completed run per environment", () => {
    const picked = latestCompletedRuns([
      run({ id: "old", started_at: "2026-10-01T00:00:00Z" }),
      run({ id: "new", status: "partial_success" }),
      run({ id: "running", status: "running", started_at: "2026-10-20" }),
      run({ id: "b", environment_id: "env-2" }),
    ]);
    expect(picked.map((r) => r.id)).toEqual(["new", "b"]);
  });
});

describe("CoverageBanner", () => {
  it("renders the shared sentences, grouped, with environment names when mixed", () => {
    const signals: CoverageSignal[] = [
      { kind: "partial", environmentName: "prod", detail: [] },
      { kind: "partial", environmentName: "homolog", detail: ["x"] },
      { kind: "permission", environmentName: "prod", detail: ["sec"] },
    ];
    const html = renderToStaticMarkup(<CoverageBanner signals={signals} />);
    expect(html).toContain('role="status"');
    expect(html).toContain(COVERAGE_MESSAGES.partial);
    expect(html).toContain(COVERAGE_MESSAGES.permission);
    expect(html).toContain("prod: execução marcada como parcial");
    expect(html).toContain("homolog: x");
    expect(html.indexOf(COVERAGE_MESSAGES.partial)).toBeLessThan(
      html.indexOf(COVERAGE_MESSAGES.permission),
    );
  });

  it("renders nothing without signals", () => {
    expect(renderToStaticMarkup(<CoverageBanner signals={[]} />)).toBe("");
  });

  it("is the single coverage notice of Dashboard, Findings and Reports", () => {
    for (const source of [dashboardSource, findingsSource, reportsSource]) {
      expect(source).toContain("useRunCoverage(");
    }
    expect(dashboardSource).toContain("<CoverageBanner signals=");
    expect(reportsSource).toContain("<CoverageBanner signals=");
    // Findings passes the signals to the browser, which renders the banner
    // above the list (not only when it is empty).
    expect(findingsSource).toContain("coverageSignals={coverage.signals}");
  });
});

describe("partial score is not inflated", () => {
  const item: ScopeAggregate = {
    database_name: "app",
    schema_name: "public",
    status: "available",
    score: 92,
    confidence: 0.75,
    tables: 10,
    missing_collectors: [],
  };
  it("labels the score and shows its confidence", () => {
    expect(scoreTitle(item, false)).toBe("92/100 (parcial)");
    expect(scoreSubtitle(item, false)).toContain("confiança 75%");
    expect(scoreTitle({ ...item, confidence: 1 }, true)).toBe(
      "92/100 (parcial)",
    );
    expect(scoreTitle({ ...item, confidence: 1 }, false)).toBe("92/100");
    expect(scoreTitle({ ...item, score: null }, true)).toBe(
      "cobertura insuficiente",
    );
  });
});

const calls = vi.hoisted(() => ({ coverage: [] as string[] }));
vi.mock("../services/api", () => ({
  api: {
    auditRuns: async () => ({
      items: [
        {
          id: "r-new",
          environment_id: "env-1",
          environment_name: "prod",
          status: "partial_success",
          started_at: "2026-10-10T00:00:00Z",
          errors: [],
          warnings: [],
        },
        {
          id: "r-old",
          environment_id: "env-1",
          environment_name: "prod",
          status: "success",
          started_at: "2026-10-01T00:00:00Z",
          errors: [],
          warnings: [],
        },
      ],
    }),
    auditRunCoverage: async (id: string) => {
      calls.coverage.push(id);
      return { items: [] };
    },
    environmentCapabilities: async () => null,
  },
}));

describe("useRunCoverage", () => {
  afterEach(cleanup);

  function Probe({ runId }: { runId?: string }) {
    const state = useRunCoverage("env-1", runId);
    return (
      <p>
        {state.loading
          ? "loading"
          : `${state.runs.map((r) => r.id).join()}|${state.signals.map((s) => s.kind).join()}`}
      </p>
    );
  }

  it("describes the latest completed run, partial included", async () => {
    calls.coverage = [];
    const { container } = await render(<Probe />);
    await flush();
    expect(container.textContent).toBe("r-new|partial");
    expect(calls.coverage).toEqual(["r-new"]);
  });

  it("describes the run chosen on the Reports screen", async () => {
    const { container } = await render(<Probe runId="r-old" />);
    await flush();
    expect(container.textContent).toBe("r-old|");
  });
});
