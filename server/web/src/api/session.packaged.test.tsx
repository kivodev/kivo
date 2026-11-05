import { waitFor } from "@testing-library/react";
import { http as untyped } from "msw";
import { describe, expect, it, vi } from "vitest";
import { renderApp, screen } from "@/test/render";
import { onRequest, scenarios, sent, serveMockApi, server } from "@/test/server";
import { packagedToken, setPackagedServer } from "@/tv/packaged";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn() }));

const serverB = { address: "https://b.local:8484", serverId: "server-b" };

/** A packaged app that keeps its server and has no token. */
function serveSignedOut() {
  return serveMockApi(scenarios.signedOut, (api) => {
    api.session = { ...api.session, kind: "app", token: "token-for-a" };
  });
}

/** Holds the answers to a route until the returned function is called. */
function hold(path: string): () => void {
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  server.use(
    untyped.post(`*/api/v1${path}`, async () => {
      await gate;
      return undefined;
    }),
  );
  return release;
}

// A sign-in answered as the app starts again on another server is the old
// server's. Its token must not be kept for, or sent to, the new one.
describe("a sign-in answered after the server changed", () => {
  it("drops a password sign-in's token", async () => {
    const api = serveSignedOut();
    onRequest("post", "/sessions", () => {
      api.signedIn = true;
    });
    const release = hold("/sessions");
    const { user } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Sign in with password" }));
    await user.type(await screen.findByLabelText("Username"), "anna");
    await user.type(screen.getByLabelText("Password"), "secret");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(sent("POST", "/sessions")).toHaveLength(1));

    setPackagedServer(serverB);
    release();

    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(packagedToken()).toBeUndefined();
    expect(localStorage.getItem("kivo.token")).toBeNull();
  });

  it("drops a device code's token", { timeout: 10_000 }, async () => {
    const api = serveSignedOut();
    const release = hold("/device-codes/poll");
    renderApp("/");
    await screen.findByText("K7Q-2M9");
    api.deviceCodePoll = "approved";
    await waitFor(() => expect(sent("POST", "/device-codes/poll")).toHaveLength(1), { timeout: 5000 });

    setPackagedServer(serverB);
    release();

    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(packagedToken()).toBeUndefined();
    expect(localStorage.getItem("kivo.token")).toBeNull();
  });
});
