import { useEffect, useState } from "react";
import { Button, ErrorBanner, Select } from "../../components/ui";
import { formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type { AuditRun, FindingChange } from "../../types";

const CHANGE_OPTIONS = [
  { value: "", label: "Todas as mudanças" },
  { value: "added", label: "Novos (added)" },
  { value: "removed", label: "Removidos (removed)" },
  { value: "unchanged", label: "Inalterados (unchanged)" },
];

function runLabel(run: AuditRun): string {
  const when = run.started_at
    ? run.started_at.slice(0, 16).replace("T", " ")
    : "sem data";
  return `${when} · ${run.status} · ${run.id.slice(0, 8)}`;
}

export function FindingsDiffPanel({
  environmentId,
}: {
  environmentId: string | null;
}) {
  const [runs, setRuns] = useState<AuditRun[]>([]);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [change, setChange] = useState("");
  const [items, setItems] = useState<FindingChange[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!environmentId) {
      setRuns([]);
      setFrom("");
      setTo("");
      setItems(null);
      return;
    }
    let active = true;
    void api
      .auditRuns({ environment_id: environmentId })
      .then((result) => {
        if (!active) return;
        const next = result.items.slice(0, 40);
        setRuns(next);
        setFrom((current) => current || next[1]?.id || next[0]?.id || "");
        setTo((current) => current || next[0]?.id || "");
      })
      .catch(() => {
        if (active) setRuns([]);
      });
    return () => {
      active = false;
    };
  }, [environmentId]);

  const compare = () => {
    if (!environmentId || !from || !to) return;
    setBusy(true);
    setError(null);
    void api
      .findingsDiff(environmentId, from, to, change)
      .then((result) => setItems(result.items))
      .catch((cause: unknown) => {
        setItems(null);
        setError(formatError(cause, "Não foi possível comparar as execuções."));
      })
      .finally(() => setBusy(false));
  };

  const options = runs.map((run) => ({ value: run.id, label: runLabel(run) }));

  return (
    <section className="rounded-xl border border-slate-700/80 bg-slate-900 p-5">
      <h2 className="text-sm font-medium text-slate-100">
        O que mudou entre duas execuções
      </h2>
      <p className="mt-1 text-sm text-slate-400">
        Compara achados já gravados no snapshot store. Nada é escrito no banco
        auditado.
      </p>
      {!environmentId ? (
        <p className="mt-3 text-sm text-slate-400">
          Selecione um ambiente para comparar execuções.
        </p>
      ) : (
        <>
          <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Select
              label="Execução anterior"
              value={from}
              onChange={(event) => {
                setFrom(event.target.value);
                setItems(null);
              }}
              options={
                options.length
                  ? options
                  : [{ value: "", label: "Sem execuções" }]
              }
            />
            <Select
              label="Execução recente"
              value={to}
              onChange={(event) => {
                setTo(event.target.value);
                setItems(null);
              }}
              options={
                options.length
                  ? options
                  : [{ value: "", label: "Sem execuções" }]
              }
            />
            <Select
              label="Filtro"
              value={change}
              onChange={(event) => {
                setChange(event.target.value);
                setItems(null);
              }}
              options={CHANGE_OPTIONS}
            />
            <div className="flex items-end">
              <Button onClick={compare} disabled={busy || !from || !to}>
                {busy ? "Comparando…" : "Comparar"}
              </Button>
            </div>
          </div>
          {error ? (
            <div className="mt-3">
              <ErrorBanner message={error} onRetry={compare} />
            </div>
          ) : null}
          {items ? (
            <div className="mt-4 overflow-x-auto">
              <table className="w-full text-left text-sm text-slate-200">
                <thead className="text-xs text-slate-400">
                  <tr>
                    <th className="py-2 pr-3 font-medium">Mudança</th>
                    <th className="py-2 pr-3 font-medium">Severidade</th>
                    <th className="py-2 pr-3 font-medium">Objeto</th>
                    <th className="py-2 font-medium">Achado</th>
                  </tr>
                </thead>
                <tbody>
                  {items.length === 0 ? (
                    <tr>
                      <td className="py-3 text-slate-400" colSpan={4}>
                        Nenhum achado neste recorte.
                      </td>
                    </tr>
                  ) : (
                    items.map((item) => (
                      <tr
                        key={`${item.change}:${item.object_key}:${item.title}`}
                        className="border-t border-slate-800"
                      >
                        <td className="py-2 pr-3">{item.change}</td>
                        <td className="py-2 pr-3">{item.severity}</td>
                        <td className="py-2 pr-3">{item.object_key}</td>
                        <td className="py-2">{item.title}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}
