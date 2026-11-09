import { waitFor } from "@testing-library/react";
import { delay } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { closeEvents, http, mockIds, onRequest, problem, scenarios, sent, serveMockApi, server } from "@/test/server";

describe("Sign-in", () => {
  it("sends a signed-out visit to sign-in, and back to the same page after", async () => {
    const api = serveMockApi(scenarios.signedOut);
    server.use(http.post("/api/v1/sessions", () => problem(401, "invalidCredentials"), { once: true }));
    onRequest("post", "/sessions", () => {
      api.signedIn = true;
    });
    const { router, user } = renderApp(`/libraries/${mockIds.movies}?sort=addedAt`);

    await user.type(await screen.findByLabelText("Username"), "anna");
    await user.type(screen.getByLabelText("Password"), "wrong password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Wrong username or password.");

    await user.clear(screen.getByLabelText("Password"));
    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("heading", { name: "Movies", level: 1 })).toBeInTheDocument();
    expect(locationOf(router)).toBe(`/libraries/${mockIds.movies}?sort=addedAt`);
    expect(sent("POST", "/sessions").map((r) => r.body)).toMatchObject([
      { username: "anna", password: "wrong password" },
      { username: "anna", password: "correct horse" },
    ]);
  });

  it("drops what the browser cached for whoever was signed in before", async () => {
    const api = serveMockApi(scenarios.signedOut);
    onRequest("post", "/sessions", () => {
      api.signedIn = true;
    });
    const { queryClient, user } = renderApp("/");
    queryClient.setQueryData(["get", "/api/v1/lists"], { items: [{ id: "the previous user's" }] });
    await user.type(await screen.findByLabelText("Username"), "anna");
    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    await screen.findByRole("navigation", { name: "Main" });
    expect(queryClient.getQueryData(["get", "/api/v1/lists"])).toBeUndefined();
  });

  it("says how long to wait once sign-in is throttled", async () => {
    serveMockApi(scenarios.signedOut);
    server.use(http.post("/api/v1/sessions", () => problem(429, "throttled", { retryAfterSeconds: 30 })));
    const { user } = renderApp("/");
    await user.type(await screen.findByLabelText("Username"), "anna");
    await user.type(screen.getByLabelText("Password"), "wrong password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByText("Too many attempts. Try again in 30 seconds.")).toBeInTheDocument();
  });

  it("returns to the page an expired session was on", async () => {
    serveMockApi();
    const { router, user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await screen.findByRole("heading", { name: "Dune: Part Two" });

    // The session ends elsewhere, and the next request gets a 401.
    server.use(http.patch("/api/v1/works/{id}/user-state", () => problem(401, "unauthenticated")));
    await user.click(screen.getByRole("button", { name: "Mark as watched" }));

    await screen.findByRole("heading", { name: "Sign in" });
    await waitFor(() => expect(locationOf(router)).toContain("/sign-in"));
    expect(router.state.location.search).toEqual({ redirect: `/works/${mockIds.dunePartTwo}` });
  });

  it("returns to the page a session ended on the event socket was on", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await screen.findByRole("heading", { name: "Dune: Part Two" });

    // The session is revoked, and the server closes the event socket for
    // it.
    await waitFor(() => expect(closeEvents(4401)).toBeGreaterThan(0));

    await screen.findByRole("heading", { name: "Sign in" });
    expect(router.state.location.search).toEqual({ redirect: `/works/${mockIds.dunePartTwo}` });
  });

  it("signs out from the user menu", async () => {
    serveMockApi();
    const { router, user, queryClient } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Account menu for anna" }));
    await user.click(screen.getByRole("menuitem", { name: "Sign out" }));
    await screen.findByRole("heading", { name: "Sign in" });
    expect(router.state.location.pathname).toBe("/sign-in");
    expect(sent("DELETE", "/sessions/current")).toHaveLength(1);
    expect(queryClient.getQueryCache().findAll({ queryKey: ["get", "/api/v1/home"] })).toEqual([]);
  });

  it("stays signed in and says why when signing out fails", async () => {
    serveMockApi();
    server.use(http.delete("/api/v1/sessions/current", () => problem(503, "notReady"), { once: true }));
    const { router, user } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Account menu for anna" }));
    await user.click(screen.getByRole("menuitem", { name: "Sign out" }));
    expect(await screen.findByText(/^Couldn't sign out\./)).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/");

    await user.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByRole("heading", { name: "Sign in" });
    expect(sent("DELETE", "/sessions/current")).toHaveLength(2);
  });

  it("treats a 401 from a sign-in path's other methods as signed out", async () => {
    serveMockApi();
    server.use(http.get("/api/v1/sessions", () => problem(401, "unauthenticated")));
    const { router } = renderApp("/settings");
    await screen.findByRole("heading", { name: "Sign in" });
    expect(router.state.location.search).toEqual({ redirect: "/settings" });
  });
});

describe("Sign in with another device", () => {
  afterEach(() => vi.restoreAllMocks());

  it("shows a code another device approves, then signs this browser in", { timeout: 10_000 }, async () => {
    const api = serveMockApi(scenarios.signedOut);
    const { router, user } = renderApp("/settings");
    await user.click(await screen.findByRole("button", { name: "Sign in with another device" }));
    expect(await screen.findByText("K7Q-2M9")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "QR code to link this browser" })).toBeInTheDocument();
    // A new code replaces it as it expires, so it shows no time left.
    expect(screen.queryByText(/expires/i)).not.toBeInTheDocument();
    expect(sent("POST", "/device-codes")[0]?.body).toEqual({ name: "Safari", kind: "browser" });
    api.deviceCodePoll = "approved";
    await waitFor(() => expect(locationOf(router)).toBe("/settings"), { timeout: 5000 });
    expect(sent("POST", "/device-codes/poll")[0]?.body).toEqual({ secret: "secret" });
  });

  it("opens on the code in a TV's browser, and starts a new code when one is declined", {
    timeout: 10_000,
  }, async () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0)");
    const api = serveMockApi(scenarios.signedOut, (a) => {
      a.deviceCodePoll = "denied";
    });
    const { user } = renderApp("/");
    expect(await screen.findByText("K7Q-2M9")).toBeInTheDocument();
    expect(
      await screen.findByText("Sign-in was declined. Here's a new code.", {}, { timeout: 5000 }),
    ).toBeInTheDocument();
    await waitFor(() => expect(sent("POST", "/device-codes")).toHaveLength(2));
    expect(api.deviceCodePoll).toBe("waiting");
    await user.click(screen.getByRole("button", { name: "Sign in with password" }));
    expect(await screen.findByLabelText("Username")).toBeInTheDocument();
  });

  it("starts a code again by itself once the server says it will take one, keeping Retry", {
    timeout: 10_000,
  }, async () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0)");
    serveMockApi(scenarios.signedOut);
    server.use(
      http.post("/api/v1/device-codes", () => problem(429, "throttled", { retryAfterSeconds: 1 }), { once: true }),
    );
    renderApp("/");
    expect(await screen.findByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(sent("POST", "/device-codes")).toHaveLength(1);
    expect(await screen.findByText("K7Q-2M9", {}, { timeout: 4000 })).toBeInTheDocument();
    expect(sent("POST", "/device-codes")).toHaveLength(2);
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  });

  it("names a Tizen TV for its make, and starts a new code the moment one expires", {
    timeout: 10_000,
  }, async () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0)");
    serveMockApi(scenarios.signedOut);
    // The poll interval is long, so only the expiry can start the next code.
    server.use(
      http.post(
        "/api/v1/device-codes",
        ({ response }) =>
          response(201).json({
            code: "K7Q2M9",
            secret: "secret",
            expiresAt: new Date(Date.now() + 1000).toISOString(),
            interval: 60,
            linkUrl: "http://192.168.1.10:8484/link?code=K7Q2M9",
          }),
        { once: true },
      ),
    );
    renderApp("/");
    expect(await screen.findByText("K7Q-2M9")).toBeInTheDocument();
    await waitFor(() => expect(sent("POST", "/device-codes")).toHaveLength(2), { timeout: 4000 });
    expect(sent("POST", "/device-codes")[0]?.body).toMatchObject({ name: "Samsung TV (Tizen 7.0)" });
  });

  it("keeps a code for its life on the server's clock, however far the TV's clock is ahead", {
    timeout: 10_000,
  }, async () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0)");
    serveMockApi(scenarios.signedOut);
    // The server's clock is 15 minutes behind, so by the TV's clock the
    // code expired 10 minutes ago.
    server.use(
      http.post("/api/v1/device-codes", ({ response }) => {
        const serverNow = Date.now() - 15 * 60_000;
        return response(201).json(
          {
            code: "K7Q2M9",
            secret: "secret",
            expiresAt: new Date(serverNow + 5 * 60_000).toISOString(),
            interval: 60,
            linkUrl: "http://192.168.1.10:8484/link?code=K7Q2M9",
          },
          { headers: { Date: new Date(serverNow).toUTCString() } },
        );
      }),
    );
    renderApp("/");
    expect(await screen.findByText("K7Q-2M9")).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 1500));
    expect(sent("POST", "/device-codes")).toHaveLength(1);
  });

  it("signs in when the code is approved just before it expires, though the answer comes after", {
    timeout: 10_000,
  }, async () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0)");
    const api = serveMockApi(scenarios.signedOut);
    // The first poll goes at 1 second, a moment before the code expires,
    // and its answer, the approval, takes a second more to come.
    server.use(
      http.post(
        "/api/v1/device-codes",
        ({ response }) =>
          response(201).json({
            code: "K7Q2M9",
            secret: "secret",
            expiresAt: new Date(Date.now() + 1500).toISOString(),
            interval: 1,
            linkUrl: "http://192.168.1.10:8484/link?code=K7Q2M9",
          }),
        { once: true },
      ),
      http.post("/api/v1/device-codes/poll", async ({ response }) => {
        await delay(1000);
        api.signedIn = true;
        return response(201).json(api.session);
      }),
    );
    const { router } = renderApp("/settings");
    expect(await screen.findByText("K7Q-2M9")).toBeInTheDocument();
    await waitFor(() => expect(locationOf(router)).toBe("/settings"), { timeout: 5000 });
    expect(sent("POST", "/device-codes")).toHaveLength(1);
  });

  it("says why and offers Retry when an approved code finds no session", { timeout: 10_000 }, async () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0)");
    serveMockApi(scenarios.signedOut, (a) => {
      a.deviceCodePoll = "approved";
    });
    // Signed out until the poll, then the server fails to give the user.
    server.use(
      http.get("/api/v1/users/me", () =>
        sent("POST", "/device-codes/poll").length > 0 ? problem(503, "notReady") : problem(401, "unauthenticated"),
      ),
    );
    renderApp("/");
    expect(await screen.findByRole("button", { name: "Retry" }, { timeout: 5000 })).toBeInTheDocument();
  });
});
