import { useEffect, useState } from "react";
import { navigationSections } from "../components/layout/Shell";
import { PageHeader } from "../components/PageHeader";
import { Accordion, Button, ErrorBanner, Skeleton } from "../components/ui";
import {
  type DocTopic,
  GUIDE_PATH,
  parseGuideTopics,
} from "../lib/guideTopics";
import type { NavigationSection } from "../types";

export interface DocsPageProps {
  onNavigate?: (section: NavigationSection) => void;
  /** Topics already parsed (tests); otherwise fetched from the guide. */
  initialTopics?: DocTopic[];
}

function guideURL() {
  return `${import.meta.env.BASE_URL ?? "/"}${GUIDE_PATH}`;
}

function OpenTopic({
  topic,
  onNavigate,
}: {
  topic: NavigationSection;
  onNavigate: (section: NavigationSection) => void;
}) {
  return (
    <div className="pt-1">
      <Button type="button" onClick={() => onNavigate(topic)}>
        Abrir {topic}
      </Button>
    </div>
  );
}

function TopicBody({
  topic,
  onNavigate,
}: {
  topic: DocTopic;
  onNavigate?: (section: NavigationSection) => void;
}) {
  return (
    <div className="space-y-8">
      <p>{topic.intro}</p>
      <div>
        <h4 className="text-xs font-medium tracking-wide text-slate-400">
          Como usar
        </h4>
        <ol className="mt-3 list-decimal space-y-3 pl-5">
          {topic.steps.map((step) => (
            <li key={step} className="pl-1">
              {step}
            </li>
          ))}
        </ol>
      </div>
      {topic.notes.length > 0 ? (
        <div>
          <h4 className="text-xs font-medium tracking-wide text-slate-400">
            Vale saber
          </h4>
          <ul className="mt-3 list-disc space-y-3 pl-5">
            {topic.notes.map((note) => (
              <li key={note} className="pl-1">
                {note}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {topic.terms.length > 0 ? (
        <div>
          <h4 className="text-xs font-medium tracking-wide text-slate-400">
            Termos desta tela
          </h4>
          <ul className="mt-3 space-y-3">
            {topic.terms.map((term) => (
              <li key={term.name}>
                <span className="font-medium text-slate-100">{term.name}</span>
                {" — "}
                {term.meaning}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {topic.goTo && onNavigate ? (
        <OpenTopic topic={topic.goTo} onNavigate={onNavigate} />
      ) : null}
    </div>
  );
}

/**
 * The topics live in docs/guia-do-usuario.md (section "Ajuda por tela"),
 * served as a static file and fetched when the page opens, so the JS chunk
 * carries no copy of the guide (UI-10).
 */
export function DocsPage({ onNavigate, initialTopics }: DocsPageProps) {
  const [topics, setTopics] = useState<DocTopic[] | null>(
    initialTopics ?? null,
  );
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (initialTopics) return;
    const controller = new AbortController();
    setError(null);
    fetch(guideURL(), { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const parsed = parseGuideTopics(
          await response.text(),
          navigationSections,
        );
        if (parsed.length === 0) throw new Error("guia sem tópicos");
        setTopics(parsed);
      })
      .catch((cause: unknown) => {
        if (controller.signal.aborted) return;
        setError(
          `Não foi possível abrir os tópicos de ajuda (${cause instanceof Error ? cause.message : "erro"}).`,
        );
      });
    return () => controller.abort();
  }, [initialTopics, reloadKey]);

  return (
    <div className="space-y-4">
      <PageHeader
        eyebrow="Ajuda"
        title="Documentação"
        description="Escolha um tópico. O texto abre logo abaixo do título, com o passo a passo e o significado dos termos. Clique de novo para fechar. O DB Auditor só observa: ele não altera o banco analisado."
      />
      {error ? (
        <ErrorBanner
          message={error}
          onRetry={() => setReloadKey((key) => key + 1)}
        >
          <p className="mt-2">
            <a className="underline" href={guideURL()}>
              Abrir o guia do usuário em texto
            </a>
          </p>
        </ErrorBanner>
      ) : null}
      {!topics && !error ? (
        <div role="status" aria-label="Carregando tópicos de ajuda">
          <Skeleton className="h-64 w-full" />
        </div>
      ) : null}
      {topics ? (
        <Accordion
          items={topics.map((topic) => ({
            id: topic.id,
            title: topic.title,
            content: <TopicBody topic={topic} onNavigate={onNavigate} />,
          }))}
          defaultOpenId="comecar"
        />
      ) : null}
    </div>
  );
}
