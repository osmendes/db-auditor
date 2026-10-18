import type { ReactNode } from "react";
import { cn } from "../../lib/cn";
import {
  EMPTY_CAUSE_ACTION,
  type EmptyCause,
  type EmptySubject,
  emptyCauseCopy,
} from "../../lib/emptyCause";
import { Button } from "./Button";

export interface EmptyStateProps {
  /**
   * Why the list is empty (UI-09). Each cause has its own copy and CTA:
   * never → start a run, zero → open the run coverage, filter → clear the
   * filter, dsn → go to Ambientes. `title`/`description` override the copy.
   */
  cause?: EmptyCause;
  /** What the list holds, e.g. `{ none: "Nenhum achado", plural: "achados" }`. */
  subject?: EmptySubject;
  /** Handler of the standard CTA for `cause`. Without it no CTA is shown. */
  onAction?: () => void;
  actionLabel?: string;
  /** Disables the standard CTA (e.g. the role cannot start runs). */
  actionDisabled?: boolean;
  title?: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({
  cause,
  subject,
  onAction,
  actionLabel,
  actionDisabled,
  title,
  description,
  action,
  className,
}: EmptyStateProps) {
  const copy = cause ? emptyCauseCopy(cause, subject) : null;
  const heading = title ?? copy?.title ?? "";
  const text = description ?? copy?.description;
  const cta =
    action ??
    (cause && onAction ? (
      <Button
        variant={cause === "filter" ? "secondary" : "primary"}
        onClick={onAction}
        disabled={actionDisabled}
      >
        {actionLabel ?? EMPTY_CAUSE_ACTION[cause]}
      </Button>
    ) : null);
  return (
    <div
      className={cn(
        "rounded-xl border border-dashed border-slate-700 bg-slate-900/40 px-6 py-10 text-center",
        className,
      )}
      data-empty-cause={cause}
    >
      <p className="text-base font-medium text-slate-100">{heading}</p>
      {text ? (
        <p className="mx-auto mt-2 max-w-lg text-sm leading-relaxed text-slate-400">
          {text}
        </p>
      ) : null}
      {cta ? <div className="mt-4 flex justify-center">{cta}</div> : null}
    </div>
  );
}
