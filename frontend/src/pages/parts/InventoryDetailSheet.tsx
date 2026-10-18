import { Sheet } from "../../components/ui";
import { formatBytes } from "../../lib/format";
import { inventoryTargetKey } from "../../lib/inventoryTarget";
import { CAGGAssessmentPanel } from "../CAGGAssessmentPanel";
import { FunctionAssessmentPanel } from "../FunctionAssessmentPanel";
import { HypertableAssessmentPanel } from "../HypertableAssessmentPanel";
import { IndexAssessmentPanel } from "../IndexAssessmentPanel";
import {
  FunctionDetail,
  HypertableDetail,
  IndexDetail,
  ViewDetail,
} from "../InventoryDetails";
import { InventoryObjectPanel } from "../InventoryObjectPanel";
import { TableAssessmentPanel } from "../TableAssessmentPanel";
import { ViewAssessmentPanel } from "../ViewAssessmentPanel";
import { fmtNum, targetFor } from "./inventoryUi";
import { useInventoryModel } from "./useInventoryExplorer";

/** Detail sheet of the selected object; markup moved verbatim from InventoryExplorer (UI-06). */
export function InventoryDetailSheet() {
  const {
    envId,
    isMongo,
    selectedRun,
    detailError,
    sheetOpen,
    detailHistory,
    detailStats,
    detailWorkload,
    closeSheet,
    visibleSelectedItem,
    visibleSnapshot,
    selectedTable,
    navigateToRelatedTable,
    selectedIndex,
    selectedView,
    selectedCagg,
    selectedFn,
    selectedHt,
    inventory,
  } = useInventoryModel();
  return (
    <Sheet
      open={sheetOpen}
      onOpenChange={(open) => {
        if (!open) closeSheet();
      }}
      title={
        visibleSelectedItem
          ? targetFor(visibleSelectedItem).name
          : (inventory?.name ?? "Detalhe")
      }
      wide
    >
      {detailError ? (
        <p role="alert" className="mb-4 text-sm text-red-300">
          {detailError}
        </p>
      ) : null}
      {!visibleSelectedItem && !detailError && inventory ? (
        <p role="status" className="text-sm text-slate-400">
          Carregando detalhes…
        </p>
      ) : null}
      {selectedTable && isMongo ? (
        <div className="space-y-2 text-sm text-slate-200">
          <p>
            Coleção {selectedTable.database_name}.{selectedTable.table_name}
          </p>
          <p>Documentos estimados: {fmtNum(selectedTable.row_estimate)}</p>
          <p>
            Armazenamento e índices:{" "}
            {formatBytes(selectedTable.total_size_bytes)}
          </p>
          <p>
            O auditor leu apenas metadados. Regras específicas do PostgreSQL não
            são aplicadas a esta coleção.
          </p>
        </div>
      ) : selectedTable ? (
        <TableAssessmentPanel
          key={inventoryTargetKey(
            envId ?? selectedTable.environment_id,
            selectedTable.audit_run_id,
            targetFor({ kind: "tables", item: selectedTable }),
          )}
          t={selectedTable}
          env={envId ?? selectedTable.environment_id}
          history={detailHistory}
          columnStats={detailStats}
          workload={detailWorkload}
          onNavigate={navigateToRelatedTable}
        />
      ) : null}
      {visibleSelectedItem?.kind === "views" && envId && selectedRun ? (
        <ViewAssessmentPanel
          key={inventoryTargetKey(
            envId,
            selectedRun,
            targetFor(visibleSelectedItem),
          )}
          view={visibleSelectedItem.item}
          environment={envId}
          run={selectedRun}
          snapshot={visibleSnapshot}
        />
      ) : null}
      {visibleSelectedItem?.kind === "indexes" && envId && selectedRun ? (
        <IndexAssessmentPanel
          key={inventoryTargetKey(
            envId,
            selectedRun,
            targetFor(visibleSelectedItem),
          )}
          index={visibleSelectedItem.item}
          environment={envId}
          run={selectedRun}
          snapshot={visibleSnapshot}
        />
      ) : null}
      {visibleSelectedItem?.kind === "functions" && envId && selectedRun ? (
        <FunctionAssessmentPanel
          key={inventoryTargetKey(
            envId,
            selectedRun,
            targetFor(visibleSelectedItem),
          )}
          fn={visibleSelectedItem.item}
          environment={envId}
          run={selectedRun}
          snapshot={visibleSnapshot}
        />
      ) : null}
      {visibleSelectedItem?.kind === "hypertables" && envId && selectedRun ? (
        <HypertableAssessmentPanel
          key={inventoryTargetKey(
            envId,
            selectedRun,
            targetFor(visibleSelectedItem),
          )}
          hypertable={visibleSelectedItem.item}
          environment={envId}
          run={selectedRun}
          snapshot={visibleSnapshot}
        />
      ) : null}
      {selectedCagg && envId && selectedRun ? (
        <CAGGAssessmentPanel
          key={inventoryTargetKey(
            envId,
            selectedRun,
            targetFor({ kind: "caggs", item: selectedCagg }),
          )}
          cagg={selectedCagg}
          environment={envId}
          run={selectedRun}
          snapshot={visibleSnapshot}
        />
      ) : null}
      {visibleSelectedItem &&
      visibleSelectedItem.kind !== "tables" &&
      (visibleSelectedItem.kind !== "views" || !selectedRun) &&
      (visibleSelectedItem.kind !== "indexes" || !selectedRun) &&
      (visibleSelectedItem.kind !== "functions" || !selectedRun) &&
      (visibleSelectedItem.kind !== "hypertables" || !selectedRun) &&
      (visibleSelectedItem.kind !== "caggs" || !selectedRun) &&
      envId ? (
        <InventoryObjectPanel
          key={inventoryTargetKey(
            envId,
            selectedRun ?? "",
            targetFor(visibleSelectedItem),
          )}
          target={targetFor(visibleSelectedItem)}
          environment={envId}
          run={selectedRun}
          snapshot={visibleSnapshot}
          overview={
            <>
              {selectedIndex ? <IndexDetail i={selectedIndex} /> : null}
              {selectedView ? <ViewDetail v={selectedView} /> : null}
              {selectedFn ? <FunctionDetail f={selectedFn} /> : null}
              {selectedHt ? <HypertableDetail h={selectedHt} /> : null}
            </>
          }
        />
      ) : null}
      {!selectedTable &&
      !selectedIndex &&
      !selectedView &&
      !selectedFn &&
      !selectedHt &&
      !selectedCagg &&
      !inventory &&
      !detailError ? (
        <p className="text-sm text-slate-400">Nenhum detalhe.</p>
      ) : null}
    </Sheet>
  );
}
