import { useEffect, useState } from "react";
import {
  inventoryDetailPageControls,
  inventoryDetailState,
  useInventoryDetailPage,
} from "../../components/InventoryDetailData";
import type { InventoryDetailTab } from "../../components/InventoryDetailTabs";
import { api } from "../../services/api";
import type {
  Finding,
  GrantSnapshot,
  HypertableChunkDetail,
  HypertableDimensionDetail,
  HypertableHistoryPoint,
  HypertableJobDetail,
  HypertablePolicyDetail,
  HypertableSnapshot,
  IndexSnapshot,
  RLSPolicySnapshot,
  SnapshotCompleteness,
} from "../../types";
import type { HypertableDetailSnapshot } from "../../types/openapi";

/** Data and state of HypertableAssessmentPanel (UI-06 extraction). */
export function date(value: string | null | undefined): string {
  return value ? new Date(value).toLocaleString("pt-BR") : "Não informado";
}

export function jobStatus(value: string | null): string {
  if (!value) return "Sem resultado coletado";
  if (value.toLowerCase() === "success") return "Concluído";
  if (value.toLowerCase() === "failed") return "Falhou";
  return value;
}

export function useHypertableAssessment({
  hypertable,
  environment,
  run,
  snapshot,
}: {
  hypertable: HypertableSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<HypertableDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const {
    database_name: database,
    schema_name: schema,
    hypertable_name: name,
  } = hypertable;
  const scope = JSON.stringify([environment, run, database, schema, name]);
  const args = [environment, run, database, schema, name] as const;
  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .hypertableDetail(...args, controller.signal)
      .then((item) => {
        if (!cancelled) setDetail(item);
      })
      .catch((error: unknown) => {
        if (!cancelled) setDetailError(error);
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [scope]);
  const dimensions = useInventoryDetailPage<HypertableDimensionDetail>(
    tab === "structure",
    scope,
    (offset, signal) => api.hypertableDimensions(...args, 50, offset, signal),
  );
  const chunks = useInventoryDetailPage<HypertableChunkDetail>(
    tab === "structure",
    scope,
    (offset, signal) => api.hypertableChunks(...args, 50, offset, signal),
  );
  const policies = useInventoryDetailPage<HypertablePolicyDetail>(
    tab === "relationships",
    scope,
    (offset, signal) => api.hypertablePolicies(...args, 50, offset, signal),
  );
  const jobs = useInventoryDetailPage<HypertableJobDetail>(
    tab === "performance",
    scope,
    (offset, signal) => api.hypertableJobs(...args, 50, offset, signal),
  );
  const history = useInventoryDetailPage<HypertableHistoryPoint>(
    tab === "performance",
    scope,
    (offset, signal) => api.hypertableHistory(...args, 50, offset, signal),
  );
  const indexes = useInventoryDetailPage<IndexSnapshot>(
    tab === "relationships",
    scope,
    (offset, signal) => api.hypertableIndexes(...args, 50, offset, signal),
  );
  const grants = useInventoryDetailPage<GrantSnapshot>(
    tab === "security" && !!detail?.base_table_observed,
    scope,
    (offset, signal) => api.hypertableGrants(...args, 50, offset, signal),
  );
  const rls = useInventoryDetailPage<RLSPolicySnapshot>(
    tab === "security" && !!detail?.base_table_observed,
    scope,
    (offset, signal) => api.hypertableRLSPolicies(...args, 50, offset, signal),
  );
  const findings = useInventoryDetailPage<Finding>(
    tab === "recommendations",
    scope,
    (offset, signal) => api.hypertableFindings(...args, 50, offset, signal),
  );
  const detailState = inventoryDetailState(
    detailError,
    detailLoading,
    detail !== null,
  );
  const target = { kind: "hypertables" as const, database, schema, name };
  const sectionState = (page: {
    error: unknown;
    loading: boolean;
    data: unknown;
  }) => inventoryDetailState(page.error, page.loading, page.data !== null);
  const controls = (
    page: {
      data: { page: { total: number } } | null;
      offset: number;
      setOffset: (value: number) => void;
    },
    label: string,
  ) =>
    inventoryDetailPageControls(
      page.data?.page.total ?? 0,
      page.offset,
      page.setOffset,
      label,
    );
  return {
    args,
    chunks,
    controls,
    database,
    detail,
    detailError,
    detailLoading,
    detailState,
    dimensions,
    environment,
    findings,
    grants,
    history,
    hypertable,
    indexes,
    jobs,
    name,
    policies,
    rls,
    run,
    schema,
    scope,
    sectionState,
    setDetail,
    setDetailError,
    setDetailLoading,
    setTab,
    snapshot,
    tab,
    target,
  };
}

export type HypertableAssessmentState = ReturnType<
  typeof useHypertableAssessment
>;
