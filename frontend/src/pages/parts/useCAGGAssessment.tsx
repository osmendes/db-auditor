import { useEffect, useState } from "react";
import {
  inventoryDetailPageControls,
  inventoryDetailState,
  useInventoryDetailPage,
} from "../../components/InventoryDetailData";
import type { InventoryDetailTab } from "../../components/InventoryDetailTabs";
import { api } from "../../services/api";
import type {
  CAGGHistoryPoint,
  CAGGRefreshPolicy,
  CAGGSnapshot,
  DependencySnapshot,
  Finding,
  GrantSnapshot,
  SnapshotCompleteness,
} from "../../types";
import type { CAGGDetailSnapshot } from "../../types/openapi";

/** Data and state of CAGGAssessmentPanel (UI-06 extraction). */
export function date(value: string | null | undefined): string {
  return value ? new Date(value).toLocaleString("pt-BR") : "Não informado";
}

export function refreshStatus(value: string | null): string {
  if (!value) return "Não coletado";
  if (value.toLowerCase() === "success") return "Concluído";
  if (value.toLowerCase() === "failed") return "Falhou";
  return value;
}

export function useCAGGAssessment({
  cagg,
  environment,
  run,
  snapshot,
}: {
  cagg: CAGGSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<CAGGDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const {
    database_name: database,
    schema_name: schema,
    view_name: name,
  } = cagg;
  const scope = JSON.stringify([environment, run, database, schema, name]);
  const args = [environment, run, database, schema, name] as const;
  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .caggDetail(...args)
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
    };
  }, [scope]);
  const policies = useInventoryDetailPage<CAGGRefreshPolicy>(
    tab === "performance",
    scope,
    (offset) => api.caggRefreshPolicies(...args, 50, offset),
  );
  const history = useInventoryDetailPage<CAGGHistoryPoint>(
    tab === "performance",
    scope,
    (offset) => api.caggHistory(...args, 50, offset),
  );
  const dependencies = useInventoryDetailPage<DependencySnapshot>(
    tab === "relationships",
    scope,
    (offset) => api.caggDependencies(...args, 50, offset),
  );
  const grants = useInventoryDetailPage<GrantSnapshot>(
    tab === "security",
    scope,
    (offset) => api.caggGrants(...args, 50, offset),
  );
  const findings = useInventoryDetailPage<Finding>(
    tab === "recommendations",
    scope,
    (offset) => api.caggFindings(...args, 50, offset),
  );
  const detailState = inventoryDetailState(
    detailError,
    detailLoading,
    detail !== null,
  );
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
    cagg,
    controls,
    database,
    dependencies,
    detail,
    detailError,
    detailLoading,
    detailState,
    environment,
    findings,
    grants,
    history,
    name,
    policies,
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
  };
}

export type CAGGAssessmentState = ReturnType<typeof useCAGGAssessment>;
