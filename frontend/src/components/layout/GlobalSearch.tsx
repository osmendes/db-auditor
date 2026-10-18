import { useEffect, useId, useMemo, useRef, useState } from "react";
import { formatError } from "../../lib/errors";
import {
  actionForHit,
  autoOpenHit,
  groupHits,
  isUUID,
  SEARCH_DEBOUNCE_MS,
  SEARCH_MAX_CHARS,
  type SearchAction,
  searchableQuery,
} from "../../lib/globalSearch";
import { api } from "../../services/api";
import type { SearchHit, SearchResult } from "../../types/openapi";

type SearchFn = (
  params: { q: string; environment_id?: string; limit?: number },
  signal?: AbortSignal,
) => Promise<SearchResult>;

export interface GlobalSearchProps {
  environmentId: string | null;
  onNavigate: (action: SearchAction) => void;
  search?: SearchFn;
  debounceMs?: number;
}

/**
 * UI-07 global search. Queries GET /api/v1/search, which reads only the
 * snapshot store (never the audited database). A pasted UUID opens the
 * matching finding (or run) directly.
 */
export function GlobalSearch({
  environmentId,
  onNavigate,
  search = api.search,
  debounceMs = SEARCH_DEBOUNCE_MS,
}: GlobalSearchProps) {
  const id = useId();
  const listId = `${id}-results`;
  const inputRef = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState("");
  const [result, setResult] = useState<SearchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);

  const query = searchableQuery(value);
  const groups = useMemo(() => groupHits(result?.items ?? []), [result]);
  const flat = useMemo(() => groups.flatMap((group) => group.hits), [groups]);

  const reset = () => {
    setValue("");
    setResult(null);
    setOpen(false);
    setActive(-1);
    setError(null);
  };

  const go = (action: SearchAction | null) => {
    if (!action) return;
    onNavigate(action);
    reset();
    inputRef.current?.blur();
  };

  useEffect(() => {
    if (!query) {
      setResult(null);
      setLoading(false);
      setError(null);
      return;
    }
    const controller = new AbortController();
    const uuid = isUUID(query);
    setLoading(true);
    setError(null);
    const timer = setTimeout(
      () => {
        search(
          {
            q: query,
            environment_id: environmentId ?? undefined,
          },
          controller.signal,
        )
          .then((res) => {
            if (controller.signal.aborted) return;
            setResult(res);
            setActive(res.items.length > 0 ? 0 : -1);
            setLoading(false);
            const exact = uuid ? autoOpenHit(res.items) : null;
            if (exact) go(actionForHit(exact));
          })
          .catch((cause: unknown) => {
            if (controller.signal.aborted) return;
            setResult(null);
            setLoading(false);
            setError(formatError(cause, "A busca falhou."));
          });
      },
      uuid ? 0 : debounceMs,
    );
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [query, environmentId, debounceMs, search]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        inputRef.current?.focus();
        setOpen(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown" && flat.length > 0) {
      event.preventDefault();
      setOpen(true);
      setActive((index) => (index + 1) % flat.length);
    } else if (event.key === "ArrowUp" && flat.length > 0) {
      event.preventDefault();
      setOpen(true);
      setActive((index) => (index <= 0 ? flat.length - 1 : index - 1));
    } else if (event.key === "Enter") {
      event.preventDefault();
      const hit = active >= 0 ? flat[active] : undefined;
      if (hit) {
        go(actionForHit(hit));
      } else if (query && isUUID(query)) {
        // UUID abre finding, even before the lookup answers.
        go({
          type: "finding",
          id: query.toLowerCase(),
          environmentId: undefined,
        });
      }
    } else if (event.key === "Escape") {
      if (value) reset();
      else inputRef.current?.blur();
    }
  };

  const showPanel = open && Boolean(query);
  const activeId =
    showPanel && active >= 0 && flat[active]
      ? `${id}-opt-${active}`
      : undefined;
  let index = -1;

  return (
    <div className="relative mt-3">
      <label htmlFor={`${id}-input`} className="sr-only">
        Busca global
      </label>
      <input
        ref={inputRef}
        id={`${id}-input`}
        type="search"
        role="combobox"
        aria-expanded={showPanel}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={activeId}
        aria-describedby={`${id}-hint`}
        autoComplete="off"
        spellCheck={false}
        maxLength={SEARCH_MAX_CHARS}
        placeholder="Buscar objeto, regra, execução, finding"
        value={value}
        onChange={(event) => {
          setValue(event.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        onKeyDown={onKeyDown}
        className="w-full rounded-md border border-slate-500 bg-slate-950 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-500 focus:border-[var(--focus-ring)] focus:outline-none focus:ring-1 focus:ring-[var(--focus-ring)]"
      />
      <span id={`${id}-hint`} className="sr-only">
        Digite ao menos 2 caracteres ou cole o UUID de um finding. Ctrl+K foca a
        busca.
      </span>
      <div
        className={
          showPanel
            ? "absolute left-0 right-0 z-30 mt-1 max-h-96 overflow-y-auto rounded-md border border-slate-700 bg-slate-950 p-1 shadow-xl"
            : "hidden"
        }
      >
        <p className="sr-only" aria-live="polite">
          {loading
            ? "Buscando…"
            : result
              ? `${flat.length} resultado${flat.length === 1 ? "" : "s"}`
              : ""}
        </p>
        {loading && !result ? (
          <p className="px-2 py-1.5 text-xs text-slate-400">Buscando…</p>
        ) : null}
        {error ? (
          <p role="alert" className="px-2 py-1.5 text-xs text-red-300">
            {error}
          </p>
        ) : null}
        {result && flat.length === 0 && !loading ? (
          <p className="px-2 py-1.5 text-xs text-slate-400">
            {isUUID(result.query)
              ? "Nenhum finding ou execução com esse identificador."
              : "Nada encontrado no inventário, findings, execuções ou regras."}
          </p>
        ) : null}
        <div id={listId} role="listbox" aria-label="Resultados da busca">
          {groups.map((group) => (
            <div key={group.kind}>
              <p
                aria-hidden="true"
                className="px-2 pt-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500"
              >
                {group.label}
                {result?.truncated?.[groupKey(group.kind)]
                  ? " (mais resultados — refine a busca)"
                  : ""}
              </p>
              {group.hits.map((hit) => {
                index += 1;
                const position = index;
                return (
                  <div
                    key={`${hit.kind}:${hit.id}`}
                    id={`${id}-opt-${position}`}
                    role="option"
                    aria-selected={position === active}
                    tabIndex={-1}
                    onMouseDown={(event) => event.preventDefault()}
                    onMouseEnter={() => setActive(position)}
                    onClick={() => go(actionForHit(hit))}
                    onKeyDown={() => undefined}
                    className={`cursor-pointer rounded px-2 py-1.5 text-sm ${
                      position === active
                        ? "bg-slate-800 text-slate-50"
                        : "text-slate-200"
                    }`}
                  >
                    <HitLine hit={hit} group={group.label} />
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function groupKey(kind: string): string {
  return kind === "inventory"
    ? "inventory"
    : kind === "rule"
      ? "rules"
      : `${kind}s`;
}

function HitLine({ hit, group }: { hit: SearchHit; group: string }) {
  return (
    <span className="block min-w-0">
      <span className="sr-only">{group}: </span>
      <span className="block truncate">{hit.title}</span>
      <span className="block truncate text-xs text-slate-400">
        {hit.severity ? `${hit.severity} · ` : ""}
        {hit.status && hit.kind === "finding" ? `${hit.status} · ` : ""}
        {hit.subtitle}
      </span>
    </span>
  );
}
