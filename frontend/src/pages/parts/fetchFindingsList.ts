import type { PageSize } from "../../components/ui/PaginationControls";
import { fetchAllPages } from "../../lib/pagination";
import { api } from "../../services/api";
import type { Finding } from "../../types";

export type FindingsEmptyKind = "never" | "zero" | "filter" | "dsn" | null;
export type FindingsCoverageKind = "partial" | "gap" | "permission" | null;

export async function fetchFindingsList(input: {
  presetCategory?: "performance" | "security";
  severityFilter: string;
  statusFilter: string;
  environmentId: string | null;
  mine: boolean;
  overdueOnly: boolean;
  pageSize: PageSize;
  offset: number;
}): Promise<{
  items: Finding[];
  total: number;
  emptyKind: FindingsEmptyKind;
  coverageKind?: FindingsCoverageKind;
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
        api.findingsCategoryPage(presetCategory, {
          ...categoryFilters,
          offset: pageOffset,
          limit,
        }),
      );
      items = rows;
      total = rows.length;
      count = rows.length;
    } else {
      const res = await api.findingsCategoryPage(presetCategory, {
        ...categoryFilters,
        offset,
        limit: pageSize,
      });
      items = res.items;
      total = res.page.total;
      count = res.page.total;
    }
  } else if (pageSize === "all") {
    const rows = await fetchAllPages((pageOffset, limit) =>
      api.findingsPage({ ...filters, offset: pageOffset, limit }),
    );
    items = rows;
    total = rows.length;
    count = rows.length;
  } else {
    const res = await api.findingsPage({
      ...filters,
      offset,
      limit: pageSize,
    });
    items = res.items;
    total = res.page.total;
    count = res.page.total;
  }
  let emptyKind: FindingsEmptyKind = null;
  let coverageKind: FindingsCoverageKind | undefined;
  if (count === 0) {
    const unfiltered = await api.findingsPage({
      environment_id: environmentId || undefined,
      offset: 0,
      limit: 1,
    });
    if (unfiltered.page.total > 0) {
      emptyKind = "filter";
    } else {
      const [runs, connections] = await Promise.all([
        api.auditRuns({ environment_id: environmentId || undefined }),
        api.connectionStatus(),
      ]);
      const scoped = environmentId
        ? connections.items.filter(
            (item) => item.environment_id === environmentId,
          )
        : connections.items;
      const missingDsn =
        scoped.length > 0 && scoped.every((item) => !item.dsn_configured);
      const collected = runs.items.some(
        (run) => run.status === "success" || run.status === "partial_success",
      );
      emptyKind = missingDsn ? "dsn" : collected ? "zero" : "never";
      coverageKind = runs.items.some((run) => run.status === "partial_success")
        ? "partial"
        : null;
    }
  } else {
    emptyKind = null;
  }
  return { items, total, emptyKind, coverageKind };
}
