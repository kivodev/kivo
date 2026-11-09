import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { bookIds } from "@/api/mocks/books";
import { mockIds } from "@/api/mocks/fixtures";
import { renderApp, screen } from "@/test/render";
import { onRequest, sent, serveMockApi } from "@/test/server";

describe("Book page", () => {
  it("shows the authors, narrators and series as links, the chapters and the files", async () => {
    serveMockApi();
    renderApp(`/works/${bookIds.leviathan}`);
    expect(await screen.findByRole("heading", { name: "Leviathan Wakes", level: 1 })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "James S. A. Corey" })).toHaveAttribute("href", `/people/${bookIds.corey}`);
    expect(screen.getByText(/Narrated by/)).toHaveTextContent("Narrated by Jefferson Mays");
    expect(screen.getByText(/Book 1 of/)).toHaveTextContent("Book 1 of The Expanse");
    expect(screen.getByRole("link", { name: "The Expanse" })).toHaveAttribute(
      "href",
      `/book-series/${bookIds.expanse}`,
    );
    expect(screen.getByText(/3 chapters/)).toBeInTheDocument();
    const files = screen.getByRole("heading", { name: "3 files" }).parentElement ?? document.body;
    expect(
      within(files)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(["101 - Opening Credits.mp32:00", "202 - Chapter 1.mp340:00", "303 - Chapter 2.mp338:00"]);
    const chapters = screen.getByRole("heading", { name: "Chapters" }).parentElement ?? document.body;
    expect(
      within(chapters)
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Play from 01 - Opening Credits", "Play from 02 - Chapter 1", "Play from 03 - Chapter 2"]);
    expect(screen.getByRole("button", { name: "Play" })).toBeEnabled();
  });

  it("marks the book watched and adds it to My List for you", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.hailMary}`);
    await user.click(await screen.findByRole("button", { name: "Mark as watched" }));
    await user.click(await screen.findByRole("button", { name: "Add to My List" }));
    await waitFor(() =>
      expect(sent("PATCH", `/works/${bookIds.hailMary}/user-state`).map((r) => r.body)).toEqual([{ played: true }]),
    );
    await waitFor(() =>
      expect(sent("POST", `/lists/${mockIds.myList}/works`).map((r) => r.body)).toEqual([{ workId: bookIds.hailMary }]),
    );
  });

  it("offers admins Match, not Fix match or Unmatch", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.hailMary}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Project Hail Mary" }));
    const menu = await screen.findByRole("menu");
    expect(within(menu).queryByRole("menuitem", { name: "Fix match" })).not.toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: "Unmatch" })).not.toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Match" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Edit" })).toBeInTheDocument();
  });

  it("matches a book to the Audible book chosen in Match", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.hailMary}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Project Hail Mary" }));
    await user.click(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Match" }));
    const dialog = await screen.findByRole("dialog", { name: "Match: Project Hail Mary" });
    // Searched by the book's title and first author.
    expect(within(dialog).getByRole("textbox", { name: "Author" })).toHaveValue("Andy Weir");
    await waitFor(() => expect(sent("GET", "/works/{id}/matches")).toHaveLength(1));
    const query = sent("GET", "/works/{id}/matches")[0]?.url.searchParams;
    expect(query?.get("query")).toBe("Project Hail Mary");
    expect(query?.get("author")).toBe("Andy Weir");
    expect(query?.has("year")).toBe(false);

    const matches = await within(dialog).findByRole("list", { name: "Matches" });
    const first = within(matches).getAllByRole("listitem")[0] ?? document.body;
    expect(first).toHaveTextContent("Project Hail Mary (2021)");
    expect(first).toHaveTextContent("By Andy Weir");
    expect(first).toHaveTextContent("Narrated by Ray Porter");
    expect(first).toHaveTextContent("16h 10m");
    await user.click(within(first).getByRole("button", { name: "Choose Project Hail Mary (2021)" }));
    await waitFor(() =>
      expect(sent("POST", "/works/{id}/match").at(-1)?.body).toEqual({ matchId: "audible:book:B08G9PRS1K" }),
    );
  });
});

describe("Person page", () => {
  it("groups an author's books by series in series order, and lists what they narrated", async () => {
    serveMockApi();
    renderApp(`/people/${bookIds.corey}`);
    expect(await screen.findByRole("heading", { name: "James S. A. Corey", level: 1 })).toBeInTheDocument();
    // Audible's biography and photo.
    expect(screen.getByText(/pen name of Daniel Abraham/)).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "James S. A. Corey" })).toBeInTheDocument();
    const series = screen.getByRole("heading", { name: "The Expanse", level: 2 }).parentElement ?? document.body;
    expect(
      within(series)
        .getAllByText(/^Book /)
        .map((p) => p.textContent),
    ).toEqual(["Book 1", "Book 1.5", "Book 2"]);
    expect(
      within(series)
        .getAllByRole("link")
        .map((a) => a.textContent)
        .filter((t) => t?.includes("Wakes") || t?.includes("Churn") || t?.includes("War")),
    ).toEqual(["Leviathan WakesJames S. A. Corey", "The ChurnJames S. A. Corey", "Caliban's WarJames S. A. Corey"]);

    renderApp(`/people/${bookIds.porter}`);
    const narrated = (await screen.findByRole("heading", { name: "Narrated" })).parentElement ?? document.body;
    expect(within(narrated).getByText("Project Hail Mary")).toBeInTheDocument();
  });

  it("shows a director's movies, newest first, reached from a movie's Cast & Crew, with no Rename", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(await screen.findByRole("link", { name: /^Denis Villeneuve/ }));
    expect(await screen.findByRole("heading", { name: "Denis Villeneuve", level: 1 })).toBeInTheDocument();
    const movies = screen.getByRole("heading", { name: "Movies" }).closest("section") ?? document.body;
    expect(
      within(movies)
        .getAllByRole("link", { name: /^Dune/ })
        .map((a) => a.getAttribute("href")),
    ).toEqual([`/works/${mockIds.dunePartTwo}`, `/works/${mockIds.dune}`]);
    expect(screen.queryByRole("button", { name: "Rename" })).not.toBeInTheDocument();
  });

  it("renames, and follows a merge to the person the books now credit", async () => {
    const api = serveMockApi();
    // The server merges Jefferson Mays into Ray Porter, and answers with
    // Ray Porter's page.
    onRequest("patch", "/people/{id}", () => {
      const porter = api.people[bookIds.porter];
      if (porter) api.people[bookIds.mays] = porter;
    });
    const { user, router } = renderApp(`/people/${bookIds.mays}`);
    await user.click(await screen.findByRole("button", { name: "Rename" }));
    const dialog = await screen.findByRole("dialog", { name: "Rename Jefferson Mays" });
    const name = within(dialog).getByRole("textbox", { name: "Name" });
    await user.clear(name);
    await user.type(name, "Ray Porter");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/people/${bookIds.porter}`));
    expect(sent("PATCH", "/people/{id}").at(-1)?.body).toEqual({ name: "Ray Porter" });
  });
});

describe("Series page", () => {
  it("lists the books in series order, labelled with their positions", async () => {
    serveMockApi();
    renderApp(`/book-series/${bookIds.expanse}`);
    expect(await screen.findByRole("heading", { name: "The Expanse", level: 1 })).toBeInTheDocument();
    expect(screen.getByText("3 books")).toBeInTheDocument();
    expect(screen.getAllByText(/^Book /).map((p) => p.textContent)).toEqual(["Book 1", "Book 1.5", "Book 2"]);
  });
});
