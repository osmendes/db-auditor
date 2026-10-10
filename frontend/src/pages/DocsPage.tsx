import { useEffect, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import type { NavigationSection } from "../types";

export interface DocsPageProps {
  onNavigate?: (section: NavigationSection) => void;
}

export function DocsPage(_props: DocsPageProps) {
  const [body, setBody] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let active = true;
    void fetch("/guia-do-usuario.md")
      .then((response) => {
        if (!response.ok) throw new Error(String(response.status));
        return response.text();
      })
      .then((text) => {
        if (active) setBody(text);
      })
      .catch(() => {
        if (active) setFailed(true);
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="space-y-4">
      <PageHeader
        eyebrow="Ajuda"
        title="Documentação"
        description="O guia do usuário é carregado à parte, fora do pacote da aplicação."
      />
      {failed ? (
        <p role="alert" className="text-sm text-rose-300">
          Não foi possível carregar o guia.
        </p>
      ) : null}
      {body ? (
        <article className="max-w-3xl whitespace-pre-wrap text-sm leading-relaxed text-slate-300">
          {body}
        </article>
      ) : failed ? null : (
        <p role="status" className="text-sm text-slate-400">
          Carregando o guia…
        </p>
      )}
    </div>
  );
}
