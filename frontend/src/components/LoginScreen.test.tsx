// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider } from "../context/ThemeContext";
import { byText, cleanup, click, render } from "../test/dom";
import {
  AUTH_ERROR_CLASS,
  LoginScreen,
  type LoginScreenProps,
  SessionRestoreScreen,
} from "./LoginScreen";

function props(patch: Partial<LoginScreenProps> = {}): LoginScreenProps {
  return {
    username: "",
    password: "",
    code: "",
    mfa: false,
    error: "",
    busy: false,
    onUsername: () => {},
    onPassword: () => {},
    onCode: () => {},
    onSubmit: (event) => event.preventDefault(),
    ...patch,
  };
}

afterEach(async () => {
  await cleanup();
  window.localStorage.clear();
});

describe("LoginScreen (UI-04)", () => {
  it("uses autocomplete username/current-password and labelled inputs", async () => {
    const { container } = await render(
      <ThemeProvider>
        <LoginScreen {...props()} />
      </ThemeProvider>,
    );
    const user = container.querySelector<HTMLInputElement>(
      'input[autocomplete="username"]',
    );
    const pass = container.querySelector<HTMLInputElement>(
      'input[autocomplete="current-password"]',
    );
    expect(user?.closest("label")?.textContent).toContain("Usuário");
    expect(pass?.type).toBe("password");
    expect(pass?.closest("label")?.textContent).toContain("Senha");
    expect(container.textContent).toContain("até 8 horas");
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it("announces errors with role=alert linked to the fields", async () => {
    const { container } = await render(
      <ThemeProvider>
        <LoginScreen {...props({ error: "Usuário ou senha inválidos." })} />
      </ThemeProvider>,
    );
    const alert = container.querySelector('[role="alert"]');
    expect(alert?.textContent).toBe("Usuário ou senha inválidos.");
    expect(alert?.className).toBe(AUTH_ERROR_CLASS);
    const user = container.querySelector('input[autocomplete="username"]');
    expect(user?.getAttribute("aria-invalid")).toBe("true");
    expect(user?.getAttribute("aria-describedby")).toBe(alert?.id);
  });

  it("asks the one-time code with autocomplete one-time-code", async () => {
    const { container } = await render(
      <ThemeProvider>
        <LoginScreen {...props({ mfa: true, error: "Código inválido." })} />
      </ThemeProvider>,
    );
    const otp = container.querySelector('input[autocomplete="one-time-code"]');
    expect(otp).not.toBeNull();
    expect(otp?.getAttribute("aria-describedby")).toBe(
      container.querySelector('[role="alert"]')?.id,
    );
    expect(byText(container, "Confirmar código", "button")).not.toBeNull();
  });

  it("follows the persisted theme and toggles it from the login", async () => {
    window.localStorage.setItem("db-auditor-theme", "light");
    const { container } = await render(
      <ThemeProvider>
        <LoginScreen {...props()} />
      </ThemeProvider>,
    );
    expect(document.documentElement.classList.contains("light")).toBe(true);
    click(byText(container, "Tema escuro", "button"));
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(window.localStorage.getItem("db-auditor-theme")).toBe("dark");
  });
});

describe("SessionRestoreScreen", () => {
  it("uses the design-system button and role=alert for restore errors", async () => {
    const retry = vi.fn();
    const { container } = await render(
      <ThemeProvider>
        <SessionRestoreScreen error="API fora do ar." onRetry={retry} />
      </ThemeProvider>,
    );
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(
      "API fora do ar.",
    );
    const button = byText(container, "Tentar novamente", "button");
    expect(button?.className).toContain("focus-visible:ring-2");
    click(button);
    expect(retry).toHaveBeenCalledOnce();
  });

  it("reports the check in progress as a status", async () => {
    const { container } = await render(
      <ThemeProvider>
        <SessionRestoreScreen error="" onRetry={() => {}} />
      </ThemeProvider>,
    );
    expect(container.querySelector('[role="status"]')?.textContent).toBe(
      "Verificando sessão…",
    );
  });
});
