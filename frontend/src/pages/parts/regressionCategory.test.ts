import { describe, expect, it } from "vitest";
import { regressionCategoryLabel } from "./regressionCategory";

describe("regressionCategoryLabel", () => {
  it("names every regression category, including workload (#104)", () => {
    expect(regressionCategoryLabel("findings")).toBe(
      "Achados altos ou críticos",
    );
    expect(regressionCategoryLabel("structure")).toBe(
      "Tabelas ou índices removidos ou alterados",
    );
    expect(regressionCategoryLabel("workload")).toBe(
      "Consultas lentas ou pesadas novas",
    );
  });
  it("shows an unknown category as is instead of mislabeling it", () => {
    expect(regressionCategoryLabel("future")).toBe("future");
  });
});
