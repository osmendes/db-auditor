import { useEffect, useState } from "react";
import {
  type InventoryDetailTab,
  InventoryDetailTabs,
} from "../components/InventoryDetailTabs";
import { formatError } from "../lib/errors";
import { api } from "../services/api";
import type {
  AuditBaseline,
  ColumnSnapshot,
  ColumnStatSnapshot,
  ConstraintSnapshot,
  DependencySnapshot,
  Finding,
  GrantSnapshot,
  IndexSnapshot,
  RelationshipGraph,
  RLSPolicySnapshot,
  ScopeScore,
  TableAssessment,
  TableHistoryPoint,
  TableSnapshot,
  TriggerSnapshot,
  WorkloadSnapshot,
} from "../types";
import { TableDetail } from "./InventoryDetails";
import { AssessmentRunNotice } from "./parts/AssessmentSummary";
import { useCollection } from "./parts/assessmentCollection";
import { tablePermalink } from "./parts/RelationshipDiagram";
import {
  TableBaselineBox,
  TableFindingsTab,
  TableOverviewTab,
  TableRelationshipsTab,
  TableSecurityTab,
  TableStructureTab,
} from "./parts/TableAssessmentTabs";

export {
  AssessmentRunNotice,
  AssessmentScoreSummary,
} from "./parts/AssessmentSummary";
export {
  RelationshipDiagram,
  visibleRelationshipEdges,
} from "./parts/RelationshipDiagram";

export function TableAssessmentPanel({
  t,
  env,
  history,
  columnStats,
  workload,
  onNavigate,
}: {
  t: TableSnapshot;
  env: string;
  history: TableHistoryPoint[];
  columnStats: ColumnStatSnapshot[];
  workload: WorkloadSnapshot[];
  onNavigate: (schema: string, table: string) => void;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [assessment, setAssessment] = useState<TableAssessment | null>(null);
  const [graph, setGraph] = useState<RelationshipGraph | null>(null);
  const [baseline, setBaseline] = useState<AuditBaseline | null>(null);
  const [baselineToken, setBaselineToken] = useState("");
  const [scopeScore, setScopeScore] = useState<ScopeScore | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const key = `${env}/${t.audit_run_id}/${t.database_name}/${t.schema_name}/${t.table_name}`;
  const args = [
    env,
    t.audit_run_id,
    t.database_name,
    t.schema_name,
    t.table_name,
  ] as const;
  useEffect(() => {
    let cancelled = false;
    setTab("overview");
    setAssessment(null);
    setGraph(null);
    setLoading(true);
    api
      .tableAssessment(...args)
      .then((v) => {
        if (!cancelled) {
          setAssessment(v);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(formatError(cause, "Assessment indisponível"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // key is the canonical identity of this assessment.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  useEffect(() => {
    let active = true;
    void api
      .auditBaseline(env, t.database_name, t.schema_name, t.table_name)
      .then((value) => {
        if (active) setBaseline(value);
      })
      .catch(() => {
        if (active) setBaseline(null);
      });
    void api
      .scopeScore(
        env,
        t.audit_run_id,
        t.database_name,
        t.schema_name,
        t.table_name,
      )
      .then((value) => {
        if (active) setScopeScore(value);
      })
      .catch(() => {
        if (active) setScopeScore(null);
      });
    return () => {
      active = false;
    };
  }, [key]);

  const approveBaseline = async () => {
    if (
      !window.confirm(
        `Aprovar a execução ${t.audit_run_id} como baseline da tabela ${t.database_name}.${t.schema_name}.${t.table_name}?`,
      )
    )
      return;
    try {
      api.setReportToken(baselineToken);
      setBaseline(
        await api.selectAuditBaseline(
          env,
          t.audit_run_id,
          t.database_name,
          t.schema_name,
          t.table_name,
        ),
      );
      setError(null);
    } catch (cause: unknown) {
      setError(
        formatError(cause, "Não foi possível aprovar o baseline da tabela"),
      );
    }
  };
  useEffect(() => {
    if (tab !== "relationships" || !assessment) return;
    let cancelled = false;
    api
      .tableGraph(...args)
      .then((v) => {
        if (!cancelled) setGraph(v);
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(formatError(cause, "Grafo indisponível"));
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, tab, assessment]);
  const columns = useCollection<ColumnSnapshot>(
    assessment !== null && tab === "structure",
    key,
    (offset, limit) =>
      api.columns(env, {
        audit_run_id: t.audit_run_id,
        database: t.database_name,
        schema: t.schema_name,
        table: t.table_name,
        limit,
        offset,
      }),
  );
  const constraints = useCollection<ConstraintSnapshot>(
    assessment !== null && tab === "structure",
    key,
    (offset, limit) =>
      api.constraints(env, {
        audit_run_id: t.audit_run_id,
        database: t.database_name,
        schema: t.schema_name,
        table: t.table_name,
        limit,
        offset,
      }),
  );
  const indexes = useCollection<IndexSnapshot>(
    assessment !== null && tab === "structure",
    key,
    (offset, limit) =>
      api.indexes(env, {
        audit_run_id: t.audit_run_id,
        database: t.database_name,
        schema: t.schema_name,
        table: t.table_name,
        limit,
        offset,
      }),
  );
  const dependencies = useCollection<DependencySnapshot>(
    assessment !== null && tab === "relationships",
    key,
    (offset, limit) => api.tableDependencies(...args, limit, offset),
  );
  const grants = useCollection<GrantSnapshot>(
    assessment !== null && tab === "security",
    key,
    (offset, limit) => api.tableGrants(...args, limit, offset),
  );
  const triggers = useCollection<TriggerSnapshot>(
    assessment !== null && tab === "security",
    key,
    (offset, limit) => api.tableTriggers(...args, limit, offset),
  );
  const policies = useCollection<RLSPolicySnapshot>(
    assessment !== null && tab === "security",
    key,
    (offset, limit) => api.tableRLSPolicies(...args, limit, offset),
  );
  const findings = useCollection<Finding>(
    assessment !== null && tab === "recommendations",
    key,
    (offset, limit) => api.tableFindings(...args, limit, offset),
  );
  if (!loading && !assessment) {
    return (
      <p role="alert" className="text-sm text-red-300">
        {error ?? "Assessment não encontrado para este run."}
      </p>
    );
  }
  const table = assessment?.table ?? t;
  return (
    <div className="space-y-4">
      <a
        href={tablePermalink(env, t)}
        className="text-xs text-cyan-300 underline"
      >
        Link direto para esta tabela e execução
      </a>
      {loading ? (
        <p role="status" className="text-sm text-slate-400">
          Carregando assessment…
        </p>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-red-300">
          {error}
        </p>
      ) : null}
      <AssessmentRunNotice partial={assessment?.run.partial ?? false} />
      <TableBaselineBox
        assessment={assessment}
        baseline={baseline}
        scopeScore={scopeScore}
        baselineToken={baselineToken}
        setBaselineToken={setBaselineToken}
        approveBaseline={approveBaseline}
      />
      <InventoryDetailTabs tab={tab} onTabChange={setTab}>
        {tab === "overview" ? (
          <TableOverviewTab t={t} assessment={assessment} />
        ) : null}
        {tab === "structure" ? (
          <TableStructureTab
            columns={columns}
            constraints={constraints}
            indexes={indexes}
          />
        ) : null}
        {tab === "relationships" ? (
          <TableRelationshipsTab
            t={t}
            env={env}
            graph={graph}
            dependencies={dependencies}
            onNavigate={onNavigate}
          />
        ) : null}
        {tab === "performance" ? (
          <TableDetail
            t={table}
            history={history}
            columnStats={columnStats}
            workload={workload}
          />
        ) : null}
        {tab === "security" ? (
          <TableSecurityTab
            assessment={assessment}
            grants={grants}
            policies={policies}
            triggers={triggers}
          />
        ) : null}
        {tab === "recommendations" ? (
          <TableFindingsTab findings={findings} />
        ) : null}
      </InventoryDetailTabs>
    </div>
  );
}
