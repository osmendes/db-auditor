import {
  type FormEvent,
  lazy,
  type ReactNode,
  Suspense,
  useCallback,
  useEffect,
  useState,
} from "react";
import { LoginScreen, SessionRestoreScreen } from "./components/LoginScreen";
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
const ReportsPage = lazy(() =>
  import("./pages/ReportsPage").then((m) => ({ default: m.ReportsPage })),
);
const SchemaDriftPage = lazy(() =>
  import("./pages/SchemaDriftPage").then((m) => ({
    default: m.SchemaDriftPage,
  })),
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

function SessionApp() {
  const [user, setUser] = useState<{ username: string; role: string } | null>(
    null,
  );
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [mfaToken, setMfaToken] = useState("");
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
      const result = await api.login(username, password);
      if ("mfa_required" in result) {
        setMfaToken(result.mfa_token);
        setPassword("");
        return;
      }
      setUser(result);
      setPassword("");
    } catch (cause) {
      setError(formatError(cause, "Não foi possível entrar."));
    } finally {
      setBusy(false);
    }
  };
  const confirmCode = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      setUser(await api.confirmTotp(mfaToken, code));
      setMfaToken("");
      setCode("");
    } catch (cause) {
      setError(formatError(cause, "Código inválido."));
    } finally {
      setBusy(false);
    }
  };
  if (restoring || restoreError) {
    return (
      <SessionRestoreScreen
        error={restoreError}
        onRetry={() => void restore()}
      />
    );
  }
  if (!user) {
    return (
      <LoginScreen
        username={username}
        password={password}
        code={code}
        mfa={mfaToken !== ""}
        error={error}
        busy={busy}
        onUsername={setUsername}
        onPassword={setPassword}
        onCode={setCode}
        onSubmit={(event) =>
          void (mfaToken ? confirmCode(event) : login(event))
        }
      />
    );
  }
  return (
    <AppProvider>
      <AppRoutes
        onLogout={() => {
          void api.logout().finally(() => setUser(null));
        }}
      />
    </AppProvider>
  );
}

export function App() {
  return (
    <ThemeProvider>
      <SessionApp />
    </ThemeProvider>
  );
}
