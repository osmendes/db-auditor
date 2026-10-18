import { useEffect, useState } from "react";
import { PriorityList } from "../../components/PriorityList";
import { formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type { PriorityItem } from "../../types";

/** Top five open findings by the shared priority formula (AN-02). */
export function FindingsPriorities({
  environmentId,
  openFinding,
}: {
  environmentId: string | null;
  openFinding: (id: string) => void;
}) {
  const [items, setItems] = useState<PriorityItem[]>([]);
  const [candidates, setCandidates] = useState(0);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    api
      .findingPriorities(
        { environment_id: environmentId ?? undefined, limit: 5 },
        controller.signal,
      )
      .then((res) => {
        setItems(res.items);
        setCandidates(res.candidates);
        setError(null);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) {
          setError(formatError(e, "Prioridades indisponíveis."));
        }
      });
    return () => controller.abort();
  }, [environmentId]);

  if (!items.length && !error) {
    return null;
  }
  return (
    <details
      className="mb-4 rounded-xl border border-slate-800 bg-slate-950/40 p-4"
      open
    >
      <summary className="cursor-pointer text-sm font-medium text-slate-200">
        Cinco prioridades
        {candidates ? ` entre ${candidates} achados abertos` : ""}
      </summary>
      {error ? (
        <p role="alert" className="mt-2 text-xs text-amber-300">
          {error}
        </p>
      ) : (
        <PriorityList items={items} openFinding={openFinding} />
      )}
    </details>
  );
}
