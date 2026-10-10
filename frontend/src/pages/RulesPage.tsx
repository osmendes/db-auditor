import { useEffect, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Input,
  Skeleton,
} from "../components/ui";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { api } from "../services/api";
import type { EffectiveRule } from "../types";
import { FindingsPage } from "./FindingsPage";

export function RulesPage() {
  const { environmentId, selectedEnvironment } = useApp();
  const [schema, setSchema] = useState("public");
  const [items, setItems] = useState<EffectiveRule[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState("");

  useEffect(() => {
    if (!environmentId) {
      setItems([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    api
      .rules(environmentId, schema)
      .then((res) => {
        if (!cancelled) setItems(res.items);
      })
      .catch((e) => {
        if (!cancelled)
          setError(formatError(e, "Não foi possível carregar as regras."));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [environmentId, schema]);

  const toggle = async (rule: EffectiveRule) => {
    if (!environmentId || !api.hasRole("operator")) return;
    setBusy(rule.rule_id);
    setError(null);
    try {
      await api.putRule(environmentId, rule.rule_id, {
        schema,
        enabled: !rule.enabled,
        parameters: {},
      });
      const res = await api.rules(environmentId, schema);
      setItems(res.items);
    } catch (e) {
      setError(formatError(e, "A política não foi gravada."));
    } finally {
      setBusy("");
    }
  };

  return (
    <>
      <PageHeader
        eyebrow="Política"
        title="Regras efetivas"
        description="Desligar uma regra vale para o schema informado e não reescreve findings já gravados. Parâmetro desconhecido é recusado pela API."
      />
      {!environmentId ? (
        <div className="mt-6">
          <EmptyState
            title="Escolha um ambiente"
            description="A política é por ambiente e, quando informado, por schema."
          />
        </div>
      ) : (
        <div className="mt-6 space-y-4">
          <Card>
            <Input
              label="Schema"
              value={schema}
              onChange={(e) => setSchema(e.target.value)}
            />
          </Card>
          {error ? <ErrorBanner message={error} /> : null}
          {loading ? <Skeleton className="h-40 w-full" /> : null}
          {!loading && items.length === 0 ? (
            <EmptyState
              title={
                selectedEnvironment?.engine === "mongodb"
                  ? "Regras não aplicáveis"
                  : "Nenhuma regra"
              }
              description={
                selectedEnvironment?.engine === "mongodb"
                  ? "As regras atuais analisam PostgreSQL e TimescaleDB. O inventário MongoDB não recebe diagnósticos desses mecanismos."
                  : "O catálogo ainda não respondeu."
              }
            />
          ) : null}
          <div className="grid gap-2">
            {items
              .filter(
                (rule) =>
                  rule.rule_id === "model.wide_table" ||
                  rule.category === "model" ||
                  rule.category === "security",
              )
              .slice(0, 40)
              .map((rule) => (
                <div
                  key={rule.rule_id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-slate-800 px-3 py-2"
                >
                  <div className="min-w-0">
                    <p className="truncate font-mono text-sm text-slate-100">
                      {rule.rule_id}
                    </p>
                    <p className="text-xs text-slate-400">
                      {rule.rule_version} · {rule.category}
                      {rule.effective_parameters?.min_columns != null
                        ? ` · min_columns ${String(rule.effective_parameters.min_columns)}`
                        : ""}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge tone={rule.enabled ? "success" : "neutral"}>
                      {rule.enabled ? "ativa" : "desligada"}
                    </Badge>
                    {api.hasRole("operator") ? (
                      <Button
                        type="button"
                        variant="ghost"
                        disabled={busy === rule.rule_id}
                        onClick={() => void toggle(rule)}
                      >
                        {rule.enabled
                          ? "Desligar neste schema"
                          : "Ligar neste schema"}
                      </Button>
                    ) : null}
                  </div>
                </div>
              ))}
          </div>
        </div>
      )}
      <FindingsPage />
    </>
  );
}
