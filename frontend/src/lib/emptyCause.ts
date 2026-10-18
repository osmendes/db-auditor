/**
 * Why a list is empty (UI-09). The four causes need different actions, so
 * they never share a sentence:
 * - never:  no completed collection yet → start a run;
 * - zero:   the collection finished and found nothing → check its coverage;
 * - filter: data exists but the current filter hides it → clear the filter;
 * - dsn:    the environment has no connection configured → go to Ambientes.
 */
export type EmptyCause = "never" | "zero" | "filter" | "dsn";

export interface EmptySubject {
  /** "Nenhum achado", "Nenhuma execução"… */
  none: string;
  /** "achados", "execuções"… */
  plural: string;
}

export const DEFAULT_SUBJECT: EmptySubject = {
  none: "Nenhum item",
  plural: "itens",
};

export const EMPTY_CAUSE_ACTION: Record<EmptyCause, string> = {
  never: "Iniciar uma execução",
  zero: "Ver cobertura da execução",
  filter: "Limpar filtros",
  dsn: "Ir para Ambientes",
};

export function emptyCauseCopy(
  cause: EmptyCause,
  subject: EmptySubject = DEFAULT_SUBJECT,
): { title: string; description: string } {
  switch (cause) {
    case "never":
      return {
        title: "Nenhuma coleta concluída",
        description: `Ainda não há coleta concluída neste ambiente, então não há ${subject.plural} para mostrar. Inicie uma execução para gerar dados.`,
      };
    case "zero":
      return {
        title: `${subject.none} na última coleta`,
        description: `A coleta terminou e não registrou ${subject.plural}. Isso não prova ausência de risco se a cobertura estiver parcial.`,
      };
    case "filter":
      return {
        title: `${subject.none} neste filtro`,
        description: `Há ${subject.plural}, mas os filtros atuais escondem todos. Limpe os filtros para ver os demais.`,
      };
    case "dsn":
      return {
        title: "Ambiente sem conexão configurada",
        description:
          "O ambiente não tem DSN configurado. Peça a um operador para concluir a conexão antes de auditar.",
      };
  }
}

export interface EmptyCauseInput {
  /** The list is filtered (search, select, toggle) beyond the environment. */
  filtered: boolean;
  /** Rows that exist without the filter (`null` when unknown). */
  unfilteredTotal?: number | null;
  /** Every connection in scope lacks a DSN. */
  dsnMissing: boolean;
  /** A run finished (success or partial) in scope. */
  collected: boolean;
}

/** Order matters: a filter hides data, a missing DSN explains "never". */
export function resolveEmptyCause({
  filtered,
  unfilteredTotal,
  dsnMissing,
  collected,
}: EmptyCauseInput): EmptyCause {
  if (filtered && (unfilteredTotal == null || unfilteredTotal > 0)) {
    return "filter";
  }
  if (dsnMissing) return "dsn";
  return collected ? "zero" : "never";
}
