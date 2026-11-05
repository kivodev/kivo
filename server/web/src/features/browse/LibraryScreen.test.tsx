import { fireEvent, waitFor, within } from "@testing-library/react";
import { http as untyped } from "msw";
import { describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockId, mockIds, scenarios, sendEvent, sent, serveMockApi, server } from "@/test/server";
import { browseSearchSchema, toFilters } from "./browseSearch";

const movies = `/libraries/${mockIds.movies}`;

/** The library grid requests sent, oldest first. */
const browses = () => sent("GET", "/libraries/{id}/works");

/** The query the library grid last asked for. */
const lastBrowse = () => browses().at(-1)?.url.searchParams;

describe("browse search params", () => {
  it("keeps valid values and drops invalid ones instead of failing the page", () => {
    expect(
      browseSearchSchema.parse({ sort: "addedAt", played: "bogus", resolution: ["4k", "8k"], yearFrom: 1990 }),
    ).toEqual({
      sort: "addedAt",
      yearFrom: 1990,
    });
  });

  it("sends the match filter for admins only, and neither the tab nor the view", () => {
    expect(toFilters({ matchState: "unmatched", tab: "collections", view: "list" }, true)).toEqual({
      matchState: "unmatched",
    });
    expect(toFilters({ matchState: "unmatched" }, false)).toEqual({});
  });
});

describe("Library view", () => {
  it("shows the count and the grid sorted by title, with the letter rail", async () => {
    serveMockApi();
    renderApp(movies);
    expect(await screen.findByRole("heading", { name: "Movies", level: 1 })).toBeInTheDocument();
    expect(await screen.findByText("238 movies")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: /^Arrival/ }).length).toBeGreaterThan(0);
    expect(browses().at(-1)?.url.pathname).toBe(`/api/v1/libraries/${mockIds.movies}/works`);
    expect(lastBrowse()?.get("sort")).toBeNull();
    const rail = screen.getByRole("slider", { name: "Jump to letter" });
    expect(rail).toHaveAttribute("aria-valuetext", "A");
  });

  it("leaves Go to library off a card in its library's own grid", async () => {
    serveMockApi();
    const { user } = renderApp(movies);
    await user.click((await screen.findAllByRole("button", { name: "More actions for Arrival" }))[0] as HTMLElement);
    const menu = await screen.findByRole("menu");
    expect(within(menu).getByRole("menuitem", { name: /^Mark as/ })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: "Go to library" })).not.toBeInTheDocument();
  });

  it("switches library from its title: Favorites, the pinned ones, then More libraries", async () => {
    serveMockApi(scenarios.unpinned);
    const { user, router } = renderApp(movies);
    const title = await screen.findByRole("heading", { name: "Movies", level: 1 });
    await user.click(within(title).getByRole("button", { name: "Movies" }));
    const menu = await screen.findByRole("menu", { name: "Movies" });
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((i) => i.textContent),
    ).toEqual(["Favorites", "Movies", "TV Shows", "Audiobooks", "Home Videos"]);
    expect(within(menu).getByRole("group", { name: "More libraries" })).toHaveTextContent("Home Videos");
    await user.click(within(menu).getByRole("menuitem", { name: "Home Videos" }));
    expect(await screen.findByRole("heading", { name: "Home Videos", level: 1 })).toBeInTheDocument();
    expect(locationOf(router)).toBe(`/libraries/${mockIds.videos}`);
  });

  it("scrolls the whole grid to the letter scrubbed to on the rail, and # back to the top", async () => {
    // Scrolling moves the window and then fires the scroll event, as in a
    // browser, over a page tall enough to hold the grid.
    Object.defineProperty(document.documentElement, "scrollHeight", { value: 100_000, configurable: true });
    const scrollTo = (top: number) => {
      Object.defineProperty(window, "scrollY", { value: top, configurable: true });
      setTimeout(() => window.dispatchEvent(new Event("scroll")));
    };
    vi.spyOn(window, "scrollTo").mockImplementation(((options: ScrollToOptions) =>
      scrollTo(options.top ?? 0)) as typeof window.scrollTo);
    serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");
    const rail = screen.getByRole("slider", { name: "Jump to letter" });
    // The rail marks the letter of the top row in view.
    const current = () => rail.getAttribute("aria-valuetext");
    await waitFor(() => expect(current()).toBe("A"));

    // A finger pressed at the top of the rail and dragged down to W. Each
    // letter takes a tenth of the rail's height.
    vi.spyOn(rail, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 44, 270));
    rail.setPointerCapture = () => {};
    rail.hasPointerCapture = () => true;
    fireEvent.pointerDown(rail, { pointerId: 1, button: 0, clientY: 5 });
    fireEvent.pointerMove(rail, { pointerId: 1, clientY: 120 });
    fireEvent.pointerMove(rail, { pointerId: 1, clientY: 235 });
    expect(await screen.findByText("W", { selector: '[data-part="letter-rail-bubble"]' })).toBeInTheDocument();
    fireEvent.pointerUp(rail, { pointerId: 1, clientY: 235 });
    expect((await screen.findAllByRole("link", { name: /^Wild/ })).length).toBeGreaterThan(0);
    await waitFor(() => expect(current()).toBe("W"));
    expect(window.scrollY).toBeGreaterThan(0);
    // Its page is read at its offset.
    const pages = browses();
    expect(pages.some((r) => Number(r.url.searchParams.get("offset")) > 0)).toBe(true);
    expect(pages.some((r) => r.url.searchParams.has("after"))).toBe(false);
    // The grid is still the whole list, A to Z, so the URL is unchanged.
    expect(router.state.location.search).toEqual({});
    expect(screen.getAllByRole("link", { name: /^Velvet/ }).length).toBeGreaterThan(0);

    // Home, on the keyboard, is #.
    rail.focus();
    await user.keyboard("{Home}");
    await waitFor(() => expect(window.scrollY).toBe(0));
    await waitFor(() => expect(current()).toBe("A"));
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });

  it("refetches in place when root works are added to its library, keeping the viewer's place", async () => {
    Object.defineProperty(document.documentElement, "scrollHeight", { value: 100_000, configurable: true });
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    const api = serveMockApi();
    renderApp(movies);
    await screen.findByText("238 movies");
    Object.defineProperty(window, "scrollY", { value: 4000, configurable: true });
    window.dispatchEvent(new Event("scroll"));
    const rail = screen.getByRole("slider", { name: "Jump to letter" });
    await waitFor(() => expect(rail).not.toHaveAttribute("aria-valuetext", "A"));
    const first = api.browse[mockIds.movies]?.[0];
    if (!first) throw new Error("no movies");
    api.browse[mockIds.movies] = [
      { ...first, id: mockId(4242), title: "Aardvark" },
      ...(api.browse[mockIds.movies] ?? []),
    ];
    const facets = sent("GET", "/libraries/{id}/facets").length;
    scrollTo.mockClear();

    sendEvent({ type: "event", kind: "libraryChanged", libraryIds: [mockIds.movies] });
    expect(await screen.findByText("239 movies", {}, { timeout: 3000 })).toBeInTheDocument();
    expect(sent("GET", "/libraries/{id}/facets")).toHaveLength(facets + 1);
    // Still where the viewer was. There's no scroll, and the rail's letter
    // isn't back at A.
    expect(window.scrollY).toBe(4000);
    expect(rail).not.toHaveAttribute("aria-valuetext", "A");
    expect(scrollTo).not.toHaveBeenCalled();
    Reflect.deleteProperty(window, "scrollY");
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });

  it("offers the library's own genres and content ratings as filters", async () => {
    serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");
    await user.click(screen.getByRole("button", { name: "Filter" }));
    await user.click(await screen.findByRole("checkbox", { name: "Documentary" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ genre: ["Documentary"] }));
    const group = screen.getByText("Content rating").parentElement ?? document.body;
    expect(
      within(group)
        .getAllByRole("checkbox")
        .map((c) => c.closest("label")?.textContent),
    ).toEqual(["G", "PG", "PG-13", "R"]);
    await user.click(screen.getByRole("checkbox", { name: "PG-13" }));
    await waitFor(() =>
      expect(router.state.location.search).toEqual({ genre: ["Documentary"], contentRating: ["PG-13"] }),
    );
    expect(screen.queryByRole("radio", { name: "1900s" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Remove filter PG-13" })).toBeInTheDocument();
  });

  it("offers only the resolutions and dynamic ranges the library has", async () => {
    serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");
    await user.click(screen.getByRole("button", { name: "Filter" }));
    const options = (label: string) =>
      within(screen.getByText(label).parentElement ?? document.body)
        .getAllByRole("checkbox")
        .map((c) => c.closest("label")?.textContent);
    expect(options("Resolution")).toEqual(["4K", "1080p"]);
    expect(options("Dynamic range")).toEqual(["HDR"]);
    await user.click(screen.getByRole("checkbox", { name: "4K" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ resolution: ["4k"] }));
    await user.keyboard("{Escape}");

    // A library without HDR has no dynamic range filter.
    await router.navigate({ to: "/libraries/$libraryId", params: { libraryId: mockIds.shows } });
    await screen.findByText(/^\d+ shows?$/);
    await user.click(screen.getByRole("button", { name: "Filter" }));
    expect(options("Resolution")).toEqual(["1080p"]);
    expect(screen.queryByText("Dynamic range")).not.toBeInTheDocument();
  });

  it("asks for a resolution in the URL that the library doesn't offer, which finds nothing", async () => {
    const api = serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");
    api.browse[mockIds.movies] = [];
    await router.navigate({
      to: "/libraries/$libraryId",
      params: { libraryId: mockIds.movies },
      search: { resolution: ["720p"] },
    });
    expect(await screen.findByRole("heading", { name: "No movies match these filters" })).toBeInTheDocument();
    expect(lastBrowse()?.getAll("resolution").join(",")).toContain("720p");
    expect(screen.getByRole("button", { name: "Remove filter 720p" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Filter, some on" }));
    expect(screen.queryByRole("checkbox", { name: "720p" })).not.toBeInTheDocument();
  });

  it("puts sort and filters in the URL and the query, with removable chips", async () => {
    const api = serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");

    await user.click(screen.getByRole("button", { name: /Sort/ }));
    await user.click(await screen.findByRole("option", { name: "Date added" }));
    await waitFor(() => expect(locationOf(router)).toBe(`${movies}?sort=addedAt`));
    // The title sort stays on screen until the new sort's first page loads.
    await waitFor(() => expect(screen.queryByRole("slider", { name: "Jump to letter" })).not.toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "Filter" }));
    api.browse[mockIds.movies] = (api.browse[mockIds.movies] ?? []).filter((w) => w.userState?.played);
    await user.click(await screen.findByRole("radio", { name: "Watched" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ sort: "addedAt", played: "played" }));
    expect(await screen.findByText("3 movies")).toBeInTheDocument();
    expect(lastBrowse()?.get("sort")).toBe("addedAt");
    expect(lastBrowse()?.get("played")).toBe("played");
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "Remove filter Watched" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ sort: "addedAt" }));
  });

  it("offers Last episode added in a shows library only", async () => {
    serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");
    await user.click(screen.getByRole("button", { name: /Sort/ }));
    await screen.findByRole("option", { name: "Date added" });
    expect(screen.queryByRole("option", { name: "Last episode added" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");

    await router.navigate({ to: `/libraries/${mockIds.shows}` });
    await screen.findByRole("heading", { name: "TV Shows", level: 1 });
    await user.click(await screen.findByRole("button", { name: /Sort/ }));
    await user.click(await screen.findByRole("option", { name: "Last episode added" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ sort: "lastEpisodeAddedAt" }));
    expect(lastBrowse()?.get("sort")).toBe("lastEpisodeAddedAt");
  });

  it("shows the next library at once, its title from the list of libraries, and its grid as it arrives", async () => {
    serveMockApi();
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      untyped.get(`*/api/v1/libraries/${mockIds.shows}/works`, async () => {
        await held;
      }),
    );
    const { router } = renderApp(movies);
    await screen.findByText("238 movies");
    void router.navigate({ to: `/libraries/${mockIds.shows}` });
    await waitFor(() => expect(screen.getByRole("heading", { name: "TV Shows", level: 1 })).toBeInTheDocument());
    expect(screen.getByText("Loading…")).toBeInTheDocument();
    expect(screen.queryByText("238 movies")).not.toBeInTheDocument();
    release();
    expect(await screen.findByText(/^\d+ shows?$/)).toBeInTheDocument();
    expect(screen.queryByText("Loading…")).not.toBeInTheDocument();
  });

  it("opens as this browser last left it, and as linked when a link has its own state", async () => {
    serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");
    await user.click(screen.getByRole("button", { name: /Sort/ }));
    await user.click(await screen.findByRole("option", { name: "Date added" }));
    await user.click(screen.getByRole("button", { name: "List" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ sort: "addedAt", view: "list" }));

    await router.navigate({ to: "/" });
    await router.navigate({ to: "/libraries/$libraryId", params: { libraryId: mockIds.movies } });
    await waitFor(() => expect(router.state.location.search).toEqual({ sort: "addedAt", view: "list" }));
    expect(lastBrowse()?.get("sort")).toBe("addedAt");

    // Each library keeps its own.
    await router.navigate({ to: "/libraries/$libraryId", params: { libraryId: mockIds.shows } });
    await waitFor(() => expect(router.state.location.pathname).toBe(`/libraries/${mockIds.shows}`));
    expect(router.state.location.search).toEqual({});

    await router.navigate({
      to: "/libraries/$libraryId",
      params: { libraryId: mockIds.movies },
      search: { played: "played" },
    });
    await waitFor(() => expect(lastBrowse()?.get("played")).toBe("played"));
    expect(router.state.location.search).toEqual({ played: "played" });

    // Clearing everything stays cleared, and is what the library opens on next.
    await user.click(await screen.findByRole("button", { name: "Remove filter Watched" }));
    await waitFor(() => expect(router.state.location.search).toEqual({}));
    await router.navigate({ to: "/" });
    await router.navigate({ to: "/libraries/$libraryId", params: { libraryId: mockIds.movies } });
    await screen.findByText("238 movies");
    expect(router.state.location.search).toEqual({});
  });

  it("lists unmatched items in the grid with a badge, and filters to them from Match, with no banner", async () => {
    const api = serveMockApi();
    const { router, user } = renderApp(movies);
    await screen.findByText("238 movies");
    expect(screen.queryByText(/unmatched\./)).not.toBeInTheDocument();
    api.browse[mockIds.movies] = (api.browse[mockIds.movies] ?? []).filter((w) => w.id === mockIds.holiday);
    await user.click(screen.getByRole("button", { name: "Filter" }));
    await user.click(await screen.findByRole("radio", { name: "Unmatched" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ matchState: "unmatched" }));
    expect(await screen.findByText("1 movie")).toBeInTheDocument();
    expect(lastBrowse()?.get("matchState")).toBe("unmatched");
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Filter, some on" })).toBeInTheDocument();
    expect(screen.getByText("Holiday 2019 Final", { selector: "div" })).toBeInTheDocument();
    expect(screen.getAllByText("Unmatched").length).toBeGreaterThan(0);
  });

  it("hides the match filter from viewers", async () => {
    serveMockApi(scenarios.viewer);
    renderApp(`${movies}?matchState=unmatched`);
    expect(await screen.findByText("238 movies")).toBeInTheDocument();
    expect(lastBrowse()?.get("matchState")).toBeNull();
  });

  it("explains an empty filter result and clears it in one action", async () => {
    const api = serveMockApi();
    api.browse[mockIds.movies] = [];
    const { router, user } = renderApp(`${movies}?yearFrom=1900&yearTo=1909`);
    expect(await screen.findByRole("heading", { name: "No movies match these filters" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(router.state.location.search).toEqual({}));
  });

  it("names the played state when it's the only filter", async () => {
    const api = serveMockApi();
    api.browse[mockIds.movies] = [];
    renderApp(`${movies}?played=unplayed`);
    expect(await screen.findByRole("heading", { name: "No unwatched movies" })).toBeInTheDocument();
  });

  it("shows the sections as chips: Collections beside Movies, and none for a library of one", async () => {
    serveMockApi();
    const { router, user } = renderApp(movies);
    const sections = await screen.findByRole("navigation", { name: "Library sections" });
    const collections = await within(sections).findByRole("button", { name: "Collections" });
    expect(within(sections).getByRole("button", { name: "Movies" })).toHaveAttribute("aria-current", "page");
    // The Collections tab lists the library's collections, each opening its page.
    await user.click(collections);
    expect(await screen.findByText("1 collection")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /^Dune Collection/ })).toHaveAttribute(
      "href",
      `/collections/${mockIds.duneCollection}`,
    );
    expect(screen.getByText("2 movies")).toBeInTheDocument();
    await router.navigate({ to: "/libraries/$libraryId", params: { libraryId: mockIds.shows } });
    await screen.findByText(/^\d+ shows?$/);
    expect(screen.queryByRole("navigation", { name: "Library sections" })).not.toBeInTheDocument();
  });

  it("shows Collections as the facets say, reading the collections only when it opens", async () => {
    const api = serveMockApi();
    const facets = api.facets[mockIds.movies];
    if (!facets) throw new Error("no facets");
    facets.collections = false;
    renderApp(movies);
    await screen.findByText("238 movies");
    expect(screen.queryByRole("navigation", { name: "Library sections" })).not.toBeInTheDocument();
    expect(sent("GET", "/libraries/{id}/collections")).toHaveLength(0);
  });
});
