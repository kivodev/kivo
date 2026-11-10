import { expect, type Page, type Request, test } from "@playwright/test";
import { mockIds } from "@/api/mocks/fixtures";

// A 3,000-work library moved through by each input the grid has: a fling,
// the letter rail scrubbed down to Z and back and forth through S to Z, and
// a remote's arrow key held down through the whole list and back up. While
// it moves fast the grid fetches no pages and loads no artwork; whatever the
// input, the rows, posters and decoded pixels it holds stay those of about a
// screen and a half, the same on every pass. A slow browse, though, shows
// posters as the rows come.

const rail = ["#", ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ"];

type Sample = { nodes: number; imgs: number; pixels: number };
type Pass = {
  peak: Sample;
  rest: Sample;
  /** Posters loaded during the pass, whether from the network or the cache. */
  loaded: number;
  pages: { asked: number; done: number; cancelled: number };
};

/**
 * Samples the page every frame: its elements, the grid's poster images and
 * their decoded pixels, at their natural size or, for an SVG, as drawn.
 */
async function watch(page: Page) {
  await page.addInitScript(() => {
    const sample = () => {
      let pixels = 0;
      const posters = document.querySelectorAll<HTMLImageElement>('[data-part="grid"] img[src*="/images/"]');
      for (const img of posters) {
        if (!img.complete) continue;
        const svg = img.naturalWidth === 0 || img.currentSrc.includes("svg");
        pixels += svg
          ? img.clientWidth * img.clientHeight * devicePixelRatio ** 2
          : img.naturalWidth * img.naturalHeight;
      }
      return { nodes: document.getElementsByTagName("*").length, imgs: posters.length, pixels };
    };
    const w = window as unknown as { peak: Sample; sample: () => Sample; loaded: number };
    w.sample = sample;
    w.peak = { nodes: 0, imgs: 0, pixels: 0 };
    w.loaded = 0;
    document.addEventListener(
      "load",
      (e) => {
        if (e.target instanceof HTMLImageElement && e.target.src.includes("/images/")) w.loaded++;
      },
      true,
    );
    const frame = () => {
      const now = sample();
      w.peak = {
        nodes: Math.max(w.peak.nodes, now.nodes),
        imgs: Math.max(w.peak.imgs, now.imgs),
        pixels: Math.max(w.peak.pixels, now.pixels),
      };
      requestAnimationFrame(frame);
    };
    requestAnimationFrame(frame);
  });
  const pages = { asked: 0, done: 0, cancelled: 0 };
  // A page of the grid: a work query read in the library.
  const isPage = (r: Request) =>
    new URL(r.url()).pathname.endsWith("/api/v1/works/query") && r.postDataJSON()?.library === mockIds.movies;
  page.on("request", (r) => {
    if (isPage(r)) pages.asked++;
  });
  page.on("requestfinished", (r) => {
    if (isPage(r)) pages.done++;
  });
  page.on("requestfailed", (r) => {
    if (isPage(r)) pages.cancelled++;
  });
  const settle = async () => {
    await page.waitForTimeout(600);
    await expect
      .poll(() => page.evaluate(() => document.querySelectorAll('[data-part="grid"] img[src*="/images/"]').length))
      .toBeGreaterThan(0);
    await page.waitForTimeout(300);
  };
  /** Runs one pass of an input, from rest to rest, and what it held. */
  const pass = async (run: () => Promise<void>): Promise<Pass> => {
    const before = { ...pages };
    await page.evaluate(() => {
      const w = window as unknown as { peak: Sample; sample: () => Sample; loaded: number };
      w.peak = w.sample();
      w.loaded = 0;
    });
    await run();
    await settle();
    const { peak, rest, loaded } = await page.evaluate(() => {
      const w = window as unknown as { peak: Sample; sample: () => Sample; loaded: number };
      return { peak: w.peak, rest: w.sample(), loaded: w.loaded };
    });
    return {
      peak,
      rest,
      loaded,
      pages: {
        asked: pages.asked - before.asked,
        done: pages.done - before.done,
        cancelled: pages.cancelled - before.cancelled,
      },
    };
  };
  return { pass, settle };
}

async function open(page: Page) {
  const watched = await watch(page);
  await page.goto(`libraries/${mockIds.movies}?mock=long`);
  await expect(page.locator('[data-part="card"]').first()).toBeVisible();
  // At rest mid-list, the grid renders rows past the view on both sides: the most it holds.
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight / 2));
  await watched.settle();
  const atRest = await page.evaluate(() => (window as unknown as { sample: () => Sample }).sample());
  return { ...watched, atRest };
}

/** Each pass holds about what the grid holds at rest, a partly shown row more at most, and no more on a later pass than the first. */
function expectBounded(passes: Pass[], atRest: Sample, label: string) {
  const measured = JSON.stringify({ atRest, passes });
  test.info().annotations.push({ type: label, description: measured });
  console.log(`${test.info().project.name} ${label} ${measured}`);
  for (const p of passes) {
    expect(p.peak.nodes).toBeLessThan(atRest.nodes * 1.5);
    expect(p.peak.imgs).toBeLessThanOrEqual(atRest.imgs * 1.2);
    expect(p.peak.pixels).toBeLessThanOrEqual(atRest.pixels * 1.2);
    expect(p.rest.imgs).toBeLessThanOrEqual(atRest.imgs * 1.2);
    // The posters of where the pass stops, not of the rows it passed: those
    // in view at rest, and once more if a slow frame stopped it on the way.
    expect(p.loaded).toBeLessThanOrEqual(atRest.imgs * 1.2 * 2);
  }
  // Pages load where a pass stops, not for each row it passes: the rendered
  // rows' and a screen past them, at most twice, as a slow frame on the way
  // can stop it for a moment.
  const perStop = Math.ceil((2 * atRest.imgs) / 100) + 1;
  for (const p of passes) expect(p.pages.asked).toBeLessThanOrEqual(2 * perStop);
  const [first, ...later] = passes;
  for (const p of later) {
    expect(p.peak.nodes).toBeLessThanOrEqual((first?.peak.nodes ?? 0) * 1.2);
  }
}

test("a fling down the list and back", async ({ page }) => {
  const { pass, atRest } = await open(page);
  const fling = (by: number) =>
    page.evaluate(
      (by) =>
        new Promise<void>((done) => {
          let steps = 60;
          const timer = setInterval(() => {
            window.scrollBy(0, by);
            if (--steps > 0) return;
            clearInterval(timer);
            done();
          }, 16);
        }),
      by,
    );
  const passes: Pass[] = [];
  for (let i = 0; i < 3; i++) {
    passes.push(await pass(() => fling(1500)));
    passes.push(await pass(() => fling(-1500)));
  }
  expectBounded(passes, atRest, "fling");
});

test("the letter rail scrubbed to Z and back and forth through S to Z", async ({ page }) => {
  const { pass, atRest } = await open(page);
  const slider = page.getByRole("slider", { name: "Jump to letter" });
  const box = await slider.boundingBox();
  if (!box) throw new Error("the rail has no box");
  const x = box.x + box.width / 2;
  const at = (letter: string) => box.y + (rail.indexOf(letter) + 0.5) * (box.height / rail.length);
  const scrub = async (letters: string[], hold: number) => {
    await page.mouse.move(x, at(letters[0] ?? "#"));
    await page.mouse.down();
    for (const letter of letters) {
      await page.mouse.move(x, at(letter));
      await page.waitForTimeout(hold);
    }
    // Held still on the last letter: still moving until let go.
    await page.waitForTimeout(500);
    expect(await page.evaluate(() => document.querySelectorAll('[data-part="grid"] img[src*="/images/"]').length)).toBe(
      0,
    );
    await page.mouse.up();
  };
  const passes: Pass[] = [];
  for (let i = 0; i < 3; i++) {
    passes.push(await pass(() => scrub(rail, 40)));
    const wiggle = [..."STUVWXYZYXWVUTSTUVWXYZ"];
    passes.push(await pass(() => scrub(wiggle, 80)));
  }
  await expect(slider).toHaveAttribute("aria-valuetext", "Z");
  expectBounded(passes, atRest, "rail");
});

test("a remote's arrow key held down through the list and back up", async ({ page }) => {
  const { pass, atRest } = await open(page);
  const columns = await page.evaluate(
    () =>
      getComputedStyle(document.querySelector('[data-part="grid"] > div') as Element).gridTemplateColumns.split(" ")
        .length,
  );
  const rows = Math.ceil(3000 / columns);
  const focusedIndex = () =>
    page.evaluate(() => Number(document.activeElement?.closest("[data-index]")?.getAttribute("data-index")));
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.locator('[data-index="0"] a').first().focus();
  // A held key: the press, then its repeats 30 a second, sent in the page
  // so they come as evenly as a remote's rather than at the driver's pace.
  const hold = async (key: string, presses: number) => {
    await page.keyboard.down(key);
    await page.evaluate(
      ({ key, repeats }) =>
        new Promise<void>((done) => {
          let left = repeats;
          const timer = setInterval(() => {
            const init = { key, repeat: true, bubbles: true, cancelable: true };
            document.activeElement?.dispatchEvent(new KeyboardEvent("keydown", init));
            if (--left > 0) return;
            clearInterval(timer);
            done();
          }, 33);
        }),
      { key, repeats: presses - 1 },
    );
    await page.keyboard.up(key);
  };
  const passes: Pass[] = [];
  for (let i = 0; i < 2; i++) {
    passes.push(await pass(() => hold("ArrowDown", rows)));
    expect(await focusedIndex()).toBeGreaterThanOrEqual(3000 - columns);
    // The card focused at the end is shown, and its focus ring with it.
    const focused = page.locator(":focus");
    await expect(focused).toBeInViewport();
    expect(await focused.evaluate((el) => el.matches(":focus-visible"))).toBe(true);
    passes.push(await pass(() => hold("ArrowUp", rows)));
    expect(await focusedIndex()).toBeLessThan(columns);
  }
  expectBounded(passes, atRest, "keys");
});

/**
 * Scrolls by a speed in pixels a second that each 16 ms step updates, until
 * it falls under 10, and samples the share of the cards in view without
 * their poster: each frame while it moves, and for a second after.
 */
async function blankWhile(page: Page, speed: number, decay: number) {
  return page.evaluate(
    ({ speed, decay }) =>
      new Promise<{ moving: number; worst: number; settledMs: number }>((done) => {
        const blank = () => {
          const cells = [...document.querySelectorAll<HTMLElement>("[data-index]")].filter((c) => {
            const r = c.getBoundingClientRect();
            return r.bottom > 0 && r.top < innerHeight;
          });
          const bare = cells.filter((c) => !c.querySelector<HTMLImageElement>('img[src*="/images/"]')?.complete);
          return cells.length ? bare.length / cells.length : 0;
        };
        const seen: number[] = [];
        let v = speed;
        const timer = setInterval(() => {
          window.scrollBy(0, (v * 16) / 1000);
          seen.push(blank());
          v *= decay;
          if (v >= 10) return;
          clearInterval(timer);
          const stopped = performance.now();
          const after = () => {
            const at = performance.now() - stopped;
            if (blank() === 0 || at > 1000) {
              const mean = seen.reduce((a, b) => a + b, 0) / seen.length;
              done({
                moving: Math.round(mean * 100),
                worst: Math.round(Math.max(...seen) * 100),
                settledMs: Math.round(at),
              });
            } else requestAnimationFrame(after);
          };
          requestAnimationFrame(after);
        }, 16);
      }),
    { speed, decay },
  );
}

test("a slow browse shows posters as rows come into view, and a flick shows them as it slows", async ({ page }) => {
  await open(page);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(600);
  // A browse at 360 pixels a second, for five seconds.
  const browse = await blankWhile(page, 360, 0.9968);
  // A flick at 4,000 pixels a second, slowing as a finger's does.
  const flick = await blankWhile(page, 4000, 0.95);
  const measured = JSON.stringify({ browse, flick });
  test.info().annotations.push({ type: "blank cards (%)", description: measured });
  console.log(`${test.info().project.name} blank ${measured}`);
  expect(browse.moving).toBeLessThanOrEqual(5);
  expect(browse.settledMs).toBeLessThan(300);
  expect(flick.settledMs).toBeLessThan(300);
});

test("a mouse drag on the letter rail selects no text", async ({ page }) => {
  await page.goto(`libraries/${mockIds.movies}?mock=long`);
  const slider = page.getByRole("slider", { name: "Jump to letter" });
  await expect(page.locator('[data-part="card"]').first()).toBeVisible();
  const box = await slider.boundingBox();
  const title = await page.locator('[data-part="library-header"]').boundingBox();
  if (!box || !title) throw new Error("the rail or the header has no box");
  // Down on the rail, across the page to the library's title and back, as a hurried drag goes.
  await page.mouse.move(box.x + box.width / 2, box.y + 10);
  await page.mouse.down();
  await page.mouse.move(title.x + 10, title.y + 10, { steps: 10 });
  await page.mouse.move(box.x + box.width / 2, box.y + box.height - 10, { steps: 10 });
  await page.mouse.up();
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe("");
  await expect(slider).toBeFocused();
});

test("the letter rail shows its focus ring for the keyboard, not after a scrub", async ({ page, hasTouch }) => {
  await page.goto(`libraries/${mockIds.movies}?mock=long`);
  const slider = page.getByRole("slider", { name: "Jump to letter" });
  await expect(page.locator('[data-part="card"]').first()).toBeVisible();
  const box = await slider.boundingBox();
  if (!box) throw new Error("the rail has no box");
  const ring = () => slider.evaluate((el) => getComputedStyle(el).outlineStyle !== "none");
  // Scrubbed by the mouse, and on a phone tapped by a finger: focused, with no ring left behind.
  await page.mouse.move(box.x + box.width / 2, box.y + 10);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 5 });
  await page.mouse.up();
  await expect(slider).toBeFocused();
  expect(await ring()).toBe(false);
  if (hasTouch) {
    await page.touchscreen.tap(box.x + box.width / 2, box.y + box.height - 10);
    await expect(slider).toBeFocused();
    expect(await ring()).toBe(false);
  }
  // Tabbed away and back, it shows the ring, and its arrow keys still step through the letters.
  await page.keyboard.press("Tab");
  await page.keyboard.press("Shift+Tab");
  await expect(slider).toBeFocused();
  expect(await ring()).toBe(true);
  const before = await slider.getAttribute("aria-valuetext");
  await page.keyboard.press("ArrowUp");
  await expect(slider).not.toHaveAttribute("aria-valuetext", before ?? "");
  expect(await ring()).toBe(true);
});
