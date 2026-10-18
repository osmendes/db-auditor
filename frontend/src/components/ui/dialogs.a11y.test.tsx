// @vitest-environment happy-dom
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { byText, cleanup, click, press, render } from "../../test/dom";
import { ConfirmDialog } from "./ConfirmDialog";
import { Modal } from "./Modal";
import { Sheet } from "./Sheet";

afterEach(cleanup);

function ModalHarness({ onClose }: { onClose?: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Abrir modal
      </button>
      <Modal
        open={open}
        title="Editar"
        onClose={() => {
          onClose?.();
          setOpen(false);
        }}
        footer={<button type="button">Salvar</button>}
      >
        <input aria-label="Nome" />
      </Modal>
    </>
  );
}

function ConfirmHarness({ busy = false }: { busy?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Cancelar execução
      </button>
      <ConfirmDialog
        open={open}
        title="Cancelar a execução?"
        description="Os coletores em andamento param."
        danger
        busy={busy}
        onConfirm={() => setOpen(false)}
        onCancel={() => setOpen(false)}
      />
    </>
  );
}

function SheetWithConfirm() {
  const [sheet, setSheet] = useState(false);
  const [confirm, setConfirm] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setSheet(true)}>
        Abrir painel
      </button>
      <Sheet open={sheet} onOpenChange={setSheet} title="Detalhe">
        <button type="button" onClick={() => setConfirm(true)}>
          Descartar
        </button>
      </Sheet>
      <ConfirmDialog
        open={confirm}
        title="Descartar?"
        onConfirm={() => setConfirm(false)}
        onCancel={() => setConfirm(false)}
      />
    </>
  );
}

function openFrom(container: HTMLElement, label: string) {
  const trigger = byText(container, label, "button");
  trigger?.focus();
  click(trigger);
  return trigger;
}

describe("Modal (UI-05)", () => {
  it("moves focus inside, cycles Tab both ways and returns focus on Escape", async () => {
    const onClose = vi.fn();
    const { container } = await render(<ModalHarness onClose={onClose} />);
    const trigger = openFrom(container, "Abrir modal");
    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog?.getAttribute("aria-modal")).toBe("true");
    const title = document.getElementById(
      dialog?.getAttribute("aria-labelledby") ?? "",
    );
    expect(title?.textContent).toBe("Editar");
    const close = byText(document.body, "Fechar", "button");
    const save = byText(document.body, "Salvar", "button");
    expect(document.activeElement).toBe(close);

    save?.focus();
    expect(press("Tab").defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(close);
    press("Tab", { shiftKey: true });
    expect(document.activeElement).toBe(save);

    // Focus that escaped (e.g. a click on the page) is pulled back inside.
    trigger?.focus();
    press("Tab");
    expect(document.activeElement).toBe(close);

    press("Escape");
    expect(onClose).toHaveBeenCalledOnce();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("returns focus to the trigger when closed with the button", async () => {
    const { container } = await render(<ModalHarness />);
    const trigger = openFrom(container, "Abrir modal");
    click(byText(document.body, "Fechar", "button"));
    expect(document.activeElement).toBe(trigger);
  });
});

describe("ConfirmDialog (UI-05)", () => {
  it("traps Tab, closes on Escape and gives focus back to the trigger", async () => {
    const { container } = await render(<ConfirmHarness />);
    const trigger = openFrom(container, "Cancelar execução");
    const cancel = byText(document.body, "Cancelar", "button");
    const confirm = byText(document.body, "Confirmar", "button");
    expect(document.activeElement).toBe(cancel);
    // The backdrop is not a tab stop.
    expect(
      document
        .querySelector('[aria-label="Fechar diálogo"]')
        ?.getAttribute("tabindex"),
    ).toBe("-1");
    confirm?.focus();
    press("Tab");
    expect(document.activeElement).toBe(cancel);
    press("Tab", { shiftKey: true });
    expect(document.activeElement).toBe(confirm);
    press("Escape");
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("ignores Escape while busy", async () => {
    const { container } = await render(<ConfirmHarness busy />);
    openFrom(container, "Cancelar execução");
    press("Escape");
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  });
});

describe("Sheet (UI-05)", () => {
  it("closes only the topmost layer on Escape and restores focus step by step", async () => {
    const { container } = await render(<SheetWithConfirm />);
    const trigger = openFrom(container, "Abrir painel");
    const discard = byText(document.body, "Descartar", "button");
    expect(document.activeElement).toBe(byText(document.body, "✕", "button"));
    discard?.focus();
    click(discard);
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(2);

    press("Escape");
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(document.activeElement).toBe(discard);

    press("Escape");
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
    expect(document.activeElement).toBe(trigger);
  });
});
