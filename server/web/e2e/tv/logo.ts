import { expect, type Page } from "@playwright/test";

// The logos a TV shows as the app starts: the splash's icon, from the
// page's first frame until the first screen, and then the screen's own
// logo. Each is recorded every animation frame from the first, so a logo
// that shows at one size and then jumps to another, as the splash did
// while the TV scale arrived with the app's code, fails.

type Seen = { box: string; loaded: boolean };
type Frames = { splash: Seen[]; logo: Seen[] };

/** Records the logos' boxes each frame from the page's start. Call it before opening the page. */
export async function recordLogos(page: Page) {
  // At a TV's pace, as slow as its CPU, so the page paints before its
  // code has run, as it does on the TV.
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 6 });
  await page.addInitScript(() => {
    const frames: Frames = { splash: [], logo: [] };
    Object.assign(window, { kivoLogoFrames: frames });
    const seen = (img: HTMLImageElement | null): Seen | undefined => {
      if (!img || getComputedStyle(img).display === "none") return undefined;
      const r = img.getBoundingClientRect();
      const box = [r.x, r.y, r.width, r.height].map((n) => Math.round(n)).join(",");
      return { box, loaded: img.complete && img.naturalWidth > 0 };
    };
    const sample = () => {
      const splash = seen(document.querySelector<HTMLImageElement>('[data-part="tv-splash"] img'));
      const logo = [...document.querySelectorAll<HTMLImageElement>('[data-part="logo"] img')]
        .map(seen)
        .find((s) => s !== undefined);
      if (splash) frames.splash.push(splash);
      if (logo) frames.logo.push(logo);
      requestAnimationFrame(sample);
    };
    requestAnimationFrame(sample);
  });
}

/**
 * Checks what recordLogos saw once the first screen has shown: the splash
 * showed, its icon kept one size and place throughout, and the screen's
 * logo kept its own from its first frame, its drawing loaded.
 */
export async function expectSteadyLogos(page: Page) {
  // A few frames more, for anything that would still move.
  await page.evaluate(() => new Promise((r) => setTimeout(r, 300)));
  const frames = await page.evaluate(() => (window as unknown as { kivoLogoFrames: Frames }).kivoLogoFrames);
  expect(frames.splash.length, "frames with the splash").toBeGreaterThan(0);
  expect(new Set(frames.splash.map((s) => s.box)), "the splash icon's boxes").toEqual(new Set([frames.splash[0]?.box]));
  expect(frames.logo.length, "frames with the screen's logo").toBeGreaterThan(0);
  expect(new Set(frames.logo.map((s) => s.box)), "the screen logo's boxes").toEqual(new Set([frames.logo[0]?.box]));
  expect(frames.logo.at(-1)?.loaded, "the screen's logo loaded").toBe(true);
  expect(frames.splash.at(-1)?.loaded, "the splash icon loaded").toBe(true);
}
