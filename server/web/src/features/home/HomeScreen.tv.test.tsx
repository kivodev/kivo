import { fireEvent, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { locationOf, renderApp } from "@/test/render";
import { serveMockApi } from "@/test/server";
import { stopTv } from "@/tv/start";

// jsdom lays nothing out, so each card gets a place on a 1080p screen from
// its row and its place in the row: rows 600 apart, cards 260 apart.
function layOut() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
    const el = this as HTMLElement;
    let x = 0;
    let y = 0;
    let width = 1920;
    let height = 1080;
    if (el.dataset.tvFocusKey?.startsWith("card:")) {
      const li = el.closest("li");
      const rows = [...document.querySelectorAll("[data-row]")];
      const row = rows.indexOf(el.closest("[data-row]") as Element);
      const column = [...(li?.parentElement?.children ?? [])].indexOf(li as Element);
      x = 250 + column * 260;
      y = 100 + row * 600;
      width = 250;
      height = 400;
    }
    return { x, y, left: x, top: y, width, height, right: x + width, bottom: y + height, toJSON: () => ({}) };
  });
}

const focused = () => document.activeElement?.getAttribute("data-tv-focus-key");
const press = (key: string) => {
  fireEvent.keyDown(document.activeElement ?? window, { key });
  fireEvent.keyUp(document.activeElement ?? window, { key });
};
const back = () => fireEvent.keyDown(window, { key: "XF86Back" });

describe("Home on TV", () => {
  beforeEach(() => {
    layOut();
  });
  afterEach(() => {
    stopTv();
  });

  it("starts on the first card of the first row, and moves along a row with Right and between rows with Down", async () => {
    serveMockApi();
    renderApp("/");
    await waitFor(() => expect(focused()).toMatch(/^card:continueWatching:/));
    const first = focused();
    press("ArrowRight");
    await waitFor(() => expect(focused()).toMatch(/^card:continueWatching:/));
    expect(focused()).not.toBe(first);
    press("ArrowDown");
    // The second row is My List, whose cards end in See all.
    await waitFor(() => expect(focused()).toMatch(/^card:list:/));
    press("ArrowUp");
    await waitFor(() => expect(focused()).toMatch(/^card:continueWatching:/));
  });

  it("moves with Right along a long row to its last card", async () => {
    serveMockApi();
    renderApp("/");
    await waitFor(() => expect(focused()).toMatch(/^card:continueWatching:/));
    // The third row holds twelve movies, more than the six that fit across.
    const movies = document.querySelectorAll('[data-part="row-cards"]')[2] as HTMLElement;
    const cards = () =>
      [...movies.querySelectorAll<HTMLElement>("[data-tv-focus-key]")].map((c) => c.dataset.tvFocusKey);
    await waitFor(() => expect(cards().at(-1)).toMatch(/:see-all$/));
    press("ArrowDown");
    await waitFor(() => expect(focused()).toMatch(/^card:list:/));
    press("ArrowDown");
    await waitFor(() => expect(focused()).toBe(cards()[0]));
    for (const card of cards().slice(1, 12)) {
      press("ArrowRight");
      await waitFor(() => expect(focused()).toBe(card));
    }
  });

  it("opens a card's page with OK, and Back returns focus to that card", async () => {
    serveMockApi();
    const { router } = renderApp("/");
    await waitFor(() => expect(focused()).toMatch(/^card:continueWatching:/));
    press("ArrowRight");
    await waitFor(() => expect(focused()).not.toBeNull());
    const card = focused();
    press("Enter");
    await waitFor(() => expect(locationOf(router)).toMatch(/^\/works\//));
    // The page's Play button has focus.
    await waitFor(() => expect(focused()).toBe("play"));
    back();
    await waitFor(() => expect(locationOf(router)).toBe("/"));
    await waitFor(() => expect(focused()).toBe(card));
  });

  it("shows a long row's link as its last card and no hover controls on cards", async () => {
    serveMockApi();
    renderApp("/");
    await waitFor(() => expect(focused()).toMatch(/^card:/));
    // A row longer than the screen ends in See all, once all its cards are
    // in; a row of two shows all there is.
    const long = [...document.querySelectorAll<HTMLElement>('[data-row="recentlyAdded"]')].find(
      (r) => r.querySelectorAll("li:not([data-tv-glow-host])").length > 6,
    );
    await waitFor(() => expect(long?.querySelector('[data-part="see-all"]')).not.toBeNull());
    const myList = document.querySelector('[data-row="list"]') as HTMLElement;
    expect(myList.querySelectorAll("li:not([data-tv-glow-host])")).toHaveLength(2);
    expect(myList.querySelector('[data-part="see-all"]')).toBeNull();
    expect(document.querySelector('[data-part="card-play"]')).toBeNull();
    expect(document.querySelector('button[aria-haspopup="menu"]')).toBeNull();
  });
});
