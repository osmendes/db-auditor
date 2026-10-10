import type { ReactNode } from "react";
import { useEffect, useId, useRef } from "react";
import { cn } from "../../lib/cn";
import { useFocusTrap } from "./useFocusTrap";

export interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: ReactNode;
  children: ReactNode;
  side?: "right" | "left";
  wide?: boolean;
}

/** Side sheet (shadcn-style) without Radix — matches ConfirmDialog pattern. */
export function Sheet({
  open,
  onOpenChange,
  title,
  description,
  children,
  side = "right",
  wide = false,
}: SheetProps) {
  const titleId = useId();
  const descId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  useFocusTrap(open, panelRef);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onOpenChange(false);
      }
    };
    window.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onOpenChange]);

  if (!open) {
    return null;
  }

  const fromRight = side === "right";

  return (
    <div className="fixed inset-0 z-50 flex" role="presentation">
      <button
        type="button"
        className="absolute inset-0 bg-slate-950/70 transition-opacity"
        aria-label="Fechar painel"
        onClick={() => onOpenChange(false)}
      />
      <div
        ref={panelRef}
        className={cn(
          "relative z-10 flex h-full w-full flex-col border-slate-700 bg-slate-900 shadow-2xl",
          wide ? "max-w-xl sm:max-w-2xl" : "max-w-md sm:max-w-lg",
          fromRight ? "ml-auto border-l" : "mr-auto border-r",
        )}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
      >
        <header className="flex shrink-0 items-start justify-between gap-3 border-b border-slate-700 px-5 py-4">
          <div className="min-w-0 flex-1">
            <h2
              id={titleId}
              className="truncate text-base font-semibold text-slate-50"
            >
              {title}
            </h2>
            {description ? (
              <div
                id={descId}
                className="mt-1 text-xs leading-relaxed text-slate-400"
              >
                {description}
              </div>
            ) : null}
          </div>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            className="shrink-0 rounded-md px-2 py-1 text-sm text-slate-400 transition hover:bg-slate-800 hover:text-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
            aria-label="Fechar"
          >
            ✕
          </button>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {children}
        </div>
      </div>
    </div>
  );
}

export function DetailGrid({
  items,
}: {
  items: Array<{ label: string; value: ReactNode }>;
}) {
  return (
    <dl className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2">
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <dt className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">
            {item.label}
          </dt>
          <dd className="mt-0.5 break-all text-sm text-slate-200">
            {item.value ?? "—"}
          </dd>
        </div>
      ))}
    </dl>
  );
}

export function DetailSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="mb-5">
      <h3 className="mb-3 border-b border-slate-800 pb-1.5 text-xs font-semibold uppercase tracking-wider text-slate-400">
        {title}
      </h3>
      {children}
    </section>
  );
}
