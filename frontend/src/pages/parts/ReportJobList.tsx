import { Badge, Button, Card, EmptyState } from "../../components/ui";
import type { EmptyCause } from "../../lib/emptyCause";
import type { ReportJob } from "../../types";

const reportStatusLabels: Record<string, string> = {
  queued: "Na fila",
  running: "Em andamento",
  success: "Concluído",
  failed: "Falhou",
  canceled: "Cancelado",
};

const reportTypeLabels: Record<ReportJob["report_type"], string> = {
  executive: "Executivo",
  technical: "Técnico",
  table: "Tabela",
  architecture: "Análise completa e modelagem",
};

export function ReportJobList({
  jobs,
  act,
  reportJobActions,
  emptyCause = null,
  onEmptyAction,
}: {
  jobs: ReportJob[];
  /** never/dsn when there is no completed run to report on (UI-09). */
  emptyCause?: EmptyCause | null;
  onEmptyAction?: () => void;
  act: (job: ReportJob, action: "cancel" | "retry" | "download") => void;
  reportJobActions: (
    job: ReportJob,
    now?: number,
  ) => Array<"download" | "retry" | "cancel">;
}) {
  return (
    <>
      {jobs.length === 0 ? (
        emptyCause ? (
          <EmptyState
            cause={emptyCause}
            subject={{ none: "Nenhum relatório", plural: "relatórios" }}
            onAction={onEmptyAction}
          />
        ) : (
          <EmptyState
            title="Sem relatórios"
            description="Selecione o ambiente e solicite o primeiro relatório."
          />
        )
      ) : null}
      {jobs.map((job) => (
        <Card
          key={job.id}
          title={`${reportTypeLabels[job.report_type]} · ${job.audit_run_id.slice(0, 8)}…`}
          subtitle={`Solicitado em ${new Date(job.created_at).toLocaleString()} · expira em ${new Date(job.expires_at).toLocaleDateString()}`}
        >
          <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-slate-300">
            <Badge
              tone={
                job.status === "success" &&
                Date.parse(job.expires_at) > Date.now()
                  ? "success"
                  : job.status === "failed"
                    ? "danger"
                    : "warning"
              }
            >
              {Date.parse(job.expires_at) <= Date.now()
                ? "expirado"
                : (reportStatusLabels[job.status] ?? job.status)}
            </Badge>
            <span>Tentativa {job.attempts}/3</span>
            {job.sha256 ? (
              <span title={job.sha256}>
                SHA-256: {job.sha256.slice(0, 12)}…
              </span>
            ) : null}
          </div>
          {job.error ? (
            <p className="mt-2 text-xs text-rose-300">{job.error}</p>
          ) : null}
          <div className="mt-3 flex gap-2">
            {reportJobActions(job).includes("download") ? (
              <Button onClick={() => void act(job, "download")}>
                Baixar PDF
              </Button>
            ) : null}
            {reportJobActions(job).includes("retry") ? (
              <Button onClick={() => void act(job, "retry")}>
                Tentar novamente
              </Button>
            ) : null}
            {reportJobActions(job).includes("cancel") ? (
              <Button
                variant="secondary"
                onClick={() => void act(job, "cancel")}
              >
                Cancelar
              </Button>
            ) : null}
          </div>
        </Card>
      ))}
    </>
  );
}
