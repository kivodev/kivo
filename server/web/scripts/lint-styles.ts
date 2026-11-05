// Rejects raw style values: components
// use design tokens only. Fails on
//   - literal colours and lengths in stylesheets (tokens.css, which is
//     generated from tokens.json, is the one place they live);
//   - Tailwind arbitrary values and properties in class strings, such as
//     bg-[#123456], p-[13px] or [color:red];
//   - literal colours or lengths in a `style` prop.
// Token references such as w-(--layout-sidebar-width) are fine.
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

export type Finding = { file: string; line: number; message: string };

const colourRe =
  /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(|\b(?:white|black|red|green|blue|gray|grey)\b/;
const lengthRe = /(?<![\w-])-?\d*\.?\d+(?:px|rem|em|vh|vw|vmin|vmax|ch|pt)\b/;
const arbitraryRe = /(?:^|[\s"'`:])!?-?[a-z][\w-]*-\[[^\]\s]+\]|(?:^|[\s"'`:])\[[a-z-]+:[^\]]+\]/;

function lineOf(text: string, index: number): number {
  return text.slice(0, index).split("\n").length;
}

export function lintCss(file: string, text: string): Finding[] {
  const findings: Finding[] = [];
  const withoutComments = text.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, " "));
  withoutComments.split("\n").forEach((line, i) => {
    // Custom-property names and @-rule preludes may contain anything; only
    // values are checked.
    const value = line.includes(":") ? line.slice(line.indexOf(":") + 1) : "";
    if (value === "" || /^\s*@/.test(line)) return;
    if (colourRe.test(value)) {
      findings.push({ file, line: i + 1, message: "raw colour in stylesheet; use a colour token" });
    }
    if (lengthRe.test(value)) {
      findings.push({ file, line: i + 1, message: "raw length in stylesheet; use a token" });
    }
  });
  return findings;
}

function isStyleValueRaw(node: ts.Expression): string | undefined {
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    if (colourRe.test(node.text)) return "raw colour in style prop";
    if (lengthRe.test(node.text)) return "raw length in style prop";
  }
  if (ts.isNumericLiteral(node) && node.text !== "0") return "raw number in style prop";
  return undefined;
}

export function lintTsx(file: string, text: string): Finding[] {
  const findings: Finding[] = [];
  const source = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const report = (node: ts.Node, message: string) =>
    findings.push({ file, line: lineOf(text, node.getStart(source)), message });

  const visit = (node: ts.Node) => {
    if (
      (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node) || ts.isTemplateHead(node)) &&
      arbitraryRe.test(node.text)
    ) {
      report(node, "Tailwind arbitrary value; use a token");
    }
    if (ts.isJsxAttribute(node) && node.name.getText(source) === "style" && node.initializer) {
      const init = node.initializer;
      if (ts.isJsxExpression(init) && init.expression && ts.isObjectLiteralExpression(init.expression)) {
        for (const prop of init.expression.properties) {
          if (!ts.isPropertyAssignment(prop)) continue;
          const name = prop.name.getText(source);
          // Custom properties pass computed values to tokens-based classes.
          if (name.startsWith('"--') || name.startsWith("'--")) continue;
          const problem = isStyleValueRaw(prop.initializer);
          if (problem) report(prop, problem);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return findings;
}

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = join(dir, e.name);
    return e.isDirectory() ? walk(p) : [p];
  });
}

export function lintTree(root: string): Finding[] {
  return walk(root).flatMap((file) => {
    const rel = relative(root, file);
    if (rel.endsWith("theme/tokens.css") || rel.includes(".test.") || rel.startsWith("api/mocks")) {
      return [];
    }
    if (file.endsWith(".css")) return lintCss(rel, readFileSync(file, "utf8"));
    if (/\.tsx?$/.test(file) && !file.endsWith(".d.ts")) return lintTsx(rel, readFileSync(file, "utf8"));
    return [];
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const root = fileURLToPath(new URL("../src", import.meta.url));
  const findings = lintTree(root);
  for (const f of findings) console.error(`src/${f.file}:${f.line}: ${f.message}`);
  if (findings.length > 0) process.exit(1);
}
