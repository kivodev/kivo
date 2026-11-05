import { waitFor } from "@testing-library/react";
import { http as untyped } from "msw";
import { describe, expect, it } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { http, problem, scenarios, sent, serveMockApi, server } from "@/test/server";

describe("The invite page", () => {
  it("says who sent the link, makes the user and signs it in", async () => {
    serveMockApi(scenarios.signedOut);
    const { user, router } = renderApp("/invite/invite-token");
    expect(await screen.findByRole("heading", { name: "anna invited you to Kivo on Living room" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Username" }), "cara");
    await user.type(screen.getByLabelText("Password"), "a good password");
    await user.click(screen.getByRole("button", { name: "Create account" }));
    await waitFor(() => expect(locationOf(router)).toBe("/"));
    expect(sent("POST", "/invite-links/invite-token/accept")[0]?.body).toEqual({
      username: "cara",
      password: "a good password",
      kind: "browser",
      name: expect.any(String),
    });
  });

  it("keeps the form while it signs in, though the link is used up", async () => {
    serveMockApi(scenarios.signedOut);
    const { user, router } = renderApp("/invite/invite-token");
    await user.type(await screen.findByRole("textbox", { name: "Username" }), "cara");
    await user.type(screen.getByLabelText("Password"), "a good password");
    // The signed-in user comes back only once the test has looked, and the
    // link is used up by then.
    let answer = () => {};
    const held = new Promise<void>((resolve) => {
      answer = resolve;
    });
    server.use(untyped.get("*/api/v1/users/me", () => held));
    const asked = sent("GET", "/users/me").length;
    await user.click(screen.getByRole("button", { name: "Create account" }));
    await waitFor(() => expect(sent("GET", "/users/me").length).toBeGreaterThan(asked));
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(screen.queryByText("This invite link doesn't work")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "anna invited you to Kivo on Living room" })).toBeInTheDocument();
    answer();
    await waitFor(() => expect(locationOf(router)).toBe("/"));
  });

  it("says a dead link doesn't work", async () => {
    serveMockApi(scenarios.signedOut, (api) => {
      delete api.inviteLink;
    });
    renderApp("/invite/old-token");
    expect(
      await screen.findByText("This invite link has expired, was already used or was revoked. Ask for a new link."),
    ).toBeInTheDocument();
  });

  it("shows a taken username on its field", async () => {
    serveMockApi(scenarios.signedOut);
    server.use(http.post("/api/v1/invite-links/{token}/accept", () => problem(409, "nameTaken")));
    const { user } = renderApp("/invite/invite-token");
    await user.type(await screen.findByRole("textbox", { name: "Username" }), "anna");
    await user.type(screen.getByLabelText("Password"), "a good password");
    await user.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByText("That username is taken.")).toBeInTheDocument();
  });
});
