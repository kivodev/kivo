import { fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockId, mockIds, serveMockApi } from "@/test/server";
import { focusTv } from "@/tv/spatial";
import { stopTv } from "@/tv/start";

const focused = () => document.activeElement?.getAttribute("data-tv-focus-key");
const press = (key: string) => {
  fireEvent.keyDown(document.activeElement ?? window, { key });
  fireEvent.keyUp(document.activeElement ?? window, { key });
};

// jsdom lays nothing out, so the hero's buttons sit in a row and the
// season strip and episodes below, by focus key.
function layOut() {
  const places: Record<string, [number, number, number, number]> = {
    play: [250, 400, 250, 70],
    restart: [520, 400, 70, 70],
    "my-list": [600, 400, 70, 70],
    played: [680, 400, 70, 70],
    more: [760, 400, 70, 70],
  };
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
    const el = this as HTMLElement;
    const [x, y, width, height] = places[el.dataset.tvFocusKey ?? ""] ?? [0, 0, 1920, 1080];
    return { x, y, left: x, top: y, width, height, right: x + width, bottom: y + height, toJSON: () => ({}) };
  });
}

describe("Detail page on TV", () => {
  beforeEach(() => {
    layOut();
  });
  afterEach(() => {
    stopTv();
  });

  it("focuses Play first, and Right reaches the My List and watched buttons and the menu", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.dune}`);
    await waitFor(() => expect(focused()).toBe("play"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("my-list"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("played"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("more"));
  });

  it("opens the menu with OK, without the admin actions that are web-only, and Back closes it and returns to the button", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.dune}`);
    await waitFor(() => expect(focused()).toBe("play"));
    for (const key of ["my-list", "played", "more"]) {
      press("ArrowRight");
      await waitFor(() => expect(focused()).toBe(key));
    }
    press("Enter");
    const menu = await screen.findByRole("menu");
    expect(within(menu).getByRole("menuitem", { name: "Go to library" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /Fix match/ })).not.toBeInTheDocument();
    fireEvent.keyDown(window, { key: "XF86Back" });
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
    await waitFor(() => expect(focused()).toBe("more"));
  });

  it("offers Play from beginning beside Resume once there's progress, and plays from the start with OK", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await waitFor(() => expect(focused()).toBe("play"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("restart"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("my-list"));
    press("ArrowLeft");
    await waitFor(() => expect(focused()).toBe("restart"));
    press("Enter");
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${mockIds.dunePartTwo}`));
    expect(router.state.location.search).toMatchObject({ from: "start" });
  });

  it("shows a series' open season as a row of episode cards under its heading and episode count", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.expanse}`);
    await waitFor(() => expect(focused()).toBe("play"));
    const shelf = await waitFor(() => {
      const row = document.querySelector<HTMLElement>('[data-row="episodes"]');
      expect(row?.querySelectorAll('[data-part="episode-card"]')).toHaveLength(10);
      return row as HTMLElement;
    });
    expect(within(shelf).getByRole("heading", { name: "Season 3" })).toBeInTheDocument();
    expect(within(shelf).getByText("10 episodes")).toBeInTheDocument();
    // The number and title, then the time left in one in progress, or the runtime before the synopsis.
    const cards = [...shelf.querySelectorAll('[data-part="episode-card"]')];
    expect(cards[2]).toHaveTextContent("3. Episode 314 min left");
    expect(cards[3]).toHaveTextContent("4. Reload44m · Holden races to stop an asteroid before it reaches Earth.");
    // A card's one link is the episode's page; its still's OK plays by the row's key handler.
    expect(within(cards[3] as HTMLElement).getByRole("link")).toHaveAttribute(
      "href",
      `/works/${mockIds.expanseReload}`,
    );
    expect(shelf.querySelector('[data-part="episode-play"]')).toBeNull();
    // Up or Down enters the row on Next Up.
    expect(shelf.querySelector(`[data-tv-group="row:episodes"]`)).toHaveAttribute(
      "data-tv-enter",
      `card:episodes:${mockId(1303)}`,
    );
  });

  it("plays an episode with OK on its still", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.expanse}`);
    await waitFor(() => expect(focused()).toBe("play"));
    const card = `card:episodes:${mockIds.expanseReload}`;
    await waitFor(() => expect(document.querySelector(`[data-tv-focus-key="${card}"]`)).not.toBeNull());
    focusTv(card);
    await waitFor(() => expect(focused()).toBe(card));
    press("Enter");
    await waitFor(() => expect(locationOf(router)).toBe(`/watch/${mockIds.expanseReload}`));
  });

  it("plays the focused episode with the remote's Play button, from either stop", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.expanse}`);
    await waitFor(() => expect(focused()).toBe("play"));
    const card = `card:episodes:${mockIds.expanseReload}`;
    await waitFor(() => expect(document.querySelector(`[data-tv-focus-key="${card}"]`)).not.toBeNull());
    focusTv(card);
    await waitFor(() => expect(focused()).toBe(card));
    press("ArrowDown");
    await waitFor(() => expect(document.querySelector("[data-tv-card-text]")).not.toBeNull());
    fireEvent.keyDown(document.activeElement as Element, { key: "MediaPlayPause", keyCode: 10252 });
    await waitFor(() => expect(locationOf(router)).toBe(`/watch/${mockIds.expanseReload}`));
  });

  it("opens an episode's page with OK on its text, with Play focused, and Back returns to the card", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.expanse}`);
    await waitFor(() => expect(focused()).toBe("play"));
    const card = `card:episodes:${mockIds.expanseReload}`;
    await waitFor(() => expect(document.querySelector(`[data-tv-focus-key="${card}"]`)).not.toBeNull());
    focusTv(card);
    await waitFor(() => expect(focused()).toBe(card));
    press("ArrowDown");
    await waitFor(() => expect(document.querySelector("[data-tv-card-text]")).not.toBeNull());
    press("Enter");
    await waitFor(() => expect(locationOf(router)).toBe(`/works/${mockIds.expanseReload}`));
    await waitFor(() => expect(focused()).toBe("play"));
    // The episode's own title heads it, with its code first in the details.
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(/^Reload$/);
    expect(document.querySelector('[data-part="detail-header"]')).toHaveTextContent(/S3 · E4 · .+ · 44m/);
    // The rest of its season, opening on it with its title in the accent.
    const current = document.querySelector('[data-row="episodes"] [data-current]');
    expect(current).toHaveTextContent("4. Reload");
    expect(document.querySelector(`[data-tv-group="row:episodes"]`)).toHaveAttribute(
      "data-tv-enter",
      `card:episodes:${mockIds.expanseReload}`,
    );
    fireEvent.keyDown(window, { key: "XF86Back" });
    await waitFor(() => expect(locationOf(router)).toBe(`/works/${mockIds.expanse}`));
    await waitFor(() => expect(focused()).toBe(card));
  });

  it("opens the next episode's page from the season's row with its Play focused, though the card's key is on both pages", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.expanseReload}`);
    await waitFor(() => expect(focused()).toBe("play"));
    const next = `card:episodes:${mockId(1305)}`;
    await waitFor(() => expect(document.querySelector(`[data-tv-focus-key="${next}"]`)).not.toBeNull());
    focusTv(next);
    await waitFor(() => expect(focused()).toBe(next));
    press("ArrowDown");
    await waitFor(() => expect(document.querySelector("[data-tv-card-text]")).not.toBeNull());
    press("Enter");
    await waitFor(() => expect(locationOf(router)).toBe(`/works/${mockId(1305)}`));
    await waitFor(() => expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(/^Episode 5$/));
    await waitFor(() => expect(focused()).toBe("play"));
  });

  it("opens Play's link with OK", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.dune}`);
    await waitFor(() => expect(focused()).toBe("play"));
    press("Enter");
    await waitFor(() => expect(locationOf(router)).toMatch(/^\/watch\//));
  });
});
