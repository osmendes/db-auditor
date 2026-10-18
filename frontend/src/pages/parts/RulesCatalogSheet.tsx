import { useEffect, useState } from "react";
import {
  Badge,
  Button,
  EmptyState,
  ErrorBanner,
  Input,
  Sheet,
  Skeleton,
} from "../../components/ui";
import { useApp } from "../../context/AppContext";
import { formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type { EffectiveRule } from "../../types";

export function RulesCatalogSheet({
  open,
  onOpenChange,
  focusRule = "",
  onClearFocus,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** UI-07: rule chosen in the global search; shows only that rule. */
  focusRule?: string;
  onClearFocus?: () => void;
}) {
  const { environmentId, selectedEnvironment } = useApp();
  const [schema, setSchema] = useState("public");
  const [items, setItems] = useState<EffectiveRule[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState("");

  useEffect(() => {
    if (!open || !environmentId) {
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
      .catch((cause: unknown) => {
        if (!cancelled) {
          setError(formatError(cause, "Não foi possível carregar as regras."));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, environmentId, schema]);

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
    } catch (cause: unknown) {
      setError(formatError(cause, "A política não foi gravada."));
    } finally {
      setBusy("");
    }
  };

  const visible = items.filter(
    (rule) =>
      (!focusRule || rule.rule_id === focusRule) &&
      (rule.rule_id === "model.wide_table" ||
        rule.category === "model" ||
        rule.category === "security" ||
        rule.category === "performance"),
  );

  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title="Regras do ambiente"
      description="Aqui você vê quais verificações estão ligadas. Desligar uma regra vale só para o esquema informado e não apaga análises já gravadas."
      wide
    >
      {!environmentId ? (
        <EmptyState
          title="Escolha um ambiente"
          description="As regras são por ambiente e, quando informado, por esquema."
        />
      ) : (
        <div className="space-y-4">
          <Input
            label="Esquema"
            value={schema}
            onChange={(event) => setSchema(event.target.value)}
          />
          <p className="text-sm text-slate-400">
            Uma regra desligada deixa de gerar novos sinais nesse esquema. O
            banco analisado não é alterado.
          </p>
          {focusRule ? (
            <div className="flex flex-wrap items-center gap-2 text-sm text-slate-300">
              <span>
                Mostrando a regra <code>{focusRule}</code>, vinda da busca.
              </span>
              {onClearFocus ? (
                <Button variant="secondary" onClick={onClearFocus}>
                  Ver todas as regras
                </Button>
              ) : null}
            </div>
          ) : null}
          {error ? <ErrorBanner message={error} /> : null}
          {loading ? <Skeleton className="h-40 w-full" /> : null}
          {!loading && visible.length === 0 ? (
            <EmptyState
              title={
                selectedEnvironment?.engine === "mongodb"
                  ? "Regras não aplicáveis"
                  : "Nenhuma regra"
              }
              description={
                selectedEnvironment?.engine === "mongodb"
                  ? "As regras atuais analisam PostgreSQL e TimescaleDB. O inventário MongoDB não recebe esses diagnósticos."
                  : "O catálogo ainda não respondeu."
              }
            />
          ) : null}
          <ul className="grid gap-2">
            {visible.slice(0, 40).map((rule) => (
              <li
                key={rule.rule_id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-slate-800 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm text-slate-100">
                    {rule.rule_id}
                  </p>
                  <p className="text-xs text-slate-400">
                    {rule.rule_version} · {rule.category}
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
                      {rule.enabled ? "Desligar" : "Ligar"}
                    </Button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Sheet>
  );
}
