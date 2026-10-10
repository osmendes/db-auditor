import { useCallback, useEffect, useMemo, useState } from "react";
import type { PageSize } from "../components/ui/PaginationControls";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { labels } from "../lib/labels";
import { type SortState, sortBy } from "../lib/sort";
import { api } from "../services/api";
import type {
  ActionEvent,
  Finding,
  FindingAction,
  FindingEvent,
} from "../types";
import { FindingDetailCard } from "./parts/FindingDetailCard";
import { FindingsBrowser } from "./parts/FindingsBrowser";
import { fetchFindingsList } from "./parts/fetchFindingsList";
import { bulkTriageFindings } from "./parts/findingsBulk";
import { exportFindingsCSV, exportFindingsJSON } from "./parts/findingsExport";

export function FindingsPage({
  presetCategory,
}: {
  presetCategory?: "performance" | "security";
} = {}) {
  const {
    environmentId,
    setSection,
    search,
    setSearch,
    findingId,
    openFinding,
    openInventory,
  } = useApp();
  const [items, setItems] = useState<Finding[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState<PageSize>(20);
  const [error, setError] = useState<string | null>(null);
  const [emptyKind, setEmptyKind] = useState<
    "never" | "zero" | "filter" | "dsn" | null
  >(null);
  const [coverageKind, setCoverageKind] = useState<
    "partial" | "gap" | "permission" | null
  >(null);
  const [busy, setBusy] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [selected, setSelected] = useState<Finding | null>(null);
  const [timeline, setTimeline] = useState<FindingEvent[]>([]);
  const [action, setAction] = useState<FindingAction | null>(null);
  const [actionEvents, setActionEvents] = useState<ActionEvent[]>([]);
  const [actionStatus, setActionStatus] = useState("suggested");
  const [actionOwner, setActionOwner] = useState("");
  const [actionJustification, setActionJustification] = useState("");
  const [actionResult, setActionResult] = useState("");
  const [suppressionReason, setSuppressionReason] = useState("");
  const [suppressedUntil, setSuppressedUntil] = useState("");

  useEffect(() => {
    if (!selected) {
      setTimeline([]);
      return;
    }
    let active = true;
    void api
      .findingTimeline(selected.id)
      .then((result) => {
        if (active) setTimeline(result.items);
      })
      .catch(() => {
        if (active) setTimeline([]);
      });
    return () => {
      active = false;
    };
  }, [selected?.id]);
  useEffect(() => {
    if (!selected) {
      setAction(null);
      setActionEvents([]);
      return;
    }
    let active = true;
    void Promise.all([
      api.findingAction(selected.id),
      api.findingActionEvents(selected.id),
    ])
      .then(([item, events]) => {
        if (!active) return;
        setAction(item);
        setActionEvents(events.items);
        setActionStatus(item.status);
        setActionOwner(item.owner);
        setActionJustification(item.justification);
        setActionResult(item.result);
      })
      .catch((cause: unknown) => {
        if (active)
          setError(formatError(cause, "Falha ao carregar plano de ação"));
      });
    return () => {
      active = false;
    };
  }, [selected?.id]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set());
  const [severityFilter, setSeverityFilter] = useState(search.severity || "");
  const [statusFilter, setStatusFilter] = useState(search.status ?? "open");
  const [mine, setMine] = useState(search.mine === "1");
  const [overdueOnly, setOverdueOnly] = useState(search.overdue === "1");
  const [assignee, setAssignee] = useState("");
  const [dueAt, setDueAt] = useState("");
  const [sort, setSort] = useState<SortState | null>(null);

  const load = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      const result = await fetchFindingsList({
        presetCategory,
        severityFilter,
        statusFilter,
        environmentId,
        mine,
        overdueOnly,
        pageSize,
        offset,
      });
      setItems(result.items);
      setTotal(result.total);
      setEmptyKind(result.emptyKind);
      if (result.coverageKind !== undefined) {
        setCoverageKind(result.coverageKind);
      }
      setSelectedIds(new Set());
    } catch (err: unknown) {
      setError(formatError(err, "Falha ao listar findings"));
      setItems([]);
    } finally {
      setBusy(false);
    }
  }, [
    severityFilter,
    statusFilter,
    environmentId,
    offset,
    pageSize,
    mine,
    overdueOnly,
    presetCategory,
  ]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    setSearch({
      severity: severityFilter || null,
      status: statusFilter || null,
      mine: mine ? "1" : null,
      overdue: overdueOnly ? "1" : null,
    });
  }, [severityFilter, statusFilter, mine, overdueOnly, setSearch]);

  useEffect(() => {
    const nextSeverity = search.severity || "";
    const nextStatus = search.status ?? "open";
    const nextMine = search.mine === "1";
    const nextOverdue = search.overdue === "1";
    setSeverityFilter((current) =>
      current === nextSeverity ? current : nextSeverity,
    );
    setStatusFilter((current) =>
      current === nextStatus ? current : nextStatus,
    );
    setMine((current) => (current === nextMine ? current : nextMine));
    setOverdueOnly((current) =>
      current === nextOverdue ? current : nextOverdue,
    );
  }, [search.severity, search.status, search.mine, search.overdue]);

  useEffect(() => {
    if (!findingId) return;
    let active = true;
    void api
      .finding(findingId)
      .then((item) => {
        if (active) setSelected(item);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [findingId]);

  useEffect(() => {
    setAssignee(selected?.assignee || "");
    setDueAt(selected?.due_at ? selected.due_at.slice(0, 16) : "");
  }, [selected]);

  const saveWorkflow = async () => {
    if (!selected) return;
    try {
      const due = dueAt ? new Date(dueAt).toISOString() : undefined;
      const updated = await api.updateFindingWorkflow(
        selected.id,
        assignee.trim(),
        due,
      );
      setItems((prev) => prev.map((f) => (f.id === updated.id ? updated : f)));
      setSelected(updated);
    } catch (err: unknown) {
      setError(formatError(err, "Falha ao salvar responsável ou prazo"));
    }
  };

  const saveAction = async () => {
    if (!selected) return;
    try {
      const item = await api.updateFindingAction(selected.id, {
        status: actionStatus,
        owner: actionOwner.trim(),
        justification: actionJustification.trim(),
        result: actionResult.trim(),
      });
      setAction(item);
      setActionEvents((await api.findingActionEvents(selected.id)).items);
      setError(null);
    } catch (cause) {
      setError(formatError(cause, "Falha ao salvar plano de ação"));
    }
  };

  const triage = async (id: string, status: string) => {
    try {
      const updated = await api.updateFindingStatus(id, status);
      setItems((prev) => prev.map((f) => (f.id === id ? updated : f)));
      setSelected((cur) => (cur?.id === id ? updated : cur));
    } catch (err: unknown) {
      setError(formatError(err, "Falha na triagem"));
    }
  };

  const suppressSelected = async () => {
    if (!selected || !suppressionReason.trim() || !suppressedUntil) return;
    try {
      const updated = await api.suppressFinding(
        selected.id,
        suppressionReason.trim(),
        new Date(suppressedUntil).toISOString(),
      );
      setItems((prev) => prev.map((f) => (f.id === updated.id ? updated : f)));
      setSelected(updated);
      setSuppressionReason("");
      setSuppressedUntil("");
      setTimeline((await api.findingTimeline(updated.id)).items);
    } catch (err: unknown) {
      setError(formatError(err, "Falha ao suprimir finding"));
    }
  };

  const bulkTriage = (status: string) => {
    void bulkTriageFindings(
      status,
      selectedIds,
      setItems,
      setSelected,
      setSelectedIds,
      setBulkBusy,
      setError,
    );
  };

  const toggleOne = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const allSelected =
    items.length > 0 && items.every((f) => selectedIds.has(f.id));

  const toggleAll = () => {
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(items.map((f) => f.id)));
    }
  };

  const sortedItems = useMemo(
    () =>
      sortBy(items, sort, {
        type: (f) =>
          labels.findingCategory(f.category || f.finding_type.split(".")[0]),
        severity: (f) => f.severity,
        status: (f) => f.status,
        title: (f) => f.title,
        object: (f) => f.object_key ?? "",
        last_seen: (f) => f.last_seen_at ?? "",
      }),
    [items, sort],
  );

  const exportRows = () => {
    exportFindingsCSV(sortedItems);
  };

  const exportJson = () => {
    exportFindingsJSON(sortedItems, {
      environment_id: environmentId || null,
      severity: severityFilter || null,
      status: statusFilter || null,
    });
  };

  const selectionCount = selectedIds.size;
  return (
    <FindingsBrowser
      exportRows={exportRows}
      exportJson={exportJson}
      busy={busy}
      items={items}
      mine={mine}
      setMine={setMine}
      setOffset={setOffset}
      overdueOnly={overdueOnly}
      setOverdueOnly={setOverdueOnly}
      setSeverityFilter={setSeverityFilter}
      setStatusFilter={setStatusFilter}
      severityFilter={severityFilter}
      statusFilter={statusFilter}
      load={load}
      bulkBusy={bulkBusy}
      selectionCount={selectionCount}
      bulkTriage={bulkTriage}
      error={error}
      coverageKind={coverageKind}
      emptyKind={emptyKind}
      setSection={setSection}
      sortedItems={sortedItems}
      sort={sort}
      setSort={setSort}
      allSelected={allSelected}
      toggleAll={toggleAll}
      selectedIds={selectedIds}
      toggleOne={toggleOne}
      setSelected={setSelected}
      openFinding={openFinding}
      total={total}
      pageSize={pageSize}
      offset={offset}
      setPageSize={setPageSize}
      environmentId={environmentId}
    >
      <FindingDetailCard
        selected={selected}
        action={action}
        openInventory={openInventory}
        setSection={setSection}
        actionStatus={actionStatus}
        setActionStatus={setActionStatus}
        actionOwner={actionOwner}
        setActionOwner={setActionOwner}
        actionJustification={actionJustification}
        setActionJustification={setActionJustification}
        actionResult={actionResult}
        setActionResult={setActionResult}
        saveAction={saveAction}
        actionEvents={actionEvents}
        triage={triage}
        assignee={assignee}
        setAssignee={setAssignee}
        dueAt={dueAt}
        setDueAt={setDueAt}
        saveWorkflow={saveWorkflow}
        suppressionReason={suppressionReason}
        setSuppressionReason={setSuppressionReason}
        suppressedUntil={suppressedUntil}
        setSuppressedUntil={setSuppressedUntil}
        suppressSelected={suppressSelected}
        timeline={timeline}
      />
    </FindingsBrowser>
  );
}
