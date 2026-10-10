import {
  Badge,
  Button,
  EmptyState,
  ErrorBanner,
  Select,
  Skeleton,
  Table,
} from "../../components/ui";
import {
  type PageSize,
  PaginationControls,
} from "../../components/ui/PaginationControls";
import { labels } from "../../lib/labels";
import type { AuditRun } from "../../types";
import {
  durationLabel,
  envLabel,
  PROFILE_OPTIONS,
  STATUS_OPTIONS,
  statusTone,
} from "./auditRunUi";

export function AuditRunsList({
  statusFilter,
  setStatusFilter,
  profileFilter,
  setProfileFilter,
  setRunOffset,
  error,
  loadRuns,
  runs,
  onTriggerClick,
  busy,
  triggerEnv,
  selectedId,
  setSelectedId,
  openRun,
  runTotal,
  runPageSize,
  runOffset,
  setRunPageSize,
}: {
  statusFilter: string;
  setStatusFilter: (value: string) => void;
  profileFilter: string;
  setProfileFilter: (value: string) => void;
  setRunOffset: (value: number) => void;
  error: string | null;
  loadRuns: () => void;
  runs: AuditRun[] | null;
  onTriggerClick: () => void;
  busy: boolean;
  triggerEnv: string;
  selectedId: string | null;
  setSelectedId: (value: string) => void;
  openRun: (id: string) => void;
  runTotal: number;
  runPageSize: PageSize;
  runOffset: number;
  setRunPageSize: (value: PageSize) => void;
}) {
  return (
    <>
      <div className="grid w-full gap-3 sm:grid-cols-2">
        <Select
          label="Status"
          options={STATUS_OPTIONS}
          value={statusFilter}
          onChange={(e) => {
            setStatusFilter(e.target.value);
            setRunOffset(0);
          }}
        />
        <Select
          label="Perfil"
          options={PROFILE_OPTIONS}
          value={profileFilter}
          onChange={(e) => {
            setProfileFilter(e.target.value);
            setRunOffset(0);
          }}
        />
      </div>

      {error ? (
        <ErrorBanner message={error} onRetry={() => void loadRuns()} />
      ) : null}

      {runs === null ? (
        <Skeleton className="h-40 w-full" />
      ) : runs.length === 0 ? (
        <EmptyState
          title="Nenhuma execução"
          description="Dispare uma auditoria manual acima ou aguarde o scheduler. Ambientes demo vêm do seed local."
          action={
            <Button onClick={onTriggerClick} disabled={busy || !triggerEnv}>
              Executar agora
            </Button>
          }
        />
      ) : (
        <>
          <Table
            dense
            pagination={false}
            headers={["Perfil", "Status", "Duração", "Ambiente"]}
          >
            {runs.map((r) => (
              <tr
                key={r.id}
                className={`cursor-pointer border-t border-slate-800 hover:bg-slate-900/50 ${
                  selectedId === r.id ? "bg-slate-900/80" : ""
                }`}
                onClick={() => {
                  setSelectedId(r.id);
                  openRun(r.id);
                }}
              >
                <td className="px-3 py-1.5 text-slate-100">{r.profile}</td>
                <td className="px-3 py-1.5">
                  <Badge tone={statusTone(r.status)}>
                    {labels.runStatus(r.status)}
                  </Badge>
                </td>
                <td className="px-3 py-1.5 text-xs text-slate-400">
                  {durationLabel(r.started_at, r.finished_at)}
                  <span className="mt-0.5 block font-mono text-[10px] text-slate-500">
                    {new Date(r.started_at).toLocaleString()}
                  </span>
                </td>
                <td className="px-3 py-1.5 text-slate-200">{envLabel(r)}</td>
              </tr>
            ))}
          </Table>
          <PaginationControls
            total={runTotal}
            offset={runPageSize === "all" ? 0 : runOffset}
            size={runPageSize}
            onSizeChange={(next) => {
              setRunPageSize(next);
              setRunOffset(0);
            }}
            onOffsetChange={setRunOffset}
            label="Execuções"
          />
        </>
      )}
    </>
  );
}
