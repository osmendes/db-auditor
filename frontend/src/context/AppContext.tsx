import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  type InventoryTarget,
  inventoryTargetFromParams,
  inventoryTargetParams,
  type LegacyTableTarget,
  normalizeInventoryTarget,
} from "../lib/inventoryTarget";
import { api } from "../services/api";
import type { Environment, NavigationSection } from "../types";

const SECTION_SLUGS: Record<NavigationSection, string> = {
  Dashboard: "dashboard",
  Documentação: "docs",
  Ambientes: "environments",
  Execuções: "runs",
  Relatórios: "reports",
  Inventário: "inventory",
  Mapeamentos: "mappings",
  "Desvio de schema": "schema-drift",
  Comparar: "compare",
  Findings: "findings",
  Contas: "accounts",
  Acompanhamento: "monitoring",
  "Ações assistidas": "assisted-actions",
  Status: "status",
};

const SLUG_TO_SECTION = Object.fromEntries(
  Object.entries(SECTION_SLUGS).map(([k, v]) => [v, k]),
) as Record<string, NavigationSection>;

export interface LocationState {
  section: NavigationSection;
  env: string | null;
  runId: string | null;
  findingId: string | null;
  inventory: InventoryTarget | null;
  search: Record<string, string>;
}

export function parseLocationHash(hash: string): LocationState {
  const raw = hash.replace(/^#\/?/, "");
  const [pathPart, queryPart] = raw.split("?");
  const parts = pathPart.split("/").filter(Boolean);
  const params = new URLSearchParams(queryPart || "");
  const search: Record<string, string> = {};
  params.forEach((value, key) => {
    if (
      ![
        "env",
        "kind",
        "database",
        "schema",
        "name",
        "table",
        "signature",
      ].includes(key) &&
      value
    )
      search[key] = value;
  });
  const head = parts[0] || "dashboard";
  let section: NavigationSection = "Dashboard";
  let runId: string | null = null;
  let findingId: string | null = null;
  let inventory: InventoryTarget | null = null;
  if (head === "runs" && parts[1]) {
    section = "Execuções";
    runId = decodeURIComponent(parts[1]);
  } else if (head === "findings" && parts[1]) {
    section = "Findings";
    findingId = decodeURIComponent(parts[1]);
  } else if (head === "inventory" && parts.length >= 4) {
    section = "Inventário";
    inventory = {
      kind: "tables",
      database: decodeURIComponent(parts[1]),
      schema: decodeURIComponent(parts[2]),
      name: decodeURIComponent(parts[3]),
    };
  } else if (head === "inventory") {
    section = "Inventário";
    inventory = inventoryTargetFromParams(params);
  } else if (head === "compare") {
    section = "Comparar";
  } else if (
    head === "performance" ||
    head === "security" ||
    head === "rules"
  ) {
    section = "Findings";
    if (head === "performance" && !search.category) {
      search.category = "performance";
    }
    if (head === "security" && !search.category) {
      search.category = "security";
    }
    if (head === "rules" && !search.view) {
      search.view = "rules";
    }
  } else if (head === "overview") {
    section = "Dashboard";
  } else {
    section = SLUG_TO_SECTION[head] ?? "Dashboard";
  }
  return {
    section,
    env: params.get("env"),
    runId,
    findingId,
    inventory,
    search,
  };
}

function parseLocation(): LocationState {
  return parseLocationHash(window.location.hash);
}

export function formatLocation(state: LocationState): string {
  let path = SECTION_SLUGS[state.section];
  if (state.section === "Execuções" && state.runId) {
    path = `runs/${encodeURIComponent(state.runId)}`;
  } else if (state.section === "Findings" && state.findingId) {
    path = `findings/${encodeURIComponent(state.findingId)}`;
  }
  const query = new URLSearchParams();
  if (state.env) query.set("env", state.env);
  if (state.section === "Inventário" && state.inventory) {
    inventoryTargetParams(state.inventory).forEach((value, key) => {
      query.set(key, value);
    });
  }
  for (const [key, value] of Object.entries(state.search)) {
    if (value) query.set(key, value);
  }
  const suffix = query.size ? `?${query}` : "";
  return `#/${path}${suffix}`;
}

function sameSearch(a: Record<string, string>, b: Record<string, string>) {
  const keys = new Set([...Object.keys(a), ...Object.keys(b)]);
  for (const key of keys) {
    if ((a[key] || "") !== (b[key] || "")) return false;
  }
  return true;
}

export interface AppContextValue {
  section: NavigationSection;
  setSection: (section: NavigationSection) => void;
  environments: Environment[];
  environmentsLoading: boolean;
  environmentId: string | null;
  setEnvironmentId: (id: string | null) => void;
  selectedEnvironment: Environment | null;
  refreshEnvironments: () => Promise<void>;
  runId: string | null;
  findingId: string | null;
  inventory: InventoryTarget | null;
  search: Record<string, string>;
  setSearch: (patch: Record<string, string | null>) => void;
  openRun: (id: string) => void;
  openFinding: (id: string | null) => void;
  openInventory: (
    target: InventoryTarget | LegacyTableTarget,
    run?: string,
  ) => void;
  closeInventory: () => void;
}

const AppContext = createContext<AppContextValue | null>(null);

export function AppProvider({ children }: { children: ReactNode }) {
  const initial =
    typeof window !== "undefined"
      ? parseLocation()
      : {
          section: "Dashboard" as NavigationSection,
          env: null,
          runId: null,
          findingId: null,
          inventory: null,
          search: {},
        };

  const [section, setSectionState] = useState<NavigationSection>(
    initial.section,
  );
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [environmentsLoading, setEnvironmentsLoading] = useState(true);
  const [environmentId, setEnvironmentIdState] = useState<string | null>(
    initial.env,
  );
  const [runId, setRunId] = useState<string | null>(initial.runId);
  const [findingId, setFindingId] = useState<string | null>(initial.findingId);
  const [inventory, setInventory] = useState<InventoryTarget | null>(
    initial.inventory,
  );
  const [search, setSearchState] = useState<Record<string, string>>(
    initial.search,
  );
  const fromHistory = useRef(false);
  const boot = useRef(true);
  // Last fragment this provider wrote or replayed. Back/forward between
  // fragments fires both popstate and hashchange; handling the second one
  // again would leave `fromHistory` set and swallow the next navigation.
  const lastHash = useRef<string | null>(null);
  // Canonical fragment of the current state, to ignore a pop that would not
  // change anything (e.g. same filters in another order).
  const current = useRef<string | null>(null);

  const snapshot = useCallback(
    (): LocationState => ({
      section,
      env: environmentId,
      runId,
      findingId,
      inventory,
      search,
    }),
    [section, environmentId, runId, findingId, inventory, search],
  );

  useEffect(() => {
    const next = formatLocation(snapshot());
    current.current = next;
    if (fromHistory.current) {
      fromHistory.current = false;
      return;
    }
    lastHash.current = next;
    if (window.location.hash === next) {
      boot.current = false;
      return;
    }
    if (boot.current) {
      window.history.replaceState(null, "", next);
      boot.current = false;
      return;
    }
    window.history.pushState(null, "", next);
  }, [snapshot]);

  useEffect(() => {
    const onPop = () => {
      if (window.location.hash === lastHash.current) return;
      lastHash.current = window.location.hash;
      const parsed = parseLocation();
      if (formatLocation(parsed) === current.current) return;
      fromHistory.current = true;
      setSectionState(parsed.section);
      setEnvironmentIdState(parsed.env);
      setRunId(parsed.runId);
      setFindingId(parsed.findingId);
      setInventory(parsed.inventory);
      setSearchState(parsed.search);
    };
    window.addEventListener("hashchange", onPop);
    window.addEventListener("popstate", onPop);
    return () => {
      window.removeEventListener("hashchange", onPop);
      window.removeEventListener("popstate", onPop);
    };
  }, []);

  const setSection = useCallback((next: NavigationSection) => {
    setSectionState(next);
    setRunId(null);
    setFindingId(null);
    setInventory(null);
    setSearchState({});
  }, []);

  const setEnvironmentId = useCallback((id: string | null) => {
    setEnvironmentIdState(id);
    setInventory(null);
    setSearchState((prev) => {
      if (!prev.run) return prev;
      const { run: _run, ...next } = prev;
      return next;
    });
  }, []);

  const setSearch = useCallback((patch: Record<string, string | null>) => {
    setSearchState((prev) => {
      const next = { ...prev };
      for (const [key, value] of Object.entries(patch)) {
        if (!value) delete next[key];
        else next[key] = value;
      }
      return sameSearch(prev, next) ? prev : next;
    });
  }, []);

  const openRun = useCallback((id: string) => {
    setSectionState("Execuções");
    setRunId(id);
    setFindingId(null);
    setInventory(null);
  }, []);

  const openFinding = useCallback((id: string | null) => {
    setSectionState("Findings");
    setFindingId(id);
    setRunId(null);
    setInventory(null);
  }, []);

  const openInventory = useCallback(
    (target: InventoryTarget | LegacyTableTarget, run?: string) => {
      setSectionState("Inventário");
      setInventory(normalizeInventoryTarget(target));
      setRunId(null);
      setFindingId(null);
      setSearchState((prev) => {
        if (run) return prev.run === run ? prev : { ...prev, run };
        if (!prev.run) return prev;
        const { run: _run, ...next } = prev;
        return next;
      });
    },
    [],
  );

  const closeInventory = useCallback(() => {
    setInventory(null);
    setSearchState((prev) => {
      if (!prev.run) return prev;
      const { run: _run, ...next } = prev;
      return next;
    });
  }, []);

  const refreshEnvironments = useCallback(async () => {
    setEnvironmentsLoading(true);
    try {
      const res = await api.environments();
      setEnvironments(res.items);
      setEnvironmentIdState((prev) => {
        if (prev == null) {
          return api.hasRole("operator") ? null : (res.items[0]?.id ?? null);
        }
        if (res.items.some((e) => e.id === prev)) {
          return prev;
        }
        return null;
      });
    } catch {
      setEnvironments([]);
    } finally {
      setEnvironmentsLoading(false);
    }
  }, []);

  useEffect(() => {
    void refreshEnvironments();
  }, [refreshEnvironments]);

  const selectedEnvironment = useMemo(
    () => environments.find((e) => e.id === environmentId) ?? null,
    [environments, environmentId],
  );

  const value = useMemo(
    () => ({
      section,
      setSection,
      environments,
      environmentsLoading,
      environmentId,
      setEnvironmentId,
      selectedEnvironment,
      refreshEnvironments,
      runId,
      findingId,
      inventory,
      search,
      setSearch,
      openRun,
      openFinding,
      openInventory,
      closeInventory,
    }),
    [
      section,
      setSection,
      environments,
      environmentsLoading,
      environmentId,
      setEnvironmentId,
      selectedEnvironment,
      refreshEnvironments,
      runId,
      findingId,
      inventory,
      search,
      setSearch,
      openRun,
      openFinding,
      openInventory,
      closeInventory,
    ],
  );

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>;
}

export function useApp(): AppContextValue {
  const ctx = useContext(AppContext);
  if (!ctx) {
    throw new Error("useApp must be used within AppProvider");
  }
  return ctx;
}
