import { Sheet } from "../components/ui";
import { labels } from "../lib/labels";
import { FindingDetailCard } from "./parts/FindingDetailCard";
import { FindingsBrowser } from "./parts/FindingsBrowser";
import { RulesCatalogSheet } from "./parts/RulesCatalogSheet";
import { useFindingsPage } from "./parts/useFindingsPage";

export function FindingsPage() {
  const s = useFindingsPage();
  const {
    action,
    actionEvents,
    actionJustification,
    actionOwner,
    actionResult,
    actionStatus,
    allSelected,
    assignee,
    bulkBusy,
    bulkTriage,
    busy,
    category,
    coverage,
    dueAt,
    emptyKind,
    environmentId,
    error,
    exportJson,
    exportRows,
    items,
    load,
    mine,
    offset,
    openFinding,
    openInventory,
    overdueOnly,
    pageSize,
    rulesOpen,
    focusRule,
    saveAction,
    saveWorkflow,
    selected,
    selectedIds,
    selectionCount,
    setActionJustification,
    setActionOwner,
    setActionResult,
    setActionStatus,
    setAssignee,
    setDueAt,
    setMine,
    setOffset,
    setOverdueOnly,
    setPageSize,
    setSearch,
    setSection,
    setSelected,
    setSeverityFilter,
    setSort,
    setStatusFilter,
    setSuppressedUntil,
    setSuppressionReason,
    severityFilter,
    sort,
    sortedItems,
    statusFilter,
    suppressedUntil,
    suppressionReason,
    suppressSelected,
    timeline,
    toggleAll,
    toggleOne,
    total,
    triage,
  } = s;
  return (
    <>
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
        coverageSignals={coverage.signals}
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
        category={category}
        setCategory={(value) => {
          setOffset(0);
          setSearch({ category: value || null });
        }}
        onOpenRules={() => setSearch({ view: "rules" })}
      />
      <Sheet
        open={selected != null}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null);
            openFinding(null);
          }
        }}
        title={selected?.friendly_meaning || selected?.title || "Análise"}
        description={
          selected
            ? `${labels.severity(selected.severity)} · ${selected.object_key || "objeto não informado"}`
            : undefined
        }
        wide
      >
        <FindingDetailCard
          selected={selected}
          action={action}
          openInventory={openInventory}
          onOpenRules={() => setSearch({ view: "rules" })}
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
      </Sheet>
      <RulesCatalogSheet
        open={rulesOpen}
        focusRule={focusRule}
        onClearFocus={() => setSearch({ rule: null })}
        onOpenChange={(open) =>
          setSearch(open ? { view: "rules" } : { view: null, rule: null })
        }
      />
    </>
  );
}
