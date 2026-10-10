import type { Dispatch, ReactNode, SetStateAction } from "react";
import { useMemo } from "react";
import { CoverageBanner } from "../../components/CoverageBanner";
import { PageHeader } from "../../components/PageHeader";
import {
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Select,
  Skeleton,
} from "../../components/ui";
import type { PageSize } from "../../components/ui/PaginationControls";
import { labels } from "../../lib/labels";
import type { SortState } from "../../lib/sort";
import { api } from "../../services/api";
import type { Finding, NavigationSection } from "../../types";
import { FindingsDiffPanel } from "./FindingsDiffPanel";
import { FindingsResultTable } from "./FindingsResultTable";
import { FilterChip, SEVERITY_OPTIONS, STATUS_OPTIONS } from "./findingsUi";

export function FindingsBrowser({
  exportRows,
  exportJson,
  busy,
  items,
  mine,
  setMine,
  setOffset,
  overdueOnly,
  setOverdueOnly,
  setSeverityFilter,
  setStatusFilter,
  severityFilter,
  statusFilter,
  load,
  bulkBusy,
  selectionCount,
  bulkTriage,
  error,
  coverageKind,
  emptyKind,
  setSection,
  sortedItems,
  sort,
  setSort,
  allSelected,
  toggleAll,
  selectedIds,
  toggleOne,
  setSelected,
  openFinding,
  total,
  pageSize,
  offset,
  setPageSize,
  environmentId,
  children,
}: {
  exportRows: () => void;
  exportJson: () => void;
  busy: boolean;
  items: Finding[];
  mine: boolean;
  setMine: Dispatch<SetStateAction<boolean>>;
  setOffset: (value: number) => void;
  overdueOnly: boolean;
  setOverdueOnly: Dispatch<SetStateAction<boolean>>;
  setSeverityFilter: (value: string) => void;
  setStatusFilter: (value: string) => void;
  severityFilter: string;
  statusFilter: string;
  load: () => void;
  bulkBusy: boolean;
  selectionCount: number;
  bulkTriage: (status: string) => void;
  error: string | null;
  coverageKind: "partial" | "gap" | "permission" | null;
  emptyKind: "never" | "zero" | "filter" | "dsn" | null;
  setSection: (section: NavigationSection) => void;
  sortedItems: Finding[];
  sort: SortState | null;
  setSort: Dispatch<SetStateAction<SortState | null>>;
  allSelected: boolean;
  toggleAll: () => void;
  selectedIds: Set<string>;
  toggleOne: (id: string) => void;
  setSelected: (finding: Finding) => void;
  openFinding: (id: string | null) => void;
  total: number;
  pageSize: PageSize;
  offset: number;
  setPageSize: (value: PageSize) => void;
  environmentId: string | null;
  children?: ReactNode;
}) {
  const health = useMemo(() => {
    const open = items.filter((f) => f.status === "open");
    const byPrefix = (prefix: string) =>
      open.filter((f) => f.finding_type.startsWith(prefix)).length;
    return {
      open: open.length,
      cagg: byPrefix("cagg."),
      policy: byPrefix("policy.") + byPrefix("job."),
      inactivity: byPrefix("inactivity."),
    };
  }, [items]);

  const activeChips: Array<{ key: string; label: string; clear: () => void }> =
    [];
  if (severityFilter) {
    activeChips.push({
      key: "sev",
      label: `Severidade: ${labels.severity(severityFilter)}`,
      clear: () => {
        setSeverityFilter("");
        setOffset(0);
      },
    });
  }
  if (statusFilter) {
    activeChips.push({
      key: "status",
      label: `Status: ${labels.findingStatus(statusFilter)}`,
      clear: () => {
        setStatusFilter("");
        setOffset(0);
      },
    });
  }

  return (
    <>
      <PageHeader
        eyebrow="ACHADOS"
        title="Achados"
        description="Sinais de risco em armazenamento, índices, manutenção e segurança. Confirme cada hipótese antes de mudar o banco; o auditor não exclui objetos automaticamente."
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              variant="secondary"
              onClick={exportRows}
              disabled={busy || items.length === 0}
            >
              Exportar CSV
            </Button>
            <Button
              variant="secondary"
              onClick={exportJson}
              disabled={busy || items.length === 0}
            >
              Exportar JSON
            </Button>
          </div>
        }
      />

      <div className="mt-8 space-y-6">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Card title={String(health.open)} subtitle="Abertos" />
          <Card title={String(health.cagg)} subtitle="CAGG" />
          <Card title={String(health.policy)} subtitle="Policies/Jobs" />
          <Card title={String(health.inactivity)} subtitle="Inatividade" />
        </div>

        <FindingsDiffPanel environmentId={environmentId} />

        <div className="flex flex-wrap gap-2">
          <Button
            variant={mine ? "primary" : "secondary"}
            onClick={() => {
              setMine((value) => !value);
              setOffset(0);
            }}
          >
            Meus
          </Button>
          <Button
            variant={overdueOnly ? "primary" : "secondary"}
            onClick={() => {
              setOverdueOnly((value) => !value);
              setOffset(0);
            }}
          >
            Atrasados
          </Button>
          <Button
            variant="secondary"
            onClick={() => {
              setSeverityFilter("critical");
              setStatusFilter("open");
              setOffset(0);
            }}
          >
            Críticos
          </Button>
          <Button
            variant="secondary"
            onClick={() => {
              setStatusFilter("suppressed");
              setSeverityFilter("");
              setOffset(0);
            }}
          >
            Suprimidos
          </Button>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <Select
            label="Severidade"
            options={SEVERITY_OPTIONS}
            value={severityFilter}
            onChange={(e) => {
              setSeverityFilter(e.target.value);
              setOffset(0);
            }}
          />
          <Select
            label="Status"
            options={STATUS_OPTIONS}
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value);
              setOffset(0);
            }}
          />
        </div>

        {activeChips.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {activeChips.map((c) => (
              <FilterChip key={c.key} label={c.label} onClear={c.clear} />
            ))}
          </div>
        ) : null}

        <div className="flex flex-wrap items-center gap-2">
          <Button onClick={() => void load()} disabled={busy || bulkBusy}>
            Atualizar
          </Button>
          {selectionCount > 0 && api.hasRole("auditor") ? (
            <>
              <span className="text-xs text-slate-400">
                {selectionCount} selecionado(s)
              </span>
              <Button
                disabled={bulkBusy}
                onClick={() => void bulkTriage("acknowledged")}
              >
                Reconhecer
              </Button>
              <Button
                disabled={bulkBusy}
                onClick={() => void bulkTriage("resolved")}
              >
                Resolver
              </Button>
              <Button
                variant="secondary"
                disabled={bulkBusy}
                onClick={() => void bulkTriage("open")}
              >
                Reabrir
              </Button>
            </>
          ) : null}
          {bulkBusy ? (
            <span className="text-xs text-slate-400">Aplicando triagem…</span>
          ) : null}
        </div>

        {error ? (
          <ErrorBanner message={error} onRetry={() => void load()} />
        ) : null}
        <CoverageBanner kind={coverageKind} />
        {busy ? <Skeleton className="h-40 w-full" /> : null}

        {!busy && !error && items.length === 0 && emptyKind ? (
          <EmptyState
            title={
              emptyKind === "filter"
                ? "Nenhum achado neste filtro"
                : emptyKind === "zero"
                  ? "Nenhum achado observado"
                  : emptyKind === "dsn"
                    ? "Ambiente sem conexão configurada"
                    : "Nenhuma coleta concluída"
            }
            description={
              emptyKind === "filter"
                ? "Os filtros atuais não correspondem a nenhum achado. Limpe os filtros para ver os demais."
                : emptyKind === "zero"
                  ? "A coleta terminou sem achados. Isso não prova ausência de risco se a cobertura estiver parcial."
                  : emptyKind === "dsn"
                    ? "O ambiente não tem DSN configurado. Peça a um operador para concluir a conexão antes de auditar."
                    : "Ainda não há coleta concluída. Execute uma auditoria para gerar diagnósticos."
            }
            action={
              <div className="flex flex-wrap justify-center gap-2">
                {emptyKind === "filter" ? (
                  <Button
                    variant="secondary"
                    onClick={() => {
                      setSeverityFilter("");
                      setStatusFilter("");
                      setMine(false);
                      setOverdueOnly(false);
                    }}
                  >
                    Limpar filtros
                  </Button>
                ) : emptyKind === "dsn" ? (
                  <Button onClick={() => setSection("Ambientes")}>
                    Abrir Ambientes
                  </Button>
                ) : (
                  <Button onClick={() => setSection("Execuções")}>
                    Ir para Execuções
                  </Button>
                )}
              </div>
            }
          />
        ) : null}

        <FindingsResultTable
          busy={busy}
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
          setOffset={setOffset}
        />
        {children}
      </div>
    </>
  );
}
