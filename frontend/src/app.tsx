import {
  type FormEvent,
  lazy,
  type ReactNode,
  Suspense,
  useCallback,
  useEffect,
  useState,
} from "react";
import { navigationSections, Shell } from "./components/layout/Shell";
import { AppProvider, useApp } from "./context/AppContext";
import { ThemeProvider } from "./context/ThemeContext";
import { formatError } from "./lib/errors";
import { api } from "./services/api";
import type { NavigationSection } from "./types";

// Keep the login and navigation shell small. Each section loads when opened.
const AccountsPage = lazy(() =>
  import("./pages/AccountsPage").then((m) => ({ default: m.AccountsPage })),
);
const AssistedActionsPage = lazy(() =>
  import("./pages/AssistedActionsPage").then((m) => ({
    default: m.AssistedActionsPage,
  })),
);
const AuditRunsPage = lazy(() =>
  import("./pages/AuditRunsPage").then((m) => ({ default: m.AuditRunsPage })),
);
const DashboardPage = lazy(() =>
  import("./pages/DashboardPage").then((m) => ({ default: m.DashboardPage })),
);
const DocsPage = lazy(() =>
  import("./pages/DocsPage").then((m) => ({ default: m.DocsPage })),
);
const EnvironmentsPage = lazy(() =>
  import("./pages/EnvironmentsPage").then((m) => ({
    default: m.EnvironmentsPage,
  })),
);
const FindingsPage = lazy(() =>
  import("./pages/FindingsPage").then((m) => ({ default: m.FindingsPage })),
);
const InventoryPage = lazy(() =>
  import("./pages/InventoryPage").then((m) => ({ default: m.InventoryPage })),
);
const MappingsPage = lazy(() =>
  import("./pages/MappingsPage").then((m) => ({ default: m.MappingsPage })),
);
const MonitoringPage = lazy(() =>
  import("./pages/MonitoringPage").then((m) => ({ default: m.MonitoringPage })),
);
const PerformancePage = lazy(() =>
  import("./pages/PerformancePage").then((m) => ({
    default: m.PerformancePage,
  })),
);
const ReportsPage = lazy(() =>
  import("./pages/ReportsPage").then((m) => ({ default: m.ReportsPage })),
);
const RulesPage = lazy(() =>
  import("./pages/RulesPage").then((m) => ({ default: m.RulesPage })),
);
const SchemaDriftPage = lazy(() =>
  import("./pages/SchemaDriftPage").then((m) => ({
    default: m.SchemaDriftPage,
  })),
);
const SecurityPage = lazy(() =>
  import("./pages/SecurityPage").then((m) => ({ default: m.SecurityPage })),
);
const ServerComparePage = lazy(() =>
  import("./pages/ServerComparePage").then((m) => ({
    default: m.ServerComparePage,
  })),
);
const StatusPage = lazy(() =>
  import("./pages/StatusPage").then((m) => ({ default: m.StatusPage })),
);

export { navigationSections };

function AppRoutes({ onLogout }: { onLogout: () => void }) {
  const { section, setSection } = useApp();
  const pages: Record<NavigationSection, ReactNode> = {
    Dashboard: <DashboardPage />,
    Documentação: <DocsPage onNavigate={setSection} />,
    Ambientes: <EnvironmentsPage onNavigate={setSection} />,
    Inventário: <InventoryPage />,
    Execuções: <AuditRunsPage />,
    Relatórios: <ReportsPage />,
    Mapeamentos: <MappingsPage />,
    "Desvio de schema": <SchemaDriftPage />,
    Comparar: <ServerComparePage />,
    Findings: <FindingsPage />,
    Performance: <PerformancePage />,
    Segurança: <SecurityPage />,
    Regras: <RulesPage />,
    Status: <StatusPage />,
    Contas: <AccountsPage />,
    Acompanhamento: <MonitoringPage />,
    "Ações assistidas": <AssistedActionsPage />,
  };

  return (
    <Shell activeSection={section} onNavigate={setSection} onLogout={onLogout}>
      <Suspense fallback={<p role="status">Carregando seção…</p>}>
        {pages[section] ?? <DashboardPage />}
      </Suspense>
    </Shell>
  );
}

export function App() {
  const [user, setUser] = useState<{ username: string; role: string } | null>(
    null,
  );
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [restoring, setRestoring] = useState(true);
  const [restoreError, setRestoreError] = useState("");
  const restore = useCallback(async () => {
    setRestoring(true);
    setRestoreError("");
    try {
      setUser(await api.restoreSession());
    } catch (cause) {
      setRestoreError(
        formatError(cause, "Não foi possível verificar a sessão."),
      );
    } finally {
      setRestoring(false);
    }
  }, []);
  useEffect(() => {
    void restore();
  }, [restore]);
  useEffect(() => {
    const expire = () => setUser(null);
    window.addEventListener("auditor:session-expired", expire);
    return () => window.removeEventListener("auditor:session-expired", expire);
  }, []);
  const login = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      setUser(await api.login(username, password));
      setPassword("");
    } catch (cause) {
      setError(formatError(cause, "Não foi possível entrar."));
    } finally {
      setBusy(false);
    }
  };
  if (restoring || restoreError) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-slate-950 p-6 text-slate-100">
        <div className="w-full max-w-sm space-y-4 rounded-xl border border-slate-700 bg-slate-900 p-7">
          <h1 className="text-xl font-semibold">DB Auditor</h1>
          {restoreError ? (
            <>
              <p role="alert" className="text-sm text-rose-300">
                {restoreError}
              </p>
              <button
                type="button"
                onClick={() => void restore()}
                className="rounded bg-cyan-600 px-4 py-2 font-medium"
              >
                Tentar novamente
              </button>
            </>
          ) : (
            <p className="text-sm text-slate-400">Verificando sessão…</p>
          )}
        </div>
      </main>
    );
  }
  if (!user) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-slate-950 p-6 text-slate-100">
        <form
          onSubmit={(event) => void login(event)}
          className="w-full max-w-sm space-y-4 rounded-xl border border-slate-700 bg-slate-900 p-7"
        >
          <h1 className="text-xl font-semibold">Entrar no DB Auditor</h1>
          <p className="text-sm text-slate-400">
            Sua sessão pode ser usada em outras abas deste site por até 8 horas.
            Use Sair ao terminar.
          </p>
          <label className="block text-sm">
            Usuário
            <input
              required
              autoComplete="username"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              className="mt-1 w-full rounded border border-slate-600 bg-slate-950 p-2"
            />
          </label>
          <label className="block text-sm">
            Senha
            <input
              required
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              className="mt-1 w-full rounded border border-slate-600 bg-slate-950 p-2"
            />
          </label>
          {error ? (
            <p role="alert" className="text-sm text-rose-300">
              {error}
            </p>
          ) : null}
          <button
            disabled={busy}
            className="w-full rounded bg-cyan-600 px-4 py-2 font-medium disabled:opacity-50"
            type="submit"
          >
            {busy ? "Entrando…" : "Entrar"}
          </button>
        </form>
      </main>
    );
  }
  return (
    <ThemeProvider>
      <AppProvider>
        <AppRoutes
          onLogout={() => {
            void api.logout().finally(() => setUser(null));
          }}
        />
      </AppProvider>
    </ThemeProvider>
  );
}
