import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import type { Library } from "@/api/schemas";
import { renderApp, screen } from "@/test/render";
import { mockIds, onRequest, scenarios, sendEvent, sent, serveMockApi } from "@/test/server";

const rowOf = (name: string) => {
  const heading = screen.getByRole("heading", { name });
  const row = heading.closest<HTMLElement>('[data-part="admin-library"]');
  if (!row) throw new Error(`no row for ${name}`);
  return row;
};

const movies = (api: MockApi) => api.libraries.find((l) => l.id === mockIds.movies) as Library;

describe("Admin Libraries", () => {
  it("lists each library with its count by kind and when it was scanned, and nothing more while idle", async () => {
    serveMockApi();
    renderApp("/admin/libraries");
    await screen.findByRole("heading", { name: "Libraries" });
    const row = rowOf("Movies");
    expect(within(row).getByText("238 movies").closest("p")).toHaveTextContent(/^238 movies · Scanned 2 hours ago$/);
    expect(within(row).getByText("Scanned 2 hours ago").closest("time")).toHaveAttribute("title");
    expect(within(row).queryByText("/mnt/user/Movies")).not.toBeInTheDocument();
    expect(row.querySelector('[data-part="scan"]')).toBeNull();
    expect(within(rowOf("TV Shows")).getByText("Not scanned yet")).toBeInTheDocument();
  });

  it("warns of a folder it can't reach", async () => {
    serveMockApi(scenarios.unreachable);
    renderApp("/admin/libraries");
    await screen.findByRole("heading", { name: "Libraries" });
    expect(within(rowOf("TV Shows")).getByText("Can't reach /mnt/user/TV")).toBeInTheDocument();
    expect(within(rowOf("Movies")).queryByText(/Can't reach/)).not.toBeInTheDocument();
  });

  it("scans, and shows the step it's on in one line as it arrives", async () => {
    const api = serveMockApi();
    onRequest("post", "/libraries/{id}/stop-scan", () => {
      movies(api).scan = { libraryId: mockIds.movies, state: "idle" };
    });
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "Scan Movies" }));
    await waitFor(() => expect(sent("POST", `/libraries/${mockIds.movies}/scan`)).toHaveLength(1));
    movies(api).scan = {
      libraryId: mockIds.movies,
      state: "scanning",
      progress: { step: "probe", done: 1240, total: 3100, remainingMs: 4 * 60_000 },
    };
    sendEvent({ type: "event", kind: "tasksChanged" });

    const probe = await waitFor(
      () => {
        const step = rowOf("Movies").querySelector<HTMLElement>('[data-part="step-progress"][data-step="probe"]');
        expect(step).not.toBeNull();
        return step as HTMLElement;
      },
      { timeout: 3000 },
    );
    expect(probe).toHaveTextContent("Scanning · Reading files");
    expect(probe).toHaveTextContent("1,240 of 3,100 · about 4m left");

    await user.click(within(rowOf("Movies")).getByRole("button", { name: "Stop" }));
    await waitFor(() => expect(rowOf("Movies").querySelector('[data-part="scan"]')).toBeNull());
    // A library's own scan shows only on its row, so Tasks has nothing to
    // follow.
    expect(screen.queryByText(/View progress in/)).not.toBeInTheDocument();
  });

  it("shows Matching with no count or Stop while its files are being identified, then nothing", async () => {
    const api = serveMockApi();
    movies(api).scan = { libraryId: mockIds.movies, state: "matching" };
    renderApp("/admin/libraries");
    await screen.findByRole("heading", { name: "Libraries" });
    expect(rowOf("Movies").querySelector('[data-part="scan"]')).toHaveTextContent(/^Matching$/);
    expect(within(rowOf("Movies")).queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
    expect(rowOf("Movies").querySelector('[role="progressbar"]')).toBeNull();

    movies(api).scan = { libraryId: mockIds.movies, state: "idle" };
    sendEvent({ type: "event", kind: "tasksChanged" });
    await waitFor(() => expect(rowOf("Movies").querySelector('[data-part="scan"]')).toBeNull(), { timeout: 3000 });
  });

  it("refreshes all metadata from the menu after asking", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Refresh all metadata" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Refresh all metadata for Movies?" });
    await user.click(within(confirm).getByRole("button", { name: "Refresh all metadata" }));
    expect(sent("POST", `/libraries/${mockIds.movies}/refresh-metadata`)).toHaveLength(1);
  });

  it("offers refresh, edit and delete in a library's menu", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for TV Shows" }));
    expect((await screen.findAllByRole("menuitem")).map((m) => m.textContent)).toEqual([
      "Refresh all metadata",
      "Edit",
      "Delete library",
    ]);
  });

  it("names the refresh metadata setting's choices", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit Movies" });
    await user.click(within(dialog).getByRole("button", { name: /Refresh metadata/ }));
    expect((await screen.findAllByRole("option")).map((o) => o.textContent)).toEqual([
      "Never",
      "Every 30 days",
      "Every 60 days",
      "Every 90 days",
    ]);
  });

  it("names the preview thumbnail setting's choices", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit Movies" });
    await user.click(within(dialog).getByRole("button", { name: /Preview thumbnails/ }));
    expect((await screen.findAllByRole("option")).map((o) => o.textContent)).toEqual([
      "Never",
      "Nightly",
      "Nightly and when media is added",
    ]);
  });

  it("sets when a library prepares subtitles", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit Movies" });
    await user.click(within(dialog).getByRole("button", { name: /Subtitles/ }));
    await user.click(await screen.findByRole("option", { name: "Nightly" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(sent("PATCH", `/libraries/${mockIds.movies}`)[0]?.body).toEqual({ subtitles: "nightly" }),
    );
  });

  it("offers intro and credits markers for a shows library only", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    let dialog = await screen.findByRole("dialog", { name: "Edit Movies" });
    expect(within(dialog).queryByRole("button", { name: /Intro and credits markers/ })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "More actions for TV Shows" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    dialog = await screen.findByRole("dialog", { name: "Edit TV Shows" });
    await user.click(within(dialog).getByRole("button", { name: /Intro and credits markers/ }));
    await user.click(await screen.findByRole("option", { name: "Nightly" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("PATCH", `/libraries/${mockIds.shows}`)[0]?.body).toEqual({ markers: "nightly" }));
  });

  it("edits a library's name, folders and options", async () => {
    const api = serveMockApi();
    onRequest("patch", "/libraries/{id}", () => {
      movies(api).name = "Films";
    });
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit Movies" });

    const name = within(dialog).getByRole("textbox", { name: "Name" });
    await user.clear(name);
    await user.type(name, "Films");
    await user.click(within(dialog).getByRole("switch", { name: "Use local metadata" }));
    await user.click(within(dialog).getByRole("button", { name: /Preview thumbnails/ }));
    await user.click(await screen.findByRole("option", { name: "Never" }));
    await user.click(within(dialog).getByRole("button", { name: /Metadata language/ }));
    await user.click(await screen.findByRole("option", { name: "German (Germany)" }));
    await user.click(within(dialog).getByRole("button", { name: /Certification country/ }));
    await user.click(await screen.findByRole("option", { name: "Germany" }));
    // Add a second folder through the folder picker.
    await user.click(await within(dialog).findByRole("button", { name: "mnt" }));
    await user.click(await within(dialog).findByRole("button", { name: "user" }));
    await user.click(await within(dialog).findByRole("button", { name: "TV" }));
    await user.click(within(dialog).getByRole("button", { name: "Add this folder" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(sent("PATCH", `/libraries/${mockIds.movies}`)[0]?.body).toMatchObject({
      name: "Films",
      useLocalMetadata: false,
      previews: "never",
      folders: ["/mnt/user/Movies", "/mnt/user/TV"],
      metadataLanguage: "de-DE",
      contentRatingCountry: "DE",
    });
    expect(await screen.findByRole("heading", { name: "Films" })).toBeInTheDocument();
  });

  it("asks before Save removes a folder", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit Movies" });
    const save = within(dialog).getByRole("button", { name: "Save" });
    expect(save).toBeDisabled();
    await user.click(within(dialog).getByRole("button", { name: "Remove /mnt/user/Movies" }));
    await user.click(await within(dialog).findByRole("button", { name: "mnt" }));
    await user.click(await within(dialog).findByRole("button", { name: "user" }));
    await user.click(await within(dialog).findByRole("button", { name: "TV" }));
    await user.click(within(dialog).getByRole("button", { name: "Add this folder" }));
    await user.click(save);
    const confirm = await screen.findByRole("alertdialog", { name: "Remove 1 folder?" });
    expect(confirm).toHaveTextContent("/mnt/user/Movies");
    expect(sent("PATCH", `/libraries/${mockIds.movies}`)).toHaveLength(0);
    await user.click(within(confirm).getByRole("button", { name: "Remove and save" }));
    // Only the folders changed, so only they are sent.
    await waitFor(() =>
      expect(sent("PATCH", `/libraries/${mockIds.movies}`)[0]?.body).toEqual({ folders: ["/mnt/user/TV"] }),
    );
  });

  it("goes to a typed path in the folder picker", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "Add library" }));
    const dialog = await screen.findByRole("dialog", { name: "Add a library" });
    await user.type(within(dialog).getByRole("textbox", { name: "Folder path" }), "/mnt/user{Enter}");
    expect(await within(dialog).findByRole("button", { name: "TV" })).toBeInTheDocument();
    expect(sent("POST", "/libraries")).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: "Add this folder" }));
    expect(within(dialog).getByRole("button", { name: "Remove /mnt/user" })).toBeInTheDocument();
  });

  it("deletes a library after asking", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "More actions for Movies" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete library" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Delete Movies?" });
    expect(confirm).toHaveTextContent("Files on disk aren't touched");
    await user.click(within(confirm).getByRole("button", { name: "Delete library" }));
    await waitFor(() => expect(sent("DELETE", `/libraries/${mockIds.movies}`)).toHaveLength(1));
  });

  it("scans every library from the page's header", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/libraries");
    await user.click(await screen.findByRole("button", { name: "Scan all libraries" }));
    await waitFor(() => expect(sent("POST", "/tasks/scan-libraries/run")).toHaveLength(1));
    expect(
      await within(screen.getByRole("region", { name: "Libraries" })).findByRole("link", { name: "Tasks" }),
    ).toBeInTheDocument();
  });
});
