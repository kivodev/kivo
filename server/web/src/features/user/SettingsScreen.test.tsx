import { act, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { bookIds } from "@/api/mocks/books";
import { deviceSettings } from "@/lib/deviceSettings";
import { renderApp, screen } from "@/test/render";
import { http, mockId, mockIds, problem, sent, serveMockApi, server } from "@/test/server";

// The build's licenses.js, which the dev server and the tests don't have.
vi.mock("./licenses", () => ({
  loadLicenses: async () => [
    { name: "react", version: "19.3.0", license: "MIT", text: "MIT License\n\nCopyright (c) Meta Platforms, Inc." },
    {
      name: "Google Cast SDK",
      license: "Google Cast SDK Additional Developer Terms of Service",
      url: "https://developers.google.com/cast/docs/terms",
    },
  ],
}));

afterEach(() => {
  localStorage.clear();
  document.documentElement.dataset.theme = "dark";
});

describe("Settings", () => {
  it("shows nothing in the shell until its data is here", async () => {
    serveMockApi();
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(http.get("/api/v1/sessions", () => held));
    renderApp("/settings");
    await screen.findByRole("navigation", { name: "Main" });
    expect(screen.queryByRole("heading", { level: 1 })).not.toBeInTheDocument();
    expect(document.querySelectorAll('[aria-busy="true"], .animate-pulse')).toHaveLength(0);
    release();
    expect(await screen.findByRole("heading", { name: "Settings", level: 1 })).toBeInTheDocument();
  });

  it("opens from the user menu with your own and this device's settings", async () => {
    serveMockApi();
    const { user, router } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Account menu for anna" }));
    await user.click(screen.getByRole("menuitem", { name: "Settings" }));
    await screen.findByRole("heading", { name: "Settings", level: 1 });
    expect(router.state.location.pathname).toBe("/settings");
    const sections = [
      "Account",
      "Signed-in devices",
      "Playback",
      "Audio",
      "Subtitles",
      "Quality",
      "Appearance",
      "Home & libraries",
      "About",
    ];
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual(sections);
    // A topic's account settings come first, then this browser's.
    const headings = (section: string) =>
      within(screen.getByRole("region", { name: section }))
        .getAllByRole("heading", { level: 3 })
        .map((h) => h.textContent);
    expect(headings("Playback")).toEqual(["All your devices"]);
    expect(headings("Audio")).toEqual(["All your devices", "This browser"]);
    expect(headings("Subtitles")).toEqual(["All your devices", "This browser"]);
    expect(headings("Quality")).toEqual(["This browser"]);
    expect(headings("Appearance")).toEqual(["This browser"]);
    expect(headings("Home & libraries")).toEqual(["Home", "Libraries"]);
    const account = screen.getByRole("region", { name: "Account" });
    expect(within(account).getByText("anna")).toBeInTheDocument();
    expect(within(account).getByText("Signed in to Living room")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "About" })).getByText("1.0.0")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Audio language/ })).toHaveAccessibleName(/Original language/);
    // The closed field shows the choice alone. Its description is in the
    // list.
    expect(screen.getByRole("button", { name: /Show subtitles/ })).toHaveAccessibleName(/^Only forced Show subtitles$/);
    expect(screen.getByRole("button", { name: /Subtitle language/ })).toHaveAccessibleName(/Server default/);
    expect(screen.getByRole("switch", { name: "Remember choices per show" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "Play next episode automatically" })).toBeChecked();
    expect(screen.getByRole("button", { name: /Local network quality/ })).toHaveAccessibleName(/^Auto/);
    expect(screen.getByRole("button", { name: /Remote quality/ })).toHaveAccessibleName(/^Auto/);
    expect(screen.getByRole("button", { name: /Maximum audio channels/ })).toHaveAccessibleName(/^Auto/);
    expect(screen.getByRole("button", { name: /Subtitle size/ })).toHaveAccessibleName(/Medium/);
    expect(screen.getByRole("button", { name: /Subtitle background/ })).toHaveAccessibleName(/Shadow/);
    expect(screen.getByRole("button", { name: /Theme/ })).toHaveAccessibleName(/Dark/);
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("opens the credits for TMDB, Audnexus and jellyfin-ffmpeg from About's Credits row", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    const about = await screen.findByRole("region", { name: "About" });
    expect(within(about).getByText("Credits")).toBeInTheDocument();
    expect(within(about).queryByText(/TMDB API/)).not.toBeInTheDocument();
    await user.click(within(about).getByRole("button", { name: "View" }));

    const credits = await screen.findByRole("dialog", { name: "Credits" });
    expect(within(credits).getByRole("img", { name: "TMDB" })).toHaveAttribute("src", "tmdb.svg");
    expect(within(credits).getByRole("link", { name: "TMDB" })).toHaveAttribute("href", "https://www.themoviedb.org");
    expect(
      within(credits).getByText("This product uses the TMDB API but is not endorsed or certified by TMDB."),
    ).toBeInTheDocument();
    expect(within(credits).getByText(/^Audiobook metadata from/)).toHaveTextContent("Audiobook metadata from Audnexus");
    expect(within(credits).getByRole("link", { name: "Audnexus" })).toHaveAttribute(
      "href",
      "https://github.com/laxamentumtech/audnexus",
    );
    expect(within(credits).getByText(/^Transcoding by/)).toHaveTextContent("Transcoding by jellyfin-ffmpeg (GPL)");
    expect(within(credits).getByRole("link", { name: "jellyfin-ffmpeg" })).toHaveAttribute(
      "href",
      "https://github.com/jellyfin/jellyfin-ffmpeg",
    );

    await user.click(within(credits).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Credits" })).not.toBeInTheDocument());
  });

  it("lists the build's open source licenses from Credits, each package's text under it", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    const about = await screen.findByRole("region", { name: "About" });
    await user.click(within(about).getByRole("button", { name: "View" }));
    const credits = await screen.findByRole("dialog", { name: "Credits" });
    await user.click(within(credits).getByRole("button", { name: "Open source licenses" }));

    const licenses = await screen.findByRole("dialog", { name: "Open source licenses" });
    expect(await within(licenses).findByText("19.3.0")).toBeInTheDocument();
    const react = within(licenses).getByText("react").closest("details");
    if (!react) throw new Error("no details for react");
    expect(within(react).getByText("MIT")).toBeInTheDocument();
    await user.click(within(react).getByText("react"));
    expect(react).toHaveAttribute("open");
    expect(within(react).getByText(/Copyright \(c\) Meta Platforms/)).toBeVisible();
    expect(within(licenses).getByRole("link", { name: /Google Cast SDK/ })).toHaveAttribute(
      "href",
      "https://developers.google.com/cast/docs/terms",
    );

    await user.click(within(licenses).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Open source licenses" })).not.toBeInTheDocument());
    expect(screen.getByRole("dialog", { name: "Credits" })).toBeInTheDocument();
  });

  it("saves each user change to the server as it's made", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    const choose = async (field: RegExp, option: string) => {
      await user.click(await screen.findByRole("button", { name: field }));
      await user.click(await screen.findByRole("option", { name: option }));
    };
    await choose(/Audio language/, "Japanese");
    await choose(/Show subtitles/, "Foreign audio only");
    await choose(/Subtitle language/, "English");
    await user.click(screen.getByRole("switch", { name: "Remember choices per show" }));
    await user.click(screen.getByRole("switch", { name: "Play next episode automatically" }));
    await waitFor(() =>
      expect(sent("PATCH", "/users/me").map((r) => r.body)).toEqual([
        { playback: { audioLanguage: "ja" } },
        { playback: { subtitleMode: "smart" } },
        { playback: { subtitleLanguage: "en" } },
        { playback: { rememberTracks: false } },
        { playback: { autoplayNext: false } },
      ]),
    );
    expect(screen.getByRole("switch", { name: "Play next episode automatically" })).not.toBeChecked();
    // Success says nothing.
    expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("saves each device change in the browser", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    const choose = async (field: RegExp, option: string) => {
      await user.click(await screen.findByRole("button", { name: field }));
      await user.click(await screen.findByRole("option", { name: option }));
    };
    await choose(/Local network quality/, "Original");
    expect(localStorage.getItem("kivo.quality.local")).toBe("original");
    await choose(/Remote quality/, "720p · 4 Mb/s");
    expect(localStorage.getItem("kivo.quality.remote")).toBe("4000000");
    await choose(/Maximum audio channels/, "Stereo");
    expect(localStorage.getItem("kivo.audioChannels")).toBe("2");
    expect(deviceSettings().maxAudioChannels).toBe(2);
    await choose(/Maximum audio channels/, "7.1");
    expect(localStorage.getItem("kivo.audioChannels")).toBe("8");
    expect(deviceSettings().maxAudioChannels).toBe(8);
    await choose(/Subtitle size/, "Large");
    expect(localStorage.getItem("kivo.subtitleSize")).toBe("large");
    await choose(/Subtitle background/, "Box");
    expect(localStorage.getItem("kivo.subtitleBackground")).toBe("box");
    await choose(/Theme/, "Light");
    expect(document.documentElement.dataset.theme).toBe("light");
    // Success says nothing.
    expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("switches the dialogue boost the player starts with", async () => {
    serveMockApi();
    localStorage.setItem("kivo.dialogueBoost", "on");
    const { user } = renderApp("/settings");
    const boost = await screen.findByRole("switch", { name: "Dialogue boost" });
    expect(boost).toBeChecked();
    await user.click(boost);
    expect(boost).not.toBeChecked();
    expect(deviceSettings().dialogueBoost).toBe(false);
    await user.click(boost);
    expect(deviceSettings().dialogueBoost).toBe(true);
    expect(localStorage.getItem("kivo.dialogueBoost")).toBe("on");
  });

  it("says when the server limits your remote streams", async () => {
    serveMockApi((api) => {
      api.currentUser = { ...api.currentUser, limits: { remoteQualityCapBps: 4_000_000 } };
    });
    renderApp("/settings");
    expect(await screen.findByText(/Limited to 4 Mb\/s by the server\./)).toBeInTheDocument();
  });

  it("goes back to the original language and Auto", async () => {
    const api = serveMockApi();
    api.currentUser.playback.audioLanguage = "fr";
    localStorage.setItem("kivo.quality.local", "original");
    localStorage.setItem("kivo.audioChannels", "6");
    const { user } = renderApp("/settings");
    await user.click(await screen.findByRole("button", { name: /Audio language/ }));
    await user.click(await screen.findByRole("option", { name: "Original language" }));
    await user.click(screen.getByRole("button", { name: /Local network quality/ }));
    await user.click(await screen.findByRole("option", { name: "Auto" }));
    expect(screen.getByRole("button", { name: /Maximum audio channels/ })).toHaveAccessibleName(/^5\.1/);
    await user.click(screen.getByRole("button", { name: /Maximum audio channels/ }));
    await user.click(await screen.findByRole("option", { name: "Auto" }));
    await waitFor(() => expect(sent("PATCH", "/users/me")[0]?.body).toEqual({ playback: { audioLanguage: "" } }));
    expect(localStorage.getItem("kivo.quality.local")).toBeNull();
    expect(localStorage.getItem("kivo.audioChannels")).toBeNull();
  });

  it("disables the subtitle language while subtitles are off, keeping it", async () => {
    const api = serveMockApi();
    api.currentUser.playback.subtitleLanguage = "en";
    const { user } = renderApp("/settings");
    const language = await screen.findByRole("button", { name: /Subtitle language/ });
    expect(language).toBeEnabled();
    await user.click(screen.getByRole("button", { name: /Show subtitles/ }));
    await user.click(await screen.findByRole("option", { name: "Off" }));
    expect(language).toBeDisabled();
    expect(language).toHaveAccessibleName(/English/);
    await waitFor(() =>
      expect(sent("PATCH", "/users/me").map((r) => r.body)).toEqual([{ playback: { subtitleMode: "none" } }]),
    );
  });

  it("saves your libraries' order and which are pinned, which the sidebar follows at once", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    const order = await screen.findByRole("grid", { name: "Libraries" });
    const sidebar = screen.getByRole("navigation", { name: "Main" });
    const names = () =>
      within(order)
        .getAllByRole("row")
        .map((r) => r.textContent);
    expect(names()).toEqual(["Movies", "TV Shows", "Home Videos", "Audiobooks"]);

    await user.click(within(order).getByRole("checkbox", { name: "Pinned: Home Videos" }));
    expect(within(sidebar).queryByRole("link", { name: "Home Videos" })).not.toBeInTheDocument();
    expect(within(sidebar).getByRole("button", { name: "More libraries" })).toBeInTheDocument();

    // With a TV remote's keys alone. The arrows go to Audiobooks' handle,
    // Enter picks it up, Up moves it past its own place to above Home
    // Videos, and Enter drops it.
    const rows = within(order).getAllByRole("row");
    const focused = () => document.activeElement?.getAttribute("aria-label");
    act(() => rows[0]?.focus());
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{ArrowRight}");
    expect(focused()).toBe("Drag Audiobooks");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(focused()).toBe("Insert after Audiobooks"));
    await user.keyboard("{ArrowUp}{ArrowUp}");
    expect(focused()).toBe("Insert between TV Shows and Home Videos");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(names()).toEqual(["Movies", "TV Shows", "Audiobooks", "Home Videos"]));
    const ids = (...ls: [string, boolean][]) => ({ libraries: ls.map(([id, pinned]) => ({ id, pinned })) });
    await waitFor(() =>
      expect(sent("PATCH", "/users/me").map((r) => r.body)).toEqual([
        ids([mockIds.movies, true], [mockIds.shows, true], [mockIds.videos, false], [bookIds.audiobooks, true]),
        ids([mockIds.movies, true], [mockIds.shows, true], [bookIds.audiobooks, true], [mockIds.videos, false]),
      ]),
    );
  });

  it("hides a Home row, reorders by the handle's keyboard drag, and resets", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    const order = await screen.findByRole("grid", { name: "Home rows" });
    const rows = () =>
      within(order)
        .getAllByRole("row")
        .map((r) => r.textContent);
    expect(rows()[0]).toContain("Continue Watching");

    await user.click(within(order).getByRole("switch", { name: "Shown: Continue Watching" }));
    await waitFor(() =>
      expect(sent("PATCH", "/home/rows").at(-1)?.body).toMatchObject({
        items: expect.arrayContaining([{ id: "continueWatching", shown: false }]),
      }),
    );

    await user.click(screen.getByRole("button", { name: "Reset" }));
    await waitFor(() => expect(sent("POST", "/home/rows/reset")).toHaveLength(1));
    await waitFor(() => expect(within(order).getByRole("switch", { name: "Shown: Continue Watching" })).toBeChecked());
  });

  it("says on the row when the server refuses a change, puts the saved value back, and retries", async () => {
    serveMockApi();
    server.use(http.patch("/api/v1/users/me", () => problem(500, "internal")));
    const { user } = renderApp("/settings");
    const audio = await screen.findByRole("button", { name: /Audio language/ });
    await user.click(audio);
    await user.click(await screen.findByRole("option", { name: "Japanese" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't save. Something went wrong on the server.");
    expect(audio).toHaveAccessibleName(/Original language/);

    server.use(http.patch("/api/v1/users/me", () => problem(500, "internal")));
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(sent("PATCH", "/users/me")).toHaveLength(2));
    expect(sent("PATCH", "/users/me")[1]?.body).toEqual({ playback: { audioLanguage: "ja" } });
  });

  it("says when the server refuses your libraries' order, and sends it again", async () => {
    serveMockApi();
    server.use(http.patch("/api/v1/users/me", () => problem(500, "internal")));
    const { user } = renderApp("/settings");
    const order = await screen.findByRole("grid", { name: "Libraries" });
    await user.click(within(order).getByRole("checkbox", { name: "Pinned: Home Videos" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Couldn't save.");
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(sent("PATCH", "/users/me")).toHaveLength(2));
    expect(sent("PATCH", "/users/me")[1]?.body).toEqual(sent("PATCH", "/users/me")[0]?.body);
  });

  it("changes your password with your current one, signing out your other devices unless you say not to", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    await user.click(await screen.findByRole("button", { name: "Change password…" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Change password" }));
    expect(dialog.getByRole("switch", { name: "Sign out other devices" })).toBeChecked();
    await user.type(dialog.getByLabelText(/Current password/), "old password");
    await user.type(dialog.getByLabelText(/New password/), "a new password");
    await user.click(dialog.getByRole("switch", { name: "Sign out other devices" }));
    await user.click(dialog.getByRole("button", { name: "Change password" }));
    await waitFor(() =>
      expect(sent("POST", "/users/me/change-password").map((r) => r.body)).toEqual([
        { currentPassword: "old password", password: "a new password", signOutOthers: false },
      ]),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("says when the current password is wrong, and stays signed in", async () => {
    serveMockApi();
    server.use(http.post("/api/v1/users/me/change-password", () => problem(400, "wrongPassword")));
    const { user, router } = renderApp("/settings");
    await user.click(await screen.findByRole("button", { name: "Change password…" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Change password" }));
    await user.type(dialog.getByLabelText(/Current password/), "wrong password");
    await user.type(dialog.getByLabelText(/New password/), "a new password");
    await user.click(dialog.getByRole("button", { name: "Change password" }));
    expect(await dialog.findByText("That isn't your current password.")).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/settings");
  });

  it("lists where you're signed in, this browser marked, and signs another out once you confirm", async () => {
    serveMockApi();
    const { user, router } = renderApp("/settings");
    const sessions = within(await screen.findByRole("list", { name: "Signed-in devices" }));
    const rows = sessions.getAllByRole("listitem");
    expect(rows.map((r) => r.textContent)).toEqual([
      "Firefox on Linux x86_64Browser · This browser",
      "Living RoomAndroid TV · Last used yesterdaySign out",
    ]);
    expect(screen.getByRole("link", { name: "Link a device" })).toHaveAttribute("href", "/link");

    await user.click(sessions.getByRole("button", { name: "Sign out Living Room" }));
    const cancel = within(await screen.findByRole("alertdialog", { name: "Sign out Living Room?" }));
    await user.click(cancel.getByRole("button", { name: "Cancel" }));
    expect(sent("DELETE", "/sessions/{id}")).toHaveLength(0);

    await user.click(sessions.getByRole("button", { name: "Sign out Living Room" }));
    const confirm = within(await screen.findByRole("alertdialog", { name: "Sign out Living Room?" }));
    expect(confirm.getByText("Living Room has to sign in again.")).toBeInTheDocument();
    await user.click(confirm.getByRole("button", { name: "Sign out" }));
    await waitFor(() =>
      expect(sent("DELETE", "/sessions/{id}")[0]?.url.pathname).toMatch(new RegExp(`/sessions/${mockId(7002)}$`)),
    );
    expect(router.state.location.pathname).toBe("/settings");
  });

  it("renames a device, and says why when the server refuses the name", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    const sessions = within(await screen.findByRole("list", { name: "Signed-in devices" }));
    const row = within(sessions.getByText("Living Room").closest("li") as HTMLElement);

    let refuse = true;
    server.use(http.patch("/api/v1/sessions/{id}", () => (refuse ? problem(400, "invalidRequest") : undefined)));
    await user.click(row.getByRole("button", { name: "Rename" }));
    let dialog = within(await screen.findByRole("dialog", { name: "Rename" }));
    const name = dialog.getByRole("textbox", { name: "Name" });
    expect(name).toHaveValue("Living Room");
    await user.clear(name);
    await user.type(name, "Den");
    await user.click(dialog.getByRole("button", { name: "Save" }));
    expect(await dialog.findByRole("alert")).toBeInTheDocument();
    await user.click(dialog.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

    refuse = false;
    await user.click(row.getByRole("button", { name: "Rename" }));
    dialog = within(await screen.findByRole("dialog", { name: "Rename" }));
    await user.clear(dialog.getByRole("textbox", { name: "Name" }));
    await user.type(dialog.getByRole("textbox", { name: "Name" }), "  Den  ");
    await user.click(dialog.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(sent("PATCH", "/sessions/{id}").at(-1)?.body).toEqual({ name: "Den" });
    expect(sent("PATCH", "/sessions/{id}").at(-1)?.url.pathname).toMatch(new RegExp(`/sessions/${mockId(7002)}$`));
    expect(await sessions.findByText("Den")).toBeInTheDocument();
  });

  it("signs this browser out from Account's Sign out", async () => {
    const api = serveMockApi();
    const { user, router } = renderApp("/settings");
    const account = within((await screen.findByText("Signed in to Living room")).closest("section") as HTMLElement);
    api.signedIn = false;
    await user.click(account.getByRole("button", { name: "Sign out" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await waitFor(() => expect(router.state.location.pathname).toBe("/sign-in"));
    expect(sent("DELETE", "/sessions/current")).toHaveLength(1);
  });

  it("stays on Settings and says why when Account's Sign out fails", async () => {
    serveMockApi();
    server.use(http.delete("/api/v1/sessions/current", () => problem(503, "notReady")));
    const { user, router } = renderApp("/settings");
    const account = within((await screen.findByText("Signed in to Living room")).closest("section") as HTMLElement);
    await user.click(account.getByRole("button", { name: "Sign out" }));
    expect(await account.findByRole("alert")).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/settings");
  });

  it("signs out everywhere else once you confirm", async () => {
    serveMockApi();
    const { user } = renderApp("/settings");
    await screen.findByRole("list", { name: "Signed-in devices" });

    await user.click(screen.getByRole("button", { name: "Sign out everywhere else" }));
    const dialog = within(await screen.findByRole("alertdialog", { name: "Sign out everywhere else?" }));
    await user.click(dialog.getByRole("button", { name: "Cancel" }));
    expect(sent("DELETE", "/sessions")).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "Sign out everywhere else" }));
    const again = within(await screen.findByRole("alertdialog", { name: "Sign out everywhere else?" }));
    await user.click(again.getByRole("button", { name: "Sign out everywhere else" }));
    await waitFor(() => expect(sent("DELETE", "/sessions")[0]?.url.searchParams.get("except")).toBe("current"));
  });
});
