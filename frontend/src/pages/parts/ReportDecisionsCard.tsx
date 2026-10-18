import { Button, Card, Select } from "../../components/ui";
import { type ReportsPageState, ReportTextField } from "./useReportsPage";

/** Part of ReportsPage; same markup, moved out (UI-06). */
export function ReportDecisionsCard({ s }: { s: ReportsPageState }) {
  const {
    busy,
    database,
    decisionObject,
    decisions,
    decisionSubject,
    decisionValue,
    loadDecisions,
    saveDecision,
    setDecisionObject,
    setDecisionSubject,
    setDecisionValue,
  } = s;
  return (
    <Card title="Decisões de domínio" contentClassName="space-y-3">
      <p className="text-sm text-slate-400">
        Registre uma interpretação confirmada com autoria e histórico. O pedido
        congela a versão atual; nenhuma decisão altera o ERD observado.
      </p>
      {decisionSubject === "insertion" ? (
        <p className="text-xs text-slate-400">
          Contrato de inserção: informe JSON com column, timezone, validated,
          filled_on_insert e append_only. Exemplo:{" "}
          <code>
            {
              '{"column":"inserted_at","timezone":"UTC","validated":true,"filled_on_insert":true,"append_only":false}'
            }
          </code>
          . Com exclusões possíveis, a data só representa os registros
          presentes.
        </p>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-2">
        <ReportTextField
          label="Objeto (database.schema.tabela)"
          value={decisionObject}
          onChange={(event) => setDecisionObject(event.target.value)}
        />
        <Select
          label="Assunto"
          value={decisionSubject}
          onChange={(event) => setDecisionSubject(event.target.value)}
          options={[
            { value: "semantics", label: "Semântica" },
            { value: "insertion", label: "Contrato de inserção" },
            { value: "unit", label: "Unidade" },
            { value: "retention", label: "Retenção" },
            { value: "consumer", label: "Consumidor" },
          ]}
        />
        <ReportTextField
          label="Decisão confirmada"
          value={decisionValue}
          onChange={(event) => setDecisionValue(event.target.value)}
        />
      </div>
      <div className="flex gap-3">
        <Button
          disabled={busy || !database || !decisionObject || !decisionValue}
          onClick={() => void saveDecision()}
        >
          Registrar decisão
        </Button>
        <Button
          variant="secondary"
          disabled={!database}
          onClick={() => void loadDecisions()}
        >
          Ver decisões
        </Button>
      </div>
      <ul className="text-sm text-slate-300">
        {decisions.map((item) => (
          <li key={`${item.object_ref}/${item.subject}/${item.version}`}>
            {item.object_ref} · {item.subject} v{item.version}: {item.value} ·{" "}
            {item.author} · {item.created_at}
          </li>
        ))}
      </ul>
    </Card>
  );
}
