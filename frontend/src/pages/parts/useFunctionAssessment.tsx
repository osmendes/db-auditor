import { useEffect, useState } from "react";
import {
  inventoryDetailState,
  useInventoryDetailPage,
} from "../../components/InventoryDetailData";
import type { InventoryDetailTab } from "../../components/InventoryDetailTabs";
import { api } from "../../services/api";
import type {
  DependencySnapshot,
  Finding,
  FunctionSnapshot,
  GrantSnapshot,
  SnapshotCompleteness,
} from "../../types";
import type { FunctionDetailSnapshot } from "../../types/openapi";

/** Data and state of FunctionAssessmentPanel (UI-06 extraction). */
export function functionKind(kind: string | null | undefined): string {
  switch (kind) {
    case "f":
      return "Função";
    case "p":
      return "Procedimento";
    case "a":
      return "Agregação";
    case "w":
      return "Função de janela";
    default:
      return "Não coletado";
  }
}

export function volatilityLabel(value: string | null | undefined): string {
  return value === "i"
    ? "Imutável"
    : value === "s"
      ? "Estável"
      : value === "v"
        ? "Volátil"
        : "Não coletado";
}

export function useFunctionAssessment({
  fn,
  environment,
  run,
  snapshot,
}: {
  fn: FunctionSnapshot;
  environment: string;
  run: string;
  snapshot: SnapshotCompleteness | null;
}) {
  const [tab, setTab] = useState<InventoryDetailTab>("overview");
  const [detail, setDetail] = useState<FunctionDetailSnapshot | null>(null);
  const [detailError, setDetailError] = useState<unknown>(null);
  const [detailLoading, setDetailLoading] = useState(true);
  const scope = JSON.stringify([
    environment,
    run,
    fn.database_name,
    fn.schema_name,
    fn.function_name,
    fn.identity_arguments,
  ]);
  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    void api
      .functionDetail(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
      )
      .then((value) => {
        if (!cancelled) setDetail(value);
      })
      .catch((cause: unknown) => {
        if (!cancelled) setDetailError(cause);
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [scope]);
  const dependencies = useInventoryDetailPage<DependencySnapshot>(
    tab === "relationships",
    scope,
    (offset) =>
      api.functionDependencies(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
        50,
        offset,
      ),
  );
  const grants = useInventoryDetailPage<GrantSnapshot>(
    tab === "security",
    scope,
    (offset) =>
      api.functionGrants(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
        50,
        offset,
      ),
  );
  const findings = useInventoryDetailPage<Finding>(
    tab === "recommendations",
    scope,
    (offset) =>
      api.functionFindings(
        environment,
        run,
        fn.database_name,
        fn.schema_name,
        fn.function_name,
        fn.identity_arguments,
        50,
        offset,
      ),
  );
  const detailState = inventoryDetailState(
    detailError,
    detailLoading,
    detail !== null,
  );
  const target = {
    kind: "functions" as const,
    database: fn.database_name,
    schema: fn.schema_name,
    name: fn.function_name,
    signature: fn.identity_arguments,
  };
  return {
    dependencies,
    detail,
    detailError,
    detailLoading,
    detailState,
    environment,
    findings,
    fn,
    grants,
    run,
    scope,
    setDetail,
    setDetailError,
    setDetailLoading,
    setTab,
    snapshot,
    tab,
    target,
  };
}

export type FunctionAssessmentState = ReturnType<typeof useFunctionAssessment>;
