import { CoverageBanner } from "../components/CoverageBanner";
import { PageHeader } from "../components/PageHeader";
import { Button, Card, ErrorBanner } from "../components/ui";
import { ReportDecisionsCard } from "./parts/ReportDecisionsCard";
import { ReportJobList } from "./parts/ReportJobList";
import { ReportRequestCard } from "./parts/ReportRequestCard";
import { reportJobActions, useReportsPage } from "./parts/useReportsPage";

export { reportFormReady, reportJobActions } from "./parts/useReportsPage";

export function ReportsPage() {
  const s = useReportsPage();
  const {
    act,
    coverage,
    env,
    error,
    jobs,
    kind,
    preview,
    refresh,
    reportEmptyCause,
    runId,
    setSection,
    showPreview,
  } = s;
  return (
    <>
      <PageHeader
        eyebrow="Relatórios"
        title="Relatórios"
        description="Gere relatórios versionados a partir de uma execução específica. Acesso controlado pela conta local."
      />
      <div className="mt-8 space-y-5">
        {/* Coverage of the selected run: the PDF inherits these limits. */}
        {env && runId ? <CoverageBanner signals={coverage.signals} /> : null}
        <ReportRequestCard s={s} />
        {kind === "architecture" &&
        import.meta.env.VITE_ARCHITECTURE_REPORT_ENABLED === "true" ? (
          <ReportDecisionsCard s={s} />
        ) : null}
        {error ? (
          <ErrorBanner message={error} onRetry={() => void refresh()} />
        ) : null}
        <ReportJobList
          jobs={jobs}
          act={act}
          reportJobActions={reportJobActions}
          emptyCause={reportEmptyCause}
          onEmptyAction={() =>
            setSection(reportEmptyCause === "dsn" ? "Ambientes" : "Execuções")
          }
        />
        {jobs
          .filter(
            (job) =>
              job.report_type === "architecture" && job.status === "success",
          )
          .map((job) => (
            <Button
              key={job.id}
              variant="secondary"
              onClick={() => void showPreview(job)}
            >
              Prévia textual · {job.id.slice(0, 8)}
            </Button>
          ))}
        {preview ? (
          <Card title="Prévia Markdown">
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs text-slate-300">
              {preview}
            </pre>
          </Card>
        ) : null}
      </div>
    </>
  );
}
