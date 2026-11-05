import { waitFor, within } from "@testing-library/react";
import { HttpResponse, http as untyped } from "msw";
import { describe, expect, it, vi } from "vitest";
import { testServer } from "@/test/packagedSetup";
import { locationOf, renderApp, screen } from "@/test/render";
import { sent, serveMockApi, server } from "@/test/server";
import { packagedServer, packagedToken, setPackagedToken } from "@/tv/packaged";
import { focusTv } from "@/tv/spatial";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn() }));

const kept = testServer;

/** A signed-in packaged app on Settings. */
function renderSettings() {
  const api = serveMockApi();
  setPackagedToken("app-token");
  return { api, ...renderApp("/settings") };
}

/** Focuses a section in the list, which shows it, and waits for its heading. */
async function openSection(id: string, name: string) {
  focusTv(`settings:section:${id}`);
  return screen.findByRole("heading", { name, level: 2 });
}

const openAbout = () => openSection("about", "About");

describe("Settings in the packaged app", () => {
  it("shows the app's version and the server's in About", async () => {
    renderSettings();
    await openAbout();
    const app = await screen.findByText("App version");
    const card = app.closest<HTMLElement>("[data-part=tv-versions]") ?? document.body;
    expect(within(card).getByText("dev")).toBeInTheDocument();
    expect(within(card).getByText("Server version")).toBeInTheDocument();
    expect(within(card).getByText("1.0.0")).toBeInTheDocument();
  });

  it("signs out on the server, forgets the token and shows sign-in with the address kept", async () => {
    const { router, user } = renderSettings();
    await openSection("account", "Account");
    await user.click(await screen.findByRole("button", { name: "Sign out" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Sign out this TV?" });
    await user.click(within(dialog).getByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(locationOf(router)).toMatch(/^\/sign-in/));
    expect(sent("DELETE", "/sessions/current")).toHaveLength(1);
    expect(packagedToken()).toBeUndefined();
    expect(packagedServer()).toEqual(kept);
  });

  it("changes server: signs out on the server, forgets it and shows the address screen", async () => {
    const { router, user } = renderSettings();
    await openAbout();
    await user.click(await screen.findByRole("button", { name: "Change…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Change server?" });
    await user.click(within(dialog).getByRole("button", { name: "Change server" }));
    await screen.findByRole("heading", { name: "Your Kivo server's address" });
    expect(locationOf(router)).toBe("/server");
    expect(sent("DELETE", "/sessions/current")).toHaveLength(1);
    expect(packagedServer()).toBeUndefined();
    expect(packagedToken()).toBeUndefined();
  });

  it("forgets the token and the server anyway when the server doesn't answer the sign-out", async () => {
    const { router, user } = renderSettings();
    server.use(untyped.delete("*/api/v1/sessions/current", () => HttpResponse.error()));
    await openAbout();
    await user.click(await screen.findByRole("button", { name: "Change…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Change server?" });
    await user.click(within(dialog).getByRole("button", { name: "Change server" }));
    await screen.findByRole("heading", { name: "Your Kivo server's address" });
    expect(locationOf(router)).toBe("/server");
    expect(sent("DELETE", "/sessions/current")).toHaveLength(1);
    expect(packagedServer()).toBeUndefined();
    expect(packagedToken()).toBeUndefined();
  });
});
