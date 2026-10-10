import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ApiError } from "../lib/errors";
import { inventoryDetailState } from "./InventoryDetailData";
import { InventoryDetailAvailability } from "./InventoryDetailTabs";

describe("inventory detail availability", () => {
  it("keeps permission, error and empty coverage distinct", () => {
    const forbidden = renderToStaticMarkup(
      inventoryDetailState(
        new ApiError(403, "/detail", "private detail"),
        false,
        false,
      ),
    );
    const error = renderToStaticMarkup(
      inventoryDetailState(new Error("Falha temporária"), false, false),
    );
    const empty = renderToStaticMarkup(
      <InventoryDetailAvailability state="empty" />,
    );
    const notCollected = renderToStaticMarkup(
      <InventoryDetailAvailability state="not_collected" />,
    );
    const notApplicable = renderToStaticMarkup(
      <InventoryDetailAvailability state="not_applicable" />,
    );
    expect(forbidden).toContain("não tem permissão");
    expect(forbidden).not.toContain("private detail");
    expect(error).toContain("Falha temporária");
    expect(empty).toContain("Nenhuma informação");
    expect(notCollected).toContain("não é coletada");
    expect(notApplicable).toContain("não se aplica");
  });

  it("shows loading while an active page has not arrived", () => {
    expect(
      renderToStaticMarkup(inventoryDetailState(null, true, false)),
    ).toContain("Carregando");
    expect(inventoryDetailState(null, false, true)).toBeNull();
  });
});
