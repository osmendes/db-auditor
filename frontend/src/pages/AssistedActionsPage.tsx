import { useEffect, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import { EmptyState, ErrorBanner, Select, Skeleton } from "../components/ui";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { api } from "../services/api";
import type {
  ActionMeasurement,
  AuditRun,
  EnvironmentCapabilities,
  PageMeta,
  QualityIssue,
  QualityScan,
  TrackedAction,
} from "../types";
import { AssistedActionList } from "./parts/AssistedActionList";
import { AssistedMeasurementCard } from "./parts/AssistedMeasurementCard";
import { AssistedScanResults } from "./parts/AssistedScanResults";
import { AssistedSetupCards } from "./parts/AssistedSetupCards";
import { words } from "./parts/assistedActionLabels";

export function AssistedActionsPage() {
  const { environmentId, environments, setEnvironmentId, openFinding } =
    useApp();
  const [capabilities, setCapabilities] =
    useState<EnvironmentCapabilities | null>(null);
  const [scans, setScans] = useState<QualityScan[]>([]);
  const [actions, setActions] = useState<TrackedAction[]>([]);
  const [actionOffset, setActionOffset] = useState(0);
  const [actionPage, setActionPage] = useState<PageMeta | null>(null);
  const [runs, setRuns] = useState<AuditRun[]>([]);
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [database, setDatabase] = useState("");
  const [schema, setSchema] = useState("public");
  const [table, setTable] = useState("");
  const [limit, setLimit] = useState(500);
  const [nonNull, setNonNull] = useState("");
  const [keys, setKeys] = useState("");
  const [dateColumn, setDateColumn] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [selectedIssue, setSelectedIssue] = useState<QualityIssue | null>(null);
  const [issueStatus, setIssueStatus] = useState("suggested");
  const [owner, setOwner] = useState("");
  const [justification, setJustification] = useState("");
  const [result, setResult] = useState("");
  const [selectedAction, setSelectedAction] = useState<TrackedAction | null>(
    null,
  );
  const [measurements, setMeasurements] = useState<ActionMeasurement[]>([]);
  const [measurementOffset, setMeasurementOffset] = useState(0);
  const [measurementPage, setMeasurementPage] = useState<PageMeta | null>(null);
  const [before, setBefore] = useState("");
  const [after, setAfter] = useState("");
  const [metric, setMetric] =
    useState<ActionMeasurement["metric"]>("finding_observed");
  const [hypothesis, setHypothesis] = useState("");
  const [windowNote, setWindowNote] = useState("");
  const [workloadComparable, setWorkloadComparable] = useState(false);
  const refresh = async (env: string) => {
    setLoading(true);
    setError("");
    try {
      const [c, q, a, r] = await Promise.all([
        api.environmentCapabilities(env),
        api.qualityScans(env),
        api.trackedActions(env, actionOffset),
        api.auditRuns({ environment_id: env }),
      ]);
      setCapabilities(c);
      setScans(q.items);
      setActions(a.items);
      setActionPage(a.page);
      setRuns(
        r.items.filter(
          (run) => run.status === "success" || run.status === "partial_success",
        ),
      );
      setEnabled(q.enabled);
    } catch (cause) {
      setError(
        formatError(cause, "Não foi possível carregar as ações assistidas"),
      );
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    if (environmentId) void refresh(environmentId);
    else {
      setCapabilities(null);
      setScans([]);
      setActions([]);
      setRuns([]);
    }
  }, [environmentId, actionOffset]);
  const startScan = async () => {
    if (!environmentId) return;
    setBusy(true);
    setError("");
    try {
      await api.runQualityScan(environmentId, {
        database: database.trim(),
        schema: schema.trim(),
        table: table.trim(),
        limit,
        expected_non_null: words(nonNull),
        candidate_keys: words(keys),
        date_ranges:
          dateColumn.trim() && dateFrom && dateTo
            ? [
                {
                  column: dateColumn.trim(),
                  from: new Date(dateFrom).toISOString(),
                  to: new Date(dateTo).toISOString(),
                },
              ]
            : [],
      });
      await refresh(environmentId);
    } catch (cause) {
      setError(formatError(cause, "Diagnóstico não concluído"));
    } finally {
      setBusy(false);
    }
  };
  const chooseIssue = (item: QualityIssue) => {
    setSelectedIssue(item);
    setIssueStatus(item.status);
    setOwner(item.owner);
    setJustification(item.justification);
    setResult(item.result);
  };
  const saveIssue = async () => {
    if (!environmentId || !selectedIssue) return;
    setBusy(true);
    try {
      await api.updateQualityIssue(environmentId, selectedIssue.id, {
        status: issueStatus,
        owner,
        justification,
        result,
      });
      setSelectedIssue(null);
      await refresh(environmentId);
    } catch (cause) {
      setError(formatError(cause, "Falha ao salvar decisão"));
    } finally {
      setBusy(false);
    }
  };
  const chooseAction = async (item: TrackedAction) => {
    setSelectedAction(item);
    setMeasurementOffset(0);
    const page = await api.actionMeasurements(item.finding_id);
    setMeasurements(page.items);
    setMeasurementPage(page.page);
    setBefore("");
    setAfter("");
  };
  const measure = async () => {
    if (!selectedAction) return;
    setBusy(true);
    try {
      await api.recordActionMeasurement(selectedAction.finding_id, {
        before_run_id: before,
        after_run_id: after,
        metric,
        hypothesis: hypothesis.trim(),
        window_note: windowNote.trim(),
        workload_comparable: workloadComparable,
      });
      const page = await api.actionMeasurements(selectedAction.finding_id);
      setMeasurements(page.items);
      setMeasurementOffset(0);
      setMeasurementPage(page.page);
      await refresh(environmentId || "");
    } catch (cause) {
      setError(formatError(cause, "Medição não registrada"));
    } finally {
      setBusy(false);
    }
  };
  const exportActions = (format: "csv" | "jsonl") => {
    if (!environmentId) return;
    const link = document.createElement("a");
    link.href = api.actionExportURL(environmentId, format);
    link.download = `acoes-${new Date().toISOString().slice(0, 10)}.${format}`;
    document.body.appendChild(link);
    link.click();
    link.remove();
  };
  return (
    <>
      <PageHeader
        eyebrow="Decisões"
        title="Ações assistidas"
        description="Investigue qualidade de dados somente com autorização. O auditor guarda contagens e decisões; correções ocorrem fora dele, após revisão humana."
      />
      <div className="mt-8 space-y-5">
        <Select
          label="Ambiente"
          value={environmentId ?? ""}
          onChange={(event) => {
            setActionOffset(0);
            setEnvironmentId(event.target.value || null);
          }}
          options={[
            { value: "", label: "Selecione um ambiente" },
            ...environments.map((env) => ({ value: env.id, label: env.name })),
          ]}
        />
        {error ? (
          <ErrorBanner
            message={error}
            onRetry={() => environmentId && void refresh(environmentId)}
          />
        ) : null}
        {loading ? <Skeleton className="h-40" /> : null}
        {!environmentId ? (
          <EmptyState
            title="Selecione um ambiente"
            description="Escolha onde acompanhar ações e diagnósticos."
          />
        ) : null}
        {environmentId && !loading ? (
          <>
            <AssistedSetupCards
              capabilities={capabilities}
              enabled={enabled}
              database={database}
              setDatabase={setDatabase}
              schema={schema}
              setSchema={setSchema}
              table={table}
              setTable={setTable}
              limit={limit}
              setLimit={setLimit}
              nonNull={nonNull}
              setNonNull={setNonNull}
              keys={keys}
              setKeys={setKeys}
              dateColumn={dateColumn}
              setDateColumn={setDateColumn}
              dateFrom={dateFrom}
              setDateFrom={setDateFrom}
              dateTo={dateTo}
              setDateTo={setDateTo}
              busy={busy}
              startScan={startScan}
            />
            <AssistedScanResults
              scans={scans}
              chooseIssue={chooseIssue}
              selectedIssue={selectedIssue}
              issueStatus={issueStatus}
              setIssueStatus={setIssueStatus}
              owner={owner}
              setOwner={setOwner}
              justification={justification}
              setJustification={setJustification}
              result={result}
              setResult={setResult}
              busy={busy}
              saveIssue={saveIssue}
            />
            <AssistedActionList
              actions={actions}
              exportActions={exportActions}
              openFinding={openFinding}
              chooseAction={chooseAction}
              actionPage={actionPage}
              actionOffset={actionOffset}
              setActionOffset={setActionOffset}
            />
            <AssistedMeasurementCard
              selectedAction={selectedAction}
              before={before}
              setBefore={setBefore}
              after={after}
              setAfter={setAfter}
              runs={runs}
              metric={metric}
              setMetric={setMetric}
              hypothesis={hypothesis}
              setHypothesis={setHypothesis}
              windowNote={windowNote}
              setWindowNote={setWindowNote}
              workloadComparable={workloadComparable}
              setWorkloadComparable={setWorkloadComparable}
              busy={busy}
              measure={measure}
              measurements={measurements}
              measurementPage={measurementPage}
              measurementOffset={measurementOffset}
              setMeasurements={setMeasurements}
              setMeasurementPage={setMeasurementPage}
              setMeasurementOffset={setMeasurementOffset}
              setError={setError}
            />
          </>
        ) : null}
      </div>
    </>
  );
}
