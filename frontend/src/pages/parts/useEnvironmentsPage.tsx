import { useCallback, useEffect, useMemo, useState } from "react";
import { connectionPresentation } from "../../lib/connectionStatus";
import { formatError } from "../../lib/errors";
import { matchesSearch } from "../../lib/format";
import { labels } from "../../lib/labels";
import { type SortState, sortBy } from "../../lib/sort";
import { api } from "../../services/api";
import type {
  ConnectionStatus,
  DatabaseSnapshot,
  Environment,
  NavigationSection,
  SchemaSnapshot,
} from "../../types";

/** State and effects of EnvironmentsPage (UI-06 extraction). */
export interface EnvironmentsPageProps {
  onNavigate?: (section: NavigationSection) => void;
}

export function useEnvironmentsPage({ onNavigate }: EnvironmentsPageProps) {
  const [items, setItems] = useState<Environment[] | null>(null);
  const [connections, setConnections] = useState<ConnectionStatus[] | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [databases, setDatabases] = useState<DatabaseSnapshot[] | null>(null);
  const [schemas, setSchemas] = useState<SchemaSnapshot[] | null>(null);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
  const [reloadKey, setReloadKey] = useState(0);
  const [envSort, setEnvSort] = useState<SortState | null>(null);
  const [dbSort, setDbSort] = useState<SortState | null>(null);
  const [schemaSort, setSchemaSort] = useState<SortState | null>(null);
  const [labelName, setLabelName] = useState("");
  const [labelError, setLabelError] = useState<string | null>(null);

  const loadEnvironments = useCallback(() => {
    setError(null);
    setItems(null);
    setConnections(null);
    api
      .environments()
      .then((res) => {
        setItems(res.items);
        setSelectedId((prev) => {
          if (prev && res.items.some((item) => item.id === prev)) {
            return prev;
          }
          const first = [...res.items].sort((a, b) =>
            a.id.localeCompare(b.id, "en", { numeric: true }),
          )[0];
          return first?.id ?? null;
        });
      })
      .catch((err: unknown) => {
        setError(formatError(err, "Falha ao carregar ambientes"));
        setItems([]);
      });
    api
      .connectionStatus()
      .then((res) => setConnections(res.items))
      .catch(() => setConnections([]));
  }, []);

  useEffect(() => {
    loadEnvironments();
  }, [loadEnvironments, reloadKey]);

  useEffect(() => {
    if (!selectedId) {
      setDatabases(null);
      setSchemas(null);
      return;
    }
    let cancelled = false;
    const controller = new AbortController();
    setDatabases(null);
    setSchemas(null);
    setDetailError(null);
    Promise.all([
      api.databases(selectedId, undefined, controller.signal),
      api.schemas(selectedId, controller.signal),
    ])
      .then(([dbRes, schemaRes]) => {
        if (!cancelled) {
          setDatabases(dbRes.items);
          setSchemas(schemaRes.items);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setDetailError(formatError(err, "Falha ao carregar topologia"));
          setDatabases([]);
          setSchemas([]);
        }
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [selectedId]);

  const connectionById = useMemo(() => {
    const map = new Map<string, ConnectionStatus>();
    for (const item of connections ?? []) {
      map.set(item.environment_id, item);
    }
    return map;
  }, [connections]);

  const sortedEnvironments = useMemo(() => {
    if (!items) {
      return null;
    }
    const byId = [...items].sort((a, b) =>
      a.id.localeCompare(b.id, "en", { numeric: true }),
    );
    return sortBy(byId, envSort, {
      name: (e) => e.name,
      type: (e) => labels.envType(e.type),
      discovery: (e) => labels.discoveryMode(e.discovery_mode),
      status: (e) =>
        connectionPresentation(connectionById.get(e.id) ?? null).label,
    });
  }, [items, envSort, connectionById]);

  const filteredDatabases = useMemo(() => {
    if (!databases) {
      return null;
    }
    const filtered = databases.filter((db) =>
      matchesSearch(db.database_name, filter),
    );
    return sortBy(filtered, dbSort, {
      name: (db) => db.database_name,
      size: (db) => db.size_bytes,
      connections: (db) => db.connection_count,
    });
  }, [databases, filter, dbSort]);

  const filteredSchemas = useMemo(() => {
    if (!schemas) {
      return null;
    }
    const filtered = schemas.filter(
      (sc) =>
        matchesSearch(sc.schema_name, filter) ||
        matchesSearch(sc.database_name, filter),
    );
    return sortBy(filtered, schemaSort, {
      database: (sc) => sc.database_name,
      schema: (sc) => sc.schema_name,
      tables: (sc) => sc.table_count,
      views: (sc) => sc.view_count + sc.materialized_view_count,
      size: (sc) => sc.size_bytes,
    });
  }, [schemas, filter, schemaSort]);

  return {
    connectionById,
    connections,
    databases,
    dbSort,
    detailError,
    envSort,
    error,
    filter,
    filteredDatabases,
    filteredSchemas,
    items,
    labelError,
    labelName,
    loadEnvironments,
    onNavigate,
    reloadKey,
    schemas,
    schemaSort,
    selectedId,
    setConnections,
    setDatabases,
    setDbSort,
    setDetailError,
    setEnvSort,
    setError,
    setFilter,
    setItems,
    setLabelError,
    setLabelName,
    setReloadKey,
    setSchemas,
    setSchemaSort,
    setSelectedId,
    sortedEnvironments,
  };
}

export type EnvironmentsPageState = ReturnType<typeof useEnvironmentsPage>;
