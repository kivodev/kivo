// Compiles shared/tokens.json into src/theme/tokens.css: Tailwind v4
// `@theme` variables for the default (dark) token set, the light set as
// overrides under [data-theme="light"], and the TV build's type scale,
// motion and focus as overrides under [data-tv]. Run with
// `npm run tokens`; CI fails when the generated file is stale.
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

type TextStep = { size: string; lineHeight: string };
type ThemeSet = {
  scheme: string;
  color: Record<string, string>;
  shadow: Record<string, string>;
};
export type Tokens = {
  font: Record<string, string>;
  text: Record<string, TextStep>;
  fontWeight: Record<string, string>;
  spacing: string;
  radius: Record<string, string>;
  aspect: Record<string, string>;
  breakpoint: Record<string, string>;
  container: Record<string, string>;
  duration: Record<string, string>;
  ease: Record<string, string>;
  layout: Record<string, string>;
  /** The TV type scale and focus, in pixels on a 1080p canvas, read by the native apps' generators. */
  tv: {
    text: Record<string, TextStep>;
    focusRingWidth: string;
    focusScale: string;
    cardFocusScale: string;
    /**
     * The web client's TV build: the TV step each web text step becomes, the
     * durations it shortens, the TV shell's layout, and the glow around the
     * focused item.
     */
    web: {
      text: Record<string, string>;
      duration: Record<string, string>;
      layout: Record<string, string>;
      shadow: Record<string, string>;
    };
  };
  themes: { dark: ThemeSet; light: ThemeSet } & Record<string, ThemeSet>;
};

// Tailwind's default scales are cleared so only Kivo's tokens exist; a class
// like bg-red-500 then has nothing to resolve to.
const reset = [
  "color",
  "font",
  "text",
  "font-weight",
  "radius",
  "shadow",
  "inset-shadow",
  "drop-shadow",
  "text-shadow",
  "aspect",
  "breakpoint",
  "container",
  "ease",
  "blur",
  "perspective",
  "tracking",
  "leading",
];

const kebab = (s: string) => s.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);

function vars(prefix: string, values: Record<string, string>): string[] {
  return Object.entries(values).map(([k, v]) => `  --${prefix}-${kebab(k)}: ${v};`);
}

function themeVars(set: ThemeSet): string[] {
  return [...vars("color", set.color), ...vars("shadow", set.shadow)];
}

// The TV build sets data-tv on the root. Each web text step takes its TV
// size, so every screen grows to the TV scale without a class of its own.
function tvVars(tokens: Tokens): string[] {
  const { tv } = tokens;
  const steps = Object.keys(tokens.text).map((k) => [k, tv.text[tv.web.text[k] ?? ""]] as const);
  const missing = steps.filter(([, step]) => step === undefined).map(([k]) => k);
  if (missing.length > 0) {
    throw new Error(`TV text lacks a step for: ${missing.join(", ")}`);
  }
  return [
    ...steps.flatMap(([k, step]) =>
      step ? [`  --text-${k}: ${step.size};`, `  --text-${k}--line-height: ${step.lineHeight};`] : [],
    ),
    ...vars("duration", tv.web.duration),
    ...vars("shadow-tv", tv.web.shadow),
    // The light set again, for what TV focus lights up: a focused control or
    // row turns white and takes these colours, whatever the theme.
    ...vars("color-tv-lit", tokens.themes.light.color),
    `  --layout-focus-ring-width: ${tv.focusRingWidth};`,
    `  --tv-focus-scale: ${tv.focusScale};`,
    `  --tv-card-focus-scale: ${tv.cardFocusScale};`,
  ];
}

export function renderTokensCss(tokens: Tokens): string {
  const { dark, light } = tokens.themes;
  const missing = Object.keys(dark.color).filter((k) => !(k in light.color));
  if (missing.length > 0) {
    throw new Error(`light theme lacks colour tokens: ${missing.join(", ")}`);
  }
  const lines = [
    "/* Generated from tokens.json by `npm run tokens`. Do not edit. */",
    "",
    "@theme static {",
    ...reset.map((ns) => `  --${ns}-*: initial;`),
    `  --spacing: ${tokens.spacing};`,
    ...vars("font", tokens.font),
    ...Object.entries(tokens.text).flatMap(([k, v]) => [
      `  --text-${k}: ${v.size};`,
      `  --text-${k}--line-height: ${v.lineHeight};`,
    ]),
    ...vars("font-weight", tokens.fontWeight),
    ...vars("radius", tokens.radius),
    ...vars("aspect", tokens.aspect),
    ...vars("breakpoint", tokens.breakpoint),
    ...vars("container", tokens.container),
    ...vars("duration", tokens.duration),
    ...vars("ease", tokens.ease),
    ...vars("layout", tokens.layout),
    ...vars("layout-tv", tokens.tv.web.layout),
    `  --default-font-family: var(--font-sans);`,
    `  --default-transition-duration: var(--duration-fast);`,
    `  --default-transition-timing-function: var(--ease-standard);`,
    ...themeVars(dark),
    "}",
    "",
    "@layer kivo.theme {",
    "  :root {",
    `    color-scheme: ${dark.scheme};`,
    "  }",
    "",
    '  :root[data-theme="light"] {',
    `    color-scheme: ${light.scheme};`,
    ...themeVars(light).map((l) => `  ${l}`),
    "  }",
    "",
    "  :root[data-tv] {",
    ...tvVars(tokens).map((l) => `  ${l}`),
    "  }",
    "}",
    "",
  ];
  return lines.join("\n");
}

const tokensPath = fileURLToPath(new URL("../../../shared/tokens.json", import.meta.url));
const cssPath = fileURLToPath(new URL("../src/theme/tokens.css", import.meta.url));

export function readTokens(): Tokens {
  return JSON.parse(readFileSync(tokensPath, "utf8")) as Tokens;
}

export function generatedCssPath(): string {
  return cssPath;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  writeFileSync(cssPath, renderTokensCss(readTokens()));
}
