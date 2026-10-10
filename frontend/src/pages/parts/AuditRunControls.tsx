import type { Dispatch, SetStateAction } from "react";
import { Button, Card, Select } from "../../components/ui";
import { formatError } from "../../lib/errors";
import { api } from "../../services/api";
import type { Environment } from "../../types";
import {
  type AuditScheduleRow,
  nextRunLabel,
  SCHEDULE_PROFILES,
} from "./auditRunUi";

export function AuditRunControls({
  environments,
  triggerEnv,
  setTriggerEnv,
  busy,
  onTriggerClick,
  triggerMsg,
  schedules,
  setBusy,
  setScheduleMsg,
  setSchedules,
  scheduleMsg,
}: {
  environments: Environment[];
  triggerEnv: string;
  setTriggerEnv: (value: string) => void;
  busy: boolean;
  onTriggerClick: () => void;
  triggerMsg: string | null;
  schedules: AuditScheduleRow[];
  setBusy: Dispatch<SetStateAction<boolean>>;
  setScheduleMsg: Dispatch<SetStateAction<string | null>>;
  setSchedules: Dispatch<SetStateAction<AuditScheduleRow[]>>;
  scheduleMsg: string | null;
}) {
  return (
    <>
      {api.hasRole("operator") ? (
        <Card title="Disparo manual">
          <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end">
            <div className="min-w-0 flex-1">
              <Select
                label="Ambiente"
                options={environments.map((e) => ({
                  value: e.id,
                  label: e.name,
                }))}
                value={triggerEnv}
                onChange={(e) => setTriggerEnv(e.target.value)}
              />
            </div>
            <Button onClick={onTriggerClick} disabled={busy || !triggerEnv}>
              {busy ? "Executando…" : "Executar agora"}
            </Button>
          </div>
          {triggerMsg ? (
            <p className="mt-3 text-sm text-slate-300">{triggerMsg}</p>
          ) : null}
        </Card>
      ) : null}

      {triggerEnv ? (
        <Card title="Agenda">
          <p className="mt-1 text-sm text-slate-400">
            A agenda fica no snapshot store. Reiniciar a API não desliga o
            perfil.
          </p>
          <ul className="mt-4 space-y-3">
            {SCHEDULE_PROFILES.map((profile) => {
              const row = schedules.find(
                (item) => item.profile === profile.value,
              );
              return (
                <li
                  key={profile.value}
                  className="flex flex-wrap items-center justify-between gap-3 text-sm"
                >
                  <label className="flex items-center gap-2 text-slate-100">
                    <input
                      type="checkbox"
                      checked={Boolean(row?.enabled)}
                      disabled={!api.hasRole("operator") || busy}
                      onChange={(event) => {
                        const enabled = event.target.checked;
                        setBusy(true);
                        setScheduleMsg(null);
                        void api
                          .saveSchedule(triggerEnv, profile.value, enabled)
                          .then((saved) => {
                            setSchedules((current) => {
                              const next = current.filter(
                                (item) => item.profile !== profile.value,
                              );
                              next.push(saved);
                              return next;
                            });
                          })
                          .catch((cause: unknown) =>
                            setScheduleMsg(formatError(cause)),
                          )
                          .finally(() => setBusy(false));
                      }}
                    />
                    {profile.label}
                  </label>
                  <span className="text-slate-400">
                    Próxima: {nextRunLabel(row?.next_run_at)}
                    {row?.last_status ? ` · última ${row.last_status}` : ""}
                  </span>
                </li>
              );
            })}
          </ul>
          {scheduleMsg ? (
            <p className="mt-3 text-sm text-rose-300">{scheduleMsg}</p>
          ) : null}
        </Card>
      ) : null}
    </>
  );
}
