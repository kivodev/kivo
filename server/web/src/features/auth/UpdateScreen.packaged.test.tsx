import { waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { closeEvents, scenarios, sent, serveMockApi } from "@/test/server";
import { setPackagedToken } from "@/tv/packaged";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn() }));
vi.mock("@/tv/versions", () => ({ appVersion: "0.5.0" }));

/** The app opens on its kept server, which says it's at `version` and supports apps from `minAppVersion`. */
function start(version: string, minAppVersion?: string) {
  const api = serveMockApi(scenarios.signedOut, (api) => {
    api.server.name = "Den";
    api.server.version = version;
    api.server.minAppVersion = minAppVersion;
  });
  return { ...renderApp("/"), api };
}

describe("The packaged app's update screen", () => {
  it("says a server older than the app works with needs an update, and asks nothing more of it", async () => {
    const { router } = start("0.2.0", "0.1.0");
    await screen.findByRole("heading", { name: "This server needs an update" });
    expect(
      screen.getByText("Kivo 0.5.0 works with server 0.3.0 or newer. Den runs 0.2.0. Ask whoever runs it to update."),
    ).toBeInTheDocument();
    expect(locationOf(router)).toBe("/update");
    expect(sent("GET", "/setup")).toHaveLength(0);
    expect(sent("GET", "/users/me")).toHaveLength(0);
  });

  it("says the app needs an update for a server that no longer supports it", async () => {
    const { router } = start("0.9.0", "0.6.0");
    await screen.findByRole("heading", { name: "Update Kivo" });
    expect(screen.getByText("Den needs Kivo 0.6.0 or newer. You have 0.5.0.")).toBeInTheDocument();
    expect(locationOf(router)).toBe("/update");
  });

  it("stays on Try again while they don't work together, and goes on to sign-in once they do", async () => {
    const { api, router, user } = start("0.2.0");
    await screen.findByRole("heading", { name: "This server needs an update" });
    await user.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByRole("heading", { name: "This server needs an update" });
    expect(locationOf(router)).toBe("/update");

    api.server.version = "0.5.0";
    await user.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByText("K7Q-2M9");
    expect(locationOf(router)).toMatch(/^\/sign-in/);
  });

  it("opens the address screen from Change server", async () => {
    const { router, user } = start("0.9.0", "0.6.0");
    await user.click(await screen.findByRole("button", { name: "Change server" }));
    await screen.findByRole("heading", { name: "Your Kivo server's address" });
    expect(locationOf(router)).toBe("/server");
  });

  it("asks the server what it is again as the events reconnect, and says so when it came back too old", {
    timeout: 15_000,
  }, async () => {
    const api = serveMockApi((api) => {
      api.server.name = "Den";
      api.server.version = "0.5.0";
    });
    setPackagedToken("app-token");
    const { router } = renderApp("/");
    await waitFor(() => expect(document.activeElement?.getAttribute("data-tv-focus-key")).toMatch(/^card:/));
    expect(locationOf(router)).toBe("/");

    // The server restarts on an older release.
    api.server.version = "0.2.0";
    closeEvents(1001);
    await screen.findByRole("heading", { name: "This server needs an update" }, { timeout: 10_000 });
    expect(locationOf(router)).toBe("/update");
  });

  it("lets through a server that says nothing of the apps it supports, or isn't a release", async () => {
    const { router } = start("dev");
    await screen.findByText("K7Q-2M9");
    expect(locationOf(router)).toMatch(/^\/sign-in/);
  });
});
