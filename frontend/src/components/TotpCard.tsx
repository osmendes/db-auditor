import { useState } from "react";
import { formatError } from "../lib/errors";
import { api } from "../services/api";
import { Button, Card } from "./ui";

export function TotpCard() {
  const [secret, setSecret] = useState("");
  const [uri, setUri] = useState("");
  const [codes, setCodes] = useState<string[]>([]);
  const [confirm, setConfirm] = useState("");
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const enroll = async () => {
    setError("");
    try {
      const result = await api.enrollTotp();
      setSecret(result.secret);
      setUri(result.otpauth_uri);
      setCodes(result.recovery_codes);
      setMessage(
        "Guarde os códigos de recuperação. Eles não serão mostrados de novo.",
      );
    } catch (cause) {
      setError(formatError(cause, "Não foi possível iniciar o TOTP."));
    }
  };

  return (
    <Card className="space-y-3 p-4">
      <h2 className="text-sm font-semibold">Autenticação em dois fatores</h2>
      <p className="text-sm text-slate-400">
        TOTP opcional para viewer e auditor. O operador pode exigir o código na
        própria conta depois de confirmá-lo. Não há SMS.
      </p>
      <Button type="button" onClick={() => void enroll()}>
        Gerar segredo
      </Button>
      {secret ? (
        <>
          <p className="break-all text-xs">{secret}</p>
          <p className="break-all text-xs">{uri}</p>
          <ul className="text-xs">
            {codes.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
          <input
            className="w-full rounded border border-slate-600 bg-transparent px-2 py-1"
            value={confirm}
            onChange={(event) => setConfirm(event.target.value)}
            placeholder="Código do autenticador"
          />
          <Button
            type="button"
            onClick={() =>
              void api
                .confirmTotpEnrollment(confirm)
                .then(() => setMessage("TOTP ativo."))
                .catch((cause) =>
                  setError(formatError(cause, "Código inválido.")),
                )
            }
          >
            Confirmar
          </Button>
        </>
      ) : null}
      {api.hasRole("operator") ? (
        <Button
          type="button"
          onClick={() =>
            void api
              .requireTotp(true)
              .then(() => setMessage("TOTP exigido nesta conta."))
          }
        >
          Exigir TOTP nesta conta
        </Button>
      ) : null}
      <input
        className="w-full rounded border border-slate-600 bg-transparent px-2 py-1"
        type="password"
        value={password}
        onChange={(event) => setPassword(event.target.value)}
        placeholder="Senha atual para desativar"
      />
      <Button
        type="button"
        onClick={() =>
          void api
            .disableTotp(password, confirm)
            .then(() => setMessage("TOTP desativado."))
            .catch((cause) =>
              setError(formatError(cause, "Não foi possível desativar.")),
            )
        }
      >
        Desativar
      </Button>
      {message ? <p className="text-sm text-emerald-300">{message}</p> : null}
      {error ? <p className="text-sm text-rose-300">{error}</p> : null}
    </Card>
  );
}
