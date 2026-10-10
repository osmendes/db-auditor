import type { Dispatch, SetStateAction } from "react";
import { PageHeader } from "../../components/PageHeader";
import { ConfirmDialog } from "../../components/ui";
import type { PageSize } from "../../components/ui/PaginationControls";
import type {
  AnalysisRun,
  AuditBaseline,
  AuditRun,
  AuditRunCoverage,
  BaselineComparison,
  CollectorRun,
  Environment,
  Finding,
  ScopeScore,
} from "../../types";
import { AuditRunControls } from "./AuditRunControls";
import { AuditRunDetail } from "./AuditRunDetail";
import { AuditRunsList } from "./AuditRunsList";
import type { AuditScheduleRow } from "./auditRunUi";

export function AuditRunsView({
  polling,
  confirmOpen,
  setConfirmOpen,
  triggerEnvName,
  busy,
  executeTrigger,
  environments,
  triggerEnv,
  setTriggerEnv,
  onTriggerClick,
  triggerMsg,
  schedules,
  setBusy,
  setScheduleMsg,
  setSchedules,
  scheduleMsg,
  statusFilter,
  setStatusFilter,
  profileFilter,
  setProfileFilter,
  setRunOffset,
  error,
  loadRuns,
  runs,
  selectedId,
  setSelectedId,
  openRun,
  runTotal,
  runPageSize,
  runOffset,
  setRunPageSize,
  selected,
  setError,
  baseline,
  baselineToken,
  setBaselineToken,
  chooseBaseline,
  comparison,
  scopeScore,
  coverage,
  analysis,
  loadRunDiagnostics,
  runFindings,
  openFinding,
  collectors,
  loadCollectors,
}: {
  polling: boolean;
  confirmOpen: boolean;
  setConfirmOpen: (value: boolean) => void;
  triggerEnvName: string;
  busy: boolean;
  executeTrigger: () => void;
  environments: Environment[];
  triggerEnv: string;
  setTriggerEnv: (value: string) => void;
  onTriggerClick: () => void;
  triggerMsg: string | null;
  schedules: AuditScheduleRow[];
  setBusy: Dispatch<SetStateAction<boolean>>;
  setScheduleMsg: Dispatch<SetStateAction<string | null>>;
  setSchedules: Dispatch<SetStateAction<AuditScheduleRow[]>>;
  scheduleMsg: string | null;
  statusFilter: string;
  setStatusFilter: (value: string) => void;
  profileFilter: string;
  setProfileFilter: (value: string) => void;
  setRunOffset: (value: number) => void;
  error: string | null;
  loadRuns: () => void;
  runs: AuditRun[] | null;
  selectedId: string | null;
  setSelectedId: (value: string) => void;
  openRun: (id: string) => void;
  runTotal: number;
  runPageSize: PageSize;
  runOffset: number;
  setRunPageSize: (value: PageSize) => void;
  selected: AuditRun | null;
  setError: (value: string | null) => void;
  baseline: AuditBaseline | null;
  baselineToken: string;
  setBaselineToken: (value: string) => void;
  chooseBaseline: () => void;
  comparison: BaselineComparison | null;
  scopeScore: ScopeScore | null;
  coverage: AuditRunCoverage[] | null;
  analysis: AnalysisRun | null;
  loadRunDiagnostics: (runId: string) => void;
  runFindings: Finding[] | null;
  openFinding: (id: string | null) => void;
  collectors: CollectorRun[] | null;
  loadCollectors: (runId: string) => void;
}) {
  return (
    <>
      <PageHeader
        eyebrow="Execuções"
        title="Execuções de auditoria"
        description="Dispare coletas, acompanhe o progresso por collector e revise erros."
        actions={
          polling ? (
            <span className="inline-flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-1.5 text-xs text-amber-200">
              <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-amber-400" />
              Atualizando…
            </span>
          ) : null
        }
      />

      <ConfirmDialog
        open={confirmOpen}
        title="Disparar auditoria manual"
        description={
          <>
            Será iniciada uma coleta somente leitura no ambiente{" "}
            <strong className="text-slate-100">{triggerEnvName}</strong>.
            Nenhuma alteração é aplicada nos bancos auditados.
          </>
        }
        confirmLabel="Executar agora"
        cancelLabel="Cancelar"
        busy={busy}
        onCancel={() => {
          if (!busy) {
            setConfirmOpen(false);
          }
        }}
        onConfirm={() => {
          void executeTrigger();
        }}
      />

      <div className="mt-8 space-y-6">
        <AuditRunControls
          environments={environments}
          triggerEnv={triggerEnv}
          setTriggerEnv={setTriggerEnv}
          busy={busy}
          onTriggerClick={onTriggerClick}
          triggerMsg={triggerMsg}
          schedules={schedules}
          setBusy={setBusy}
          setScheduleMsg={setScheduleMsg}
          setSchedules={setSchedules}
          scheduleMsg={scheduleMsg}
        />
        <AuditRunsList
          statusFilter={statusFilter}
          setStatusFilter={setStatusFilter}
          profileFilter={profileFilter}
          setProfileFilter={setProfileFilter}
          setRunOffset={setRunOffset}
          error={error}
          loadRuns={loadRuns}
          runs={runs}
          onTriggerClick={onTriggerClick}
          busy={busy}
          triggerEnv={triggerEnv}
          selectedId={selectedId}
          setSelectedId={setSelectedId}
          openRun={openRun}
          runTotal={runTotal}
          runPageSize={runPageSize}
          runOffset={runOffset}
          setRunPageSize={setRunPageSize}
        />
        {selected ? (
          <AuditRunDetail
            selected={selected}
            loadRuns={loadRuns}
            setError={setError}
            baseline={baseline}
            baselineToken={baselineToken}
            setBaselineToken={setBaselineToken}
            chooseBaseline={chooseBaseline}
            comparison={comparison}
            scopeScore={scopeScore}
            coverage={coverage}
            analysis={analysis}
            loadRunDiagnostics={loadRunDiagnostics}
            runFindings={runFindings}
            openFinding={openFinding}
            collectors={collectors}
            loadCollectors={loadCollectors}
          />
        ) : null}
      </div>
    </>
  );
}
