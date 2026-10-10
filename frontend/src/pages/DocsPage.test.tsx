import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DocsPage } from "./DocsPage";

describe("DocsPage", () => {
  it("does not embed the guide in the component markup", () => {
    const html = renderToStaticMarkup(<DocsPage />);
    expect(html).toContain("Documentação");
    expect(html).toContain("Carregando o guia");
    expect(html).not.toContain("Configuração inicial (ops)");
  });
});
