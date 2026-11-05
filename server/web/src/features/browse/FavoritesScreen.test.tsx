import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import { renderApp, screen } from "@/test/render";
import { mockIds, serveMockApi } from "@/test/server";

/** Marks the sample's first episode as a favorite, alongside Dune and The Expanse. */
function favoriteEpisode(api: MockApi) {
  const episode = Object.values(api.works).find((w) => w.kind === "episode" && w.userState);
  if (!episode?.userState) throw new Error("no episode");
  episode.userState = { ...episode.userState, favorite: true };
}

function unfavoriteAll(api: MockApi) {
  for (const work of Object.values(api.works)) {
    if (work.userState) work.userState = { ...work.userState, favorite: false };
  }
}

const row = (kind: string) => document.querySelector(`[data-row="${kind}"]`) as HTMLElement;

describe("Favorites", () => {
  it("has a row for each kind with favorites, in order, an episode by its series' poster", async () => {
    serveMockApi(favoriteEpisode);
    renderApp("/favorites");
    await screen.findByRole("heading", { name: "Movies" });
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "Movies",
      "Shows",
      "Episodes",
    ]);
    expect(within(row("movie")).getByText("Dune")).toBeInTheDocument();
    expect(within(row("series")).getByText("The Expanse")).toBeInTheDocument();
    const episodes = row("episode");
    expect(episodes.querySelector('[data-part="poster"]')).not.toBeNull();
    expect(within(episodes).getByText(/^S\d+ · E\d+$/)).toBeInTheDocument();
    expect(document.title).toBe("Favorites – Kivo");
  });

  it("opens a kind's whole grid from its row's See all and its title", async () => {
    serveMockApi();
    const { user, router } = renderApp("/favorites");
    await screen.findByRole("heading", { name: "Movies" });
    await user.click(within(row("movie")).getByRole("link", { name: "See all" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/favorites/movie"));
    expect(await screen.findByText("1 movie")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Favorites · Movies/ })).toBeInTheDocument();
    expect(within(document.querySelector('[data-part="grid"]') as HTMLElement).getByText("Dune")).toBeInTheDocument();

    router.history.back();
    await screen.findByRole("heading", { name: "Shows" });
    await user.click(within(row("series")).getByRole("link", { name: "Shows" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/favorites/series"));
    expect(await screen.findByText("1 show")).toBeInTheDocument();
  });

  it("drops a work unmarked from its card's menu", async () => {
    serveMockApi();
    const { user } = renderApp("/favorites");
    await screen.findByRole("heading", { name: "Movies" });
    await user.click(within(row("movie")).getByRole("button", { name: "More actions for Dune" }));
    await user.click(await screen.findByRole("menuitem", { name: "Remove from favorites" }));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "Movies" })).toBeNull());
    expect(screen.getByRole("heading", { name: "Shows" })).toBeInTheDocument();
  });

  it("says how to mark one when there are none", async () => {
    serveMockApi(unfavoriteAll);
    renderApp("/favorites");
    expect(await screen.findByRole("heading", { name: "No favorites yet." })).toBeInTheDocument();
    expect(screen.getByText("Items you mark with ♥ appear here.")).toBeInTheDocument();
  });

  it("is in the sidebar after Search and before the libraries, and first in the library menu", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/libraries/${mockIds.movies}`);
    const sidebar = await screen.findByRole("navigation", { name: "Main" });
    await within(sidebar).findByRole("link", { name: "Movies" });
    const entries = [...sidebar.querySelectorAll("a, button, hr")].map((e) =>
      e.tagName === "HR" ? "—" : e.textContent?.trim(),
    );
    const at = (name: string) => entries.indexOf(name);
    expect(at("Search")).toBeLessThan(at("Favorites"));
    expect(entries[at("Favorites") + 1]).toBe("—");
    expect(entries[at("Favorites") + 2]).toBe("Movies");

    const switcher = await waitFor(() => {
      const button = document.querySelector<HTMLElement>('[data-part="library-switcher"]');
      if (!button) throw new Error("no library switcher yet");
      return button;
    });
    await user.click(switcher);
    const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent);
    expect(items[0]).toBe("Favorites");
    await user.click(screen.getByRole("menuitem", { name: "Favorites" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/favorites"));
  });
});
