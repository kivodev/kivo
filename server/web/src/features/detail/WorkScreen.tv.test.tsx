import { fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockIds, serveMockApi } from "@/test/server";
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
    favorite: [600, 400, 70, 70],
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

  it("focuses Play first, and Right reaches the favorite and watched buttons and the menu", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.dune}`);
    await waitFor(() => expect(focused()).toBe("play"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("favorite"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("played"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("more"));
  });

  it("opens the menu with OK, without the admin actions that are web-only, and Back closes it and returns to the button", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.dune}`);
    await waitFor(() => expect(focused()).toBe("play"));
    for (const key of ["favorite", "played", "more"]) {
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
    await waitFor(() => expect(focused()).toBe("favorite"));
    press("ArrowLeft");
    await waitFor(() => expect(focused()).toBe("restart"));
    press("Enter");
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${mockIds.dunePartTwo}`));
    expect(router.state.location.search).toMatchObject({ from: "start" });
  });

  it("opens Play's link with OK", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.dune}`);
    await waitFor(() => expect(focused()).toBe("play"));
    press("Enter");
    await waitFor(() => expect(locationOf(router)).toMatch(/^\/watch\//));
  });
});
