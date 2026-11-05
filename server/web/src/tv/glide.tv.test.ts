import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { pageScrollTarget, releaseAround, scrollPage } from "./glide";

let y = 0;

beforeEach(() => {
  y = 0;
  Object.defineProperty(window, "scrollY", { configurable: true, get: () => y });
  Object.defineProperty(document.documentElement, "scrollHeight", { configurable: true, value: 10_000 });
  vi.stubGlobal("innerHeight", 1080);
  vi.spyOn(window, "scrollTo").mockImplementation(((options: ScrollToOptions) => {
    y = options.top ?? y;
  }) as typeof window.scrollTo);
  // The page's main region: a header, and a grid beside a letter rail fixed to the screen.
  document.body.innerHTML = `
    <main id="main">
      <header id="header"></header>
      <div id="row">
        <div id="grid"></div>
        <div id="rail-box"><div id="rail" class="fixed"></div></div>
      </div>
    </main>`;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

const el = (id: string) => document.getElementById(id) as HTMLElement;

/** Records each transform an element is given. */
function transformsOf(element: HTMLElement): string[] {
  const seen: string[] = [];
  const proto = Object.getOwnPropertyDescriptor(CSSStyleDeclaration.prototype, "transform");
  Object.defineProperty(element.style, "transform", {
    configurable: true,
    get: () => seen.at(-1) ?? "",
    set: (v: string) => {
      seen.push(v);
      proto?.set?.call(element.style, v);
    },
  });
  return seen;
}

it("scrolls at once, and shows the content moving there on a transform the compositor eases away", () => {
  const seen = transformsOf(el("grid"));
  scrollPage(500, "smooth");
  expect(y).toBe(500);
  expect(pageScrollTarget()).toBe(500);
  // The content starts where the page was, 500 px lower, and eases up to its place.
  expect(seen).toEqual(["translateY(500px)", "translateY(0px)"]);
  for (const id of ["header", "grid"]) {
    expect(el(id).style.transition).toMatch(/^transform 300ms cubic-bezier/);
    expect(el(id).style.willChange).toBe("transform");
  }
});

it("leaves an element fixed to the screen alone, moving the elements around it", () => {
  scrollPage(500, "smooth");
  for (const id of ["main", "row", "rail-box", "rail"]) expect(el(id).style.transform).toBe("");
});

it("re-aims a glide from where the content shows, so presses during one run on", () => {
  scrollPage(500, "smooth");
  // Partway, the content still shows 200 px below where it's going.
  vi.spyOn(window, "getComputedStyle").mockReturnValue({
    transform: "matrix(1, 0, 0, 1, 0, 200)",
  } as CSSStyleDeclaration);
  const seen = transformsOf(el("grid"));
  scrollPage(1000, "smooth");
  expect(y).toBe(1000);
  // It shows at 300 (500, less the 200 still to go), so it starts 700 px below 1000.
  expect(seen).toEqual(["translateY(700px)", "translateY(0px)"]);
});

it("reads where the content shows from a 3D transform too", () => {
  scrollPage(500, "smooth");
  vi.spyOn(window, "getComputedStyle").mockReturnValue({
    transform: "matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 120, 0, 1)",
  } as CSSStyleDeclaration);
  const seen = transformsOf(el("grid"));
  scrollPage(500, "smooth");
  expect(seen[0]).toBe("translateY(120px)");
});

it("jumps at once for a held key's repeat, ending any glide", () => {
  scrollPage(500, "smooth");
  scrollPage(800, "instant");
  expect(y).toBe(800);
  expect(el("grid").style.transition).toBe("none");
  expect(el("grid").style.transform).toBe("translateY(0px)");
});

it("stops at the page's end", () => {
  scrollPage(50_000, "smooth");
  expect(pageScrollTarget()).toBe(10_000 - 1080);
});

/** Content that a glide moved as a whole, as Home is, before a screen with a rail fixed to the screen shows in it. */
function glideContentWithoutRail() {
  document.body.innerHTML = `
    <main id="main">
      <div id="content"><div id="grid"></div></div>
    </main>`;
  scrollPage(500, "smooth");
  expect(el("content").style.willChange).toBe("transform");
  el("content").insertAdjacentHTML("beforeend", '<div id="rail-box"><div id="rail" class="fixed"></div></div>');
}

it("lets go of the content around an element fixed to the screen as it mounts, so it's fixed to the window again", () => {
  glideContentWithoutRail();
  releaseAround(el("rail"));
  expect(el("content").style.willChange).toBe("");
  expect(el("content").style.transform).toBe("");
  expect(el("content").style.transition).toBe("");
});

it("lets go of an element that no longer moves as a whole, moving the elements in it instead", () => {
  glideContentWithoutRail();
  scrollPage(800, "smooth");
  expect(el("content").style.willChange).toBe("");
  expect(el("content").style.transform).toBe("");
  expect(el("grid").style.willChange).toBe("transform");
  expect(el("rail").style.transform).toBe("");
});
