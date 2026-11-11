import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { bookIds } from "@/api/mocks/books";
import { renderApp, screen } from "@/test/render";
import { sent, serveMockApi } from "@/test/server";

const library = `/libraries/${bookIds.audiobooks}`;

describe("Audiobooks library", () => {
  it("has Books, Authors and Series tabs, and square book cards by author", async () => {
    serveMockApi();
    const { user, router } = renderApp(library);
    const tabs = await screen.findByRole("navigation", { name: "Library sections" });
    expect(
      within(tabs)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["Books", "Authors", "Series"]);
    const card = (await screen.findByText("Project Hail Mary")).closest("[data-part=card]");
    expect(card).toHaveAttribute("data-kind", "book");
    expect(card?.querySelector("[data-part=cover]")).not.toBeNull();
    expect(card).toHaveTextContent("Andy Weir");

    await user.click(screen.getByRole("button", { name: /Sort/ }));
    expect((await screen.findAllByRole("option")).map((o) => o.textContent)).toEqual([
      "Title",
      "Author",
      "Date added",
      "Release date",
    ]);
    await user.click(screen.getByRole("option", { name: "Author" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ sort: "author" }));
    await waitFor(() =>
      expect(sent("POST", "/works/query").at(-1)?.body).toEqual({
        query: { kinds: ["book"], sort: [{ field: "author" }] },
        library: bookIds.audiobooks,
        limit: 100,
      }),
    );
    // The title sort stays on screen until the new sort's first page loads.
    await waitFor(() => expect(screen.queryByRole("slider", { name: "Jump to letter" })).not.toBeInTheDocument());

    await user.click(within(tabs).getByRole("button", { name: "Authors" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ sort: "author", tab: "authors" }));
    const corey = await screen.findByRole("link", { name: /James S. A. Corey/ });
    expect(corey).toHaveTextContent("3 books");
    expect(corey).toHaveAttribute("href", `/people/${bookIds.corey}`);
    // Authors are listed by name, so they have the letter rail whatever the
    // Books tab's sort is.
    expect(screen.getByRole("slider", { name: "Jump to letter" })).toBeInTheDocument();

    await user.click(within(tabs).getByRole("button", { name: "Series" }));
    const expanse = await screen.findByRole("link", { name: /The Expanse/ });
    expect(expanse).toHaveTextContent("3 books");
    expect(expanse).toHaveAttribute("href", `/book-series/${bookIds.expanse}`);
    expect(screen.getByRole("slider", { name: "Jump to letter" })).toBeInTheDocument();
  });

  it("names the watch status as books do, in its chip, an empty result and the filter", async () => {
    const api = serveMockApi();
    api.browse[bookIds.audiobooks] = [];
    const { user, router } = renderApp(`${library}?played=played`);
    expect(await screen.findByRole("heading", { name: "No finished books" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove filter Finished" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^Filter/ }));
    for (const name of ["Not started", "In progress", "Finished"]) {
      expect(await screen.findByRole("radio", { name })).toBeInTheDocument();
    }
    expect(screen.queryByRole("radio", { name: "Unwatched" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    await router.navigate({
      to: "/libraries/$libraryId",
      params: { libraryId: bookIds.audiobooks },
      search: { played: "unplayed" },
    });
    expect(await screen.findByRole("heading", { name: "No unstarted books" })).toBeInTheDocument();
  });

  it("filters by narrator, with no video filters", async () => {
    serveMockApi();
    const { user, router } = renderApp(library);
    await screen.findByText("Project Hail Mary");
    await user.click(screen.getByRole("button", { name: "Filter" }));
    expect(screen.queryByRole("checkbox", { name: "4K" })).not.toBeInTheDocument();
    await user.click(await screen.findByRole("radio", { name: "Ray Porter" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ narratorId: bookIds.porter }));
    expect(await screen.findByText("Narrated by Ray Porter")).toBeInTheDocument(); // the chip
    await waitFor(() =>
      expect(sent("POST", "/works/query").at(-1)?.body).toEqual({
        query: {
          kinds: ["book"],
          where: { match: "all", rules: [{ field: "narrator", op: "anyOf", values: [bookIds.porter] }] },
        },
        library: bookIds.audiobooks,
        limit: 100,
      }),
    );
    expect(
      sent("GET", "/work-fields/{field}/values").map((r) => `${r.url.pathname.split("/").at(-2)}${r.url.search}`),
    ).toEqual(["genre", "year", "narrator"].map((field) => `${field}?library=${bookIds.audiobooks}`));
  });

  it("edits the selected books together", async () => {
    serveMockApi();
    const { user } = renderApp(library);
    await screen.findByText("Project Hail Mary");
    await user.click(screen.getByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: /The Churn/ }));
    await user.click(screen.getByRole("checkbox", { name: /Caliban's War/ }));
    await user.click(screen.getByRole("button", { name: "Edit 2 books" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit 2 books" });
    await user.type(within(dialog).getByRole("textbox", { name: "Narrators" }), "Jefferson Mays");
    await user.type(within(dialog).getByRole("textbox", { name: "Series" }), "The Expanse Saga");
    await user.click(within(dialog).getByRole("radio", { name: "Clear" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(sent("PATCH", "/works").at(-1)?.body).toEqual({
      workIds: [bookIds.churn, bookIds.calibansWar],
      narrators: ["Jefferson Mays"],
      series: { name: "The Expanse Saga", keepPosition: false },
    });
  });
});
