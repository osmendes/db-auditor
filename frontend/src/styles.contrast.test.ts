import { describe, expect, it } from "vitest";
import { contrastRatio, cssBlockTokens } from "./lib/contrast";
import css from "./styles.css?raw";

// Theme defaults live in @theme; the dark block and html.light override them.
const base = cssBlockTokens(css, "@theme");
const dark = { ...base, ...cssBlockTokens(css, "html.dark") };
const light = { ...base, ...cssBlockTokens(css, "html.light") };

// Text tones used on the login, alerts and status messages.
const TEXT = [
  "color-slate-50",
  "color-slate-100",
  "color-slate-300",
  "color-slate-400",
  "color-rose-300",
  "color-rose-100",
  "color-amber-300",
  "color-emerald-300",
];
const SURFACES = ["color-slate-900", "color-slate-950"];

describe("design tokens contrast (WCAG AA)", () => {
  for (const [name, theme] of [
    ["escuro", dark],
    ["claro", light],
  ] as const) {
    it(`keeps text >= 4.5:1 on the ${name} surfaces`, () => {
      for (const text of TEXT) {
        for (const surface of SURFACES) {
          const ratio = contrastRatio(theme[text], theme[surface]);
          expect(
            ratio,
            `${text} (${theme[text]}) sobre ${surface} (${theme[surface]})`,
          ).toBeGreaterThanOrEqual(4.5);
        }
      }
    });
  }

  it("fixes the login error tone that measured ~2.8:1 in the light theme", () => {
    expect(contrastRatio("#ff535f", light["color-slate-900"])).toBeLessThan(
      4.5,
    );
    expect(
      contrastRatio(light["color-rose-300"], light["color-slate-900"]),
    ).toBeGreaterThanOrEqual(4.5);
  });

  it("keeps the input border visible (>= 3:1) in both themes", () => {
    for (const theme of [dark, light]) {
      expect(
        contrastRatio(theme["color-slate-500"], theme["color-slate-950"]),
      ).toBeGreaterThanOrEqual(3);
    }
  });
});
