import { DetailGrid, DetailSection } from "../../components/ui/Sheet";
import { formatBytes } from "../../lib/format";
import { api } from "../../services/api";
import type {
  AuditBaseline,
  ColumnSnapshot,
  ConstraintSnapshot,
  DependencySnapshot,
  Finding,
  GrantSnapshot,
  IndexSnapshot,
  RelationshipGraph,
  RLSPolicySnapshot,
  ScopeScore,
  TableAssessment,
  TableSnapshot,
  TriggerSnapshot,
} from "../../types";
import { AssessmentScoreSummary } from "./AssessmentSummary";
import { type Collection, CollectionState } from "./assessmentCollection";
import { RelationshipDiagram } from "./RelationshipDiagram";

/** Tab bodies of TableAssessmentPanel; same markup, moved out (UI-06). */
export function TableBaselineBox({
  assessment,
  baseline,
  scopeScore,
  baselineToken,
  setBaselineToken,
  approveBaseline,
}: {
  assessment: TableAssessment | null;
  baseline: AuditBaseline | null;
  scopeScore: ScopeScore | null;
  baselineToken: string;
  setBaselineToken: (value: string) => void;
  approveBaseline: () => Promise<void>;
}) {
  return (
    <div className="rounded border border-slate-700 bg-slate-900/60 p-3 text-xs text-slate-300">
      <p>
        Baseline desta tabela:{" "}
        {baseline ? `${baseline.audit_run_id.slice(0, 8)}…` : "não definido"}
      </p>
      <p>
        Score de escopo:{" "}
        {scopeScore?.score == null
          ? scopeScore?.status === "not_applicable"
            ? "não aplicável a este mecanismo"
            : "indisponível por cobertura"
          : `${scopeScore.score}/100`}{" "}
        · confiança {Math.round((scopeScore?.confidence ?? 0) * 100)}%
      </p>
      {scopeScore?.categories.map((category) => (
        <p key={category.category}>
          {category.category}: {category.score}/100 · {category.findings}{" "}
          achados · penalidade {category.penalty} · evidência positiva +
          {category.positive}
        </p>
      ))}
      {assessment?.run.status === "success" ? (
        <div className="mt-2 flex flex-wrap gap-2">
          {!api.hasSession() && (
            <input
              aria-label="Token para baseline da tabela"
              type="password"
              autoComplete="off"
              placeholder="Token de operação protegida"
              value={baselineToken}
              onChange={(event) => setBaselineToken(event.target.value)}
              className="rounded border border-slate-600 bg-slate-900 px-2 py-1 text-slate-100"
            />
          )}
          <button
            type="button"
            disabled={!api.hasRole("operator") && !baselineToken}
            className="rounded border border-slate-500 px-2 py-1 text-cyan-300 disabled:opacity-50"
            onClick={() => void approveBaseline()}
          >
            Aprovar esta execução como baseline da tabela
          </button>
        </div>
      ) : null}
    </div>
  );
}

export function TableOverviewTab({
  t,
  assessment,
}: {
  t: TableSnapshot;
  assessment: TableAssessment | null;
}) {
  return (
    <>
      <DetailSection title="Tabela">
        <DetailGrid
          items={[
            {
              label: "Banco / esquema",
              value: `${t.database_name} / ${t.schema_name}`,
            },
            { label: "Tabela", value: t.table_name },
            { label: "Tipo", value: t.relation_class || t.relkind },
            {
              label: "Linhas estimadas",
              value: t.row_estimate.toLocaleString("pt-BR"),
            },
            {
              label: "Armazenamento",
              value: formatBytes(t.total_size_bytes),
            },
            {
              label: "Chave primária",
              value: t.has_primary_key ? "Sim" : "Não",
            },
            {
              label: "Comentário",
              value: assessment?.table.comment ?? "—",
            },
            {
              label: "Área de armazenamento",
              value: assessment?.table.tablespace_name ?? "—",
            },
            {
              label: "Persistência",
              value: assessment?.table.persistence ?? "—",
            },
            {
              label: "Parâmetros de armazenamento",
              value: assessment?.table.storage_parameters.join(", ") || "—",
            },
            { label: "Execução", value: t.audit_run_id },
          ]}
        />
      </DetailSection>
      <DetailSection title="Resumo da execução">
        <DetailGrid
          items={[
            {
              label: "Colunas",
              value: assessment?.summary.columns ?? "—",
            },
            {
              label: "Restrições",
              value: assessment?.summary.constraints ?? "—",
            },
            {
              label: "Índices",
              value: assessment?.summary.indexes ?? "—",
            },
            {
              label: "Achados",
              value: assessment?.summary.findings ?? "—",
            },
            {
              label: "Permissões",
              value: assessment?.summary.grants ?? "—",
            },
            {
              label: "Dependências",
              value: assessment?.summary.dependencies ?? "—",
            },
          ]}
        />
      </DetailSection>
      {assessment ? (
        <DetailSection title="Score estrutural da tabela">
          <AssessmentScoreSummary summary={assessment.summary} />
        </DetailSection>
      ) : null}
      <p className="text-xs text-slate-400">
        Contrato v{assessment?.version ?? 1}; as coleções relacionadas são
        paginadas e sempre usam esta execução.
      </p>
    </>
  );
}

export function TableStructureTab({
  columns,
  constraints,
  indexes,
}: {
  columns: Collection<ColumnSnapshot>;
  constraints: Collection<ConstraintSnapshot>;
  indexes: Collection<IndexSnapshot>;
}) {
  return (
    <div className="space-y-5">
      <DetailSection title="Colunas">
        <CollectionState data={columns}>
          <ul className="space-y-1 text-sm">
            {columns.items.map((c) => (
              <li key={c.id} className="rounded border border-slate-700 p-2">
                <strong>{c.column_name}</strong> · {c.data_type} ·{" "}
                {c.is_nullable ? "nullable" : "not null"}
                {c.comment ? ` · ${c.comment}` : ""}
              </li>
            ))}
          </ul>
        </CollectionState>
      </DetailSection>
      <DetailSection title="Constraints">
        <CollectionState data={constraints}>
          <ul className="space-y-1 text-sm">
            {constraints.items.map((c) => (
              <li key={c.id} className="rounded border border-slate-700 p-2">
                {c.constraint_name} · {c.constraint_type} ·{" "}
                {c.is_validated ? "validada" : "não validada"}
                {c.referenced_table_name
                  ? ` → ${c.referenced_schema_name}.${c.referenced_table_name}`
                  : ""}
              </li>
            ))}
          </ul>
        </CollectionState>
      </DetailSection>
      <DetailSection title="Índices">
        <CollectionState data={indexes}>
          <ul className="space-y-1 text-sm">
            {indexes.items.map((i) => (
              <li key={i.id} className="rounded border border-slate-700 p-2">
                {i.index_name} · {i.access_method ?? "índice"} ·{" "}
                {formatBytes(i.size_bytes)}
              </li>
            ))}
          </ul>
        </CollectionState>
      </DetailSection>
    </div>
  );
}

export function TableRelationshipsTab({
  t,
  env,
  graph,
  dependencies,
  onNavigate,
}: {
  t: TableSnapshot;
  env: string;
  graph: RelationshipGraph | null;
  dependencies: Collection<DependencySnapshot>;
  onNavigate: (schema: string, table: string) => void;
}) {
  return (
    <div className="space-y-5">
      <DetailSection title="Foreign keys">
        {graph ? (
          <RelationshipDiagram
            graph={graph}
            root={t}
            env={env}
            run={t.audit_run_id}
            database={t.database_name}
            onNavigate={onNavigate}
          />
        ) : (
          <p role="status" className="text-sm text-slate-400">
            Carregando grafo…
          </p>
        )}
      </DetailSection>
      <DetailSection title="Dependências de views e funções">
        <CollectionState data={dependencies}>
          <ul className="space-y-1 text-sm">
            {dependencies.items.map((d) => (
              <li
                key={`${d.source_kind}:${d.source_schema}.${d.source_name}->${d.target_schema}.${d.target_name}`}
                className="rounded border border-slate-700 p-2"
              >
                {d.source_kind} {d.source_schema}.{d.source_name} →{" "}
                {d.target_kind} {d.target_schema}.{d.target_name}
              </li>
            ))}
          </ul>
        </CollectionState>
      </DetailSection>
    </div>
  );
}

export function TableSecurityTab({
  assessment,
  grants,
  policies,
  triggers,
}: {
  assessment: TableAssessment | null;
  grants: Collection<GrantSnapshot>;
  policies: Collection<RLSPolicySnapshot>;
  triggers: Collection<TriggerSnapshot>;
}) {
  return (
    <div className="space-y-5">
      <DetailSection title="Row Level Security">
        <p className="text-sm">
          RLS {assessment?.table.rls_enabled ? "habilitado" : "desabilitado"}
          {";"}
          FORCE {assessment?.table.rls_forced ? "habilitado" : "desabilitado"}.
        </p>
      </DetailSection>
      <DetailSection title="Permissões efetivas">
        <CollectionState data={grants}>
          <ul className="space-y-1 text-sm">
            {grants.items.map((g) => (
              <li key={g.grantee}>
                {g.grantee}: {g.privileges.join(", ")}
              </li>
            ))}
          </ul>
        </CollectionState>
      </DetailSection>
      <DetailSection title="Políticas">
        <CollectionState data={policies}>
          <ul className="space-y-1 text-sm">
            {policies.items.map((p) => (
              <li key={p.name}>
                {p.name} · {p.command ?? "ALL"} · {p.roles.join(", ")}
              </li>
            ))}
          </ul>
        </CollectionState>
      </DetailSection>
      <DetailSection title="Triggers">
        <CollectionState data={triggers}>
          <ul className="space-y-1 text-sm">
            {triggers.items.map((tr) => (
              <li key={tr.name}>
                {tr.name} · {tr.enabled} · {tr.timing ?? "—"} {tr.events ?? ""}
              </li>
            ))}
          </ul>
        </CollectionState>
      </DetailSection>
    </div>
  );
}

export function TableFindingsTab({
  findings,
}: {
  findings: Collection<Finding>;
}) {
  return (
    <DetailSection title="Findings do run">
      <CollectionState data={findings}>
        <ul className="space-y-2 text-sm">
          {findings.items.map((f) => (
            <li key={f.id} className="rounded border border-slate-700 p-3">
              <strong>{f.title}</strong>
              <span className="ml-2 text-xs text-slate-400">
                {f.severity} · {f.rule_id} v{f.rule_version} · confiança{" "}
                {Math.round((f.confidence ?? 0) * 100)}%
              </span>
              <p>{f.summary}</p>
              <p className="text-slate-300">{f.recommendation}</p>
              {f.validation ? (
                <p className="text-xs text-slate-400">
                  Validação: {f.validation}
                </p>
              ) : null}
              {f.evidence ? (
                <details className="mt-2 text-xs text-slate-300">
                  <summary>Evidência</summary>
                  <pre className="overflow-auto whitespace-pre-wrap">
                    {JSON.stringify(f.evidence, null, 2)}
                  </pre>
                </details>
              ) : null}
            </li>
          ))}
        </ul>
      </CollectionState>
    </DetailSection>
  );
}
