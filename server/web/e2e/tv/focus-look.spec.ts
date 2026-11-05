import { expect, type Locator, type Page, test } from "@playwright/test";
import { expectFocus, focusOf, press, pressUntil } from "./remote";

// How focus looks on TV, read from the page's computed styles: a card's
// artwork lifts alone on a glow its row has room for, and a control, a
// field or a row turns white with dark text, on itself rather than on the
// wrapper that takes focus. Nothing but scale, opacity and translate eases.

const white = "rgb(255, 255, 255)";

/** The computed style properties of an element, by name. */
const styleOf = (el: Locator, ...names: string[]) =>
  el.evaluate((e, props) => Object.fromEntries(props.map((p) => [p, getComputedStyle(e).getPropertyValue(p)])), names);

const focused = (page: Page) => page.locator(":focus");

async function home(page: Page) {
  await page.goto("/");
  await expectFocus(page, { key: /^card:continueWatching:/ });
}

test("a focused card lifts its artwork alone, and the caption keeps its size", async ({ page }) => {
  await home(page);
  const card = focused(page);
  const ring = card.locator("[data-tv-ring]");
  const caption = card.locator("[data-tv-ring] ~ *").first();
  await expect.poll(async () => (await styleOf(ring, "scale")).scale).toBe("1.1");
  expect((await styleOf(card, "scale")).scale).toBe("none");
  // Unscaled, the caption's box is its layout width.
  const { box, layout } = await caption.evaluate((e) => ({
    box: e.getBoundingClientRect().width,
    layout: (e as HTMLElement).offsetWidth,
  }));
  expect(box).toBeCloseTo(layout, 0);
});

test("only scale, opacity and translate ease as focus moves: no shadow, background or filter transitions", async ({
  page,
}) => {
  await home(page);
  const card = focused(page);
  const ring = card.locator("[data-tv-ring]");
  for (const el of [card, ring]) {
    const style = await styleOf(el, "transition-property", "transition-duration");
    const props = style["transition-property"]?.split(", ") ?? [];
    const durations = style["transition-duration"]?.split(", ") ?? [];
    // A transition lasting no time, such as the initial `all 0s`, is none.
    const eased = props.filter((_, i) => Number.parseFloat(durations[i % durations.length] ?? "0") > 0);
    expect(eased.join(", ")).not.toMatch(/box-shadow|filter|background-color|all/);
  }
  expect((await styleOf(ring, "filter")).filter).toBe("none");
});

test("the focused first card's lift and glow have room in its row, at the top and on the left", async ({ page }) => {
  await home(page);
  const ring = focused(page).locator("[data-tv-ring]");
  await expect.poll(async () => (await styleOf(ring, "scale")).scale).toBe("1.1");
  // The element that clips the row, around its cards' track.
  const row = focused(page).locator("xpath=ancestor::ul[1]/..");
  const [r, u] = await Promise.all([ring.boundingBox(), row.boundingBox()]);
  expect(r && u).toBeTruthy();
  if (!r || !u) return;
  // The glow reaches 44 px past the artwork, which is already grown: its
  // spread and its blur, all of which the row leaves room for, so its edge
  // isn't cut off straight.
  expect(r.y - u.y).toBeGreaterThanOrEqual(44);
  expect(r.x - u.x).toBeGreaterThanOrEqual(44);
});

/**
 * The focused card's artwork as it shows, grown, and its neighbours' in the
 * row, with how far the glow around it reaches: its spread and half its
 * blur, past which it's too faint to see.
 */
const lifted = (page: Page) =>
  page.evaluate(() => {
    const ring = document.activeElement?.querySelector("[data-tv-ring]");
    const li = ring?.closest("li");
    const box = (el: Element | null | undefined) => {
      const r = el?.querySelector("[data-tv-ring]")?.getBoundingClientRect();
      return r && { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
    };
    const glow = (ring && getComputedStyle(ring, "::after").boxShadow) ?? "";
    // The glow is the shadow with no offset: "rgba(…) 0px 0px 40px 4px".
    const [blur, spread] = (/\) 0px 0px (\d+)px (\d+)px/.exec(glow) ?? []).slice(1).map(Number);
    return {
      ring: ring && {
        ...box(li),
        width: ring.getBoundingClientRect().width,
        unscaled: (ring as HTMLElement).offsetWidth,
      },
      prev: box(li?.previousElementSibling),
      next: box(li?.nextElementSibling),
      reach: (spread ?? Number.NaN) + (blur ?? Number.NaN) / 2,
    };
  });

async function expectClearOfNeighbours(page: Page) {
  const ring = focused(page).locator("[data-tv-ring]");
  await expect.poll(async () => (await styleOf(ring, "scale")).scale).toBe("1.1");
  // The lift has finished growing the artwork.
  await expect
    .poll(async () => {
      const l = await lifted(page);
      return Math.abs((l.ring?.width ?? 0) - (l.ring?.unscaled ?? 0) * 1.1);
    })
    .toBeLessThan(1);
  const { ring: r, prev, next, reach } = await lifted(page);
  expect(reach, "the glow's reach").toBeGreaterThan(0);
  expect(r).toBeTruthy();
  if (!r) return;
  expect(prev ?? next, "a card beside it").toBeTruthy();
  if (next) expect(next.left - (r.right ?? 0), "air right of the glow").toBeGreaterThanOrEqual(reach);
  if (prev) expect((r.left ?? 0) - prev.right, "air left of the glow").toBeGreaterThanOrEqual(reach);
}

test("a focused landscape card's lift and glow stay clear of the cards beside it", async ({ page }) => {
  await home(page);
  await press(page, "ArrowRight");
  await expectFocus(page, { key: /^card:continueWatching:/ });
  await expectClearOfNeighbours(page);
});

test("a focused poster card's lift and glow stay clear of the cards beside it", async ({ page }) => {
  await home(page);
  await pressUntil(page, "ArrowDown", (f) => /^card:/.test(f.key ?? "") && !/continueWatching/.test(f.key ?? ""));
  await press(page, "ArrowRight");
  await page.waitForTimeout(300);
  const width = await focused(page)
    .locator("[data-tv-ring]")
    .evaluate((e) => (e as HTMLElement).offsetWidth);
  expect(width, "a poster").toBeLessThan(400);
  await expectClearOfNeighbours(page);
});

test("every card's artwork has the same rounded corners, with or without a progress bar, focused or not", async ({
  page,
}) => {
  await home(page);
  const corners = () =>
    page.evaluate(() =>
      [...document.querySelectorAll('[data-part="row"] li [data-tv-ring]')].slice(0, 12).map((ring) => {
        const bar = ring.querySelector('[data-part="progress"]');
        const art = ring.firstElementChild as HTMLElement;
        // The element that clips the bar: the nearest ancestor that hides overflow.
        let clip = bar?.parentElement ?? null;
        while (clip && clip !== ring && getComputedStyle(clip).overflow === "visible") clip = clip.parentElement;
        const a = art.getBoundingClientRect();
        const b = bar?.getBoundingClientRect();
        // Just inside the artwork's bottom corners, which a rounded corner leaves empty.
        const corner = (x: number) => {
          const hit = document.elementFromPoint(x, a.bottom - 1.5);
          return hit ? hit.closest('[data-part="progress"]') !== null : false;
        };
        const onScreen = a.top >= 0 && a.bottom <= window.innerHeight && a.left >= 0 && a.right <= window.innerWidth;
        return {
          radius: [getComputedStyle(art).borderBottomLeftRadius, getComputedStyle(art).borderBottomRightRadius],
          ring: getComputedStyle(ring).borderRadius,
          bar: bar
            ? {
                clippedByArtwork: clip === art,
                inside: b ? b.left >= a.left - 0.5 && b.right <= a.right + 0.5 && b.bottom <= a.bottom + 0.5 : false,
                cornerShowsBar: onScreen ? corner(a.left + 1.5) || corner(a.right - 1.5) : false,
                middleShowsBar: onScreen ? corner(a.left + a.width / 2) : true,
              }
            : undefined,
        };
      }),
    );
  const check = async () => {
    const cards = await corners();
    const withBar = cards.filter((c) => c.bar);
    expect(withBar.length, "cards with progress").toBeGreaterThan(0);
    expect(cards.length - withBar.length, "cards without progress").toBeGreaterThan(0);
    const radius = cards[0]?.radius[0];
    expect(Number.parseFloat(radius ?? "0")).toBeGreaterThan(0);
    for (const c of cards) {
      expect(c.radius).toEqual([radius, radius]);
      expect(c.ring, "the glow's corners").toBe(radius);
      if (c.bar)
        expect(c.bar).toEqual({ clippedByArtwork: true, inside: true, cornerShowsBar: false, middleShowsBar: true });
    }
  };
  // The first card, focused, has progress, as the one beside it does; the
  // last in Continue Watching has none, and is focused next.
  await check();
  await press(page, "ArrowRight", "ArrowRight");
  await page.waitForTimeout(400);
  expect(await focused(page).locator('[data-part="progress"]').count(), "a focused card without progress").toBe(0);
  await check();
});

test("a focused button turns white with dark text on itself, not on its wrapper", async ({ page }) => {
  await home(page);
  await press(page, "ArrowDown", "ArrowDown", "Enter");
  await expectFocus(page, { key: "play" });
  const wrapper = focused(page);
  const play = page.locator('[data-part="play"]');
  await expect.poll(async () => (await styleOf(play, "background-color"))["background-color"]).toBe(white);
  expect((await styleOf(wrapper, "box-shadow"))["box-shadow"]).toBe("none");
  // Play is the accent colour while something else has focus.
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "favorite" });
  await expect.poll(async () => (await styleOf(play, "background-color"))["background-color"]).not.toBe(white);
});

test("Sign in's focus is the size of the button, and a field's is on its input, not around its label", async ({
  page,
}) => {
  await page.goto("/?mock=signedOut");
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });
  await press(page, "Enter");
  await expectFocus(page, { key: "signin:username" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "signin:password" });
  const field = focused(page);
  const input = field.locator("input");
  await expect.poll(async () => (await styleOf(input, "background-color"))["background-color"]).toBe(white);
  expect((await styleOf(field, "box-shadow"))["box-shadow"]).toBe("none");
  await press(page, "ArrowDown");
  await expectFocus(page, { name: "Sign in" });
  const submit = focused(page).getByRole("button", { name: "Sign in" });
  await expect.poll(async () => (await styleOf(submit, "background-color"))["background-color"]).toBe(white);
  expect((await styleOf(focused(page), "box-shadow"))["box-shadow"]).toBe("none");
  // It's as wide as the fields above it, as is its focus stop, so the white is the button's own.
  const width = (el: Element) => (el as HTMLElement).offsetWidth;
  const fieldWidth = await page.getByLabel("Password").evaluate(width);
  expect(await submit.evaluate(width)).toBe(fieldWidth);
  expect(await focused(page).evaluate(width)).toBe(fieldWidth);
});

test("a focused key on the on-screen keyboard turns white", async ({ page }) => {
  await home(page);
  await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
  await pressUntil(page, "ArrowUp", (f) => f.name === "Search");
  await press(page, "Enter");
  await expectFocus(page, { part: "tv-key" });
  const key = focused(page).getByRole("button");
  await expect.poll(async () => (await styleOf(key, "background-color"))["background-color"]).toBe(white);
});

test("the focused tab looks different from the selected one", async ({ page }) => {
  await home(page);
  await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
  await pressUntil(page, "ArrowDown", (f) => f.name === "Movies");
  await press(page, "Enter");
  await expectFocus(page, { key: /^card:grid:/ });
  await press(page, "ArrowUp", "ArrowUp");
  await expectFocus(page, { key: "section:library" });
  const selected = page.getByRole("button", { name: "Movies", exact: true });
  const whenFocused = (await styleOf(selected, "background-color"))["background-color"];
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "section:collections" });
  const whenNot = (await styleOf(selected, "background-color"))["background-color"];
  expect(whenFocused).toBe(white);
  expect(whenNot).not.toBe(white);
});

test("a dialog's destructive button isn't filled until it has focus, then is red on white", async ({ page }) => {
  await page.goto("/settings");
  await expectFocus(page, { key: "settings:section:account" });
  await press(page, "ArrowRight");
  await expectFocus(page, { name: /Sign out/ });
  await press(page, "Enter");
  await expectFocus(page, { key: "tv:confirm:cancel" });
  const yes = page.locator('[data-tv-focus-key="tv:confirm:yes"]');
  const cancel = page.locator('[data-tv-focus-key="tv:confirm:cancel"]');
  const restYes = await styleOf(yes, "background-color");
  const restCancel = await styleOf(cancel, "background-color");
  expect((await styleOf(cancel, "background-color"))["background-color"]).toBe(white);
  // Not focused, Sign out has the same fill as an unfocused Cancel would: none of its own.
  expect(restYes["background-color"]).not.toBe(white);
  expect(restYes["background-color"]).not.toBe("rgb(240, 101, 107)");
  expect(restCancel["background-color"]).toBe(white);
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "tv:confirm:yes" });
  const focusedYes = await styleOf(yes, "background-color", "color");
  expect(focusedYes["background-color"]).toBe(white);
  expect(focusedYes.color).toBe("rgb(201, 52, 59)");
});

test("a setting's whole row takes focus and lights up, and Down goes to the next setting", async ({ page }) => {
  await page.goto("/settings");
  await expectFocus(page, { key: "settings:section:account" });
  await pressUntil(page, "ArrowDown", (f) => f.key === "settings:section:playback");
  await press(page, "ArrowRight");
  await expectFocus(page, { name: /Play next episode automatically/ });
  await expect.poll(async () => (await styleOf(focused(page), "background-color"))["background-color"]).toBe(white);
  await press(page, "ArrowDown");
  await expectFocus(page, { name: /Remember choices per show/ });
  expect((await focusOf(page)).key).toMatch(/^setting:/);
});

test("a focused episode card lifts its still, as other cards do", async ({ page }) => {
  await home(page);
  // The Expanse, among Favorites: Down lands on whichever card is nearest.
  await press(page, "ArrowDown");
  await pressUntil(page, "ArrowRight", (f) => f.name?.includes("The Expanse") ?? false);
  await press(page, "Enter");
  await expectFocus(page, { key: "play" });
  await pressUntil(page, "ArrowDown", (f) => f.key?.startsWith("card:episodes:") ?? false);
  const ring = focused(page).locator("[data-tv-ring]");
  await expect.poll(async () => (await styleOf(ring, "scale")).scale).toBe("1.1");
});

test("with reduced motion, focus changes at once: the lift and the glow don't ease", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await home(page);
  const ring = focused(page).locator("[data-tv-ring]");
  expect((await styleOf(ring, "transition-duration"))["transition-duration"]).toBe("0s");
  const glow = await ring.evaluate((e) => getComputedStyle(e, "::after").transitionDuration);
  expect(glow).toBe("0s");
});

test("a detail page's version, audio and subtitle pickers light up when focused", async ({ page }) => {
  // Dune: Part Two, which has more than one version and track to choose.
  await page.goto("/works/0190a000-0000-7000-8000-000000000065");
  await expectFocus(page, { key: "play" });
  for (const key of ["version", "subtitle"]) {
    await pressUntil(page, "ArrowDown", (f) => f.key === key);
    // The chosen value's button, what a remote sees, not a hidden part of the select.
    const face = page.locator(`[data-tv-focus-key="${key}"] button`).first();
    await expect.poll(async () => (await styleOf(face, "background-color"))["background-color"]).toBe(white);
  }
});
