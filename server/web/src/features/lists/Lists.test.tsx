import { act, fireEvent, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import { renderApp, screen } from "@/test/render";
import { http, mockIds, problem, sendEvent, sent, serveMockApi, server } from "@/test/server";

const emptyMyList = (api: MockApi) => {
  const myList = api.lists.find((l) => l.builtIn);
  if (myList) myList.workIds = [];
};

describe("Lists", () => {
  it("shows every list as a shelf of its titles, Watchlist first and the others by name", async () => {
    serveMockApi();
    const { user, router } = renderApp("/lists");
    expect(await screen.findByRole("heading", { name: "Lists", level: 1 })).toBeInTheDocument();
    await screen.findByRole("heading", { name: /^Watchlist/ });
    const names = screen.getAllByRole("heading", { level: 2 }).map((heading) => heading.textContent);
    expect(names).toEqual([
      "Watchlist2 titles",
      "Date Night1 title",
      "Every MovieSmart list238 titles",
      "Sci-fiSmart list2 titles",
      "Shows Recently AddedSmart list2 titles",
      "WeekendEmpty",
    ]);
    const shelf = (name: RegExp) =>
      screen.getByRole("heading", { name, level: 2 }).closest("[data-part='row']") as HTMLElement;
    expect(within(shelf(/^Watchlist/)).getByRole("link", { name: /^The Expanse/ })).toBeInTheDocument();
    // A shelf holds as many titles as a Home row.
    expect(within(shelf(/^Every Movie/)).getAllByRole("listitem")).toHaveLength(20);
    // A title on a manual list's shelf can be taken off it.
    fireEvent.contextMenu(within(shelf(/^Date Night/)).getByRole("link", { name: /^Dune/ }));
    await user.click(await screen.findByRole("menuitem", { name: "Remove from Date Night" }));
    await waitFor(() => expect(sent("DELETE", `/lists/${mockIds.dateNight}/works/${mockIds.dune}`)).toHaveLength(1));
    fireEvent.contextMenu(within(shelf(/^Sci-fi/)).getAllByRole("link", { name: /^Dune/ })[0] as HTMLElement);
    await screen.findByRole("menuitem", { name: "Add to List" });
    expect(screen.queryByRole("menuitem", { name: /^Remove from/ })).toBeNull();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("link", { name: "Sci-fi" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/lists/${mockIds.sciFi}`));
  });

  it("shows an empty list's shelf as a placeholder that opens the list", async () => {
    serveMockApi();
    renderApp("/lists");
    const weekend = (await screen.findByRole("heading", { name: /^Weekend/ })).closest(
      "[data-part='row']",
    ) as HTMLElement;
    const placeholder = within(weekend).getByRole("link", { name: "Nothing in this list yet" });
    expect(placeholder).toHaveAttribute("href", `/lists/${mockIds.weekend}`);
    expect(weekend.querySelector("img")).toBeNull();
  });

  it("hides Watchlist's section when it's empty", async () => {
    serveMockApi(emptyMyList);
    renderApp("/lists");
    expect(await screen.findByRole("link", { name: /^Date Night/ })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Watchlist" })).toBeNull();
  });

  it("says so when there are no lists with anything in them", async () => {
    serveMockApi((api) => {
      emptyMyList(api);
      api.lists = api.lists.filter((l) => l.builtIn);
    });
    renderApp("/lists");
    expect(await screen.findByText("Nothing in your lists yet.")).toBeInTheDocument();
  });

  it("refetches on a listsChanged event", async () => {
    let api: MockApi | undefined;
    serveMockApi((a) => {
      api = a;
    });
    renderApp("/lists");
    await screen.findByRole("link", { name: /^Date Night/ });
    if (!api) throw new Error("no api");
    api.lists = api.lists.filter((l) => l.builtIn);
    sendEvent({ type: "event", kind: "listsChanged" });
    await waitFor(() => expect(screen.queryByRole("link", { name: /^Date Night/ })).toBeNull(), { timeout: 3000 });
  });
});

describe("New List on the Lists screen", () => {
  it("names a new list and stays on the screen", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    await user.click(await screen.findByRole("button", { name: "Create" }));
    await user.click(await screen.findByRole("menuitem", { name: "New List" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByRole("textbox", { name: /Name/ }), "Road Trip");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(sent("POST", "/lists")[0]?.body).toEqual({ name: "Road Trip" });
    expect(await screen.findByRole("link", { name: /Road Trip/ })).toBeInTheDocument();
  });
});

describe("A list's page", () => {
  it("shows its works as a grid with their count, and takes a card off from its menu", async () => {
    serveMockApi();
    const { user } = renderApp(`/lists/${mockIds.myList}`);
    expect(await screen.findByRole("heading", { name: "Watchlist", level: 1 })).toBeInTheDocument();
    expect(await screen.findByText("2 titles")).toBeInTheDocument();
    const card = (await screen.findAllByRole("link", { name: /^The Expanse/ }))[0] as HTMLElement;
    fireEvent.contextMenu(card);
    await user.click(await screen.findByRole("menuitem", { name: "Remove from Watchlist" }));
    await waitFor(() => expect(sent("DELETE", `/lists/${mockIds.myList}/works/${mockIds.expanse}`)).toHaveLength(1));
    expect(await screen.findByText("1 title")).toBeInTheDocument();
  });

  it("sorts a manual list from its count line, and its shelf follows", async () => {
    serveMockApi((api) => {
      api.lists.find((l) => l.builtIn)?.workIds.reverse();
    });
    const { user } = renderApp(`/lists/${mockIds.myList}`);
    const first = () => screen.getAllByRole("link", { name: /^(Dune|The Expanse)/ })[0];
    const sort = await screen.findByRole("button", { name: /Sort/ });
    expect(sort).toHaveTextContent("Custom order");
    await waitFor(() => expect(first()).toHaveAccessibleName(/^The Expanse/));
    await user.click(sort);
    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
      "Custom order",
      "Title",
      "Release date",
      "Rating",
      "Date added to list",
    ]);
    await user.click(screen.getByRole("option", { name: "Title" }));
    await waitFor(() => expect(sent("PATCH", `/lists/${mockIds.myList}`).at(-1)?.body).toEqual({ sort: "title" }));
    await waitFor(() => expect(screen.getByRole("button", { name: /Sort/ })).toHaveTextContent("A–Z"));
    await waitFor(() => expect(first()).toHaveAccessibleName(/^Dune/));
    // A smart list's rules order it, so it has no sort.
    await user.click(screen.getAllByRole("link", { name: "Lists" }).at(-1) as HTMLElement);
    const shelf = (await screen.findByRole("heading", { name: /^Watchlist/, level: 2 })).closest(
      "[data-part='row']",
    ) as HTMLElement;
    expect(within(shelf).getAllByRole("link", { name: /^(Dune|The Expanse)/ })[0]).toHaveAccessibleName(/^Dune/);
    await user.click(screen.getByRole("link", { name: "Sci-fi" }));
    expect(await screen.findByRole("heading", { name: "Sci-fi", level: 1 })).toBeInTheDocument();
    await screen.findByText(/2 titles|Science Fiction/);
    expect(screen.queryByRole("button", { name: /Sort/ })).toBeNull();
  });

  it("reorders a manual list by its rows' handles, from the keyboard, until Done", async () => {
    serveMockApi((api) => {
      const myList = api.lists.find((l) => l.builtIn);
      if (myList) myList.sort = "title";
    });
    const { user } = renderApp(`/lists/${mockIds.myList}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Watchlist" }));
    // Watchlist can be reordered, and not renamed or deleted.
    expect(screen.getAllByRole("menuitem").map((item) => item.textContent)).toEqual(["Reorder"]);
    await user.click(screen.getByRole("menuitem", { name: "Reorder" }));
    const order = await screen.findByRole("grid", { name: "Reorder Watchlist" });
    const rows = () =>
      within(order)
        .getAllByRole("row")
        .map((row) => row.textContent);
    expect(rows()).toEqual(["Dune2021", "The Expanse2015"]);
    expect(screen.queryByRole("button", { name: /Sort/ })).toBeNull();
    expect(sent("GET", `/lists/${mockIds.myList}/works`).at(-1)?.url.search).toContain("sort=custom");

    const focused = () => document.activeElement?.getAttribute("aria-label");
    act(() => within(order).getAllByRole("row")[0]?.focus());
    await user.keyboard("{ArrowRight}");
    expect(focused()).toBe("Drag Dune");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(focused()).toMatch(/^Insert /));
    await user.keyboard("{ArrowDown}{Enter}");
    await waitFor(() => expect(sent("POST", `/lists/${mockIds.myList}/move`)).toHaveLength(1));
    expect(sent("POST", `/lists/${mockIds.myList}/move`)[0]?.body).toEqual({
      workIds: [mockIds.dune],
      after: mockIds.expanse,
    });
    await waitFor(() => expect(rows()).toEqual(["The Expanse2015", "Dune2021"]));

    // Done shows the grid again, in the custom order a move sets.
    await user.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("grid", { name: "Reorder Watchlist" })).toBeNull());
    expect(await screen.findByRole("button", { name: /Sort/ })).toHaveTextContent("Custom order");
    expect(screen.getAllByRole("link", { name: /^(Dune|The Expanse)/ })[0]).toHaveAccessibleName(/^The Expanse/);
  });

  it("puts a row back when its move fails", async () => {
    serveMockApi();
    server.use(http.post("/api/v1/lists/{id}/move", () => problem(409, "conflict")));
    const { user } = renderApp(`/lists/${mockIds.myList}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Watchlist" }));
    await user.click(screen.getByRole("menuitem", { name: "Reorder" }));
    const order = await screen.findByRole("grid", { name: "Reorder Watchlist" });
    act(() => within(order).getAllByRole("row")[0]?.focus());
    await user.keyboard("{ArrowRight}{Enter}");
    await waitFor(() => expect(document.activeElement?.getAttribute("aria-label")).toMatch(/^Insert /));
    await user.keyboard("{ArrowDown}{Enter}");
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(within(order).getAllByRole("row")[0]).toHaveTextContent("Dune");
  });

  it("renames a list, saying when the name is in use", async () => {
    serveMockApi();
    const { user } = renderApp(`/lists/${mockIds.dateNight}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Date Night" }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
    const dialog = await screen.findByRole("dialog");
    const name = within(dialog).getByRole("textbox", { name: /Name/ });
    await user.clear(name);
    await user.paste("watchlist");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));
    expect(await within(dialog).findByText("You already have a list with that name.")).toBeInTheDocument();
    await user.clear(name);
    await user.paste("Road Trip");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));
    expect(await screen.findByRole("heading", { name: "Road Trip", level: 1 })).toBeInTheDocument();
    expect(sent("PATCH", `/lists/${mockIds.dateNight}`).at(-1)?.body).toEqual({ name: "Road Trip" });
  });

  it("deletes a list and returns to Lists", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/lists/${mockIds.dateNight}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Date Night" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/lists"));
    expect(sent("DELETE", `/lists/${mockIds.dateNight}`)).toHaveLength(1);
  });
});

describe("Add to List", () => {
  const openChooser = async (user: ReturnType<typeof renderApp>["user"]) => {
    await user.click(await screen.findByRole("button", { name: "More actions for Dune: Part Two" }));
    await user.click(await screen.findByRole("menuitem", { name: "Add to List" }));
    return screen.findByRole("dialog");
  };

  it("checks the lists holding the work, and a press adds or removes", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    const dialog = await openChooser(user);
    const dateNight = await within(dialog).findByRole("checkbox", { name: /Date Night/ });
    const myList = within(dialog).getByRole("checkbox", { name: /Watchlist/ });
    expect(dateNight).not.toBeChecked();
    expect(myList).not.toBeChecked();
    await user.click(dateNight);
    await waitFor(() => expect(dateNight).toBeChecked());
    await waitFor(() =>
      expect(sent("POST", `/lists/${mockIds.dateNight}/works`)[0]?.body).toEqual({ workId: mockIds.dunePartTwo }),
    );
    await user.click(dateNight);
    await waitFor(() =>
      expect(sent("DELETE", `/lists/${mockIds.dateNight}/works/${mockIds.dunePartTwo}`)).toHaveLength(1),
    );
    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("makes a new list and adds the work to it", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    const dialog = await openChooser(user);
    await user.click(await within(dialog).findByRole("button", { name: "New List" }));
    await user.type(within(dialog).getByRole("textbox", { name: /Name/ }), "Road Trip");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    const created = await within(dialog).findByRole("checkbox", { name: /Road Trip/ });
    await waitFor(() => expect(created).toBeChecked());
    expect(sent("POST", "/lists")[0]?.body).toEqual({ name: "Road Trip" });
  });

  it("says when a new list's name is in use", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    const dialog = await openChooser(user);
    await user.click(await within(dialog).findByRole("button", { name: "New List" }));
    await user.type(within(dialog).getByRole("textbox", { name: /Name/ }), "date night");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(await within(dialog).findByText("You already have a list with that name.")).toBeInTheDocument();
  });
});
