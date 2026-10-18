import { Button, Card, Select } from "../../components/ui";
import { api } from "../../services/api";
import type { ReportJob } from "../../types";
import {
  type ReportsPageState,
  ReportTextField,
  reportFormReady,
  runStatusLabels,
  severityLabels,
} from "./useReportsPage";

/** Part of ReportsPage; same markup, moved out (UI-06). */
export function ReportRequestCard({ s }: { s: ReportsPageState }) {
  const {
    busy,
    database,
    databaseOptions,
    env,
    environments,
    kind,
    redaction,
    refresh,
    request,
    runId,
    runs,
    schema,
    setDatabase,
    setEnv,
    setKind,
    setRedaction,
    setRunId,
    setSchema,
    setSeverity,
    setTable,
    setToken,
    severity,
    table,
    token,
  } = s;
  return (
    <Card title="Solicitar relatório" contentClassName="flex flex-col gap-3">
      <p className="-mt-1.5 text-sm text-slate-400">
        O processamento é assíncrono e o arquivo expira após 30 dias.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Select
          label="Ambiente"
          value={env}
          onChange={(event) => setEnv(event.target.value)}
          options={[
            { value: "", label: "Selecione" },
            ...environments.map((item) => ({
              value: item.id,
              label: item.name,
            })),
          ]}
        />
        <Select
          label="Execução"
          value={runId}
          onChange={(event) => setRunId(event.target.value)}
          options={[
            { value: "", label: "Selecione" },
            ...runs.map((run) => ({
              value: run.id,
              label: `${run.id.slice(0, 8)}… · ${runStatusLabels[run.status] ?? run.status}`,
            })),
          ]}
        />
        <Select
          label="Tipo"
          value={kind}
          onChange={(event) => {
            const next = event.target.value as ReportJob["report_type"];
            setKind(next);
            if (next === "architecture") {
              setTable("");
              setSeverity("");
            }
          }}
          options={[
            { value: "executive", label: "Executivo" },
            { value: "technical", label: "Técnico" },
            { value: "table", label: "Tabela" },
            ...(import.meta.env.VITE_ARCHITECTURE_REPORT_ENABLED === "true"
              ? [
                  {
                    value: "architecture",
                    label: "Análise completa e modelagem",
                  },
                ]
              : []),
          ]}
        />
        <Select
          label="Severidade"
          value={severity}
          onChange={(event) => setSeverity(event.target.value)}
          options={[
            { value: "", label: "Todas" },
            ...["critical", "high", "medium", "low", "info"].map((value) => ({
              value,
              label: severityLabels[value],
            })),
          ]}
        />
        <Select
          label="Redação de dados sensíveis"
          value={redaction}
          onChange={(event) =>
            setRedaction(
              event.target.value as "none" | "identifiers" | "strict",
            )
          }
          options={[
            { value: "none", label: "Sem redação" },
            {
              value: "identifiers",
              label: "Ocultar identificadores e textos livres",
            },
            {
              value: "strict",
              label: "Rigorosa: ocultar também notas de cobertura",
            },
          ]}
        />
        {kind === "architecture" ? (
          <Select
            label="Banco da execução"
            value={database}
            onChange={(event) => setDatabase(event.target.value)}
            options={[
              { value: "", label: "Selecione" },
              ...databaseOptions.map((name) => ({
                value: name,
                label: name,
              })),
            ]}
          />
        ) : (
          <ReportTextField
            label="Banco (opcional)"
            aria-label="Banco"
            value={database}
            onChange={(event) => setDatabase(event.target.value)}
          />
        )}
        <ReportTextField
          label="Esquema (opcional)"
          aria-label="Esquema"
          value={schema}
          onChange={(event) => setSchema(event.target.value)}
        />
        <ReportTextField
          label="Tabela"
          aria-label="Tabela"
          placeholder="Obrigatória no relatório de tabela"
          value={table}
          onChange={(event) => setTable(event.target.value)}
        />
        {!api.hasSession() && (
          <ReportTextField
            label="Token de relatórios"
            aria-label="Token de relatórios"
            type="password"
            autoComplete="off"
            value={token}
            onChange={(event) => setToken(event.target.value)}
          />
        )}
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Button
          className="h-10 w-full"
          disabled={
            busy ||
            !reportFormReady(
              env,
              runId,
              token || (api.hasSession() ? "session" : ""),
              kind,
              {
                database,
                schema,
                table,
                severity,
              },
            )
          }
          onClick={() => void request()}
        >
          {kind === "architecture" ? "Gerar Markdown" : "Gerar PDF"}
        </Button>
        <Button
          className="h-10 w-full"
          variant="secondary"
          disabled={busy || !env || (!token && !api.hasSession())}
          onClick={() => void refresh()}
        >
          Atualizar histórico
        </Button>
      </div>
    </Card>
  );
}
