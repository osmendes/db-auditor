import type { PageSize } from "../../components/ui/PaginationControls";
import { fetchEmptyContext } from "../../hooks/useEmptyContext";
import { type EmptyCause, resolveEmptyCause } from "../../lib/emptyCause";
import { fetchAllPages } from "../../lib/pagination";
import { api } from "../../services/api";
import type { Finding } from "../../types";

export type FindingsEmptyKind = EmptyCause | null;

export async function fetchFindingsList(input: {
  presetCategory?: "performance" | "security";
  severityFilter: string;
  statusFilter: string;
  environmentId: string | null;
  mine: boolean;
  overdueOnly: boolean;
  pageSize: PageSize;
  offset: number;
  /** Aborts every request of this load when the caller moves on (PF-02). */
  signal?: AbortSignal;
}): Promise<{
  items: Finding[];
  total: number;
  emptyKind: FindingsEmptyKind;
}> {
  const {
    presetCategory,
    severityFilter,
    statusFilter,
    environmentId,
    mine,
    overdueOnly,
    pageSize,
    offset,
    signal,
  } = input;
  const filters = {
    severity: severityFilter || undefined,
    status: statusFilter || undefined,
    environment_id: environmentId || undefined,
    assignee: mine ? "me" : undefined,
    overdue: overdueOnly ? "1" : undefined,
  };
  let items: Finding[] = [];
  let total = 0;
  let count = 0;
  if (presetCategory) {
    const categoryFilters = {
      environment_id: environmentId || undefined,
      status: statusFilter || undefined,
    };
    if (pageSize === "all") {
      const rows = await fetchAllPages((pageOffset, limit) =>
        api.findingsCategoryPage(
          presetCategory,
          {
            ...categoryFilters,
            offset: pageOffset,
            limit,
          },
          signal,
        ),
      );
      items = rows;
      total = rows.length;
      count = rows.length;
    } else {
      const res = await api.findingsCategoryPage(
        presetCategory,
        {
          ...categoryFilters,
          offset,
          limit: pageSize,
        },
        signal,
      );
      items = res.items;
      total = res.page.total;
      count = res.page.total;
    }
  } else if (pageSize === "all") {
    const rows = await fetchAllPages((pageOffset, limit) =>
      api.findingsPage({ ...filters, offset: pageOffset, limit }, signal),
    );
    items = rows;
    total = rows.length;
    count = rows.length;
  } else {
    const res = await api.findingsPage(
      {
        ...filters,
        offset,
        limit: pageSize,
      },
      signal,
    );
    items = res.items;
    total = res.page.total;
    count = res.page.total;
  }
  let emptyKind: FindingsEmptyKind = null;
  if (count === 0) {
    const unfiltered = await api.findingsPage(
      {
        environment_id: environmentId || undefined,
        offset: 0,
        limit: 1,
      },
      signal,
    );
    const context =
      unfiltered.page.total > 0
        ? { dsnMissing: false, collected: true }
        : await fetchEmptyContext(environmentId, signal);
    emptyKind = resolveEmptyCause({
      filtered: unfiltered.page.total > 0,
      unfilteredTotal: unfiltered.page.total,
      ...context,
    });
  } else {
    emptyKind = null;
  }
  return { items, total, emptyKind };
}
