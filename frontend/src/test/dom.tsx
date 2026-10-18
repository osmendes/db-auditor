/**
 * Minimal DOM test helpers (no Testing Library). Use them in files that start
 * with `// @vitest-environment happy-dom`.
 */
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

export interface Rendered {
  container: HTMLElement;
  rerender: (ui: ReactNode) => Promise<void>;
  unmount: () => Promise<void>;
}

const mounted: Array<() => Promise<void>> = [];

export async function render(ui: ReactNode): Promise<Rendered> {
  const container = document.createElement("div");
  document.body.appendChild(container);
  let root: Root | null = null;
  await act(async () => {
    root = createRoot(container);
    root.render(ui);
  });
  const unmount = async () => {
    await act(async () => root?.unmount());
    container.remove();
  };
  mounted.push(unmount);
  return {
    container,
    rerender: async (next) => {
      await act(async () => root?.render(next));
    },
    unmount,
  };
}

/** Unmounts everything rendered by `render`; call from afterEach. */
export async function cleanup() {
  while (mounted.length > 0) {
    await mounted.pop()?.();
  }
  document.body.innerHTML = "";
}

export async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

export function press(
  key: string,
  init: KeyboardEventInit = {},
  target: EventTarget = document.activeElement ?? document.body,
) {
  const event = new KeyboardEvent("keydown", {
    key,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  act(() => {
    target.dispatchEvent(event);
  });
  return event;
}

export function click(element: Element | null) {
  if (!element) throw new Error("elemento ausente");
  act(() => {
    (element as HTMLElement).click();
  });
}

/** Sets a controlled input value the way React listens for it. */
export function type(element: Element | null, value: string) {
  if (!(element instanceof HTMLInputElement)) {
    throw new Error("input ausente");
  }
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  act(() => {
    setter?.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

export function byText(root: ParentNode, text: string, selector = "*") {
  return (
    Array.from(root.querySelectorAll<HTMLElement>(selector)).find(
      (el) => el.textContent?.trim() === text,
    ) ?? null
  );
}
