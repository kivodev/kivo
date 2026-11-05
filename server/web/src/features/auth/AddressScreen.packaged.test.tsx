import { waitFor } from "@testing-library/react";
import { HttpResponse, http as untyped } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { testServer } from "@/test/packagedSetup";
import { locationOf, renderApp, screen } from "@/test/render";
import { scenarios, sent, serveMockApi, server } from "@/test/server";
import { packagedServer, packagedToken, setPackagedServer, setPackagedToken } from "@/tv/packaged";
import { restartApp } from "@/tv/restart";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn() }));
// A floor above the mock server's own version, so a test can put a server
// on either side of it.

/** Renders a fresh install, types an address and presses Open. The server answers the probe with `probe` when given. */
async function open(address: string, version = "2.0.0", probe?: () => Response) {
  const api = serveMockApi(scenarios.signedOut, (api) => {
    api.server.version = version;
  });
  if (probe) server.use(untyped.get("*/api/v1/server", probe));
  const view = renderApp("/");
  await screen.findByRole("heading", { name: "Your Kivo server's address" });
  await view.user.type(screen.getByLabelText("Address"), address);
  await view.user.click(screen.getByRole("button", { name: "Open" }));
  return { ...view, api };
}

const probes = () => sent("GET", "/server").map((r) => r.url.href);

describe("The packaged app's address screen", () => {
  // A fresh install: the setup keeps a server for every packaged test.
  beforeEach(() => setPackagedServer(undefined));

  it("is the first screen of a fresh install, and asks nothing of any server", async () => {
    serveMockApi();
    const { router } = renderApp("/");
    await screen.findByRole("heading", { name: "Your Kivo server's address" });
    expect(locationOf(router)).toBe("/server");
    expect(sent("GET", "/setup")).toHaveLength(0);
    expect(sent("GET", "/users/me")).toHaveLength(0);
  });

  it("opens an address without a scheme over https only, keeps the server and starts the app again on it", async () => {
    const { api, router } = await open("nas.local:8484");
    await waitFor(() => expect(restartApp).toHaveBeenCalledOnce());
    expect(probes()).toEqual(["https://nas.local:8484/api/v1/server"]);
    expect(packagedServer()).toEqual({ address: "https://nas.local:8484", serverId: api.server.id });
    expect(locationOf(router)).toBe("/server");
  });

  it("goes on to sign-in without starting again when the address is the server the app started on", async () => {
    const { router, user } = await open(testServer.address);
    await user.click(await screen.findByRole("button", { name: "Open" }));
    await screen.findByText("K7Q-2M9");
    expect(packagedServer()).toEqual(testServer);
    expect(restartApp).not.toHaveBeenCalled();
    expect(locationOf(router)).toMatch(/^\/sign-in/);
  });

  it("keeps a reverse proxy's path, whatever it's called", async () => {
    const { api } = await open("https://nas.local/tv/");
    await waitFor(() => expect(restartApp).toHaveBeenCalledOnce());
    expect(probes()).toEqual(["https://nas.local/tv/api/v1/server"]);
    expect(packagedServer()).toEqual({ address: "https://nas.local/tv", serverId: api.server.id });
  });

  it("opens plain HTTP only when typed with http://, after saying it's unencrypted", async () => {
    const { api, user } = await open("http://nas.local:8484");
    expect(await screen.findByText("Connecting without encryption. Press Open to continue.")).toBeInTheDocument();
    expect(packagedServer()).toBeUndefined();
    await user.click(screen.getByRole("button", { name: "Open" }));
    await waitFor(() => expect(restartApp).toHaveBeenCalledOnce());
    expect(probes()).toEqual(["http://nas.local:8484/api/v1/server", "http://nas.local:8484/api/v1/server"]);
    expect(packagedServer()).toEqual({ address: "http://nas.local:8484", serverId: api.server.id });
  });

  it("refuses any other scheme without asking", async () => {
    await open("ftp://nas.local");
    expect(await screen.findByText("That isn't an address.")).toBeInTheDocument();
    expect(probes()).toEqual([]);
  });

  it("says when an address answers but isn't a Kivo server", async () => {
    await open("router.local", undefined, () => HttpResponse.html("<html><body>Router</body></html>"));
    expect(
      await screen.findByText(
        "router.local answered, but it isn't a Kivo server. Check the address and port, and that the server is up to date.",
      ),
    ).toBeInTheDocument();
    expect(packagedServer()).toBeUndefined();
  });

  it("tries an address that doesn't answer once more, then says it couldn't connect securely", {
    timeout: 15_000,
  }, async () => {
    await open("nas.local", undefined, () => HttpResponse.error());
    expect(await screen.findByText("Waking the server…")).toBeInTheDocument();
    expect(
      await screen.findByText(/^Couldn't connect securely to nas.local\./, undefined, { timeout: 8000 }),
    ).toBeInTheDocument();
    expect(probes()).toHaveLength(2);
    expect(packagedServer()).toBeUndefined();
  });

  it("forgets the kept server's token when it opens a new one, revoking its session there", async () => {
    setPackagedServer(testServer);
    setPackagedToken("old-token");
    serveMockApi(scenarios.signedOut, (api) => {
      api.server.version = "2.0.0";
      api.server.id = "another";
    });
    const { user } = renderApp("/server");
    const field = await screen.findByLabelText("Address");
    expect(field).toHaveValue(testServer.address);
    await user.clear(field);
    await user.type(field, "nas.local");
    await user.click(screen.getByRole("button", { name: "Open" }));
    await waitFor(() => expect(restartApp).toHaveBeenCalledOnce());
    expect(localStorage.getItem("kivo.token")).toBeNull();
    // The old server's session is revoked there, as Change server does.
    const [revoke] = sent("DELETE", "/sessions/current");
    expect(revoke?.url.origin).toBe(testServer.address);
  });

  it("forgets the kept session when a new address answers with the kept server's ID", async () => {
    // A server's ID is public, so another host can answer with it.
    const api = serveMockApi(scenarios.signedOut, (api) => {
      api.server.version = "2.0.0";
    });
    setPackagedServer(testServer);
    setPackagedToken("old-token");
    const { user } = renderApp("/server");
    const field = await screen.findByLabelText("Address");
    await user.clear(field);
    await user.type(field, "nas.local");
    await user.click(screen.getByRole("button", { name: "Open" }));
    await waitFor(() => expect(restartApp).toHaveBeenCalledOnce());
    expect(packagedToken()).toBeUndefined();
    expect(sent("DELETE", "/sessions/current").map((r) => r.url.origin)).toEqual([testServer.address]);
    expect(packagedServer()).toEqual({ address: "https://nas.local", serverId: api.server.id });
  });

  it("says a plain-HTTP address is unencrypted when it isn't the kept one", async () => {
    setPackagedServer({ address: "http://old.local", serverId: "another" });
    serveMockApi(scenarios.signedOut, (api) => {
      api.server.version = "2.0.0";
    });
    const { user } = renderApp("/server");
    const field = await screen.findByLabelText("Address");
    await user.clear(field);
    await user.type(field, "http://nas.local");
    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByText("Connecting without encryption. Press Open to continue.")).toBeInTheDocument();
  });

  it("says a server that isn't set up yet is to be set up in a browser", async () => {
    serveMockApi(scenarios.setup, (api) => {
      api.server.version = "2.0.0";
    });
    const { router, user } = renderApp("/");
    await user.type(await screen.findByLabelText("Address"), "nas.local");
    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(
      await screen.findByText(
        "nas.local is a Kivo server that isn't set up yet. Set it up in a browser, then press Open.",
      ),
    ).toBeInTheDocument();
    expect(packagedServer()).toBeUndefined();
    expect(locationOf(router)).toBe("/server");
  });
});

describe("The packaged app as it starts", () => {
  it("sends a kept server that doesn't answer to the address screen, with its address to open again", async () => {
    let down = true;
    const api = serveMockApi((api) => {
      api.server.version = "2.0.0";
    });
    server.use(untyped.get("*/api/v1/server", () => (down ? HttpResponse.error() : undefined)));
    setPackagedServer({ address: "http://kivo.test", serverId: api.server.id });
    setPackagedToken("app-token");
    const { router, user } = renderApp("/");
    expect(
      await screen.findByText("Can't reach http://kivo.test. Check the address and that the server is running."),
    ).toBeInTheDocument();
    expect(locationOf(router)).toMatch(/^\/server/);
    expect(screen.getByLabelText("Address")).toHaveValue("http://kivo.test");
    expect(sent("GET", "/setup")).toHaveLength(0);
    // Once it answers, Open opens it again, still signed in.
    down = false;
    await user.click(screen.getByRole("button", { name: "Open" }));
    await waitFor(() => expect(locationOf(router)).toBe("/"));
    expect(packagedToken()).toBe("app-token");
    expect(sent("DELETE", "/sessions/current")).toHaveLength(0);
  });

  it("sends a kept address that answers but isn't Kivo to the address screen, saying so", async () => {
    serveMockApi();
    server.use(untyped.get("*/api/v1/server", () => HttpResponse.html("<html><body>Router</body></html>")));
    const { router } = renderApp("/");
    expect(await screen.findByText(/^http:\/\/kivo\.test answered, but it isn't a Kivo server\./)).toBeInTheDocument();
    expect(locationOf(router)).toMatch(/^\/server/);
    expect(screen.getByLabelText("Address")).toHaveValue("http://kivo.test");
  });

  it("asks a kept server that didn't answer again", async () => {
    let version: string | undefined;
    serveMockApi();
    server.use(
      untyped.get("*/api/v1/server", ({ request }) =>
        version === undefined
          ? HttpResponse.error()
          : HttpResponse.json({ id: "server-1", name: "Kivo", version, url: new URL(request.url).origin }),
      ),
    );
    const { router } = renderApp("/");
    await screen.findByText(/^Can't reach http:\/\/kivo\.test\./);
    version = "2.0.0";
    await router.navigate({ to: "/" });
    await waitFor(() => expect(locationOf(router)).not.toMatch(/^\/server/));
    expect(probes()).toHaveLength(2);
  });

  it("sends a kept server that isn't set up yet to the address screen, rather than round and round", async () => {
    serveMockApi(scenarios.setup, (api) => {
      api.server.version = "2.0.0";
    });
    const { router } = renderApp("/");
    expect(await screen.findByText(/isn't set up yet\. Set it up in a browser/)).toBeInTheDocument();
    expect(locationOf(router)).toMatch(/^\/server/);
  });
});
