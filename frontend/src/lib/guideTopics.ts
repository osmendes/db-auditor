/**
 * Parses the "Ajuda por tela" part of docs/guia-do-usuario.md (UI-10).
 * The Docs page fetches that file at runtime instead of bundling a copy, so
 * the screen and the guide cannot drift apart.
 */
import type { NavigationSection } from "../types";

export interface DocTerm {
  name: string;
  meaning: string;
}

export interface DocTopic {
  id: string;
  title: string;
  intro: string;
  steps: string[];
  notes: string[];
  terms: DocTerm[];
  goTo?: NavigationSection;
}

export const GUIDE_PATH = "guia-do-usuario.md";
const START = "<!-- ajuda-por-tela:inicio -->";
const END = "<!-- ajuda-por-tela:fim -->";

type Block = "intro" | "steps" | "notes" | "terms";
const HEADINGS: Record<string, Block> = {
  "**Como usar**": "steps",
  "**Vale saber**": "notes",
  "**Termos desta tela**": "terms",
};

export function parseGuideTopics(
  markdown: string,
  sections: readonly NavigationSection[] = [],
): DocTopic[] {
  const start = markdown.indexOf(START);
  const end = markdown.indexOf(END);
  if (start < 0 || end < start) return [];
  const lines = markdown.slice(start + START.length, end).split("\n");
  const topics: DocTopic[] = [];
  let topic: DocTopic | null = null;
  let block: Block = "intro";
  const intro: string[] = [];
  const flush = () => {
    if (topic) {
      topic.intro = intro.join(" ").trim();
      topics.push(topic);
    }
    intro.length = 0;
  };
  for (const raw of lines) {
    const line = raw.trim();
    if (line.startsWith("### ")) {
      flush();
      topic = {
        id: "",
        title: line.slice(4).trim(),
        intro: "",
        steps: [],
        notes: [],
        terms: [],
      };
      block = "intro";
      continue;
    }
    if (!topic || line === "") continue;
    const meta = line.match(
      /^<!--\s*topic:\s*([\w-]+)(?:\s*\|\s*abrir:\s*(.+?))?\s*-->$/,
    );
    if (meta) {
      topic.id = meta[1];
      const target = meta[2]?.trim() as NavigationSection | undefined;
      if (target && (sections.length === 0 || sections.includes(target))) {
        topic.goTo = target;
      }
      continue;
    }
    if (HEADINGS[line]) {
      block = HEADINGS[line];
      continue;
    }
    if (block === "intro") {
      intro.push(line);
    } else if (block === "steps") {
      const step = line.match(/^\d+\.\s+(.*)$/);
      if (step) topic.steps.push(step[1]);
    } else if (block === "notes") {
      if (line.startsWith("- ")) topic.notes.push(line.slice(2));
    } else {
      const term = line.match(/^- \*\*(.+?)\*\*\s+—\s+(.*)$/);
      if (term) topic.terms.push({ name: term[1], meaning: term[2] });
    }
  }
  flush();
  return topics.filter((item) => item.id !== "");
}
