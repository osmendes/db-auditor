import { CoverageBanner } from "../../components/CoverageBanner";
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
  setSection,
}: {
  kpis: DashboardKPIs;
  scores: ScopeAggregate[];
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
              subtitle={`${item.database_name}.${item.schema_name ?? ""} · ${item.tables} ${item.status === "not_applicable" ? "coleções" : "tabelas"}`}
              title={
                item.status === "not_applicable"
                  ? "não aplicável"
                  : item.score == null
                    ? "cobertura insuficiente"
                    : `${item.score}/100`
              }
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
      {kpis.inventory_status !== "complete" ? (
        <CoverageBanner
          kind={kpis.inventory_status === "partial" ? "partial" : null}
        />
      ) : null}
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
