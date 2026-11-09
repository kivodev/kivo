import { fireEvent, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import { renderApp, screen } from "@/test/render";
import { mockIds, sendEvent, sent, serveMockApi } from "@/test/server";

const emptyMyList = (api: MockApi) => {
  const myList = api.lists.find((l) => l.builtIn);
  if (myList) myList.workIds = [];
};

describe("Lists", () => {
  it("shows My List as a row with its count, then the other lists with theirs", async () => {
    serveMockApi();
    const { user, router } = renderApp("/lists");
    expect(await screen.findByRole("heading", { name: "Lists", level: 1 })).toBeInTheDocument();
    const myList = await screen.findByRole("heading", { name: "My List" });
    const row = myList.closest("[data-part='row']") as HTMLElement;
    expect(within(row).getByText("2")).toBeInTheDocument();
    expect(await within(row).findByRole("link", { name: /^Dune/ })).toBeInTheDocument();
    const dateNight = screen.getByRole("link", { name: /^Date Night\s*1$/ });
    await user.click(dateNight);
    await waitFor(() => expect(router.state.location.pathname).toBe(`/lists/${mockIds.dateNight}`));
  });

  it("hides My List's section when it's empty", async () => {
    serveMockApi(emptyMyList);
    renderApp("/lists");
    expect(await screen.findByRole("link", { name: /^Date Night/ })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "My List" })).toBeNull();
  });

  it("says so when there are no lists with anything in them", async () => {
    serveMockApi((api) => {
      emptyMyList(api);
      api.lists = api.lists.filter((l) => l.builtIn);
    });
    renderApp("/lists");
    expect(await screen.findByText("No lists yet")).toBeInTheDocument();
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
    expect(await screen.findByRole("heading", { name: "My List", level: 1 })).toBeInTheDocument();
    expect(await screen.findByText("2 titles")).toBeInTheDocument();
    const card = (await screen.findAllByRole("link", { name: /^The Expanse/ }))[0] as HTMLElement;
    expect(screen.queryByRole("button", { name: "More actions for My List" })).toBeNull();
    fireEvent.contextMenu(card);
    await user.click(await screen.findByRole("menuitem", { name: "Remove from this list" }));
    await waitFor(() => expect(sent("DELETE", `/lists/${mockIds.myList}/works/${mockIds.expanse}`)).toHaveLength(1));
    expect(await screen.findByText("1 title")).toBeInTheDocument();
  });

  it("renames a list, saying when the name is in use", async () => {
    serveMockApi();
    const { user } = renderApp(`/lists/${mockIds.dateNight}`);
    await user.click(await screen.findByRole("button", { name: "More actions for Date Night" }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
    const dialog = await screen.findByRole("dialog");
    const name = within(dialog).getByRole("textbox", { name: /Name/ });
    await user.clear(name);
    await user.paste("my list");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));
    expect(await within(dialog).findByText("A list with that name already exists.")).toBeInTheDocument();
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
    const myList = within(dialog).getByRole("checkbox", { name: /My List/ });
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
    expect(await within(dialog).findByText("A list with that name already exists.")).toBeInTheDocument();
  });
});
