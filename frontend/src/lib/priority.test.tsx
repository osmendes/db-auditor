import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PriorityList } from "../components/PriorityList";
import { FindingImpactGain } from "../pages/parts/FindingImpactGain";
import type { Finding, FindingAction, PriorityItem } from "../types";
import {
  impactLabel,
  impactPeriod,
  impactUnit,
  priorityDescription,
} from "./priority";

afterEach(() => vi.unstubAllGlobals());

const estimated = {
  status: "estimated",
  metric: "index_bytes",
  value: 40,
  unit: "bytes",
  denominator: 100,
  denominator_unit: "bytes de todos os índices da tabela",
  ratio: 0.4,
  period: "25/09/2026 12:00 UTC a 02/10/2026 12:00 UTC",
  level: "alto",
  basis: "pg_relation_size",
  label: "40 bytes de 100 bytes (40.0%)",
};

const item: PriorityItem = {
  priority: {
    finding_id: "f-123",
    rank: 1,
    score: 2.1,
    max_score: 7.5,
    severity: "medium",
    impact_level: "alto",
    impact_label: estimated.label,
    effort: "médio",
    confidence: 0.7,
    confidence_label: "média",
    risk: "Criar ou remover índice pode aumentar escrita.",
    reason: "",
    formula: "priority-v1",
  },
  finding: { id: "f-123", title: "Índice sem uso" } as Finding,
};

function action(overrides: Partial<FindingAction> = {}): FindingAction {
  return {
    finding_id: "f-123",
    environment_id: "env",
    rule_id: "index.unused",
    target: "db.public.idx",
    severity: "medium",
    confidence: 0.7,
    evidence: {},
    coverage: "complete",
    suggestion: "",
    plan: {
      category: "query_and_index",
      expected_benefit: "",
      risk: "",
      prerequisites: "",
      confirmation: "",
      validation: "",
      false_positive_risk: "",
    },
    status: "suggested",
    owner: "",
    justification: "",
    result: "",
    ...overrides,
  };
}

describe("impact helpers", () => {
  it("show unit and period only with a measured estimate", () => {
    expect(impactUnit(estimated)).toBe(
      "bytes / bytes de todos os índices da tabela",
    );
    expect(impactPeriod(estimated)).toContain("25/09/2026");
    expect(impactUnit(undefined)).toBe("não informada");
    expect(impactPeriod({ ...estimated, period: "" })).toBe("não informado");
    expect(impactLabel(null)).toBe("impacto não estimado");
    expect(impactUnit({ ...estimated, status: "not_estimated" })).toBe(
      "não informada",
    );
  });

  it("describes a ranked item with impact, effort and confidence", () => {
    expect(priorityDescription(item.priority)).toBe(
      "Prioridade 1 · pontuação 2.10 / 7.5 · impacto alto · esforço médio · confiança média (70%)",
    );
  });
});

describe("PriorityList", () => {
  it("points each item to its finding id with score, impact, effort, confidence and risk", () => {
    const html = renderToStaticMarkup(
      <PriorityList items={[item]} openFinding={() => {}} />,
    );
    expect(html).toContain("<ol");
    expect(html).toContain("achado f-123");
    expect(html).toContain("2.10 / 7.5");
    expect(html).toContain("esforço médio");
    expect(html).toContain("confiança média");
    expect(html).toContain("Risco: Criar ou remover índice");
  });
});

describe("FindingImpactGain", () => {
  it("keeps hypothesis, predicted and measured gain apart and shows costs and the query contract", () => {
    const html = renderToStaticMarkup(
      <FindingImpactGain
        action={action({
          impact_estimate: estimated,
          gain: {
            hypothesis: "Índice sem uso.",
            predicted: "no máximo 40%",
            measured: "não medido: exige nova coleta",
            measured_status: "not_measured",
          },
          plan: {
            ...action().plan,
            query: {
              sql: "SELECT 1 FROM pg_catalog.pg_index LIMIT 100;",
              engine: "postgresql",
              capability: "pg_stat_user_indexes",
              min_permission: "pg_read_all_stats",
              params: ["$1 schema"],
              row_limit: 100,
              timeout_ms: 5000,
              impact_warning: "Execute em transação READ ONLY.",
            },
            maintenance: {
              kind: "retention",
              capability: "TimescaleDB",
              preconditions: "backup restaurável",
              read_cost: "r",
              write_cost: "w",
              storage_cost: "s",
              window: "janela",
              risk: "perda",
              post_validation: "nova coleta",
              business_approval: true,
            },
          },
        })}
      />,
    );
    for (const want of [
      "Unidade: </dt>",
      "bytes / bytes de todos os índices da tabela",
      "25/09/2026 12:00 UTC",
      "Hipótese</dt><dd>Índice sem uso.",
      "Ganho previsto</dt><dd>no máximo 40%",
      'data-status="not_measured"',
      "Custos de retenção (TimescaleDB)",
      "Exige aprovação de negócio",
      "Permissão mínima: </dt>",
      "até 100 linhas · timeout 5000 ms",
      "Execute em transação READ ONLY.",
    ]) {
      expect(html).toContain(want);
    }
  });

  it("falls back to not estimated and not measured without data", () => {
    const html = renderToStaticMarkup(<FindingImpactGain action={action()} />);
    expect(html).toContain("impacto não estimado");
    expect(html).toContain("não informada");
    expect(html).toContain("não medido");
    expect(html).not.toContain("Custos de");
  });
});

describe("priorities API", () => {
  it("requests the ranked top five for the environment", async () => {
    const fetch = vi.fn().mockResolvedValue(
      Response.json({
        items: [],
        candidates: 0,
        candidate_limit: 500,
        formula: "priority-v1",
        max_score: 7.5,
      }),
    );
    vi.stubGlobal("fetch", fetch);
    const { api } = await import("../services/api");
    await api.findingPriorities({ environment_id: "env-1", limit: 5 });
    expect(fetch.mock.calls[0][0]).toContain(
      "/api/v1/findings/priorities?environment_id=env-1&limit=5",
    );
  });
});
