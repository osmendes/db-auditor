import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Input,
  Skeleton,
  Table,
} from "../components/ui";
import { useApp } from "../context/AppContext";
import { useLatestSignal } from "../hooks/useLatestSignal";
import { formatError } from "../lib/errors";
import { labels } from "../lib/labels";
import { api } from "../services/api";
import type { Environment, MappingCandidate, ObjectMapping } from "../types";

function statusTone(
  status: string,
): "success" | "warning" | "danger" | "neutral" {
  if (status === "validated") {
    return "success";
  }
  if (status === "suggested" || status === "manual") {
    return "warning";
  }
  if (status === "rejected") {
    return "danger";
  }
  return "neutral";
}

export function MappingsPage() {
  const { environments, environmentId } = useApp();
  const [items, setItems] = useState<ObjectMapping[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);
  const [sourceEnv, setSourceEnv] = useState("");
  const [targetEnv, setTargetEnv] = useState("");
  const [suggesting, setSuggesting] = useState(false);
  const [candidates, setCandidates] = useState<MappingCandidate[] | null>(null);
  const [suggestMsg, setSuggestMsg] = useState<string | null>(null);

  useEffect(() => {
    if (environments.length >= 2) {
      setSourceEnv((prev) => prev || environments[0].id);
      setTargetEnv((prev) => prev || environments[1].id);
    } else if (environments.length === 1) {
      setSourceEnv(environments[0].id);
    }
  }, [environments]);

  const nextSignal = useLatestSignal();
  // Latest filter/environment wins; the previous list request is aborted (PF-02).
  const load = useCallback(() => {
    const signal = nextSignal();
    setItems(null);
    setError(null);
    api
      .mappings(
        {
          status: statusFilter || undefined,
          source_environment_id: environmentId || undefined,
        },
        signal,
      )
      .then((res) => {
        if (!signal.aborted) setItems(res.items);
      })
      .catch((err: unknown) => {
        if (signal.aborted) return;
        setError(formatError(err, "Falha ao listar mapeamentos"));
        setItems([]);
      });
  }, [statusFilter, environmentId, nextSignal]);

  useEffect(() => {
    load();
  }, [load]);

  const setStatus = async (id: string, status: string) => {
    setBusyId(id);
    try {
      await api.updateMappingStatus(id, status);
      load();
    } catch (err: unknown) {
      setError(formatError(err, "Falha ao atualizar mapeamento"));
    } finally {
      setBusyId(null);
    }
  };

  const envName = (id: string, list: Environment[]) =>
    list.find((e) => e.id === id)?.name ?? id.slice(0, 8);

  const runSuggest = async () => {
    if (!sourceEnv || !targetEnv || sourceEnv === targetEnv) {
      setSuggestMsg("Selecione dois ambientes diferentes.");
      return;
    }
    setSuggesting(true);
    setSuggestMsg(null);
    setCandidates(null);
    try {
      const [srcTables, tgtTables] = await Promise.all([
        api.tables(sourceEnv, { limit: 200, offset: 0 }),
        api.tables(targetEnv, { limit: 200, offset: 0 }),
      ]);
      const toRef = (t: {
        database_name: string;
        schema_name: string;
        table_name: string;
      }) => ({
        database: t.database_name,
        schema: t.schema_name,
        object_type: "table",
        object_name: t.table_name,
      });
      const res = await api.suggestMappings({
        source: srcTables.items.map(toRef),
        target: tgtTables.items.map(toRef),
      });
      setCandidates(res.items);
      setSuggestMsg(
        res.items.length === 0
          ? "Nenhuma sugestão com confidence suficiente. Confira se ambos os ambientes têm inventário."
          : `${res.items.length} sugestão(ões) encontrada(s). Aceite as desejadas abaixo.${srcTables.page.has_more || tgtTables.page.has_more ? " Esta sugestão analisou somente as primeiras 200 tabelas de cada ambiente; refine o inventário antes de avaliar ambientes grandes." : ""}`,
      );
    } catch (err: unknown) {
      setSuggestMsg(formatError(err, "Falha ao sugerir mapeamentos"));
      setCandidates([]);
    } finally {
      setSuggesting(false);
    }
  };

  const acceptCandidate = async (c: MappingCandidate) => {
    const key = `${c.source_schema}.${c.source_object_name}`;
    setBusyId(key);
    try {
      await api.createMapping({
        source_environment_id: sourceEnv,
        target_environment_id: targetEnv,
        source_database: c.source_database,
        source_schema: c.source_schema,
        source_object_type: c.source_object_type,
        source_object_name: c.source_object_name,
        target_database: c.target_database,
        target_schema: c.target_schema,
        target_object_type: c.target_object_type,
        target_object_name: c.target_object_name,
        relation_type: c.relation_type || "equivalent",
        confidence: c.confidence,
        status: "suggested",
        source_fingerprint: c.source_fingerprint,
        target_fingerprint: c.target_fingerprint,
        fingerprint_algorithm: "v1",
      });
      setCandidates((prev) =>
        (prev ?? []).filter(
          (x) =>
            !(
              x.source_object_name === c.source_object_name &&
              x.target_object_name === c.target_object_name
            ),
        ),
      );
      load();
    } catch (err: unknown) {
      setError(formatError(err, "Falha ao criar mapeamento"));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <>
      <PageHeader
        eyebrow="MAPEAMENTOS"
        title="Mapeamentos"
        description="Correspondências entre ambientes, confidence e validação. Sugira a partir do inventário coletado."
      />

      <div className="mt-8 space-y-6">
        <Card title="Sugerir a partir do inventário">
          <div className="mt-2 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <label className="grid gap-1 text-xs text-slate-400">
              Origem
              <select
                className="rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-100"
                value={sourceEnv}
                onChange={(e) => setSourceEnv(e.target.value)}
              >
                {environments.map((e) => (
                  <option key={e.id} value={e.id}>
                    {e.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="grid gap-1 text-xs text-slate-400">
              Destino
              <select
                className="rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-100"
                value={targetEnv}
                onChange={(e) => setTargetEnv(e.target.value)}
              >
                {environments.map((e) => (
                  <option key={e.id} value={e.id}>
                    {e.name}
                  </option>
                ))}
              </select>
            </label>
            <div className="flex items-end">
              <Button onClick={() => void runSuggest()} disabled={suggesting}>
                {suggesting ? "Sugerindo…" : "Sugerir mapeamentos"}
              </Button>
            </div>
          </div>
          {suggestMsg ? (
            <p className="mt-3 text-sm text-slate-300">{suggestMsg}</p>
          ) : null}
          {candidates && candidates.length > 0 ? (
            <div className="mt-4 overflow-x-auto">
              <Table headers={["Origem", "Destino", "Conf.", "Ação"]}>
                {candidates.map((c) => {
                  const key = `${c.source_schema}.${c.source_object_name}`;
                  return (
                    <tr
                      key={`${key}-${c.target_object_name}`}
                      className="border-t border-slate-800"
                    >
                      <td className="px-4 py-3 font-mono text-xs text-slate-100">
                        {c.source_schema}.{c.source_object_name}
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-100">
                        {c.target_schema}.{c.target_object_name}
                      </td>
                      <td className="px-4 py-3 text-slate-300">
                        {(c.confidence * 100).toFixed(0)}%
                      </td>
                      <td className="px-4 py-3">
                        <Button
                          variant="secondary"
                          disabled={busyId === key}
                          onClick={() => void acceptCandidate(c)}
                        >
                          Aceitar
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </Table>
            </div>
          ) : null}
        </Card>

        <Input
          label="Filtrar status"
          placeholder="suggested, validated, rejected, manual"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
        />

        {error ? <ErrorBanner message={error} onRetry={() => load()} /> : null}

        {items === null ? (
          <Skeleton className="h-40 w-full" />
        ) : items.length === 0 ? (
          <EmptyState
            title="Nenhum mapeamento"
            description="Use Sugerir mapeamentos acima (com inventário nos dois ambientes) ou crie via API."
            action={
              <Button onClick={() => void runSuggest()} disabled={suggesting}>
                Sugerir agora
              </Button>
            }
          />
        ) : (
          <Table
            headers={[
              "Origem",
              "Destino",
              "Relação",
              "Conf.",
              "Status",
              "Ações",
            ]}
          >
            {items.map((m) => (
              <tr key={m.id} className="border-t border-slate-800">
                <td className="px-4 py-3 font-mono text-xs text-slate-100">
                  <span className="block text-[10px] text-slate-500">
                    {envName(m.source_environment_id, environments)}
                  </span>
                  {m.source_database}.{m.source_schema}/{m.source_object_name}
                </td>
                <td className="px-4 py-3 font-mono text-xs text-slate-100">
                  <span className="block text-[10px] text-slate-500">
                    {envName(m.target_environment_id, environments)}
                  </span>
                  {m.target_database}.{m.target_schema}/{m.target_object_name}
                </td>
                <td className="px-4 py-3 text-slate-300">{m.relation_type}</td>
                <td className="px-4 py-3 text-slate-300">
                  {(m.confidence * 100).toFixed(0)}%
                </td>
                <td className="px-4 py-3">
                  <Badge tone={statusTone(m.status)}>
                    {labels.mappingStatus(m.status)}
                  </Badge>
                </td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="secondary"
                      disabled={busyId === m.id}
                      onClick={() => void setStatus(m.id, "validated")}
                    >
                      Validar
                    </Button>
                    <Button
                      variant="ghost"
                      disabled={busyId === m.id}
                      onClick={() => void setStatus(m.id, "rejected")}
                    >
                      Rejeitar
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </Table>
        )}
      </div>
    </>
  );
}
