import { useEffect, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import {
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Select,
  Skeleton,
} from "../components/ui";
import { useApp } from "../context/AppContext";
import { useEmptyContext } from "../hooks/useEmptyContext";
import { useLatestSignal } from "../hooks/useLatestSignal";
import { resolveEmptyCause } from "../lib/emptyCause";
import { formatError, isAbortError } from "../lib/errors";
import { api } from "../services/api";
import type { AuditAnnotation, AuditRun, RegressionAlert } from "../types";
import { regressionCategoryLabel } from "./parts/regressionCategory";

export function MonitoringPage() {
  const { environmentId, environments, setEnvironmentId, openRun, setSection } =
    useApp();
  const [runs, setRuns] = useState<AuditRun[]>([]);
  const [annotations, setAnnotations] = useState<AuditAnnotation[]>([]);
  const [alerts, setAlerts] = useState<RegressionAlert[]>([]);
  const [baseline, setBaseline] = useState<string>("");
  const [selectedRun, setSelectedRun] = useState("");
  const [comparison, setComparison] = useState<{
    status: string;
    reason: string;
    delta?: number;
  } | null>(null);
  const [kind, setKind] = useState<AuditAnnotation["kind"]>("deployment");
  const [note, setNote] = useState("");
  const [ackReason, setAckReason] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const noRuns = Boolean(environmentId) && !loading && runs.length === 0;
  const emptyContext = useEmptyContext(environmentId, noRuns);
  const monitoringEmptyCause = resolveEmptyCause({
    filtered: false,
    dsnMissing: emptyContext.dsnMissing,
    collected: false,
  });
  const [error, setError] = useState<string | null>(null);

  const nextSignal = useLatestSignal();
  // Latest call wins: switching environment aborts the previous load, so the
  // old environment's runs/alerts never land on the new one (PF-02).
  const refresh = async (env: string) => {
    const signal = nextSignal();
    setLoading(true);
    try {
      const [runResult, annotationResult, alertResult] = await Promise.all([
        api.auditRuns({ environment_id: env }, signal),
        api.annotations(env, signal),
        api.regressions(env, signal),
      ]);
      if (signal.aborted) return;
      const completed = runResult.items.filter(
        (run) => run.status === "success" || run.status === "partial_success",
      );
      setRuns(completed);
      setSelectedRun((current) =>
        completed.some((run) => run.id === current)
          ? current
          : (completed[0]?.id ?? ""),
      );
      setAnnotations(annotationResult.items);
      setAlerts(alertResult.items);
      try {
        const current = await api.auditBaseline(env, "", "", "", signal);
        if (!signal.aborted) setBaseline(current.audit_run_id);
      } catch {
        if (!signal.aborted) setBaseline("");
      }
      if (!signal.aborted) setError(null);
    } catch (cause) {
      if (signal.aborted || isAbortError(cause)) return;
      setError(
        formatError(cause, "Não foi possível carregar o acompanhamento"),
      );
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  };
  useEffect(() => {
    if (environmentId) void refresh(environmentId);
    else {
      nextSignal();
      setLoading(false);
      setRuns([]);
      setAnnotations([]);
      setAlerts([]);
    }
  }, [environmentId]);
  useEffect(() => {
    if (!environmentId || !selectedRun) {
      setComparison(null);
      return;
    }
    let active = true;
    const controller = new AbortController();
    void api
      .compareScopeScore(
        environmentId,
        selectedRun,
        undefined,
        undefined,
        undefined,
        controller.signal,
      )
      .then((value) => {
        if (active) setComparison(value);
      })
      .catch(() => {
        if (active) setComparison(null);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [environmentId, selectedRun, baseline]);

  const selectBaseline = async () => {
    if (!environmentId || !selectedRun) return;
    setBusy(true);
    try {
      await api.selectAuditBaseline(environmentId, selectedRun);
      setBaseline(selectedRun);
      setError(null);
      await refresh(environmentId);
    } catch (cause) {
      setError(
        formatError(
          cause,
          "A execução precisa ter cobertura completa para ser referência",
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  const addAnnotation = async () => {
    if (!environmentId || !note.trim()) return;
    setBusy(true);
    try {
      await api.createAnnotation(environmentId, {
        audit_run_id: selectedRun || undefined,
        kind,
        note: note.trim(),
        occurred_at: new Date().toISOString(),
      });
      setNote("");
      await refresh(environmentId);
    } catch (cause) {
      setError(formatError(cause, "Falha ao registrar anotação"));
    } finally {
      setBusy(false);
    }
  };
  const acknowledge = async (id: string) => {
    if (!environmentId || !ackReason[id]?.trim()) return;
    setBusy(true);
    try {
      await api.acknowledgeRegression(environmentId, id, ackReason[id]);
      await refresh(environmentId);
    } catch (cause) {
      setError(formatError(cause, "Falha ao reconhecer regressão"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <PageHeader
        eyebrow="Histórico"
        title="Acompanhamento e regressões"
        description="Compare apenas execuções com cobertura e análise suficientes. Lacunas não representam melhora nem zero achados."
      />
      <div className="mt-8 space-y-5">
        <Select
          label="Ambiente"
          value={environmentId ?? ""}
          onChange={(e) => setEnvironmentId(e.target.value || null)}
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
        {loading ? <Skeleton className="h-36" /> : null}
        {!environmentId ? (
          <EmptyState
            title="Selecione um ambiente"
            description="Escolha o ambiente para acompanhar execuções e regressões."
          />
        ) : null}
        {environmentId && !loading ? (
          <>
            <Card
              title="Referência aprovada e nota"
              subtitle="Uma mudança da referência fica registrada no histórico do auditor."
            >
              {runs.length === 0 ? (
                <EmptyState
                  className="mt-3"
                  cause={monitoringEmptyCause}
                  subject={{
                    none: "Nenhuma comparação",
                    plural: "comparações",
                  }}
                  onAction={() =>
                    setSection(
                      monitoringEmptyCause === "dsn"
                        ? "Ambientes"
                        : "Execuções",
                    )
                  }
                />
              ) : (
                <div className="mt-3 space-y-3 text-sm text-slate-200">
                  <Select
                    label="Execução atual"
                    value={selectedRun}
                    onChange={(e) => setSelectedRun(e.target.value)}
                    options={runs.map((run) => ({
                      value: run.id,
                      label: `${run.id.slice(0, 8)} · ${run.status} · ${new Date(run.started_at).toLocaleString()}`,
                    }))}
                  />
                  <p>Referência: {baseline || "nenhuma aprovada"}</p>
                  {comparison ? (
                    <p role="status">
                      {comparison.status === "comparable"
                        ? `Variação da nota: ${(comparison.delta ?? 0) >= 0 ? "+" : ""}${comparison.delta} pontos. `
                        : "Comparação indisponível. "}
                      {comparison.reason}
                    </p>
                  ) : null}
                  {api.hasRole("auditor") ? (
                    <Button
                      disabled={busy || !selectedRun}
                      onClick={() => void selectBaseline()}
                    >
                      Aprovar execução como referência
                    </Button>
                  ) : null}
                </div>
              )}
            </Card>
            <Card
              title="Eventos operacionais"
              subtitle="Anote implantação, manutenção ou incidente para interpretar a série temporal."
            >
              {api.hasRole("auditor") ? (
                <div className="mt-3 space-y-2">
                  <Select
                    label="Tipo"
                    value={kind}
                    onChange={(e) =>
                      setKind(e.target.value as AuditAnnotation["kind"])
                    }
                    options={[
                      { value: "deployment", label: "Implantação" },
                      { value: "maintenance", label: "Manutenção" },
                      { value: "incident", label: "Incidente" },
                      { value: "note", label: "Nota" },
                    ]}
                  />
                  <textarea
                    aria-label="Descrição do evento"
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    className="w-full rounded border border-slate-600 bg-slate-900 p-2 text-sm"
                  />
                  <Button
                    disabled={busy || !note.trim()}
                    onClick={() => void addAnnotation()}
                  >
                    Registrar evento
                  </Button>
                </div>
              ) : null}
              {annotations.length ? (
                <ul className="mt-3 space-y-2 text-sm text-slate-300">
                  {annotations.map((item) => (
                    <li key={item.id}>
                      {new Date(item.occurred_at).toLocaleString()} ·{" "}
                      {item.kind} · {item.note} ({item.actor})
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="mt-3 text-sm text-slate-400">
                  Nenhum evento anotado.
                </p>
              )}
            </Card>
            <Card
              title="Alertas de regressão persistente"
              subtitle="Gerados apenas após duas execuções completas e analisadas em sequência com piora frente à referência."
            >
              {alerts.length ? (
                <ul className="mt-3 space-y-4">
                  {alerts.map((item) => (
                    <li
                      key={item.id}
                      className="rounded border border-slate-700 p-3 text-sm text-slate-200"
                    >
                      <p>
                        {regressionCategoryLabel(item.category)} · {item.status}
                      </p>
                      <p className="text-xs text-slate-400">
                        Execução {item.audit_run_id} · referência{" "}
                        {item.baseline_run_id}
                      </p>
                      <p className="text-xs text-slate-400">
                        Evidência: {JSON.stringify(item.evidence)}
                      </p>
                      <Button
                        variant="secondary"
                        onClick={() => openRun(item.audit_run_id)}
                      >
                        Abrir execução
                      </Button>
                      {item.status === "open" && api.hasRole("auditor") ? (
                        <div className="mt-2 flex gap-2">
                          <input
                            aria-label={`Motivo para reconhecer alerta ${item.id}`}
                            value={ackReason[item.id] ?? ""}
                            onChange={(e) =>
                              setAckReason((prev) => ({
                                ...prev,
                                [item.id]: e.target.value,
                              }))
                            }
                            className="rounded border border-slate-600 bg-slate-900 p-2"
                          />
                          <Button
                            disabled={busy || !ackReason[item.id]?.trim()}
                            onClick={() => void acknowledge(item.id)}
                          >
                            Reconhecer
                          </Button>
                        </div>
                      ) : null}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="mt-3 text-sm text-slate-400">
                  Nenhuma regressão persistente confirmada.
                </p>
              )}
            </Card>
          </>
        ) : null}
      </div>
    </>
  );
}
