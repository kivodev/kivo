import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/postcss";
import postcss, { type AtRule, type ChildNode } from "postcss";
import { describe, expect, it } from "vitest";
import { generatedCssPath, readTokens, renderTokensCss } from "../../scripts/tokens.ts";

const entry = fileURLToPath(new URL("./index.css", import.meta.url));
const layerOrder = ["kivo.theme", "kivo.base", "kivo.components", "kivo.utilities"];

async function compiledCss() {
  const result = await postcss([tailwindcss()]).process(readFileSync(entry, "utf8"), { from: entry });
  return result.root;
}

describe("tokens", () => {
  it("tokens.css is generated from tokens.json (run npm run tokens)", () => {
    expect(readFileSync(generatedCssPath(), "utf8")).toBe(renderTokensCss(readTokens()));
  });

  // TV body text is 28 px on a 1080p canvas, and nothing is below 24 px.
  it("gives every text step a TV size of at least 24 px under data-tv", () => {
    const tokens = readTokens();
    for (const step of Object.keys(tokens.text)) {
      const tv = tokens.tv.text[tokens.tv.web.text[step] ?? ""];
      expect(tv, step).toBeDefined();
      expect(Number.parseFloat(tv?.size ?? "0"), step).toBeGreaterThanOrEqual(24);
    }
    expect(renderTokensCss(tokens)).toContain(":root[data-tv] {");
  });

  it("the dark and light themes define the same tokens, so switching sets restyles everything", () => {
    const { dark, light } = readTokens().themes;
    expect(Object.keys(light.color).sort()).toEqual(Object.keys(dark.color).sort());
    expect(Object.keys(light.shadow).sort()).toEqual(Object.keys(dark.shadow).sort());
  });
});

describe("cascade layers", () => {
  it("declares Kivo's layer order before any rule", () => {
    const source = readFileSync(entry, "utf8");
    const statement = source
      .match(/^@layer ([^;{]+);/m)?.[1]
      ?.split(",")
      .map((s) => s.trim());
    expect(statement).toEqual(layerOrder);
  });

  // Unlayered CSS beats every layer whatever its specificity (CSS Cascade
  // 5). With every Kivo rule layered, a plain stylesheet overrides any
  // component without !important.
  it("puts every style rule in a cascade layer", async () => {
    const root = await compiledCss();
    const allowedTopLevel = new Set(["layer", "property", "keyframes", "charset", "import"]);
    const unlayered = root.nodes.filter(
      (node: ChildNode) =>
        node.type === "rule" || (node.type === "atrule" && !allowedTopLevel.has((node as AtRule).name)),
    );
    expect(unlayered.map((n) => n.toString().slice(0, 80))).toEqual([]);
  });

  it("puts Kivo's rules in Kivo's layers", async () => {
    const root = await compiledCss();
    const layers = new Set<string>();
    root.walkAtRules("layer", (rule) => {
      if (rule.nodes) layers.add(rule.params);
    });
    // Tailwind's own `properties` layer only sets fallbacks for its internal variables.
    expect([...layers].filter((l) => l !== "properties").sort()).toEqual(
      layerOrder.filter((l) => layers.has(l)).sort(),
    );
    expect(layers.has("kivo.utilities")).toBe(true);
  });
});

describe("TV build", () => {
  // A focused control or row turns white and takes the light set's colours
  // (index.css), so every light colour has a lit twin under data-tv.
  it("gives the TV every light colour as a lit colour, for what focus lights up", () => {
    const tokens = readTokens();
    const tv = renderTokensCss(tokens).split(":root[data-tv] {")[1] ?? "";
    for (const [name, value] of Object.entries(tokens.themes.light.color)) {
      expect(tv, name).toContain(`--color-tv-lit-${name}: ${value};`);
    }
  });

  // A remote moves focus, so nothing on TV shows a hover state.
  it("applies hover styles only outside the TV build", async () => {
    const root = await compiledCss();
    const hovers: string[] = [];
    root.walkRules((rule) => {
      if (rule.selector.includes(":hover")) hovers.push(rule.selector);
    });
    expect(hovers.length).toBeGreaterThan(0);
    expect(hovers.filter((s) => !s.includes(":root:not([data-tv])"))).toEqual([]);
  });
});
