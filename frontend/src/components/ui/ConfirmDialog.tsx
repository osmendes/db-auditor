import type { ReactNode } from "react";
import { useId, useRef } from "react";
import { Button } from "./Button";
import { useFocusTrap } from "./useFocusTrap";

export interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** When true, confirm uses danger emphasis (destructive actions). */
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "Confirmar",
  cancelLabel = "Cancelar",
  danger = false,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const titleId = useId();
  const descId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  // Tab cycles inside, Escape cancels (unless busy), focus returns to the
  // trigger on close (UI-05).
  useFocusTrap(open, panelRef, {
    onEscape: () => {
      if (!busy) onCancel();
    },
  });

  if (!open) {
    return null;
  }

  const dismiss = () => {
    if (!busy) {
      onCancel();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button
        type="button"
        className="absolute inset-0 bg-slate-950/70"
        aria-label="Fechar diálogo"
        tabIndex={-1}
        disabled={busy}
        onClick={dismiss}
      />
      <div
        ref={panelRef}
        className="relative z-10 w-full max-w-md rounded-xl border border-slate-700 bg-slate-900 shadow-xl"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
      >
        <header className="border-b border-slate-700 px-5 py-4">
          <h2 id={titleId} className="text-lg font-semibold text-slate-50">
            {title}
          </h2>
        </header>
        {description ? (
          <div
            id={descId}
            className="px-5 py-4 text-sm leading-relaxed text-slate-300"
          >
            {description}
          </div>
        ) : (
          <div className="px-5 py-2" />
        )}
        <footer className="flex flex-wrap justify-end gap-2 border-t border-slate-700 px-5 py-4">
          <Button variant="secondary" onClick={onCancel} disabled={busy}>
            {cancelLabel}
          </Button>
          <Button
            className={
              danger
                ? "border-rose-600 bg-rose-600 text-white hover:bg-rose-500"
                : undefined
            }
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? "Aguarde…" : confirmLabel}
          </Button>
        </footer>
      </div>
    </div>
  );
}
