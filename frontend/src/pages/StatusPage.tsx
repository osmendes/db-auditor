import { useEffect, useState } from "react";
import { Card, ErrorBanner, Table } from "../components/ui";
import { formatError } from "../lib/errors";
import { formatBytes } from "../lib/format";
import { labels } from "../lib/labels";
import { api } from "../services/api";
import type { StatusResponse } from "../types";

function tone(status: string): string {
  const s = status.toLowerCase();
  if (s === "ok" || s === "ready" || s === "success") {
    return "text-emerald-300";
  }
  if (s === "failed" || s === "unavailable") {
    return "text-rose-300";
  }
  return "text-amber-200";
}

/** Operational status of the auditor service (Sprint 10). */
export function StatusPage() {
  const [data, setData] = useState<StatusResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await api.status(controller.signal);
        if (!cancelled) {
          setData(res);
        }
      } catch (e) {
        if (!cancelled) {
          setError(formatError(e, "Falha ao carregar status"));
          setData(null);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [reloadKey]);

  return (
    <>
      <p className="text-xs font-bold tracking-[0.12em] text-emerald-300">
        OPERAÇÃO
      </p>
      <h1 className="mt-2 text-3xl font-semibold text-slate-50 md:text-4xl">
        Status do serviço
      </h1>
      <p className="mt-3 max-w-3xl text-sm leading-relaxed text-slate-400">
        Saúde da API, snapshot store, runs recentes e findings abertos. Dados
        vêm exclusivamente de{" "}
        <code className="text-slate-300">GET /api/v1/status</code>.
      </p>

      {loading && (
        <p className="mt-8 text-sm text-slate-400">Carregando status…</p>
      )}
      {error && (
        <div className="mt-8">
          <ErrorBanner
            title="API indisponível ou parcial"
            message={error}
            onRetry={() => setReloadKey((k) => k + 1)}
          />
        </div>
      )}

      {data && (
        <>
          <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Card
              subtitle="API"
              title={data.api_status}
              className={tone(data.api_status)}
            />
            <Card
              subtitle="Snapshot store"
              title={data.database_status}
              className={tone(data.database_status)}
            />
            <Card
              subtitle="Ambientes"
              title={String(data.environments_count)}
            />
            <Card
              subtitle="Findings abertos"
              title={String(data.open_findings)}
            />
          </div>

          <div className="mt-8 rounded-lg border border-slate-800 bg-slate-900/50 p-4">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <h2 className="text-sm font-semibold text-slate-200">
                Runs recentes
              </h2>
              <span className="text-xs text-slate-500">
                {data.service} · v{data.version} · falhas recentes:{" "}
                {data.failed_runs_recent}
              </span>
            </div>
            {data.recent_runs.length === 0 ? (
              <p className="mt-3 text-sm text-slate-500">
                Nenhum run registrado.
              </p>
            ) : (
              <div className="mt-3">
                <Table
                  dense
                  headers={[
                    "ID",
                    "Ambiente",
                    "Profile",
                    "Status",
                    "Início (UTC)",
                  ]}
                >
                  {data.recent_runs.map((run) => (
                    <tr key={run.id}>
                      <td className="py-2 pr-3 font-mono text-xs">
                        {run.id.slice(0, 8)}
                      </td>
                      <td className="py-2 pr-3 font-mono text-xs">
                        {run.environment_id.slice(0, 8)}
                      </td>
                      <td className="py-2 pr-3">{run.profile}</td>
                      <td className={`py-2 pr-3 ${tone(run.status)}`}>
                        {labels.runStatus(run.status)}
                      </td>
                      <td className="py-2 text-xs text-slate-400">
                        {run.started_at}
                      </td>
                    </tr>
                  ))}
                </Table>
              </div>
            )}
          </div>

          {data.storage && (
            <div className="mt-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
              {(
                [
                  ["Histórico de runs", data.storage.snapshots_bytes],
                  ["Jobs de PDF", data.storage.report_jobs_bytes],
                  ["PDFs armazenados", data.storage.pdf_artifacts_bytes],
                  ["Diagnósticos", data.storage.quality_history_bytes],
                  ["Ações e medições", data.storage.action_history_bytes],
                ] as const
              ).map(([label, bytes]) => (
                <Card key={label} subtitle={label} title={formatBytes(bytes)} />
              ))}
            </div>
          )}

          {data.notes && data.notes.length > 0 && (
            <ul className="mt-4 list-disc space-y-1 pl-5 text-xs text-amber-200/90">
              {data.notes.map((n) => (
                <li key={n}>{n}</li>
              ))}
            </ul>
          )}
        </>
      )}
    </>
  );
}
