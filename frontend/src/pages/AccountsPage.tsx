import { useEffect, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import { TotpCard } from "../components/TotpCard";
import {
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  Skeleton,
} from "../components/ui";
import { useApp } from "../context/AppContext";
import { formatError } from "../lib/errors";
import { api } from "../services/api";
import type { AuditorAccount } from "../types";

export function AccountsPage() {
  const { environments } = useApp();
  const [accounts, setAccounts] = useState<AuditorAccount[]>([]);
  const [selected, setSelected] = useState<AuditorAccount | null>(null);
  const [role, setRole] = useState<AuditorAccount["role"]>("viewer");
  const [active, setActive] = useState(true);
  const [scope, setScope] = useState<string[]>([]);
  const [resetPassword, setResetPassword] = useState("");
  const [createName, setCreateName] = useState("");
  const [createPassword, setCreatePassword] = useState("");
  const [createRole, setCreateRole] =
    useState<AuditorAccount["role"]>("viewer");
  const [createScope, setCreateScope] = useState<string[]>([]);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(api.hasRole("operator"));
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState("");

  const refresh = async () => {
    if (!api.hasRole("operator")) return;
    setLoading(true);
    try {
      setAccounts((await api.accounts()).items);
      setError(null);
    } catch (cause) {
      setError(formatError(cause, "Não foi possível listar as contas"));
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    void refresh();
  }, []);

  const choose = (item: AuditorAccount) => {
    setSelected(item);
    setRole(item.role);
    setActive(item.active);
    setScope(item.environments);
    setResetPassword("");
    setMessage("");
  };

  const changeOwnPassword = async () => {
    setBusy(true);
    try {
      await api.changePassword(currentPassword, newPassword);
      setCurrentPassword("");
      setNewPassword("");
    } catch (cause) {
      setError(formatError(cause, "Não foi possível trocar a senha"));
    } finally {
      setBusy(false);
    }
  };

  const save = async () => {
    if (!selected) return;
    setBusy(true);
    try {
      const result = await api.updateAccount(selected.id, {
        role,
        active,
        environments: scope,
        new_password: resetPassword || undefined,
      });
      setSelected(result.user);
      setResetPassword("");
      setMessage("Conta atualizada. As sessões anteriores foram revogadas.");
      await refresh();
      if (selected.username === api.currentUser()) {
        await api.logout().catch(() => undefined);
        window.dispatchEvent(new Event("auditor:session-expired"));
      }
    } catch (cause) {
      setError(formatError(cause, "Não foi possível atualizar a conta"));
    } finally {
      setBusy(false);
    }
  };

  const create = async () => {
    setBusy(true);
    try {
      await api.createAccount({
        username: createName.trim(),
        password: createPassword,
        role: createRole,
        environments: createScope,
      });
      setCreateName("");
      setCreatePassword("");
      setCreateScope([]);
      setMessage(
        "Conta criada. Entregue a senha por canal seguro e revise o acesso antes do primeiro uso.",
      );
      await refresh();
    } catch (cause) {
      setError(formatError(cause, "Não foi possível criar a conta"));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    if (!selected) return;
    setBusy(true);
    try {
      await api.revokeAccountSessions(selected.id);
      setMessage("Sessões revogadas.");
      if (selected.username === api.currentUser()) {
        await api.logout().catch(() => undefined);
        window.dispatchEvent(new Event("auditor:session-expired"));
      }
    } catch (cause) {
      setError(formatError(cause, "Falha ao revogar sessões"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <PageHeader
        eyebrow="Acesso"
        title="Contas e sessões"
        description="Gerencie seu acesso ao auditor. Alterações de senha ou permissão encerram as sessões anteriores."
      />
      <div className="mt-8 space-y-5">
        <TotpCard />
        {error ? (
          <ErrorBanner message={error} onRetry={() => void refresh()} />
        ) : null}
        {message ? (
          <p role="status" className="text-sm text-emerald-300">
            {message}
          </p>
        ) : null}
        <Card
          title="Trocar minha senha"
          subtitle="Depois da troca, entre novamente em todas as abas."
        >
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <input
              aria-label="Senha atual"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
            />
            <input
              aria-label="Nova senha, mínimo 16 caracteres"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className="rounded border border-slate-600 bg-slate-900 px-3 py-2 text-sm text-slate-100"
            />
          </div>
          <div className="mt-3">
            <Button
              disabled={busy || !currentPassword || newPassword.length < 16}
              onClick={() => void changeOwnPassword()}
            >
              Trocar senha
            </Button>
          </div>
        </Card>
        {api.hasRole("operator") ? (
          <div className="grid gap-5 lg:grid-cols-2">
            <Card
              title="Criar conta"
              subtitle="A nova conta começa com acesso aos ambientes permitidos pelo papel escolhido."
            >
              <div className="mt-3 space-y-3 text-sm text-slate-200">
                <input
                  aria-label="Nome da nova conta"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  className="w-full rounded border border-slate-600 bg-slate-900 p-2"
                />
                <input
                  aria-label="Senha da nova conta"
                  type="password"
                  autoComplete="new-password"
                  value={createPassword}
                  onChange={(e) => setCreatePassword(e.target.value)}
                  className="w-full rounded border border-slate-600 bg-slate-900 p-2"
                />
                <label className="block">
                  Papel{" "}
                  <select
                    value={createRole}
                    onChange={(e) =>
                      setCreateRole(e.target.value as AuditorAccount["role"])
                    }
                    className="rounded border border-slate-600 bg-slate-900 p-2"
                  >
                    <option value="viewer">Visualizador</option>
                    <option value="auditor">Auditor</option>
                    <option value="operator">Operador</option>
                  </select>
                </label>
                {createRole !== "operator" ? (
                  <fieldset>
                    <legend>Ambientes permitidos</legend>
                    {environments.map((env) => (
                      <label key={env.id} className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={createScope.includes(env.id)}
                          onChange={(e) =>
                            setCreateScope((prev) =>
                              e.target.checked
                                ? [...prev, env.id]
                                : prev.filter((id) => id !== env.id),
                            )
                          }
                        />
                        {env.name}
                      </label>
                    ))}
                  </fieldset>
                ) : null}
                <Button
                  disabled={
                    busy ||
                    !createName.trim() ||
                    createPassword.length < 16 ||
                    (createRole !== "operator" && createScope.length === 0)
                  }
                  onClick={() => void create()}
                >
                  Criar conta
                </Button>
              </div>
            </Card>
            <Card
              title="Contas"
              subtitle="Somente operadores podem alterar acesso."
            >
              {loading ? (
                <Skeleton className="mt-3 h-24" />
              ) : accounts.length === 0 ? (
                <EmptyState
                  title="Nenhuma conta"
                  description="Nenhuma conta disponível."
                />
              ) : (
                <ul className="mt-3 space-y-2">
                  {accounts.map((item) => (
                    <li key={item.id}>
                      <button
                        type="button"
                        onClick={() => choose(item)}
                        aria-pressed={selected?.id === item.id}
                        className="w-full rounded border border-slate-700 px-3 py-2 text-left text-sm text-slate-200 hover:bg-slate-800 focus-visible:ring-2 focus-visible:ring-emerald-400"
                      >
                        {item.username} · {item.role} ·{" "}
                        {item.active ? "ativa" : "desativada"}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
            {selected ? (
              <Card
                title={`Editar ${selected.username}`}
                subtitle="Papel, ambiente e senha administrativa. Combine a entrega da senha por canal seguro fora do auditor."
              >
                <div className="mt-3 space-y-3 text-sm text-slate-200">
                  <label className="block">
                    Papel{" "}
                    <select
                      value={role}
                      onChange={(e) =>
                        setRole(e.target.value as AuditorAccount["role"])
                      }
                      className="ml-2 rounded border border-slate-600 bg-slate-900 p-2"
                    >
                      <option value="viewer">Visualizador</option>
                      <option value="auditor">Auditor</option>
                      <option value="operator">Operador</option>
                    </select>
                  </label>
                  <label className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={active}
                      onChange={(e) => setActive(e.target.checked)}
                    />{" "}
                    Conta ativa
                  </label>
                  <fieldset className="space-y-1">
                    <legend>Ambientes permitidos (vazio: todos)</legend>
                    {environments.map((env) => (
                      <label key={env.id} className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={scope.includes(env.id)}
                          onChange={(e) =>
                            setScope((prev) =>
                              e.target.checked
                                ? [...prev, env.id]
                                : prev.filter((id) => id !== env.id),
                            )
                          }
                        />
                        {env.name}
                      </label>
                    ))}
                  </fieldset>
                  <label className="block">
                    Nova senha administrativa (opcional)
                    <input
                      type="password"
                      value={resetPassword}
                      onChange={(e) => setResetPassword(e.target.value)}
                      autoComplete="new-password"
                      className="mt-1 w-full rounded border border-slate-600 bg-slate-900 p-2"
                    />
                  </label>
                  <div className="flex gap-2">
                    <Button
                      disabled={
                        busy || (!!resetPassword && resetPassword.length < 16)
                      }
                      onClick={() => void save()}
                    >
                      Salvar acesso
                    </Button>
                    <Button
                      variant="secondary"
                      disabled={busy}
                      onClick={() => void revoke()}
                    >
                      Revogar sessões
                    </Button>
                  </div>
                </div>
              </Card>
            ) : null}
          </div>
        ) : null}
      </div>
    </>
  );
}
