import { fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderApp, screen } from "@/test/render";
import { sent, serveMockApi } from "@/test/server";
import { focusTv } from "@/tv/spatial";
import { stopTv } from "@/tv/start";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn() }));
// The build's licenses.js, which the dev server and the tests don't have.
vi.mock("./licenses", () => ({
  loadLicenses: async () => [{ name: "react", version: "19.3.0", license: "MIT", text: "MIT License" }],
}));

const focused = () => document.activeElement?.getAttribute("data-tv-focus-key");
const press = (key: string) => {
  fireEvent.keyDown(document.activeElement ?? window, { key });
  fireEvent.keyUp(document.activeElement ?? window, { key });
};
const back = () => fireEvent.keyDown(window, { key: "XF86Back" });

/** Focuses a section in the list, which shows it, and waits for its heading. */
async function openSection(id: string, name: string) {
  focusTv(`settings:section:${id}`);
  await waitFor(() => expect(focused()).toBe(`settings:section:${id}`));
  return screen.findByRole("heading", { name, level: 2 });
}

describe("Settings on TV", () => {
  afterEach(() => {
    stopTv();
    localStorage.clear();
  });

  it("lists the sections, shows the focused one beside the list, and moves between them", async () => {
    serveMockApi();
    renderApp("/settings");
    await screen.findByRole("heading", { name: "Settings", level: 1 });
    const list = screen.getByRole("navigation", { name: "Settings sections" });
    expect(
      within(list)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["Account", "Playback", "Audio", "Subtitles", "Quality", "Display", "Home", "Libraries", "About"]);
    // The first section shows, with focus on it in the list.
    await waitFor(() => expect(focused()).toBe("settings:section:account"));
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual(["Account"]);
    expect(screen.queryByRole("button", { name: "Change password…" })).not.toBeInTheDocument();

    // Focus on another section shows it, OK goes into it, and coming back
    // to the list lands on the section it left.
    await openSection("playback", "Playback");
    expect(screen.queryByRole("heading", { name: "Account", level: 2 })).not.toBeInTheDocument();
    // OK on a button is the browser's click, which jsdom doesn't make from a key.
    fireEvent.click(document.activeElement as Element);
    await waitFor(() => expect(focused()).toMatch(/^setting:/));
    focusTv("settings:sections");
    await waitFor(() => expect(focused()).toBe("settings:section:playback"));
  });

  it("marks only the settings kept on the TV, and shows About with the server's address and its QR code", async () => {
    serveMockApi();
    renderApp("/settings");
    await openSection("audio", "Audio");
    // No group headings: a setting kept on the TV says so beside its label.
    expect(screen.queryByText("All your devices")).not.toBeInTheDocument();
    expect(screen.queryByText("This browser")).not.toBeInTheDocument();
    const tags = document.querySelectorAll('[data-part="this-tv"]');
    expect(tags.length).toBeGreaterThan(0);
    expect(screen.getByText("Audio language").parentElement?.querySelector('[data-part="this-tv"]')).toBeNull();

    await openSection("about", "About");
    const about = screen.getByRole("region", { name: "About" });
    expect(within(about).getByRole("img", { name: "QR code of the server address" })).toBeInTheDocument();
    expect(within(about).getByText("http://192.168.1.10:8484")).toBeInTheDocument();
    expect(within(about).getByText("To manage this server, open this address in a browser.")).toBeInTheDocument();
  });

  it("shows only the version in About, on a row the remote can reach, and no Change server", async () => {
    serveMockApi();
    renderApp("/settings");
    await openSection("about", "About");
    const card = (await screen.findByText("Version")).closest<HTMLElement>('[data-part="tv-versions"]');
    if (!card) throw new Error("no versions");
    const rows = [...card.querySelectorAll<HTMLElement>('[data-part="version"]')];
    expect(rows.map((r) => r.textContent)).toEqual([expect.stringMatching(/^Version.+/)]);
    const key = rows[0]?.dataset.tvFocusKey ?? "";
    focusTv(key);
    await waitFor(() => expect(focused()).toBe(key));
    expect(within(card).queryByRole("button")).not.toBeInTheDocument();
    expect(screen.queryByText("Change server")).not.toBeInTheDocument();
  });

  it("opens the credits from About's Credits row with OK, as plain text, and Back closes them", async () => {
    serveMockApi();
    renderApp("/settings");
    await openSection("about", "About");
    const about = screen.getByRole("region", { name: "About" });
    const row = within(about).getByRole("button", { name: "View" });
    const key = row.closest<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey ?? "";
    focusTv(key);
    await waitFor(() => expect(focused()).toBe(key));
    press("Enter");

    const credits = await screen.findByRole("dialog", { name: "Credits" });
    await waitFor(() => expect(focused()).toBe("tv:credits:close"));
    expect(within(credits).getByRole("img", { name: "TMDB" })).toBeInTheDocument();
    expect(
      within(credits).getByText("This product uses the TMDB API but is not endorsed or certified by TMDB."),
    ).toBeInTheDocument();
    expect(within(credits).getByText("Audiobook metadata from Audnexus")).toBeInTheDocument();
    expect(within(credits).getByText("Transcoding by jellyfin-ffmpeg (GPL)")).toBeInTheDocument();
    expect(within(credits).queryByRole("link")).not.toBeInTheDocument();

    back();
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Credits" })).not.toBeInTheDocument());
    await waitFor(() => expect(focused()).toBe(key));
  });

  it("lists the open source licenses from Credits as rows, without their text, and Back returns to Credits", async () => {
    serveMockApi();
    renderApp("/settings");
    await openSection("about", "About");
    const about = screen.getByRole("region", { name: "About" });
    const row = within(about).getByRole("button", { name: "View" });
    const key = row.closest<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey ?? "";
    focusTv(key);
    await waitFor(() => expect(focused()).toBe(key));
    press("Enter");
    await screen.findByRole("dialog", { name: "Credits" });
    await waitFor(() => expect(focused()).toBe("tv:credits:close"));
    focusTv("credits:licenses");
    await waitFor(() => expect(focused()).toBe("credits:licenses"));
    press("Enter");

    const licenses = await screen.findByRole("dialog", { name: "Open source licenses" });
    await waitFor(() => expect(focused()).toBe("tv:licenses:close"));
    expect(await within(licenses).findByText("react")).toBeInTheDocument();
    expect(within(licenses).getByText("MIT")).toBeInTheDocument();
    expect(within(licenses).queryByText("MIT License")).not.toBeInTheDocument();

    back();
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Open source licenses" })).not.toBeInTheDocument());
    expect(screen.getByRole("dialog", { name: "Credits" })).toBeInTheDocument();
    await waitFor(() => expect(focused()).toBe("credits:licenses"));
  });

  it("flips a switch with OK", async () => {
    serveMockApi();
    renderApp("/settings");
    await openSection("playback", "Playback");
    const toggle = await screen.findByRole("switch", { name: "Play next episode automatically" });
    const was = (toggle as HTMLInputElement).checked;
    focusTv(toggle.closest<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey ?? "");
    await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focusable"));
    press("Enter");
    await waitFor(() => expect(sent("PATCH", "/users/me")).toHaveLength(1));
    expect(sent("PATCH", "/users/me")[0]?.body).toMatchObject({ playback: { autoplayNext: !was } });
  });

  it("asks before signing out, with Cancel focused inside a dialog that traps focus, and Back cancels", async () => {
    serveMockApi();
    renderApp("/settings");
    const signOut = await screen.findByRole("button", { name: "Sign out" });
    const key = signOut.closest<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey ?? "";
    focusTv(key);
    await waitFor(() => expect(focused()).toBe(key));
    press("Enter");

    const dialog = await screen.findByRole("alertdialog", { name: "Sign out this TV?" });
    await waitFor(() => expect(focused()).toBe("tv:confirm:cancel"));
    expect(dialog.querySelector("[data-tv-trap]")).not.toBeNull();
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("tv:confirm:yes"));
    // The trap holds focus in at the dialog's edge.
    press("ArrowRight");
    press("ArrowUp");
    expect(focused()).toBe("tv:confirm:yes");

    back();
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(sent("DELETE", "/sessions/current")).toHaveLength(0);
    await waitFor(() => expect(focused()).toBe(key));
  });
});
