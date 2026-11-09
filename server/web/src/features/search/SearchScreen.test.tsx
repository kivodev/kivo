import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { http, mockIds, scenarios, sent, serveMockApi, server } from "@/test/server";

describe("Search", () => {
  it("keeps the query in the URL and groups results by kind", async () => {
    serveMockApi();
    const { router, user } = renderApp("/search");
    await user.type(await screen.findByRole("searchbox", { name: "Search" }), "dune");
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "dune" }));
    expect(await screen.findByRole("heading", { name: "Movies" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Collections" })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: /^Dune/ }).length).toBeGreaterThan(1);
    expect(sent("GET", "/search").at(-1)?.url.searchParams.get("q")).toBe("dune");
  });

  it("shows nothing but the box on the web while nothing is typed", async () => {
    serveMockApi();
    renderApp("/search");
    await screen.findByRole("searchbox", { name: "Search" });
    expect(screen.queryByText("Search your libraries by title, person or collection")).not.toBeInTheDocument();
  });

  it("gives a movie's card watch state and Go to library", async () => {
    serveMockApi();
    const { user } = renderApp("/search?q=dune");
    await user.click(await screen.findByRole("button", { name: "More actions for Dune" }));
    expect(
      within(await screen.findByRole("menu"))
        .getAllByRole("menuitem")
        .map((i) => i.textContent),
    ).toEqual(["Play", "Mark as unwatched", "Add to List", "Go to library"]);
  });

  it("puts the people who match in a row above the works, each opening their page", async () => {
    serveMockApi();
    renderApp("/search?q=denis");
    const people = await screen.findByRole("heading", { name: "People" });
    const movies = screen.getByRole("heading", { name: "Movies" });
    expect(people.compareDocumentPosition(movies) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByRole("link", { name: /^Denis Villeneuve/ })).toHaveAttribute(
      "href",
      `/people/${mockIds.villeneuve}`,
    );
    expect(screen.getByText("Director · 2 titles")).toBeInTheDocument();
  });

  it("puts the collections that match in a row after the works, each opening its page", async () => {
    serveMockApi();
    renderApp("/search?q=dune");
    const collections = await screen.findByRole("heading", { name: "Collections" });
    const movies = screen.getByRole("heading", { name: "Movies" });
    expect(movies.compareDocumentPosition(collections) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const card = screen.getByRole("link", { name: /^Dune Collection/ });
    expect(card).toHaveAttribute("href", `/collections/${mockIds.duneCollection}`);
    expect(within(card).getByText("2 movies")).toBeInTheDocument();
  });

  it("keeps the last results, dimmed, until the next ones arrive", async () => {
    const api = serveMockApi();
    // The second query's results, which have no people, wait until
    // released.
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get("/api/v1/search", async ({ query, response }) => {
        if (query.get("q") !== "dune: part") return;
        await held;
        return response(200).json({
          works: api.search.filter((w) => w.id === mockIds.dunePartTwo),
          people: [],
          collections: [],
        });
      }),
    );
    const { router, user } = renderApp("/search?q=dune");
    const people = await screen.findByRole("heading", { name: "People" });
    await user.type(screen.getByRole("searchbox", { name: "Search" }), ": part");
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "dune: part" }));
    // The people aren't in the new results, but they stay until the new
    // results arrive.
    await waitFor(() => expect(people.closest("[aria-busy]")).toHaveAttribute("aria-busy", "true"));
    expect(people).toBeInTheDocument();
    release();
    await waitFor(() => expect(screen.queryByRole("heading", { name: "People" })).not.toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "Movies" }).closest("[aria-busy]")).toBeNull();
    expect(screen.getByRole("link", { name: /^Dune: Part Two/ })).toBeInTheDocument();
  });

  it("says when nothing matched, and points admins at Unmatched", async () => {
    const api = serveMockApi();
    api.search = [];
    api.searchPeople = [];
    api.searchCollections = [];
    const { router, user } = renderApp("/search?q=dnue");
    expect(await screen.findByRole("heading", { name: "No results for “dnue”." })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View unmatched items" })).toHaveAttribute(
      "href",
      `/libraries/${mockIds.movies}?matchState=unmatched`,
    );
    await user.click(screen.getByRole("button", { name: "Clear search" }));
    await waitFor(() => expect(router.state.location.search).toEqual({}));
  });

  it("doesn't point viewers at Unmatched", async () => {
    const api = serveMockApi(scenarios.viewer);
    api.search = [];
    api.searchPeople = [];
    api.searchCollections = [];
    renderApp("/search?q=dnue");
    await screen.findByRole("heading", { name: "No results for “dnue”." });
    expect(screen.queryByText("View unmatched items")).not.toBeInTheDocument();
  });
});
