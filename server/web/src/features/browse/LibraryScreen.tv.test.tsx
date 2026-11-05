import { fireEvent, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { locationOf, renderApp } from "@/test/render";
import { mockIds, serveMockApi } from "@/test/server";
import { stopTv } from "@/tv/start";

const movies = `/libraries/${mockIds.movies}`;

// jsdom lays nothing out, so the grid's cards sit in six columns by their
// index, below the header's controls.
function layOut() {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
    const el = this as HTMLElement;
    let x = 0;
    let y = 0;
    let width = 1920;
    let height = 1080;
    const cell = el.closest("[data-index]");
    if (el.dataset.tvFocusKey?.startsWith("card:") && cell) {
      const index = Number(cell.getAttribute("data-index"));
      x = 250 + (index % 6) * 260;
      y = 400 + Math.floor(index / 6) * 500;
      width = 250;
      height = 450;
    } else if (el.dataset.tvGroup === "grid") {
      x = 250;
      y = 400;
      width = 1560;
      height = 3000;
    } else if (el.dataset.tvFocusKey === "letter-rail") {
      x = 1850;
      y = 288;
      width = 64;
      height = 700;
    } else if (el.dataset.tvFocusKey === "sort" || el.dataset.tvFocusKey === "filter") {
      x = el.dataset.tvFocusKey === "sort" ? 250 : 1600;
      y = 200;
      width = 150;
      height = 60;
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
const indexOfFocused = () => Number(document.activeElement?.closest("[data-index]")?.getAttribute("data-index"));

describe("Library grid on TV", () => {
  beforeEach(() => {
    layOut();
  });
  afterEach(() => {
    stopTv();
  });

  it("lays out six posters across and focuses the first card, with no list view", async () => {
    serveMockApi();
    renderApp(movies);
    await waitFor(() => expect(focused()).toMatch(/^card:grid:/));
    expect(indexOfFocused()).toBe(0);
    const firstRow = document.querySelector('[data-part="grid"] > div:not([data-part])') as HTMLElement;
    expect(firstRow.children).toHaveLength(6);
    expect(document.querySelector('[data-part="letter-rail"]')).not.toBeNull();
    expect(document.querySelector('button[aria-label="List"]')).toBeNull();
  });

  it("moves between cards by index, a row for Up and Down, and Up from the first row reaches the header", async () => {
    serveMockApi();
    renderApp(movies);
    await waitFor(() => expect(focused()).toMatch(/^card:grid:/));
    press("ArrowRight");
    await waitFor(() => expect(indexOfFocused()).toBe(1));
    press("ArrowDown");
    await waitFor(() => expect(indexOfFocused()).toBe(7));
    press("ArrowDown");
    await waitFor(() => expect(indexOfFocused()).toBe(13));
    press("ArrowUp");
    await waitFor(() => expect(indexOfFocused()).toBe(7));
    press("ArrowUp");
    await waitFor(() => expect(indexOfFocused()).toBe(1));
    press("ArrowUp");
    await waitFor(() => expect(focused()).toBe("sort"));
  });

  it("opens a card with OK and Back returns focus to the same card", async () => {
    serveMockApi();
    const { router } = renderApp(movies);
    await waitFor(() => expect(focused()).toMatch(/^card:grid:/));
    press("ArrowDown");
    press("ArrowRight");
    await waitFor(() => expect(indexOfFocused()).toBe(7));
    const card = focused();
    press("Enter");
    await waitFor(() => expect(locationOf(router)).toMatch(/^\/works\//));
    back();
    await waitFor(() => expect(locationOf(router)).toBe(movies));
    await waitFor(() => expect(focused()).toBe(card));
  });

  it("reaches the letter rail with Right from the last column, picks letters with Up and Down, and Left returns to the grid", async () => {
    serveMockApi();
    renderApp(movies);
    await waitFor(() => expect(focused()).toMatch(/^card:grid:/));
    for (let i = 0; i < 5; i++) press("ArrowRight");
    await waitFor(() => expect(indexOfFocused()).toBe(5));
    // The last column's Right is left to spatial navigation, which finds the rail at the right.
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("letter-rail"));
    // Up and Down pick a letter that has works, which the rail marks.
    const picked = () => document.querySelector("[data-picked]")?.textContent;
    await waitFor(() => expect(picked()).toBe("A"));
    press("ArrowDown");
    press("ArrowDown");
    await waitFor(() => expect(picked()).not.toBe("A"));
    press("ArrowUp");
    await waitFor(() => expect(picked()).not.toBeUndefined());
    // Left goes back to the grid.
    press("ArrowLeft");
    await waitFor(() => expect(focused()).toMatch(/^card:grid:/));
  });
});
