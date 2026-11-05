import { fireEvent, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { onRequest, scenarios, sent, serveMockApi } from "@/test/server";
import { packagedClientId, packagedServer, packagedToken } from "@/tv/packaged";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn() }));

const frame2025 =
  "Mozilla/5.0 (SMART-TV; LINUX; Tizen 9.0) AppleWebKit/537.36 (KHTML, like Gecko) 120.0.6099.5/9.0 TV Safari/537.36";

/** A packaged app that has its server but no token. */
function serveSignedOut() {
  const api = serveMockApi(scenarios.signedOut, (api) => {
    api.session = { ...api.session, kind: "app", token: "app-token" };
  });
  return api;
}

describe("Sign in in the packaged app", () => {
  beforeEach(() => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(frame2025);
  });

  it("shows sign-in for a kept server without a token, keeping the address", async () => {
    serveSignedOut();
    const { router } = renderApp("/");
    await screen.findByText("K7Q-2M9");
    expect(locationOf(router)).toMatch(/^\/sign-in/);
    expect(packagedServer()?.address).toBe("http://kivo.test");
  });

  it("asks for an app session by code, and keeps the token the approved code answers with", {
    timeout: 10_000,
  }, async () => {
    const api = serveSignedOut();
    const { router } = renderApp("/");
    await screen.findByText("K7Q-2M9");
    expect(sent("POST", "/device-codes")[0]?.body).toEqual({
      name: "Samsung TV (Tizen 9.0)",
      kind: "app",
      clientId: packagedClientId(),
    });
    api.deviceCodePoll = "approved";
    await waitFor(() => expect(packagedToken()).toBe("app-token"), { timeout: 5000 });
    await waitFor(() => expect(locationOf(router)).toBe("/"));
  });

  it("asks for an app session with a password, and keeps its token", async () => {
    const api = serveSignedOut();
    onRequest("post", "/sessions", () => {
      api.signedIn = true;
    });
    const { router, user } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Sign in with password" }));
    await user.type(await screen.findByLabelText("Username"), "anna");
    await user.type(screen.getByLabelText("Password"), "secret");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(packagedToken()).toBe("app-token"));
    expect(sent("POST", "/sessions")[0]?.body).toEqual({
      username: "anna",
      password: "secret",
      name: "Samsung TV (Tizen 9.0)",
      kind: "app",
      clientId: packagedClientId(),
    });
    await waitFor(() => expect(locationOf(router)).toBe("/"));
  });

  it("offers Change server, which opens the address screen with the kept address, and Back returns", async () => {
    serveSignedOut();
    const { router, user } = renderApp("/");
    await screen.findByText("K7Q-2M9");
    await user.click(screen.getByRole("button", { name: "Change server" }));
    expect(await screen.findByLabelText("Address")).toHaveValue("http://kivo.test");
    expect(locationOf(router)).toBe("/server");
    // The server is kept until another is opened, so Back has somewhere to go.
    expect(packagedServer()?.address).toBe("http://kivo.test");
    fireEvent.keyDown(window, { key: "XF86Back" });
    await waitFor(() => expect(locationOf(router)).toMatch(/^\/sign-in/));
    expect(screen.queryByRole("alertdialog", { name: "Exit Kivo?" })).not.toBeInTheDocument();
  });

  it("keeps one client ID, so signing in again replaces the TV's session", () => {
    expect(packagedClientId()).toBe(packagedClientId());
  });
});
