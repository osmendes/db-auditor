import { Card } from "../../components/ui";
import type {
  DashboardKPIs,
  NavigationSection,
  ScopeAggregate,
} from "../../types";
import { formatBytes, KpiGroup } from "./DashboardCharts";

export function DashboardKpis({
  kpis,
  scores,
  scorePartial = false,
  setSection,
}: {
  kpis: DashboardKPIs;
  scores: ScopeAggregate[];
  /** The scored run is partial: label the number instead of showing it bare. */
  scorePartial?: boolean;
  setSection: (section: NavigationSection) => void;
}) {
  return (
    <div className="mt-8 space-y-6">
      <KpiGroup title="Risco">
        <Card
          subtitle="Achados abertos"
          title={String(kpis.open_findings)}
          onClick={() => setSection("Findings")}
        />
        <Card
          subtitle="Críticos / altos"
          title={`${kpis.critical_findings} / ${kpis.high_findings}`}
          onClick={() => setSection("Findings")}
        />
        <Card
          subtitle="Execuções concluídas / com falha"
          title={`${kpis.successful_runs_recent} / ${kpis.failed_runs_recent}`}
          onClick={() => setSection("Execuções")}
        />
      </KpiGroup>
      {scores.length > 0 ? (
        <KpiGroup title="Nota por esquema">
          {scores.slice(0, 6).map((item) => (
            <Card
              key={`${item.database_name}.${item.schema_name ?? ""}`}
              subtitle={scoreSubtitle(item, scorePartial)}
              title={scoreTitle(item, scorePartial)}
            />
          ))}
        </KpiGroup>
      ) : null}
      <KpiGroup title="Capacidade">
        <Card
          subtitle="Total de bancos"
          title={kpis.databases == null ? "—" : String(kpis.databases)}
          onClick={() => setSection("Inventário")}
        />
        <Card
          subtitle="Total de esquemas"
          title={kpis.schemas == null ? "—" : String(kpis.schemas)}
          onClick={() => setSection("Inventário")}
        />
        <Card
          subtitle="Total de Tabelas"
          title={kpis.tables == null ? "—" : String(kpis.tables)}
          onClick={() => setSection("Inventário")}
        />
        <Card
          subtitle="Armazenamento total"
          title={formatBytes(kpis.total_storage_bytes)}
          onClick={() => setSection("Inventário")}
        />
        <Card subtitle="Tabelas temporais" title={String(kpis.hypertables)} />
        <Card subtitle="Políticas" title={String(kpis.policies)} />
      </KpiGroup>
      {kpis.inventory_status === "empty" ? (
        <p className="text-sm text-amber-300" role="status">
          Ainda não há inventário concluído para o ambiente selecionado.
        </p>
      ) : null}
      <KpiGroup title="Operação">
        <Card
          subtitle="Ambientes"
          title={String(kpis.environments)}
          onClick={() => setSection("Ambientes")}
        />
        <Card
          subtitle="Tarefas agendadas"
          title={String(kpis.jobs_scheduled)}
        />
      </KpiGroup>
    </div>
  );
}

/** Not inflating a partial score: it carries "parcial" and its confidence. */
export function scoreTitle(item: ScopeAggregate, runPartial: boolean): string {
  if (item.status === "not_applicable") return "não aplicável";
  if (item.score == null) return "cobertura insuficiente";
  return runPartial || item.confidence < 1
    ? `${item.score}/100 (parcial)`
    : `${item.score}/100`;
}

export function scoreSubtitle(
  item: ScopeAggregate,
  runPartial: boolean,
): string {
  const scope = `${item.database_name}.${item.schema_name ?? ""} · ${item.tables} ${item.status === "not_applicable" ? "coleções" : "tabelas"}`;
  if (item.score == null || item.status === "not_applicable") return scope;
  return runPartial || item.confidence < 1
    ? `${scope} · confiança ${Math.round(item.confidence * 100)}%`
    : scope;
}
