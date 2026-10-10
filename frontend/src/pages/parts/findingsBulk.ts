import type { Dispatch, SetStateAction } from "react";
import { api } from "../../services/api";
import type { Finding } from "../../types";

export async function bulkTriageFindings(
  status: string,
  selectedIds: Set<string>,
  setItems: Dispatch<SetStateAction<Finding[]>>,
  setSelected: Dispatch<SetStateAction<Finding | null>>,
  setSelectedIds: Dispatch<SetStateAction<Set<string>>>,
  setBulkBusy: Dispatch<SetStateAction<boolean>>,
  setError: Dispatch<SetStateAction<string | null>>,
) {
  const ids = Array.from(selectedIds);
  if (ids.length === 0) {
    return;
  }
  setBulkBusy(true);
  setError(null);
  let failed = 0;
  const updatedMap = new Map<string, Finding>();
  await Promise.all(
    ids.map(async (id) => {
      try {
        const updated = await api.updateFindingStatus(id, status);
        updatedMap.set(id, updated);
      } catch {
        failed += 1;
      }
    }),
  );
  setItems((prev) =>
    prev.map((f) => {
      const next = updatedMap.get(f.id);
      return next ?? f;
    }),
  );
  setSelected((cur) => {
    if (!cur) {
      return cur;
    }
    return updatedMap.get(cur.id) ?? cur;
  });
  setSelectedIds(new Set());
  setBulkBusy(false);
  if (failed > 0) {
    const ok = ids.length - failed;
    setError(`Triagem em lote parcial: ${ok} ok, ${failed} falha(s).`);
  }
}
