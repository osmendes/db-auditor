import { Badge, Button, Card, EmptyState } from "../../components/ui";
import type { ConnectionStatus, NavigationSection } from "../../types";

export function DashboardConnections({
  connections,
  setSection,
}: {
  connections: ConnectionStatus[];
  setSection: (section: NavigationSection) => void;
}) {
  return (
    <section className="mt-8" aria-labelledby="conn-heading">
      <h2 id="conn-heading" className="text-sm font-medium text-slate-300">
        Conexões
      </h2>
      {connections.length === 0 ? (
        <div className="mt-3">
          <EmptyState
            title="Nenhum ambiente configurado"
            description="Configure AUDITOR_TARGET_DSN_* no backend e reinicie a API. Depois valide em Status."
            action={
              <Button type="button" onClick={() => setSection("Status")}>
                Ir para Status
              </Button>
            }
          />
        </div>
      ) : (
        <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {connections.map((c) => {
            const ok = c.dsn_configured && c.reachable;
            const statusLabel = ok
              ? "Conectado"
              : c.dsn_configured
                ? "Indisponível"
                : "DSN ausente";
            return (
              <Card key={c.environment_id}>
                <div className="flex items-center justify-between gap-3">
                  <strong className="min-w-0 truncate text-base font-semibold text-slate-50">
                    {c.environment_name}
                  </strong>
                  <Badge tone={ok ? "success" : "danger"}>{statusLabel}</Badge>
                </div>
                {c.server_version ? (
                  <p className="mt-2 font-mono text-xs text-slate-400">
                    PG {c.server_version}
                    {c.latency_ms != null ? ` · ${c.latency_ms} ms` : ""}
                  </p>
                ) : null}
                {c.error ? (
                  <p className="mt-2 truncate text-xs text-rose-300">
                    {c.error}
                  </p>
                ) : null}
              </Card>
            );
          })}
        </div>
      )}
    </section>
  );
}
