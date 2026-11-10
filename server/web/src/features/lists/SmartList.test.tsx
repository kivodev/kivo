import { fireEvent, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import type { WorkQuery } from "@/api/schemas";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockIds, sent, serveMockApi } from "@/test/server";

const savedQuery = (api: MockApi, change: (query: WorkQuery) => void) => {
  const list = api.lists.find((l) => l.id === mockIds.sciFi);
  if (list?.smart) change(list.smart.query);
};

type User = ReturnType<typeof renderApp>["user"];

const openBuilder = async (user: User) => {
  await user.click(await screen.findByRole("button", { name: "Create" }));
  await user.click(await screen.findByRole("menuitem", { name: "New Smart List" }));
  return screen.findByRole("dialog");
};

const choose = async (user: User, trigger: HTMLElement, option: string) => {
  await user.click(trigger);
  await user.click(await screen.findByRole("option", { name: option }));
};

describe("The Lists screen's + menu", () => {
  it("takes a relative date as a number and a unit, and saves it as a period", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.type(await within(dialog).findByRole("textbox", { name: /Name/ }), "Recent");
    await user.click(within(dialog).getByRole("button", { name: "Add Rule" }));
    const row = dialog.querySelector("[data-part=rule]") as HTMLElement;
    await choose(user, within(row).getByRole("button", { name: /Field/ }), "Date added");
    await choose(user, within(row).getByRole("button", { name: /Condition/ }), "in the last");
    await user.type(within(row).getByRole("textbox", { name: "How many" }), "2");
    await choose(user, within(row).getByRole("button", { name: /Unit/ }), "weeks");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("POST", "/lists").length).toBeGreaterThan(0));
    expect(JSON.stringify(sent("POST", "/lists").at(-1)?.body)).toContain('"values":["P2W"]');
    expect(await screen.findByText(/in the last 2 weeks/)).toBeInTheDocument();
  });

  it("starts a new rule with no field, and Save waits for one", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.type(await within(dialog).findByRole("textbox", { name: /Name/ }), "Unset");
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeEnabled();
    await user.click(within(dialog).getByRole("button", { name: "Add Rule" }));
    const row = dialog.querySelector("[data-part=rule]") as HTMLElement;
    expect(within(row).getByRole("button", { name: /Field/ })).toHaveTextContent("Choose");
    expect(within(row).queryByRole("button", { name: /Condition/ })).toBeNull();
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("offers New List and New Smart List", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    await user.click(await screen.findByRole("button", { name: "Create" }));
    expect(await screen.findByRole("menuitem", { name: "New List" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "New Smart List" })).toBeInTheDocument();
  });

  it("marks a smart list's shelf with its sparkle and says its count", async () => {
    serveMockApi();
    renderApp("/lists");
    const heading = await screen.findByRole("heading", { name: /^Sci-fi/, level: 2 });
    expect(heading).toHaveTextContent(/^Sci-fiSmart list2 titles$/);
    expect(heading.querySelector("svg.size-4")).not.toBeNull();
    const shelf = heading.closest("[data-part='row']") as HTMLElement;
    expect(shelf.querySelectorAll("[data-part='row-cards'] > li")).toHaveLength(2);
  });
});

describe("The smart list builder", () => {
  it("builds a rule with typed and chosen values, shows the live count and saves", async () => {
    serveMockApi();
    const { user, router } = renderApp("/lists");
    const dialog = await openBuilder(user);
    const name = await within(dialog).findByRole("textbox", { name: /Name/ });
    await user.type(name, "Nineties");
    expect(within(dialog).getByRole("button", { name: "Show: Movies" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Libraries: All" })).toBeInTheDocument();

    // A first count of every movie, then the rules narrow it.
    await within(dialog).findByText(/movies match/);

    await user.click(within(dialog).getByRole("button", { name: "Add Rule" }));
    const row = () => dialog.querySelector("[data-part=rule]") as HTMLElement;
    await choose(user, within(row()).getByRole("button", { name: /Field/ }), "Year");
    await choose(user, within(row()).getByRole("button", { name: /Condition/ }), "between");
    const inputs = within(row()).getAllByRole("textbox", { name: "Year" });
    expect(inputs).toHaveLength(2);
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
    await user.type(inputs[0] as HTMLElement, "2020");
    await user.type(inputs[1] as HTMLElement, "2024");

    await user.click(within(dialog).getByRole("button", { name: "Add Rule" }));
    const genre = dialog.querySelectorAll("[data-part=rule]")[1] as HTMLElement;
    await choose(user, within(genre).getByRole("button", { name: /Field/ }), "Genre");
    await user.click(within(genre).getByRole("button", { name: /Genre:/ }));
    await user.click(await screen.findByRole("checkbox", { name: "Science Fiction" }));
    await user.keyboard("{Escape}");

    await waitFor(() =>
      expect(sent("POST", "/works/query").at(-1)?.body).toEqual({
        limit: 1,
        query: {
          kinds: ["movie"],
          where: {
            match: "all",
            rules: [
              { field: "year", op: "between", values: ["2020", "2024"] },
              { field: "genre", op: "anyOf", values: ["Science Fiction"] },
            ],
          },
        },
      }),
    );
    await within(dialog).findByText("1 movie matches");

    const save = within(dialog).getByRole("button", { name: "Save" });
    await waitFor(() => expect(save).toBeEnabled());
    await user.click(save);
    await waitFor(() => expect(router.state.location.pathname).toMatch(/^\/lists\/./));
    expect(sent("POST", "/lists").at(-1)?.body).toEqual({
      name: "Nineties",
      smart: {
        query: {
          kinds: ["movie"],
          where: {
            match: "all",
            rules: [
              { field: "year", op: "between", values: ["2020", "2024"] },
              { field: "genre", op: "anyOf", values: ["Science Fiction"] },
            ],
          },
        },
      },
    });
  });

  it("searches a person field's values and picks one by name", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.click(await within(dialog).findByRole("button", { name: "Add Rule" }));
    const row = dialog.querySelector("[data-part=rule]") as HTMLElement;
    await choose(user, within(row).getByRole("button", { name: /Field/ }), "Director");
    await user.click(within(row).getByRole("button", { name: /Director:/ }));
    await user.type(await screen.findByRole("searchbox", { name: "Search" }), "denis");
    await waitFor(() => expect(sent("GET", "/work-fields/{field}/values").at(-1)?.url.search).toBe("?q=denis"));
    await user.click(await screen.findByRole("checkbox", { name: "Denis Villeneuve" }));
    await user.keyboard("{Escape}");
    expect(await within(row).findByRole("button", { name: "Director: Denis Villeneuve" })).toBeInTheDocument();
  });

  it("adds a group of rules, and removing its last rule removes the group", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.type(await within(dialog).findByRole("textbox", { name: /Name/ }), "Grouped");
    await user.click(within(dialog).getByRole("button", { name: "Add Group" }));
    const row = () => dialog.querySelector("[data-part=rule]") as HTMLElement;
    await choose(user, within(row()).getByRole("button", { name: /Field/ }), "Title");
    await user.type(within(row()).getByRole("textbox", { name: "Title" }), "dune");
    await choose(user, within(dialog).getByRole("button", { name: /Match/ }), "All");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("POST", "/lists")).toHaveLength(1));
    expect(sent("POST", "/lists")[0]?.body).toEqual({
      name: "Grouped",
      smart: {
        query: {
          kinds: ["movie"],
          where: {
            match: "all",
            groups: [{ match: "all", rules: [{ field: "title", op: "contains", values: ["dune"] }] }],
          },
        },
      },
    });
  });

  it("removes a group with its last rule", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.click(await within(dialog).findByRole("button", { name: "Add Group" }));
    expect(within(dialog).getAllByRole("button", { name: /Match/ })).toHaveLength(1);
    await user.click(within(dialog).getByRole("button", { name: "Remove rule" }));
    expect(within(dialog).queryAllByRole("button", { name: /Match/ })).toHaveLength(0);
  });

  it("starts a new group at Match Any, and doesn't let the root's match be picked", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    expect(await within(dialog).findByText("Match All of These")).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: /Match/ })).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Add Group" }));
    expect(within(dialog).getByRole("button", { name: /Match/ })).toHaveTextContent("Any");
    expect(dialog.querySelectorAll("[data-part=rule]")).toHaveLength(1);
  });

  it("shows a saved root matching any as it is, its library rule among the rules", async () => {
    serveMockApi((api) =>
      savedQuery(api, (q) => {
        q.where = {
          match: "any",
          rules: [
            { field: "library", op: "anyOf", values: [mockIds.movies] },
            { field: "genre", op: "anyOf", values: ["Comedy"] },
          ],
        };
      }),
    );
    const { user } = renderApp(`/lists/${mockIds.sciFi}`);
    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Match Any of These")).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: /^Libraries:/ })).toBeNull();
    expect(dialog.querySelectorAll("[data-part=rule]")).toHaveLength(2);
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("PATCH", `/lists/${mockIds.sciFi}`)).toHaveLength(1));
    expect(sent("PATCH", `/lists/${mockIds.sciFi}`)[0]?.body).toMatchObject({
      smart: {
        query: {
          where: {
            match: "any",
            rules: [
              { field: "library", op: "anyOf", values: [mockIds.movies] },
              { field: "genre", op: "anyOf", values: ["Comedy"] },
            ],
          },
        },
      },
    });
  });

  it("offers each sort in both directions, and saves the one chosen", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.type(await within(dialog).findByRole("textbox", { name: /Name/ }), "Worst");
    await user.click(within(dialog).getByRole("button", { name: /Sort/ }));
    for (const name of [
      "Title, A to Z",
      "Title, Z to A",
      "Rating, highest first",
      "Rating, lowest first",
      "Runtime, shortest first",
      "Runtime, longest first",
      "Date added, newest first",
      "Date added, oldest first",
    ]) {
      expect(await screen.findByRole("option", { name })).toBeInTheDocument();
    }
    await user.click(screen.getByRole("option", { name: "Rating, lowest first" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("POST", "/lists")).toHaveLength(1));
    expect(sent("POST", "/lists")[0]?.body).toMatchObject({
      smart: { query: { sort: [{ field: "rating", direction: "asc" }] } },
    });
  });

  it("offers only the sorts of the chosen kinds, dropping one that no longer fits", async () => {
    serveMockApi((api) => {
      const runtime = api.workFields.find((f) => f.field === "runtimeMs");
      if (runtime) runtime.kinds = ["movie"];
    });
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    const sort = await within(dialog).findByRole("button", { name: /Sort/ });
    await choose(user, sort, "Runtime, longest first");
    expect(sort).toHaveTextContent("Runtime, longest first");

    await user.click(within(dialog).getByRole("button", { name: "Show: Movies" }));
    await user.click(await screen.findByRole("checkbox", { name: "Shows" }));
    await user.click(screen.getByRole("checkbox", { name: "Movies" }));
    await user.keyboard("{Escape}");

    await waitFor(() => expect(sort).toHaveTextContent("Title, A to Z"));
    await user.click(sort);
    expect(await screen.findByRole("option", { name: "Rating, highest first" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /^Runtime/ })).toBeNull();
  });

  it("offers only sortable fields, and saves random alone", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.type(await within(dialog).findByRole("textbox", { name: /Name/ }), "Shuffle");
    await user.click(within(dialog).getByRole("button", { name: /Sort/ }));
    expect(await screen.findByRole("option", { name: "Rating, highest first" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /^Genre/ })).toBeNull();
    await user.click(screen.getByRole("option", { name: "Random" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("POST", "/lists")).toHaveLength(1));
    expect(sent("POST", "/lists")[0]?.body).toMatchObject({ smart: { query: { sort: [{ field: "random" }] } } });
  });

  it("says when the name is in use", async () => {
    serveMockApi();
    const { user } = renderApp("/lists");
    const dialog = await openBuilder(user);
    await user.type(await within(dialog).findByRole("textbox", { name: /Name/ }), "date night");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByText("You already have a list with that name.")).toBeInTheDocument();
  });

  it("refuses a query of a newer version, saying so", async () => {
    serveMockApi((api) => savedQuery(api, (q) => Object.assign(q, { version: 2 })));
    const { user } = renderApp(`/lists/${mockIds.sciFi}`);
    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText(/made by a newer version of Kivo/)).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("refuses a field missing from the catalog, saying so", async () => {
    serveMockApi((api) =>
      savedQuery(api, (q) => {
        q.where = { match: "all", rules: [{ field: "mood", op: "anyOf", values: ["cozy"] }] };
      }),
    );
    const { user } = renderApp(`/lists/${mockIds.sciFi}`);
    await user.click(await screen.findByRole("button", { name: "Edit" }));
    expect(await within(await screen.findByRole("dialog")).findByText(/rule this version/)).toBeInTheDocument();
    expect(sent("PATCH", `/lists/${mockIds.sciFi}`)).toHaveLength(0);
  });

  it("edits a saved query, keeping what it doesn't show and saving a list with errors", async () => {
    serveMockApi((api) => {
      const list = api.lists.find((l) => l.id === mockIds.sciFi);
      if (!list?.smart) return;
      list.smart.errors = [{ field: "/smart/query/where/rules/0/values/0", code: "invalid" }];
      list.smart.query.maxWorks = 25;
      list.workIds = [];
    });
    const { user } = renderApp(`/lists/${mockIds.sciFi}`);
    expect(await screen.findByText("These rules no longer work")).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByRole("textbox", { name: /Name/ })).toHaveValue("Sci-fi");
    expect(await within(dialog).findByRole("button", { name: "Genre: Science Fiction" })).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("PATCH", `/lists/${mockIds.sciFi}`)).toHaveLength(1));
    expect(sent("PATCH", `/lists/${mockIds.sciFi}`)[0]?.body).toEqual({
      name: "Sci-fi",
      smart: {
        query: {
          kinds: ["movie"],
          where: { match: "all", rules: [{ field: "genre", op: "anyOf", values: ["Science Fiction"] }] },
          sort: [{ field: "rating" }],
          maxWorks: 25,
        },
      },
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});

describe("A smart list's page", () => {
  it("shows its count, a line of its rules and its grid, with Edit and Delete and no Remove", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/lists/${mockIds.sciFi}`);
    expect(await screen.findByRole("heading", { name: "Sci-fi", level: 1 })).toBeInTheDocument();
    expect(await screen.findByText("2 movies. Science Fiction")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();

    const card = (await screen.findAllByRole("link", { name: /^Dune/ }))[0] as HTMLElement;
    fireEvent.contextMenu(card);
    expect(await screen.findByRole("menu")).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Remove from Sci-fi" })).toBeNull();
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "More actions for Sci-fi" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/lists"));
    expect(sent("DELETE", `/lists/${mockIds.sciFi}`)).toHaveLength(1);
  });

  it("reads its rules: values with or, units, durations, set rules and groups", async () => {
    serveMockApi((api) =>
      savedQuery(api, (q) => {
        q.where = {
          match: "all",
          rules: [
            { field: "library", op: "anyOf", values: [mockIds.movies] },
            { field: "genre", op: "anyOf", values: ["Comedy", "Drama"] },
            { field: "genre", op: "noneOf", values: ["Horror", "Sport"] },
            { field: "runtimeMs", op: "gte", values: ["5400000"] },
            { field: "year", op: "isNotSet" },
            { field: "rating", op: "isSet" },
            { field: "addedAt", op: "within", values: ["P30D"] },
          ],
          groups: [
            {
              match: "any",
              rules: [
                { field: "year", op: "between", values: ["1990", "1999"] },
                { field: "title", op: "contains", values: ["dune"] },
              ],
            },
            {
              match: "all",
              rules: [
                { field: "rating", op: "gte", values: ["7"] },
                { field: "year", op: "lte", values: ["2000"] },
              ],
            },
          ],
        };
      }),
    );
    renderApp(`/lists/${mockIds.sciFi}`);
    expect(
      await screen.findByText(
        "2 movies. Comedy or Drama, no Horror or Sport, Runtime at least 90 min, Year isn't set, Rating is set, " +
          "Date added in the last 30 days, (Year 1990 to 1999 or Title contains dune), (Rating at least 7 and Year at most 2000)",
      ),
    ).toBeInTheDocument();
  });

  it("says when it has nothing in it, a manual list's words", async () => {
    serveMockApi((api) => {
      const list = api.lists.find((l) => l.id === mockIds.dateNight);
      if (list) list.workIds = [];
    });
    renderApp(`/lists/${mockIds.dateNight}`);
    expect(await screen.findByText("Nothing in this list yet.")).toBeInTheDocument();
  });

  it("renames without sending the query", async () => {
    serveMockApi();
    const { user } = renderApp(`/lists/${mockIds.sciFi}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Sci-fi" }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
    const dialog = await screen.findByRole("dialog");
    await user.clear(within(dialog).getByRole("textbox", { name: /Name/ }));
    await user.paste("Space");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));
    await waitFor(() => expect(sent("PATCH", `/lists/${mockIds.sciFi}`).at(-1)?.body).toEqual({ name: "Space" }));
  });
});

describe("Add to List", () => {
  it("doesn't offer smart lists", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Dune: Part Two" }));
    await user.click(await screen.findByRole("menuitem", { name: "Add to List" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByRole("checkbox", { name: /Date Night/ });
    expect(within(dialog).queryByRole("checkbox", { name: /Sci-fi/ })).toBeNull();
  });
});

describe("Save as Smart List", () => {
  const movies = `/libraries/${mockIds.movies}`;

  it("turns a filtered grid into a query named for the library, dropping the match state", async () => {
    serveMockApi();
    const { user, router } = renderApp(
      `${movies}?genre=%5B%22Comedy%22%5D&yearFrom=1990&yearTo=1999&played=unplayed&sort=rating&matchState=unmatched`,
    );
    await user.click(await screen.findByRole("button", { name: "Save as Smart List" }));
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByRole("textbox", { name: /Name/ })).toHaveValue("Movies");
    expect(await within(dialog).findByRole("button", { name: "Libraries: Movies" })).toBeInTheDocument();
    expect(await within(dialog).findByRole("button", { name: "Genre: Comedy" })).toBeInTheDocument();
    await user.clear(within(dialog).getByRole("textbox", { name: /Name/ }));
    await user.paste("Nineties");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(router.state.location.pathname).toMatch(/^\/lists\//));
    expect(sent("POST", "/lists").at(-1)?.body).toEqual({
      name: "Nineties",
      smart: {
        query: {
          kinds: ["movie"],
          where: {
            match: "all",
            rules: [
              { field: "library", op: "anyOf", values: [mockIds.movies] },
              { field: "playState", op: "anyOf", values: ["unplayed"] },
              { field: "genre", op: "anyOf", values: ["Comedy"] },
              { field: "year", op: "between", values: ["1990", "1999"] },
            ],
          },
          sort: [{ field: "rating" }],
        },
      },
    });
    expect(locationOf(router)).toContain("/lists/");
  });

  it("isn't offered for only a match state filter or with no filters", async () => {
    serveMockApi();
    renderApp(`${movies}?matchState=unmatched`);
    await screen.findByRole("button", { name: "Filter, some on" });
    expect(screen.queryByRole("button", { name: "Save as Smart List" })).toBeNull();
  });
});
