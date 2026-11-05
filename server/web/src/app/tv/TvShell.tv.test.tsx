import { fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { bookIds } from "@/api/mocks/books";
import { mockIds, scenarios } from "@/api/mocks/fixtures";
import { sendEvent } from "@/api/mocks/handlers";
import { locationOf, renderApp, screen } from "@/test/render";
import { http, problem, serveMockApi, server } from "@/test/server";
import { focusTv, isTvPaused } from "@/tv/spatial";
import { stopTv } from "@/tv/start";
import { railKeys } from "./TvRail";

const back = () => fireEvent.keyDown(window, { key: "XF86Back" });
const railOpen = () => document.querySelector('[data-part="tv-rail-scrim"]') !== null;
const rail = () => screen.findByRole("navigation", { name: "Main" });

describe("TV shell", () => {
  afterEach(() => {
    stopTv();
    vi.unstubAllGlobals();
  });

  it("has the rail in place of the sidebar: Search, Home, Favorites, the pinned libraries, More libraries and Settings, and no Admin", async () => {
    serveMockApi(scenarios.unpinned);
    renderApp("/");
    const nav = await rail();
    expect(document.documentElement).toHaveAttribute("data-tv");
    const entries = () => [...nav.querySelectorAll("a, button")].map((e) => e.textContent);
    // Closed, it's a quiet column: the four destinations, and no libraries.
    expect(railOpen()).toBe(false);
    expect(entries()).toEqual(["Search", "Home", "Favorites", "Settings"]);
    expect(within(nav).getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
    // Open, every pinned library is there, with More libraries for the rest.
    focusTv(railKeys.home);
    await waitFor(() => expect(railOpen()).toBe(true));
    const open = entries();
    expect(open.slice(0, 3)).toEqual(["Search", "Home", "Favorites"]);
    expect(open.at(-2)).toBe("More libraries");
    expect(open.at(-1)).toBe("Settings");
    expect(open).toContain("Movies");
    expect(open).not.toContain("Home Videos");
    expect(screen.queryByRole("link", { name: /Admin/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Account menu/ })).not.toBeInTheDocument();
  });

  it("keeps the library the screen is in on the closed rail, dimming the rest", async () => {
    serveMockApi();
    renderApp(`/libraries/${mockIds.movies}`);
    const nav = await rail();
    const movies = await within(nav).findByRole("link", { name: "Movies" });
    expect(movies).toHaveAttribute("aria-current", "page");
    expect(movies).not.toHaveClass("opacity-40");
    expect(within(nav).getByRole("link", { name: "Home" })).toHaveClass("opacity-40");
    expect(railOpen()).toBe(false);
  });

  it("opens the rail on a destination with Back, focused on that destination, and a second Back goes Home", async () => {
    serveMockApi();
    const { router } = renderApp("/search");
    const nav = await rail();
    // Search is lazy, and takes focus as it mounts.
    await screen.findByRole("searchbox", { name: "Search" });
    back();
    const search = within(nav).getByRole("link", { name: "Search" });
    await waitFor(() => expect(search).toHaveFocus());
    await waitFor(() => expect(railOpen()).toBe(true));
    back();
    await waitFor(() => expect(locationOf(router)).toBe("/"));
  });

  it("marks More libraries on a library that isn't pinned, and opens the rest of the libraries under it", async () => {
    serveMockApi(scenarios.unpinned);
    renderApp(`/libraries/${mockIds.videos}`);
    const nav = await rail();
    const more = within(nav).getByRole("button", { name: "More libraries" });
    expect(more).toHaveAttribute("aria-current", "page");
    back();
    await waitFor(() => expect(more).toHaveFocus());
    fireEvent.keyDown(more, { key: "Enter" });
    fireEvent.keyUp(more, { key: "Enter" });
    expect(await within(nav).findByRole("link", { name: "Home Videos" })).toBeInTheDocument();
  });

  it("marks the library a work is in on its page, and Back there still goes to the page below", async () => {
    serveMockApi();
    const { router } = renderApp("/");
    const nav = await rail();
    await router.navigate({ to: "/works/$workId", params: { workId: mockIds.dunePartTwo } });
    await screen.findByRole("heading", { name: "Dune: Part Two", level: 1 });
    await waitFor(() =>
      expect(within(nav).getByRole("link", { name: "Movies" })).toHaveAttribute("aria-current", "page"),
    );
    expect(within(nav).getByRole("link", { name: "Home" })).not.toHaveAttribute("aria-current");
    await waitFor(() => expect(document.activeElement?.getAttribute("data-tv-focus-key")).toBe("play"));
    back();
    await waitFor(() => expect(locationOf(router)).toBe("/"));
  });

  it("marks a book's library from its page", async () => {
    serveMockApi();
    renderApp(`/works/${bookIds.hailMary}`);
    const nav = await rail();
    await waitFor(() =>
      expect(within(nav).getByRole("link", { name: "Audiobooks" })).toHaveAttribute("aria-current", "page"),
    );
  });

  it("marks More libraries on the page of a work whose library isn't pinned", async () => {
    serveMockApi((api) => {
      const movies = api.libraries.find((l) => l.id === mockIds.movies);
      if (movies) movies.pinned = false;
    });
    renderApp(`/works/${mockIds.dunePartTwo}`);
    const nav = await rail();
    await waitFor(() =>
      expect(within(nav).getByRole("button", { name: "More libraries" })).toHaveAttribute("aria-current", "page"),
    );
  });

  it("keeps Home marked on a page that isn't in a library", async () => {
    serveMockApi();
    renderApp(`/people/${mockIds.villeneuve}`);
    const nav = await rail();
    await screen.findByRole("heading", { level: 1 });
    expect(within(nav).getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
  });

  it("asks to exit on Back at Home with nothing to focus, with No focused, and No goes back", async () => {
    serveMockApi();
    renderApp("/");
    await rail();
    back();
    const dialog = await screen.findByRole("alertdialog", { name: "Exit Kivo?" });
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "No" })).toHaveFocus());
    back();
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
  });

  it("keeps the remote on the player's route when its data fails: Try again has focus, and Back leaves", async () => {
    serveMockApi();
    server.use(http.get("/api/v1/works/{id}", () => problem(503, "notReady")));
    const { router } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await screen.findByRole("button", { name: "Try again" });
    await waitFor(() => expect(document.activeElement?.getAttribute("data-tv-focus-key")).toBe("load-error:retry"));
    expect(isTvPaused()).toBe(false);
    back();
    await waitFor(() => expect(locationOf(router)).toBe("/"));
  });

  it("gives the player every key once it is on the screen, and takes them back as it closes", async () => {
    serveMockApi();
    const { router } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await waitFor(() => expect(document.querySelector('[data-part="player"]')).not.toBeNull());
    await waitFor(() => expect(isTvPaused()).toBe(true));
    await router.navigate({ to: "/" });
    await waitFor(() => expect(isTvPaused()).toBe(false));
  });

  it("keeps the remote on a play that fails before the shell has seen the player", async () => {
    // A slow TV: the shell sees the page change well after the play failed.
    const Observer = MutationObserver;
    vi.stubGlobal(
      "MutationObserver",
      class extends Observer {
        constructor(changed: MutationCallback) {
          super((records, observer) => setTimeout(() => changed(records, observer), 200));
        }
      },
    );
    serveMockApi();
    server.use(http.post("/api/v1/plays", () => problem(503, "notReady")));
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await screen.findByRole("button", { name: "Try again" });
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(isTvPaused()).toBe(false);
  });

  it("keeps the splash up until the first screen shows", async () => {
    const splash = () => document.querySelector('[data-part="tv-splash"]');
    serveMockApi();
    renderApp("/");
    expect(splash()).not.toBeNull();
    await rail();
    expect(splash()).toBeNull();
  });

  it("takes the splash down for an error when where the app opens can't load", async () => {
    serveMockApi();
    server.use(http.get("/api/v1/setup", () => problem(503, "notReady")));
    renderApp("/");
    await screen.findByRole("button", { name: "Try again" });
    expect(document.querySelector('[data-part="tv-splash"]')).toBeNull();
  });

  it("keeps history as deep as a destination and the pages opened from it, however often the rail is used", async () => {
    serveMockApi();
    const { router } = renderApp("/");
    const nav = await rail();
    const index = () => router.history.location.state.__TSR_index;
    for (const destination of ["Search", "Favorites", "Home", "Search"]) {
      await router.navigate({ to: "/works/$workId", params: { workId: mockIds.dunePartTwo } });
      await screen.findByRole("heading", { name: "Dune: Part Two", level: 1 });
      expect(index()).toBe(1);
      fireEvent.click(within(nav).getByRole("link", { name: destination }));
      await waitFor(() =>
        expect(within(nav).getByRole("link", { name: destination })).toHaveAttribute("aria-current", "page"),
      );
      expect(index()).toBe(0);
    }
    expect(locationOf(router)).toBe("/search");
  });

  it("has no admin area: its pages go Home", async () => {
    serveMockApi();
    const { router } = renderApp("/admin");
    await rail();
    await waitFor(() => expect(locationOf(router)).toBe("/"));
  });

  it("offers the server-update Reload to the remote: Up from the first row reaches it, and OK reloads", async () => {
    // jsdom lays nothing out: the banner sits at the top, the cards under it.
    vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
      const key = (this as HTMLElement).dataset.tvFocusKey ?? "";
      const [x, y, width, height] =
        key === "banner:reload"
          ? [200, 0, 100, 40]
          : key.startsWith("card:")
            ? [200, 200, 200, 300]
            : [0, 0, 1920, 1080];
      return { x, y, left: x, top: y, width, height, right: x + width, bottom: y + height, toJSON: () => ({}) };
    });
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    serveMockApi();
    renderApp("/");
    await waitFor(() => expect(document.activeElement?.getAttribute("data-tv-focus-key")).toMatch(/^card:/));
    // A snapshot with another version says the server was updated.
    sendEvent({ type: "snapshot", serverVersion: "9.9.9" } as unknown as Parameters<typeof sendEvent>[0]);
    await screen.findByText("The server was updated. Reload to use the new version.");

    fireEvent.keyDown(document.activeElement ?? window, { key: "ArrowUp" });
    fireEvent.keyUp(document.activeElement ?? window, { key: "ArrowUp" });
    await waitFor(() => expect(document.activeElement?.getAttribute("data-tv-focus-key")).toBe("banner:reload"));
    fireEvent.keyDown(document.activeElement ?? window, { key: "Enter" });
    fireEvent.keyUp(document.activeElement ?? window, { key: "Enter" });
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
