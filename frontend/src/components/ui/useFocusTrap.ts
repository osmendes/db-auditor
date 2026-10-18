import { type RefObject, useEffect, useRef } from "react";

const FOCUSABLE = [
  "a[href]",
  "button:not([disabled])",
  "textarea:not([disabled])",
  "input:not([disabled]):not([type='hidden'])",
  "select:not([disabled])",
  "summary",
  "[tabindex]",
  "[contenteditable='true']",
].join(",");

/** Tabbable descendants in DOM order (hidden/inert/tabindex=-1 excluded). */
export function tabbable(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) =>
      el.tabIndex >= 0 &&
      el.getAttribute("tabindex") !== "-1" &&
      !el.hasAttribute("disabled") &&
      !el.closest("[hidden],[inert],[aria-hidden='true']"),
  );
}

// Open traps, innermost last. Only the top one reacts to Tab/Escape, so a
// ConfirmDialog over a Sheet does not close both with a single Escape.
const stack: symbol[] = [];

export interface FocusTrapOptions {
  /** Called when Escape is pressed while this trap is the topmost one. */
  onEscape?: () => void;
}

/**
 * Keeps keyboard focus inside `containerRef` while `open`:
 * - focuses the first tabbable element (or the container itself);
 * - Tab / Shift+Tab cycle inside, also when focus escaped (e.g. backdrop click);
 * - Escape calls `onEscape`;
 * - on close, focus returns to the element that had it before (the trigger).
 */
export function useFocusTrap(
  open: boolean,
  containerRef: RefObject<HTMLElement | null>,
  options: FocusTrapOptions = {},
) {
  const onEscape = useRef(options.onEscape);
  onEscape.current = options.onEscape;

  useEffect(() => {
    if (!open) return;
    const node = containerRef.current;
    if (!node) return;
    const id = Symbol("focus-trap");
    stack.push(id);
    const trigger =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    if (!node.hasAttribute("tabindex")) node.setAttribute("tabindex", "-1");
    if (!node.contains(document.activeElement)) {
      (tabbable(node)[0] ?? node).focus();
    }

    const onKey = (event: KeyboardEvent) => {
      if (stack[stack.length - 1] !== id) return;
      if (event.key === "Escape") {
        if (onEscape.current) {
          event.preventDefault();
          event.stopPropagation();
          onEscape.current();
        }
        return;
      }
      if (event.key !== "Tab") return;
      const items = tabbable(node);
      if (items.length === 0) {
        event.preventDefault();
        node.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      const inside = node.contains(active);
      if (event.shiftKey && (!inside || active === first || active === node)) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && (!inside || active === last)) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("keydown", onKey, true);
      const index = stack.indexOf(id);
      if (index >= 0) stack.splice(index, 1);
      if (trigger?.isConnected) trigger.focus();
    };
  }, [open, containerRef]);
}
