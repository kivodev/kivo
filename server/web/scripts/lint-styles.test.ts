import { describe, expect, it } from "vitest";
import { lintCss, lintTsx } from "./lint-styles.ts";

describe("lintCss", () => {
  it.each([
    ["a { color: #123456; }"],
    ["a { background: rgb(0 0 0 / 50%); }"],
    ["a { color: white; }"],
    ["a { padding: 13px; }"],
    ["a { margin-top: 1.5rem; }"],
  ])("rejects %s", (css) => {
    expect(lintCss("x.css", css)).not.toEqual([]);
  });

  it.each([
    ["a { color: var(--color-text); }"],
    ["a { padding: calc(var(--spacing) * 4); }"],
    ["/* #fff in a comment */ a { margin: 0; }"],
    ["@media (width >= 40rem) {"],
  ])("accepts %s", (css) => {
    expect(lintCss("x.css", css)).toEqual([]);
  });
});

describe("lintTsx", () => {
  it.each([
    ['<div className="bg-[#123456]" />'],
    ['<div className="p-[13px] text-text" />'],
    ['<div className="hover:w-[3rem]" />'],
    ['<div className="[color:red]" />'],
    ['const c = "mt-4 rounded-[7px]";'],
    ['<div style={{ color: "#fff" }} />'],
    ['<div style={{ width: "12px" }} />'],
    ["<div style={{ height: 300 }} />"],
  ])("rejects %s", (code) => {
    expect(lintTsx("x.tsx", code)).not.toEqual([]);
  });

  it.each([
    ['<div className="bg-surface p-4 text-text-muted" />'],
    ['<div className="w-(--layout-sidebar-width) aspect-poster" />'],
    // biome-ignore lint/suspicious/noTemplateCurlyInString: source code under test.
    ["<div style={{ transform: `translateY(${y}px)`, height: total }} />"],
    // biome-ignore lint/suspicious/noTemplateCurlyInString: source code under test.
    ['<div style={{ "--progress": `${pct}%` }} />'],
    ['const a = "items[0]";'],
  ])("accepts %s", (code) => {
    expect(lintTsx("x.tsx", code)).toEqual([]);
  });
});
