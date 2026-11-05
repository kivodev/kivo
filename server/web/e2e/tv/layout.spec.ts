import { expect, type Page, test } from "@playwright/test";
import { back, expectFocus, focusOf, overscan, press, pressUntil } from "./remote";

// The TV build's layout as a remote moves through it: where rows end and
// stop, what a short row shows, the library while it loads and its letter
// rail, Settings' two panes and a detail page's hero and episodes.

const firstCard = /^card:continueWatching:/;
const expanse = "0190a000-0000-7000-8000-0000000000c8";
const holiday = "0190a000-0000-7000-8000-00000000006a";
const leviathan = "0190a000-0000-7000-8000-0000000001f5";

/** Waits for the page to stop scrolling, as a glide ends. */
async function settle(page: Page) {
  let last = -1;
  for (let i = 0; i < 40; i++) {
    // The page scrolls at once and its content eases into place on a
    // transition, so it has settled once neither moves.
    const { y, moving } = await page.evaluate(() => ({
      y: window.scrollY,
      moving: document.getAnimations().some((a) => a.playState === "running"),
    }));
    if (y === last && !moving) return;
    last = y;
    await page.waitForTimeout(100);
  }
}

test.describe("Home", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expectFocus(page, { key: firstCard });
  });

  test("the rail sits 16 pixels from the screen's edge, its icons as far from the content as before", async ({
    page,
  }) => {
    const left = () =>
      page.evaluate(() => {
        const icon = document.querySelector('[data-part="tv-rail-entry"] svg')?.getBoundingClientRect();
        const title = document.querySelector('[data-part="row"] h2')?.getBoundingClientRect();
        const item = document.querySelector('[data-part="tv-rail-entry"]')?.getBoundingClientRect();
        return { item: item?.left, icon: icon?.left, content: title?.left };
      });
    // The icons are inset by the item's own padding, and the content stays
    // 124 pixels past them.
    const closed = await left();
    expect(closed).toEqual({ item: 16, icon: 44, content: 168 });
    expect((closed.content ?? 0) - (closed.icon ?? 0)).toBe(124);
    // Opening the rail leaves its icons where they were.
    await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
    await page.waitForTimeout(400);
    expect(await left()).toMatchObject({ item: 16, icon: 44 });
  });

  test("the first row starts just below the screen's top inset", async ({ page }) => {
    const { y } = await overscan(page);
    expect(y).toBe(32);
    const top = await page.evaluate(
      () => document.querySelector('[data-part="row"]')?.getBoundingClientRect().top ?? 0,
    );
    expect(top).toBeGreaterThanOrEqual(y);
    expect(top, "no more than a line's room above it").toBeLessThanOrEqual(y + 48);
  });

  test("a long row's cards run on to the screen's right edge", async ({ page }) => {
    const cards = () =>
      page.evaluate(
        () =>
          document.activeElement?.closest('[data-part="row"]')?.querySelectorAll("li:not([data-tv-glow-host])")
            .length ?? 0,
      );
    for (let i = 0; i < 6 && (await cards()) < 8; i++) {
      await press(page, "ArrowDown");
      await settle(page);
    }
    const shows = await page.evaluate(() => {
      const row = document.activeElement?.closest('[data-part="row"]');
      const cards = [...(row?.querySelectorAll("li:not([data-tv-glow-host])") ?? [])];
      // A card that crosses the content's right edge shows up to the screen's.
      const edge = cards.find((c) => {
        const r = c.getBoundingClientRect();
        return r.left < 1880 && r.right > 1910;
      });
      if (!edge) return "no card crosses the edge";
      const r = edge.getBoundingClientRect();
      const hit = document.elementFromPoint(1910, r.top + r.height / 3);
      return edge.contains(hit) ? "shows" : `hidden at 1910 by ${hit?.tagName}`;
    });
    expect(shows).toBe("shows");
  });

  test("a row moved along keeps its focused card on screen as the window resizes", async ({ page }) => {
    const cards = () =>
      page.evaluate(
        () =>
          document.activeElement?.closest('[data-part="row"]')?.querySelectorAll("li:not([data-tv-glow-host])")
            .length ?? 0,
      );
    for (let i = 0; i < 6 && (await cards()) < 8; i++) {
      await press(page, "ArrowDown");
      await settle(page);
    }
    for (let i = 0; i < (await cards()) - 1; i++) await press(page, "ArrowRight");
    await page.waitForTimeout(500);
    await page.setViewportSize({ width: 960, height: 540 });
    await expect
      .poll(() =>
        page.evaluate(() => {
          const card = document.activeElement?.closest("li")?.getBoundingClientRect();
          return card ? card.left >= 0 && card.right <= window.innerWidth : false;
        }),
      )
      .toBe(true);
  });

  test("Down to the last row leaves nothing of the row above at the screen's top", async ({ page }) => {
    const rows = await page.locator('[data-part="row"]').count();
    for (let i = 0; i < rows - 1; i++) await press(page, "ArrowDown");
    await settle(page);
    const { above, top } = await page.evaluate(() => {
      const row = document.activeElement?.closest('[data-part="row"]');
      const prev = row?.previousElementSibling;
      return { above: prev?.getBoundingClientRect().bottom ?? 0, top: row?.getBoundingClientRect().top ?? 0 };
    });
    expect(above, "the row above is wholly off screen").toBeLessThanOrEqual(0);
    expect(top).toBeGreaterThanOrEqual((await overscan(page)).y - 1);
  });

  test("every row's cards sit the row gap apart, the same in every row, and still fit their count across", async ({
    page,
  }) => {
    const rows = await page.evaluate(() => {
      const gap = Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--layout-tv-row-gap"));
      return [...document.querySelectorAll('[data-part="row"]')].map((r) => {
        const cards = [...r.querySelectorAll("li:not([data-tv-glow-host])")].map((li) => li.getBoundingClientRect());
        const width = r.getBoundingClientRect().width;
        const card = cards[0]?.width ?? 0;
        return {
          gap,
          gaps: cards.slice(1).map((c, i) => Math.round((c.left - (cards[i]?.right ?? 0)) * 10) / 10),
          across: Math.round((width + gap) / (card + gap)),
          fits: Math.round(width - (Math.round((width + gap) / (card + gap)) * (card + gap) - gap)),
          landscape: (cards[0]?.width ?? 0) > 400,
        };
      });
    });
    expect(rows[0]?.gap, "a row gap wider than the grid's 28 pixels").toBe(56);
    for (const r of rows) {
      for (const g of r.gaps) expect(g).toBeCloseTo(r.gap, 0);
      expect(r.across).toBe(r.landscape ? 3 : 6);
      expect(r.fits, "the cards fill the content's width").toBe(0);
    }
  });

  test("a card's caption lines up with its artwork's left edge, as the row's title does", async ({ page }) => {
    const edges = await page.evaluate(() =>
      [...document.querySelectorAll('[data-part="row"]')].map((r) => {
        const ring = r.querySelectorAll("li [data-tv-ring]")[1];
        const caption = ring?.nextElementSibling?.firstElementChild;
        // The first card's place, which its focused artwork grows out of.
        const first = r.querySelector("li:not([data-tv-glow-host])");
        return {
          caption: Math.round(caption?.getBoundingClientRect().left ?? -1),
          artwork: Math.round(ring?.getBoundingClientRect().left ?? -2),
          title: Math.round(r.querySelector("h2")?.getBoundingClientRect().left ?? -3),
          first: Math.round(first?.getBoundingClientRect().left ?? -4),
        };
      }),
    );
    for (const e of edges.filter((e) => e.artwork >= 0)) {
      expect(e.caption).toBe(e.artwork);
      expect(e.title).toBe(e.first);
    }
  });

  test("the titles are in the build's own Inter, at their semibold weight, not the TV's sans-serif", async ({
    page,
  }) => {
    const { loaded, weight, sources } = await page.evaluate(async () => {
      await document.fonts.ready;
      const h2 = document.querySelector('[data-part="row"] h2');
      return {
        loaded: [...document.fonts].filter(
          (f) => f.family.replace(/"/g, "") === "Inter Variable" && f.status === "loaded",
        ).length,
        weight: h2 ? getComputedStyle(h2).fontWeight : "",
        sources: performance
          .getEntriesByType("resource")
          .map((e) => e.name)
          .filter((n) => n.includes(".woff2")),
      };
    });
    expect(loaded, "Inter faces loaded").toBeGreaterThan(0);
    expect(weight).toBe("600");
    expect(sources.some((s) => s.includes("inter-latin-wght"))).toBe(true);
    for (const s of sources) expect(new URL(s).origin, "the page's own origin").toBe(new URL(page.url()).origin);
  });

  test("See all ends only a row longer than the screen", async ({ page }) => {
    const rows = await page.evaluate(() =>
      [...document.querySelectorAll('[data-part="row"]')].map((r) => {
        const gap = Number.parseFloat(getComputedStyle(r.querySelector("ul") ?? r).columnGap) || 0;
        return {
          cards: r.querySelectorAll("li:not(:has([data-part=see-all]))").length,
          seeAll: r.querySelector('[data-part="see-all"]') !== null,
          across: Math.round(
            (r.getBoundingClientRect().width + gap) /
              ((r.querySelector("li:not([data-tv-glow-host])")?.getBoundingClientRect().width ?? 1) + gap),
          ),
        };
      }),
    );
    expect(rows.some((r) => r.seeAll)).toBe(true);
    for (const r of rows)
      if (r.cards <= r.across) expect(r, "a short row has no See all").toMatchObject({ seeAll: false });
  });
});

test.describe("Library", () => {
  test("the grid's cards sit Home's row gap apart, so a focused card and its glow clear its neighbours", async ({
    page,
  }) => {
    await page.goto("/");
    await expectFocus(page, { key: firstCard });
    await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
    await pressUntil(page, "ArrowDown", (f) => f.name === "Movies");
    await press(page, "Enter");
    await expectFocus(page, { key: /^card:grid:/ });
    // The second card of the second row, with neighbours on every side.
    await press(page, "ArrowDown", "ArrowRight");
    await page.waitForTimeout(400);
    const { gap, focused, left, right, above } = await page.evaluate(() => {
      const ring = (c: Element | null | undefined) => c?.querySelector("[data-tv-ring]")?.getBoundingClientRect();
      const cells = [...document.querySelectorAll('[data-part="grid"] [data-index]')];
      const at = (i: number) => cells.find((c) => c.getAttribute("data-index") === String(i));
      const first = ring(at(0));
      const second = ring(at(1));
      const caption = at(1)?.querySelector("[data-tv-ring] ~ *")?.getBoundingClientRect();
      return {
        gap: (second?.left ?? 0) - (first?.right ?? 0),
        focused: ring(document.activeElement),
        left: ring(at(6))?.right,
        right: ring(at(8))?.left,
        above: caption?.bottom,
      };
    });
    const row = Number.parseFloat(
      await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--layout-tv-row-gap")),
    );
    expect(Math.round(gap), "the gap between columns").toBe(row);
    // The glow reaches 24 pixels past the lifted artwork.
    const glow = 24;
    expect((focused?.left ?? 0) - glow - (left ?? 0), "air to the left").toBeGreaterThan(0);
    expect((right ?? 0) - ((focused?.right ?? 0) + glow), "air to the right").toBeGreaterThan(0);
    expect((focused?.top ?? 0) - (above ?? 0), "clear of the caption above").toBeGreaterThan(0);
  });

  test("while it loads, it shows its grid's columns, and the rail closes at once", async ({ page }) => {
    await page.goto("/?delay=1500");
    await expectFocus(page, { key: firstCard });
    await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
    await pressUntil(page, "ArrowDown", (f) => f.name === "Movies");
    await press(page, "Enter");
    await page.waitForTimeout(300);
    expect((await page.locator('[data-part="tv-rail"]').boundingBox())?.width, "the rail closed").toBeLessThan(200);
    const columns = await page.evaluate(() => {
      const busy = [...document.querySelectorAll('[aria-busy="true"]')].find((b) => b.querySelector(".grid"));
      const cells = [...(busy?.querySelector(".grid")?.children ?? [])];
      return new Set(cells.map((c) => Math.round(c.getBoundingClientRect().left))).size;
    });
    expect(columns).toBe(6);
    await expectFocus(page, { key: /^card:grid:/ });
    const loaded = await page.evaluate(() => {
      const cells = [...document.querySelectorAll('[data-part="grid"] [data-index]')].slice(0, 6);
      return cells.map((c) => Math.round(c.getBoundingClientRect().left));
    });
    expect(new Set(loaded).size).toBe(6);
  });

  test("the grid fills the width to the letter rail, which sits at the screen's right inset", async ({ page }) => {
    await page.goto("/");
    await expectFocus(page, { key: firstCard });
    await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
    await pressUntil(page, "ArrowDown", (f) => f.name === "Movies");
    await press(page, "Enter");
    await expectFocus(page, { key: /^card:grid:/ });
    const { x } = await overscan(page);
    expect(x).toBe(48);
    const edge = 1920 - x;
    const { grid, rail, filter, across } = await page.evaluate(() => {
      const cells = [...document.querySelectorAll('[data-part="grid"] [data-index]')].map((c) =>
        c.getBoundingClientRect(),
      );
      const top = Math.min(...cells.map((c) => c.top));
      const rail = document.querySelector('[data-part="letter-rail-focus"]')?.getBoundingClientRect();
      const filter = [...document.querySelectorAll('[data-part="library-header"] button')]
        .find((b) => b.textContent?.includes("Filter"))
        ?.getBoundingClientRect();
      return {
        grid: Math.max(...cells.map((c) => c.right)),
        rail: rail && { left: rail.left, right: rail.right },
        filter: filter?.right,
        across: cells.filter((c) => c.top === top).length,
      };
    });
    expect(across).toBe(6);
    expect(rail?.right, "the letter rail's right edge").toBeLessThanOrEqual(edge);
    expect(rail?.right, "the letter rail's right edge").toBeGreaterThanOrEqual(edge - 80);
    expect(filter, "the Filter button lines up with the rail").toBe(rail?.right);
    // The grid's last column ends a gap short of the letter rail, no more.
    expect(grid).toBeLessThan(rail?.left ?? 0);
    expect(grid).toBeGreaterThanOrEqual((rail?.left ?? 0) - 24);
    expect(grid, "the grid's right edge").toBeGreaterThanOrEqual(edge - 120);
  });

  test("one glow lights the focused card, following focus across the grid, and goes when focus leaves it", async ({
    page,
  }) => {
    await page.goto("/");
    await expectFocus(page, { key: firstCard });
    await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
    await pressUntil(page, "ArrowDown", (f) => f.name === "Movies");
    await press(page, "Enter");
    await expectFocus(page, { key: /^card:grid:/ });
    const lit = () =>
      page.evaluate(() => {
        const glow = document.querySelector('[data-part="tv-glow"]');
        const sheen = document.querySelector('[data-part="tv-sheen"]')?.getBoundingClientRect();
        const ring = document.activeElement?.querySelector("[data-tv-ring]");
        const g = glow?.getBoundingClientRect();
        const r = ring?.getBoundingClientRect();
        return {
          shown: glow ? getComputedStyle(glow).opacity : "",
          // How far the glow's box is from the focused artwork's, at its focused scale.
          off:
            g && r && sheen
              ? Math.max(
                  ...[g, sheen].flatMap((b) => [
                    Math.abs(b.left - r.left),
                    Math.abs(b.top - r.top),
                    Math.abs(b.width - r.width),
                  ]),
                )
              : -1,
          cardGlows: [...document.querySelectorAll('[data-part="grid"] [data-tv-ring]')].filter(
            (el) => getComputedStyle(el, "::after").display !== "none",
          ).length,
        };
      });
    for (const keys of [[], ["ArrowRight"], ["ArrowDown"], ["ArrowDown", "ArrowRight"]] as const) {
      await press(page, ...keys);
      await settle(page);
      const { shown, off, cardGlows } = await lit();
      expect(shown).toBe("1");
      expect(off, "the glow sits on the focused card's artwork").toBeLessThan(2);
      expect(cardGlows, "no card draws a glow of its own").toBe(0);
    }
    // Left from the first column leaves the grid for the rail.
    await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
    await expect.poll(async () => (await lit()).shown).toBe("0");
  });

  test("the letter rail is legible from across a room and marks the letter in view", async ({ page }) => {
    await page.goto("/");
    await expectFocus(page, { key: firstCard });
    await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
    await pressUntil(page, "ArrowDown", (f) => f.name === "Movies");
    await press(page, "Enter");
    await expectFocus(page, { key: /^card:grid:/ });
    const letters = await page.evaluate(() =>
      [...document.querySelectorAll('[data-part="letter-rail"] > span')].map((s) => {
        const r = s.getBoundingClientRect();
        return { size: Number.parseFloat(getComputedStyle(s).fontSize), top: r.top, bottom: r.bottom };
      }),
    );
    expect(letters).toHaveLength(27);
    for (const l of letters) expect(l.size).toBeGreaterThanOrEqual(24);
    for (let i = 1; i < letters.length; i++) {
      const pitch = (letters[i]?.top ?? 0) - (letters[i - 1]?.top ?? 0);
      expect(pitch, "each letter has a line of its own").toBeGreaterThanOrEqual(28);
    }
    for (let i = 0; i < 4; i++) await press(page, "ArrowDown");
    await settle(page);
    const { y } = await overscan(page);
    const { marked, inView } = await page.evaluate((overscan) => {
      // The first card whose artwork is mostly below the screen's top.
      const cells = [...document.querySelectorAll<HTMLElement>('[data-part="grid"] [data-index]')]
        .map((c) => ({ c, art: c.querySelector("[data-tv-ring]")?.getBoundingClientRect() }))
        .filter(({ art }) => art && art.top + art.height / 2 >= overscan)
        .sort((a, b) => Number(a.c.dataset.index) - Number(b.c.dataset.index));
      const title = cells[0]?.c.innerText.trim() ?? "";
      return {
        marked: document.querySelector('[data-part="letter-rail"] [data-current]')?.textContent,
        inView: title[0]?.toUpperCase(),
      };
    }, y);
    expect(marked).toBe(inView);
    // The rail's pick fills its own line, clear of the letters beside it.
    await pressUntil(page, "ArrowRight", (f) => f.part === "letter-rail-focus");
    await press(page, "ArrowDown");
    const pick = await page.evaluate(() => {
      const picked = document.querySelector('[data-part="letter-rail"] [data-picked]');
      const next = picked?.nextElementSibling;
      const prev = picked?.previousElementSibling;
      const p = picked?.getBoundingClientRect();
      return {
        overlapsNext: p && next ? p.bottom > next.getBoundingClientRect().top + 1 : false,
        overlapsPrev: p && prev ? p.top < prev.getBoundingClientRect().bottom - 1 : false,
      };
    });
    expect(pick).toEqual({ overlapsNext: false, overlapsPrev: false });
  });
});

test.describe("Settings", () => {
  async function openSection(page: Page, name: string) {
    await page.goto("/settings");
    await expectFocus(page, { part: "settings-section-entry", name: "Account" });
    await pressUntil(page, "ArrowDown", (f) => f.name === name);
  }

  test("Down on a section's last row stays there, and the list beside it doesn't move", async ({ page }) => {
    await openSection(page, "Playback");
    const list = await page.locator('[data-part="settings-section-entry"]').first().boundingBox();
    await press(page, "ArrowRight");
    await expectFocus(page, { key: "setting:", name: "Play next episode" });
    await press(page, "ArrowDown");
    await expectFocus(page, { key: "setting:", name: "Remember choices" });
    await press(page, "ArrowDown", "ArrowDown");
    await page.waitForTimeout(200);
    await expectFocus(page, { key: "setting:", name: "Remember choices" });
    await press(page, "ArrowUp", "ArrowUp");
    await page.waitForTimeout(200);
    await expectFocus(page, { key: "setting:", name: "Play next episode" });
    expect(await page.locator('[data-part="settings-section-entry"]').first().boundingBox()).toEqual(list);
  });

  test("Down on the last section stays on it, without scrolling", async ({ page }) => {
    await openSection(page, "About");
    await settle(page);
    const y = await page.evaluate(() => window.scrollY);
    await press(page, "ArrowDown");
    await page.waitForTimeout(300);
    await expectFocus(page, { part: "settings-section-entry", name: "About" });
    expect(await page.evaluate(() => window.scrollY)).toBe(y);
  });

  test("Back from a setting returns to its section in the list", async ({ page }) => {
    await openSection(page, "Playback");
    await press(page, "ArrowRight");
    await expectFocus(page, { key: "setting:" });
    await back(page);
    await expectFocus(page, { part: "settings-section-entry", name: "Playback" });
    // From the list, Back still opens the rail on Settings.
    await back(page);
    await expectFocus(page, { part: "tv-rail-entry", name: "Settings" });
  });
});

test.describe("Detail", () => {
  test("Down from Resume goes to the season in progress, then its episode in progress", async ({ page }) => {
    await page.goto(`/works/${expanse}`);
    await expectFocus(page, { key: "play", name: "Resume" });
    await press(page, "ArrowDown");
    await expectFocus(page, { key: "season:", name: "S3" });
    await pressUntil(page, "ArrowDown", (f) => f.key?.startsWith("card:episodes:") ?? false, 3);
    expect((await focusOf(page)).name).toContain("3. Episode 3");
  });

  test("OK on an episode opens its page with Play focused, and Back returns to that episode", async ({ page }) => {
    await page.goto(`/works/${expanse}`);
    await expectFocus(page, { key: "play", name: "Resume" });
    await pressUntil(page, "ArrowDown", (f) => f.key?.startsWith("card:episodes:") ?? false, 3);
    await press(page, "ArrowRight");
    await expectFocus(page, { key: "card:episodes:", name: "4. Reload" });
    const episode = (await focusOf(page)).key;
    await press(page, "Enter");
    await expect(page).toHaveURL(/\/works\/0190a000-0000-7000-8000-000000000518$/);
    await expectFocus(page, { key: "play", name: "Play" });
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Reload");
    await back(page);
    await expect(page).toHaveURL(new RegExp(`/works/${expanse}`));
    await expectFocus(page, { key: episode ?? "" });
  });

  test("a hero's backdrop fills the screen's width and top, fading into the page", async ({ page }) => {
    await page.goto(`/works/${expanse}`);
    await expectFocus(page, { key: "play" });
    const backdrop = await page.locator('[data-part="hero-backdrop"]').boundingBox();
    expect(backdrop?.x).toBeLessThanOrEqual(0);
    expect(backdrop?.y).toBeLessThanOrEqual(0);
    expect((backdrop?.x ?? 0) + (backdrop?.width ?? 0)).toBeGreaterThanOrEqual(1920);
  });

  test("while the page loads, its backdrop already fills the screen's width and top", async ({ page }) => {
    await page.goto("/?delay=1500");
    await expectFocus(page, { key: firstCard });
    await press(page, "Enter");
    await expect(page.locator('[aria-busy="true"] [data-part="hero"]')).toBeVisible();
    const backdrop = await page.locator('[aria-busy="true"] [data-part="hero-backdrop"]').boundingBox();
    expect(backdrop?.x).toBeLessThanOrEqual(0);
    expect(backdrop?.y).toBeLessThanOrEqual(0);
    expect((backdrop?.x ?? 0) + (backdrop?.width ?? 0)).toBeGreaterThanOrEqual(1920);
  });

  test("a series' episodes start on the first screen", async ({ page }) => {
    await page.goto(`/works/${expanse}`);
    await expectFocus(page, { key: "play" });
    const still = page.locator('[data-row="episodes"] [data-part="still"]').first();
    await expect(still).toBeVisible();
    const first = await still.boundingBox();
    expect(first?.y ?? 1080).toBeLessThanOrEqual(1080 - (await overscan(page)).y - 200);
  });

  test("a title without artwork still has a hero the height of one with it", async ({ page }) => {
    await page.goto(`/works/${holiday}`);
    await expectFocus(page, { key: "play" });
    const hero = await page.locator('[data-part="hero"]').boundingBox();
    expect(hero?.height).toBeGreaterThanOrEqual(560);
    await expect(page.locator('[data-part="hero-backdrop"]')).toBeAttached();
    // The poster, or its titled tile, fills the hero's right.
    await expect(page.locator('[data-part="hero-poster"]')).toBeVisible();
  });
});

test.describe("Connection", () => {
  test("the notice floats above the docked player, leaving focus and the page where they were", async ({ page }) => {
    // Playing a book docks its player; Back leaves it docked on Home.
    await page.goto(`/works/${leviathan}`);
    await expectFocus(page, { key: "play" });
    await press(page, "Enter");
    const dock = page.locator('[data-part="mini-player"]');
    await expect(dock).toBeVisible();
    await back(page);
    await expectFocus(page, { key: firstCard });
    await settle(page);
    const focused = await focusOf(page);
    const row = await page.locator('[data-part="row"]').first().boundingBox();
    const scrolled = await page.evaluate(() => window.scrollY);

    await page.evaluate(() => (window as unknown as { kivoMock: { serverGone: () => void } }).kivoMock.serverGone());
    const notice = page.getByRole("status").filter({ hasText: "Can't reach Living room. Reconnecting…" });
    await expect(notice).toBeVisible();
    const pill = await notice.boundingBox();
    const bar = await dock.boundingBox();
    expect((pill?.y ?? 0) + (pill?.height ?? 0)).toBeLessThan(bar?.y ?? 0);
    expect(await focusOf(page)).toEqual(focused);
    expect(await page.locator('[data-part="row"]').first().boundingBox()).toEqual(row);
    expect(await page.evaluate(() => window.scrollY)).toBe(scrolled);
  });
});
