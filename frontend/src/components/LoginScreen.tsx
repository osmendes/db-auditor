import { type FormEvent, type ReactNode, useId } from "react";
import { useTheme } from "../context/ThemeContext";
import { companyName } from "../lib/company";
import { Button, Input } from "./ui";

/**
 * Login and session-restore surfaces. Both use the design-system tokens
 * (slate scale inverted by the theme, Button/Input), so they follow the
 * persisted light/dark choice instead of a hard-coded dark card.
 */
function AuthCard({
  children,
  as = "div",
  onSubmit,
}: {
  children: ReactNode;
  as?: "div" | "form";
  onSubmit?: (event: FormEvent) => void;
}) {
  const className =
    "w-full max-w-sm space-y-4 rounded-xl border border-slate-700 bg-slate-900 p-7 text-slate-100 shadow-sm";
  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-950 p-6 text-slate-100">
      {as === "form" ? (
        <form onSubmit={onSubmit} className={className}>
          {children}
        </form>
      ) : (
        <div className={className}>{children}</div>
      )}
    </main>
  );
}

function ThemeToggle() {
  const { theme, toggleTheme } = useTheme();
  return (
    <Button
      variant="ghost"
      className="px-2 py-1 text-xs underline"
      onClick={toggleTheme}
    >
      {theme === "dark" ? "Tema claro" : "Tema escuro"}
    </Button>
  );
}

/** Text tone for authentication errors; AA in both themes (see styles.css). */
export const AUTH_ERROR_CLASS = "text-sm text-rose-300";

export function SessionRestoreScreen({
  error,
  onRetry,
}: {
  error: string;
  onRetry: () => void;
}) {
  return (
    <AuthCard>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">DB Auditor</h1>
          <p className="text-sm text-slate-400">{companyName()}</p>
        </div>
        <ThemeToggle />
      </div>
      {error ? (
        <>
          <p role="alert" className={AUTH_ERROR_CLASS}>
            {error}
          </p>
          <Button onClick={onRetry}>Tentar novamente</Button>
        </>
      ) : (
        <p role="status" className="text-sm text-slate-400">
          Verificando sessão…
        </p>
      )}
    </AuthCard>
  );
}

export interface LoginScreenProps {
  username: string;
  password: string;
  code: string;
  mfa: boolean;
  error: string;
  busy: boolean;
  onUsername: (value: string) => void;
  onPassword: (value: string) => void;
  onCode: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
}

export function LoginScreen({
  username,
  password,
  code,
  mfa,
  error,
  busy,
  onUsername,
  onPassword,
  onCode,
  onSubmit,
}: LoginScreenProps) {
  const errorId = useId();
  const described = error ? errorId : undefined;
  return (
    <AuthCard as="form" onSubmit={onSubmit}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Entrar no DB Auditor</h1>
          <p className="text-sm text-slate-400">{companyName()}</p>
        </div>
        <ThemeToggle />
      </div>
      <p className="text-sm text-slate-400">
        Sua sessão pode ser usada em outras abas deste site por até 8 horas. Use
        Sair ao terminar.
      </p>
      <Input
        label="Usuário"
        name="username"
        required
        autoComplete="username"
        value={username}
        aria-invalid={error && !mfa ? true : undefined}
        aria-describedby={mfa ? undefined : described}
        onChange={(event) => onUsername(event.target.value)}
      />
      <Input
        label="Senha"
        name="password"
        required
        type="password"
        autoComplete="current-password"
        value={password}
        aria-invalid={error && !mfa ? true : undefined}
        aria-describedby={mfa ? undefined : described}
        onChange={(event) => onPassword(event.target.value)}
      />
      {mfa ? (
        <Input
          label="Código TOTP ou recuperação"
          name="one-time-code"
          required
          autoComplete="one-time-code"
          inputMode="text"
          value={code}
          aria-invalid={error ? true : undefined}
          aria-describedby={described}
          onChange={(event) => onCode(event.target.value)}
        />
      ) : null}
      {error ? (
        <p id={errorId} role="alert" className={AUTH_ERROR_CLASS}>
          {error}
        </p>
      ) : null}
      <Button disabled={busy} className="w-full" type="submit">
        {busy ? "Entrando…" : mfa ? "Confirmar código" : "Entrar"}
      </Button>
    </AuthCard>
  );
}
