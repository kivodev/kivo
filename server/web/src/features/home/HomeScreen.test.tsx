import { fireEvent, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { WorkDetail } from "@/api/schemas";
import { renderApp, screen } from "@/test/render";
import { mockId, mockIds, scenarios, sendEvent, sent, serveMockApi, summaryOf } from "@/test/server";

describe("Home", () => {
  it("shows the server's rows in order, with library names and each row's card shape", async () => {
    serveMockApi();
    renderApp("/");
    await screen.findByRole("heading", { name: "Continue Watching" });
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "Continue Watching",
      "Favorites",
      "Recently Added in Movies",
      "Recently Added in TV Shows",
      "Recently Added in Home Videos",
      "Continue Listening",
      "Recently Added in Audiobooks",
    ]);
    const continueRow = document.querySelector('[data-row="continueWatching"]') as HTMLElement;
    expect(within(continueRow).getByText("S3 · E3")).toBeInTheDocument();
    expect(within(continueRow).getByText(/14 min left/)).toBeInTheDocument();
    // A series' next episode has its code, then the series and the episode.
    expect(within(continueRow).getByText("S1 · E1")).toBeInTheDocument();
    expect(within(continueRow).getAllByText("Severance")).not.toHaveLength(0);
    expect(within(continueRow).getByText("Good News About Hell")).toBeInTheDocument();
  });

  it("opens Favorites from the Favorites row's heading", async () => {
    serveMockApi();
    const { user, router } = renderApp("/");
    const heading = await screen.findByRole("heading", { name: "Favorites" });
    await user.click(within(heading).getByRole("link", { name: "Favorites" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/favorites"));
  });

  it("shows books in progress as square covers, with the time left", async () => {
    serveMockApi();
    renderApp("/");
    await screen.findByRole("heading", { name: "Continue Listening" });
    const listening = document.querySelector('[data-row="continueListening"]') as HTMLElement;
    expect(within(listening).getByText("Andy Weir · 8 hr 5 min left")).toBeInTheDocument();
    expect(listening.querySelector('[data-part="cover"], .aspect-square')).not.toBeNull();
  });

  it("lists pinned libraries in the sidebar, and the rest under More libraries", async () => {
    serveMockApi(scenarios.unpinned);
    const { user, router } = renderApp("/");
    const sidebar = await screen.findByRole("navigation", { name: "Main" });
    const libraries = () => [...sidebar.querySelectorAll('[data-part="sidebar-library"]')].map((l) => l.textContent);
    await waitFor(() => expect(libraries()).toEqual(["Movies", "TV Shows", "Audiobooks"]));
    const more = within(sidebar).getByRole("button", { name: "More libraries" });
    await user.click(more);
    await user.click(await screen.findByRole("menuitem", { name: "Home Videos" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/libraries/${mockIds.videos}`));
    // It shows as the current place while one of its libraries is open.
    await waitFor(() => expect(more).toHaveClass("bg-surface-raised"));
  });

  it("puts Libraries on the phone bar, opening the list, or the library this browser last opened over it", async () => {
    serveMockApi(scenarios.unpinned);
    const { user, router } = renderApp("/");
    const sidebar = await screen.findByRole("navigation", { name: "Main" });
    const tab = await within(sidebar).findByRole("link", { name: "Libraries" });
    expect(tab).toHaveAttribute("href", "/libraries");
    expect(tab).not.toHaveAttribute("aria-current");

    await router.navigate({ to: "/libraries/$libraryId", params: { libraryId: mockIds.videos } });
    // It's current on any library, pinned or not. Chosen there, it goes
    // back to the list.
    await waitFor(() => expect(tab).toHaveAttribute("aria-current", "page"));
    expect(tab).toHaveAttribute("href", "/libraries");
    await user.click(within(sidebar).getByRole("link", { name: "Home" }));
    await waitFor(() => expect(tab).toHaveAttribute("href", `/libraries/${mockIds.videos}`));
    expect(tab).not.toHaveAttribute("aria-current");

    // Back on the list, it forgets the library.
    await router.navigate({ to: "/libraries" });
    await waitFor(() => expect(tab).toHaveAttribute("aria-current", "page"));
    await user.click(within(sidebar).getByRole("link", { name: "Home" }));
    await waitFor(() => expect(tab).not.toHaveAttribute("aria-current"));
    expect(tab).toHaveAttribute("href", "/libraries");
  });

  it("badges unmatched and identifying items, which stay playable", async () => {
    serveMockApi();
    renderApp("/");
    const row = await waitFor(() => {
      const el = document.querySelector('[data-row="recentlyAdded"]');
      if (!el) throw new Error("no row");
      return el as HTMLElement;
    });
    expect(within(row).getByText("Unmatched")).toBeInTheDocument();
    expect(within(row).getByText("Identifying")).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: "Play Holiday 2019 Final" })).toBeInTheDocument();
  });

  it("counts the episodes a Recently Added card bundles, without calling them new", async () => {
    const api = serveMockApi();
    // A season bundle as the server sends it, with the count where a
    // series card's count goes.
    const season = summaryOf(api.works[mockIds.severanceSeason1] as WorkDetail);
    api.home.rows
      .find((r) => r.id === `recentlyAdded:${mockIds.shows}`)
      ?.items.push({ ...season, series: { unplayedCount: 0, recentlyAddedCount: 2 } });
    renderApp("/");
    const heading = await screen.findByRole("heading", { name: "Recently Added in TV Shows" });
    const row = heading.closest("[data-row]") as HTMLElement;
    const expanse = within(row).getByText("The Expanse").closest('[data-part="card"]') as HTMLElement;
    expect(within(expanse).getByText("3 episodes")).toBeInTheDocument();
    const season1 = within(row).getByText("Season 1 · 2 episodes").closest('[data-part="card"]') as HTMLElement;
    expect(season1).toHaveTextContent("Severance");
    expect(within(row).queryByText(/new/i)).not.toBeInTheDocument();
  });

  it("shows a root work added to a library as the server tells of it, without a reload", async () => {
    const api = serveMockApi();
    renderApp("/");
    const heading = await screen.findByRole("heading", { name: "Recently Added in Movies" });
    const recent = api.home.rows.find((r) => r.id === `recentlyAdded:${mockIds.movies}`);
    const first = recent?.items[0];
    if (!recent || !first) throw new Error("no Recently Added in Movies");
    recent.items = [{ ...first, id: mockId(4242), title: "Brand New Film" }, ...recent.items];
    const before = sent("GET", "/home").length;
    sendEvent({ type: "event", kind: "libraryChanged", libraryIds: [mockIds.movies] });
    const row = heading.closest("[data-row]") as HTMLElement;
    expect((await within(row).findAllByText("Brand New Film", {}, { timeout: 3000 })).length).toBeGreaterThan(0);
    expect(sent("GET", "/home")).toHaveLength(before + 1);
  });

  it("marks played from a card's menu", async () => {
    serveMockApi();
    const { user } = renderApp("/");
    await user.click(
      (await screen.findAllByRole("button", { name: "More actions for Dune: Part Two" }))[0] as HTMLElement,
    );
    await user.click(screen.getByRole("menuitem", { name: "Mark as watched" }));
    await waitFor(() => expect(sent("PATCH", `/works/${mockIds.dunePartTwo}/user-state`)).toHaveLength(1));
    expect(sent("PATCH", `/works/${mockIds.dunePartTwo}/user-state`)[0]?.body).toEqual({ played: true });
  });

  it("removes a card from Continue Watching and Continue Listening from its menu, and only there", async () => {
    serveMockApi();
    const { user } = renderApp("/");
    await screen.findByRole("heading", { name: "Continue Watching" });
    const watching = document.querySelector('[data-row="continueWatching"]') as HTMLElement;
    await user.click(within(watching).getByRole("button", { name: "More actions for Dune: Part Two" }));
    await user.click(screen.getByRole("menuitem", { name: "Remove from Continue Watching" }));
    await waitFor(() =>
      expect(sent("POST", `/works/${mockIds.dunePartTwo}/user-state/remove-from-continue`)).toHaveLength(1),
    );
    await waitFor(() =>
      expect(within(watching).queryByRole("button", { name: "More actions for Dune: Part Two" })).toBeNull(),
    );

    const listening = document.querySelector('[data-row="continueListening"]') as HTMLElement;
    await user.click(within(listening).getAllByRole("button", { name: /^More actions for/ })[0] as HTMLElement);
    expect(screen.getByRole("menuitem", { name: "Remove from Continue Listening" })).toBeInTheDocument();
    await user.keyboard("{Escape}");

    const favorites = document.querySelector('[data-row="favorites"]') as HTMLElement;
    await user.click(within(favorites).getAllByRole("button", { name: /^More actions for/ })[0] as HTMLElement);
    expect(screen.queryByRole("menuitem", { name: /^Remove from Continue/ })).toBeNull();
  });

  it("opens a card's menu on a long press, without opening the card", async () => {
    serveMockApi();
    const { user, router } = renderApp("/");
    const card = (await screen.findAllByRole("link", { name: /^Dune: Part Two/ }))[0] as HTMLElement;
    await user.pointer({ keys: "[TouchA>]", target: card });
    expect(await screen.findByRole("menu", {}, { timeout: 2000 })).toBeInTheDocument();
    await user.pointer({ keys: "[/TouchA]", target: card });
    expect(router.state.location.pathname).toBe("/");
  });

  it("offers Resume, Play from beginning, Go to series and Go to library in a card's menu, also on right-click", async () => {
    serveMockApi();
    const { user, router } = renderApp("/");
    const episode = (await screen.findAllByRole("link", { name: /^The Expanse/ }))[0] as HTMLElement;
    fireEvent.contextMenu(episode);
    const menu = await screen.findByRole("menu");
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((i) => i.textContent),
    ).toEqual([
      "Resume",
      "Play from beginning",
      "Mark as watched",
      "Add to favorites",
      "Remove from Continue Watching",
      "Go to series",
      "Go to library",
    ]);
    // A destination is a real link, which opens in a new tab too.
    expect(within(menu).getByRole("menuitem", { name: "Go to library" })).toHaveAttribute(
      "href",
      `/libraries/${mockIds.shows}`,
    );
    await user.click(within(menu).getByRole("menuitem", { name: "Play from beginning" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${mockId(1303)}`));
    expect(router.state.location.search).toEqual({ from: "start" });

    await router.navigate({ to: "/" });
    await user.click(
      (await screen.findAllByRole("button", { name: "More actions for Dune: Part Two" }))[0] as HTMLElement,
    );
    await user.click(await screen.findByRole("menuitem", { name: "Go to library" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/libraries/${mockIds.movies}`));
  });

  it("asks an admin to add the first library", async () => {
    serveMockApi(scenarios.empty);
    const { user } = renderApp("/");
    expect(await screen.findByRole("heading", { name: "No libraries" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add library" }));
    expect(await screen.findByRole("dialog")).toHaveTextContent("Add a library");
  });

  it("tells a viewer no libraries are shared yet", async () => {
    serveMockApi(scenarios.empty, scenarios.viewer);
    renderApp("/");
    expect(await screen.findByRole("heading", { name: "No libraries" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add library" })).not.toBeInTheDocument();
  });

  it("shows no scan progress, even to admins: that's in Admin", async () => {
    serveMockApi(scenarios.scanning);
    renderApp("/");
    expect(await screen.findByRole("heading", { name: "Continue Watching" })).toBeInTheDocument();
    expect(screen.queryByText(/Scanning/)).not.toBeInTheDocument();
  });
});
