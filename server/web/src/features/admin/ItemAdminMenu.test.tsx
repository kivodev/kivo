import { waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { client } from "@/api/client";
import type { Artwork, WorkDetail } from "@/api/schemas";
import { renderApp, screen } from "@/test/render";
import { http, mockId, mockIds, onRequest, problem, scenarios, sent, serveMockApi, server } from "@/test/server";

type UploadInit = { params: { path: { id: string } }; bodySerializer: () => FormData };

const openMenu = async (user: ReturnType<typeof renderApp>["user"], title: string) => {
  await user.click(await screen.findByRole("button", { name: `More actions for ${title}` }));
  return screen.findByRole("menu");
};

describe("Item admin menu", () => {
  it("is for admins only", async () => {
    serveMockApi(scenarios.viewer);
    renderApp(`/works/${mockIds.dunePartTwo}`);
    await screen.findByRole("heading", { name: "Dune: Part Two" });
    expect(screen.queryByRole("button", { name: /^Admin actions/ })).not.toBeInTheDocument();
  });

  it("offers Unmatch on movies and series only", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    const menu = await openMenu(user, "Dune: Part Two");
    expect(within(menu).getByRole("menuitem", { name: "Fix match" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Unmatch" })).toBeInTheDocument();
  });

  it("chooses artwork from what is offered, by type", async () => {
    const api = serveMockApi();
    onRequest("post", "/works/{id}/artwork/{artworkId}/select", (_, { artworkId }) => {
      api.artwork = api.artwork.map((a) => (a.type === "poster" ? { ...a, selected: a.id === artworkId } : a));
    });
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Choose artwork" }));

    const dialog = await screen.findByRole("dialog", { name: "Choose artwork for Dune: Part Two" });
    const tabs = within(dialog).getByRole("tablist", { name: "Artwork type" });
    expect(
      within(tabs)
        .getAllByRole("tab")
        .map((tab) => tab.textContent),
    ).toEqual(["Poster", "Background", "Logo"]);
    const posters = await within(dialog).findByRole("list", { name: "Poster choices" });
    expect(within(posters).getByRole("button", { name: /^Current/ })).toHaveAttribute("aria-pressed", "true");
    const choices = within(posters).getAllByRole("button", { name: "TMDb · English · 2000 × 3000" });
    expect(choices).toHaveLength(3);

    await user.click(choices[1] as HTMLElement);
    await waitFor(() => expect(sent("POST", "/works/{id}/artwork/{artworkId}/select")).toHaveLength(1));
    const chosen = sent("POST", "/works/{id}/artwork/{artworkId}/select")[0]?.url.pathname.split("/").at(-2);
    await waitFor(() =>
      expect(
        within(dialog).getByRole("list", { name: "Poster choices" }).querySelector("[aria-pressed=true] img"),
      ).toHaveAttribute("src", expect.stringContaining(chosen ?? "none")),
    );

    await user.click(within(tabs).getByRole("tab", { name: "Background" }));
    expect(await within(dialog).findByRole("list", { name: "Background choices" })).toBeInTheDocument();
  });

  it("uploads an image, refusing what isn't one", async () => {
    const api = serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Choose artwork" }));
    const dialog = await screen.findByRole("dialog", { name: "Choose artwork for Dune: Part Two" });
    await within(dialog).findByRole("list", { name: "Poster choices" });
    const input = dialog.ownerDocument.querySelector<HTMLInputElement>("input[type=file]");
    if (!input) throw new Error("no file input");

    // The picker offers only images. A file that gets past it, such as by
    // a drop, is still checked.
    await userEvent
      .setup({ applyAccept: false })
      .upload(input, new File(["%PDF"], "notes.pdf", { type: "application/pdf" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Choose a JPEG, PNG or WebP image.");

    // jsdom's FormData and File make no multipart body that Node's fetch
    // can send. So the request is checked as the client builds it, and
    // answered here. The listing then has the upload as the current poster.
    const uploaded = { id: mockId(77_000), width: 1000, height: 1500 };
    const art: Artwork = { id: uploaded.id, type: "poster", source: "upload", selected: true, image: uploaded };
    const post = vi.spyOn(client, "POST").mockImplementation((async (path: string) => {
      if (path !== "/api/v1/works/{id}/artwork") throw new Error(`unexpected POST ${path}`);
      api.artwork = [art, ...api.artwork.map((a) => ({ ...a, selected: false }))];
      return { data: art, response: new Response(null, { status: 201 }) };
    }) as unknown as typeof client.POST);
    const png = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], "poster.png", { type: "image/png" });
    await user.upload(input, png);
    await waitFor(() => expect(post).toHaveBeenCalledOnce());
    const [, init] = post.mock.lastCall ?? [];
    const form = (init as unknown as UploadInit).bodySerializer();
    expect(form.get("type")).toBe("poster");
    expect(form.get("file")).toBe(png);
    await waitFor(() =>
      expect(
        within(dialog)
          .getByRole("button", { name: /^Current/ })
          .querySelector("img"),
      ).toHaveAttribute("src", expect.stringContaining(uploaded.id)),
    );
    expect(within(dialog).queryByRole("alert")).not.toBeInTheDocument();
  });

  it("leaves Unmatch off an episode, which goes with its series", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.severancePilot}`);
    const menu = await openMenu(user, "Good News About Hell");
    expect(within(menu).getByRole("menuitem", { name: "Fix match" })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: "Unmatch" })).not.toBeInTheDocument();
  });

  it("says what can be matched when the server refuses a kind", async () => {
    serveMockApi();
    server.use(http.get("/api/v1/works/{id}/matches", () => problem(409, "kindMismatch")));
    const { user } = renderApp(`/works/${mockIds.severancePilot}`);
    await user.click(within(await openMenu(user, "Good News About Hell")).getByRole("menuitem", { name: "Fix match" }));
    const dialog = await screen.findByRole("dialog", { name: /^Fix match/ });
    expect(await within(dialog).findByText("Only movies and shows can be matched.")).toBeInTheDocument();
  });

  it("unmatches after asking once, keeping the page on the work", async () => {
    const api = serveMockApi();
    onRequest("post", "/works/{id}/unmatch", () => {
      const { overview: _, ...work } = api.works[mockIds.dunePartTwo] as WorkDetail;
      api.works[mockIds.dunePartTwo] = { ...work, matchState: "unmatched" };
    });
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Unmatch" }));

    const confirm = await screen.findByRole("alertdialog", { name: "Unmatch Dune: Part Two?" });
    expect(
      within(confirm).getByText("Removes metadata and artwork until it's matched again. Watch history is kept."),
    ).toBeInTheDocument();
    await user.click(within(confirm).getByRole("button", { name: "Cancel" }));
    expect(sent("POST", "/works/{id}/unmatch")).toEqual([]);

    await user.click(within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Unmatch" }));
    await user.click(
      within(await screen.findByRole("alertdialog", { name: "Unmatch Dune: Part Two?" })).getByRole("button", {
        name: "Unmatch",
      }),
    );
    await waitFor(() => expect(screen.queryByText(/Paul Atreides unites/)).not.toBeInTheDocument());
    expect(sent("POST", `/works/${mockIds.dunePartTwo}/unmatch`)).toHaveLength(1);
    expect(screen.getByRole("heading", { name: "Dune: Part Two" })).toBeInTheDocument();
  });

  it("fixes a match from matches searched by the parsed title and year", async () => {
    const api = serveMockApi();
    onRequest("post", "/works/{id}/match", () => {
      api.works[mockIds.holiday] = {
        ...(api.works[mockIds.holiday] as WorkDetail),
        title: "Holiday",
        matchState: "matched",
      };
    });
    const { user } = renderApp(`/works/${mockIds.holiday}`);
    const menu = await openMenu(user, "Holiday 2019 Final");
    await user.click(within(menu).getByRole("menuitem", { name: "Fix match" }));

    const dialog = await screen.findByRole("dialog", { name: "Fix match: Holiday 2019 Final" });
    expect(within(dialog).getByRole("textbox", { name: "Title" })).toHaveValue("Holiday 2019 Final");
    expect(within(dialog).getByRole("textbox", { name: "Year" })).toHaveValue("2019");
    const matches = await within(dialog).findByRole("list", { name: "Matches" });
    expect(within(matches).getAllByRole("listitem")).toHaveLength(2);

    // Search again under another title.
    const title = within(dialog).getByRole("textbox", { name: "Title" });
    await user.clear(title);
    await user.type(title, "Holiday");
    await user.click(within(dialog).getByRole("button", { name: "Search" }));
    await user.click(await within(dialog).findByRole("button", { name: "Choose Holiday (2019)" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByRole("heading", { name: "Holiday" })).toBeInTheDocument();
    const searches = sent("GET", "/works/{id}/matches").map((r) => r.url.searchParams.get("query"));
    expect(searches).toEqual(["Holiday 2019 Final", "Holiday"]);
    expect(sent("POST", `/works/${mockIds.holiday}/match`)[0]?.body).toEqual({ matchId: "tmdb-1" });
  });

  it("edits fields, sending only the changed ones, and shows them locked", async () => {
    const api = serveMockApi();
    onRequest("patch", "/works/{id}", () => {
      const work = api.works[mockIds.dunePartTwo] as WorkDetail;
      api.works[mockIds.dunePartTwo] = { ...work, genres: ["Science Fiction", "Epic"], lockedFields: ["genres"] };
    });
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    const menu = await openMenu(user, "Dune: Part Two");
    await user.click(within(menu).getByRole("menuitem", { name: "Edit" }));

    const dialog = await screen.findByRole("dialog", { name: "Edit Dune: Part Two" });
    expect(within(dialog).getByRole("textbox", { name: "Title" })).toBeInTheDocument();
    const genres = within(dialog).getByRole("textbox", { name: "Genres" });
    expect(genres).toHaveValue("Science Fiction, Adventure");
    await user.clear(genres);
    await user.type(genres, "Science Fiction, Epic");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(sent("PATCH", `/works/${mockIds.dunePartTwo}`)[0]?.body).toEqual({ genres: ["Science Fiction", "Epic"] });

    await user.click(within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Edit" }));
    const again = await screen.findByRole("dialog", { name: "Edit Dune: Part Two" });
    expect(within(again).getByRole("textbox", { name: "Genres (locked)" })).toBeInTheDocument();
    expect(within(again).getByRole("textbox", { name: "Title" })).toBeInTheDocument();
  });

  it("sets the content rating from the content rating country's list, locked", async () => {
    const api = serveMockApi();
    onRequest("patch", "/works/{id}", () => {
      const work = api.works[mockIds.dunePartTwo] as WorkDetail;
      api.works[mockIds.dunePartTwo] = { ...work, contentRating: "R", lockedFields: ["contentRating"] };
    });
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit Dune: Part Two" });
    await user.click(within(dialog).getByRole("button", { name: /Content rating/ }));
    await user.click(await screen.findByRole("option", { name: "R" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(sent("PATCH", `/works/${mockIds.dunePartTwo}`)[0]?.body).toEqual({ contentRating: "R" });
    expect(await screen.findByText(/ · R · /)).toBeInTheDocument();
  });

  it("refreshes metadata, saying so until the work changes", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(
      within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Refresh metadata" }),
    );
    await waitFor(() => expect(sent("POST", `/works/${mockIds.dunePartTwo}/refresh-metadata`)).toHaveLength(1));
    const menu = await openMenu(user, "Dune: Part Two");
    expect(within(menu).getByRole("menuitem", { name: "Refreshing…" })).toHaveAttribute("aria-disabled", "true");
  });

  it("shows each version's files with their paths, sizes, bitrates and tracks", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(within(await openMenu(user, "Dune: Part Two")).getByRole("menuitem", { name: "Show files" }));
    const dialog = await screen.findByRole("dialog", { name: "Files of Dune: Part Two" });
    expect(await within(dialog).findByRole("heading", { name: "4K HDR" })).toBeInTheDocument();
    expect(within(dialog).getByRole("heading", { name: "1080p" })).toBeInTheDocument();
    expect(
      within(dialog).getByText("/mnt/user/Movies/Dune: Part Two (2024)/Dune: Part Two - 4K HDR.mkv"),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("MKV · 58 GB · 47 Mb/s · 2h 46m")).toBeInTheDocument();
    expect(within(dialog).getByText("HEVC Main 10 4K HDR10")).toBeInTheDocument();
    expect(within(dialog).getByText("English · TrueHD 7.1 · Default")).toBeInTheDocument();
    expect(within(dialog).getByText("French · AC-3 5.1")).toBeInTheDocument();
    expect(within(dialog).getAllByText("English · SRT · External")).toHaveLength(1);
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
