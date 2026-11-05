import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { mockId } from "@/api/mocks/fixtures";
import { locationOf, renderApp, screen } from "@/test/render";
import { scenarios, sent, serveMockApi } from "@/test/server";

const approvals = () => sent("POST", "/device-codes/{code}/approve");
const denials = () => sent("POST", "/device-codes/{code}/deny");

describe("The /link page", () => {
  afterEach(() => vi.restoreAllMocks());

  it("says who is asking, from where and when, and asks before letting it sign in", async () => {
    serveMockApi();
    const { user } = renderApp("/link?code=K7Q2M9");
    expect(await screen.findByRole("heading", { name: "Allow Living Room to sign in as anna?" })).toBeInTheDocument();
    expect(screen.getByText("Android TV · On your home network · Just now")).toBeInTheDocument();
    expect(screen.queryByRole("note")).not.toBeInTheDocument();
    expect(approvals()).toEqual([]);
    await user.click(screen.getByRole("button", { name: "Allow" }));
    expect(await screen.findByRole("heading", { name: "Living Room is signed in" })).toBeInTheDocument();
    expect(approvals()[0]?.url.pathname).toMatch(/\/device-codes\/K7Q2M9\/approve$/);
  });

  it("warns about a device away from the home network", async () => {
    serveMockApi((api) => {
      api.waitingDevice = { name: "Den", platform: "iPad", nearby: false, startedAt: new Date().toISOString() };
    });
    renderApp("/link?code=K7Q2M9");
    expect(await screen.findByText("iPad · Not on your home network · Just now")).toBeInTheDocument();
    expect(screen.getByRole("note")).toHaveTextContent("This device isn't on your home network.");
  });

  it("denies the device on the server when it isn't allowed", async () => {
    serveMockApi();
    const { user } = renderApp("/link?code=K7Q2M9");
    await user.click(await screen.findByRole("button", { name: "Don't allow" }));
    expect(await screen.findByRole("heading", { name: "Living Room wasn't signed in" })).toBeInTheDocument();
    expect(denials()[0]?.url.pathname).toMatch(/\/device-codes\/K7Q2M9\/deny$/);
    expect(approvals()).toEqual([]);
  });

  it("doesn't look up a code for another server", async () => {
    serveMockApi();
    renderApp("/link?code=K7Q2M9&server=someone-else");
    expect(
      await screen.findByText("This device is signing in to another server. You're signed in to Living room."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Allow" })).not.toBeInTheDocument();
    expect(sent("GET", "/device-codes/{code}")).toEqual([]);
  });

  it("looks up a code for this server", async () => {
    serveMockApi();
    renderApp(`/link?code=K7Q2M9&server=${mockId(7200)}`);
    expect(await screen.findByRole("heading", { name: "Allow Living Room to sign in as anna?" })).toBeInTheDocument();
  });

  it("offers the app first on a phone", async () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)");
    serveMockApi();
    const { user } = renderApp(`/link?code=K7Q2M9&server=${mockId(7200)}`);
    const app = await screen.findByRole("link", { name: "Open in the Kivo app" });
    expect(app).toHaveAttribute("href", `kivo://link?code=K7Q2M9&server=${mockId(7200)}`);
    expect(sent("GET", "/device-codes/{code}")).toEqual([]);
    await user.click(screen.getByRole("button", { name: "Continue in browser" }));
    expect(await screen.findByRole("heading", { name: "Allow Living Room to sign in as anna?" })).toBeInTheDocument();
  });

  it("asks for a code without one, says when one didn't work, and shows the server address", async () => {
    serveMockApi((api) => {
      delete api.waitingDevice;
    });
    const { user, router } = renderApp("/link");
    expect(await screen.findByRole("heading", { name: "Link a device" })).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Show server address" }));
    expect(screen.getByRole("img", { name: "QR code of the server address" })).toBeInTheDocument();
    expect(screen.getByText("http://192.168.1.10:8484")).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Code" }), "k7q 2m9");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(locationOf(router)).toBe("/link?code=K7Q2M9"));
    expect(await screen.findByText("That code didn't work. Check the other device for a new one.")).toBeInTheDocument();
  });

  it("shows the public URL as the server address when one is set", async () => {
    serveMockApi((api) => {
      delete api.waitingDevice;
      api.server = { ...api.server, publicUrl: "https://kivo.example.com" };
    });
    const { user } = renderApp("/link");
    await user.click(await screen.findByRole("button", { name: "Show server address" }));
    expect(screen.getByRole("img", { name: "QR code of the server address" })).toBeInTheDocument();
    expect(screen.getByText("https://kivo.example.com")).toBeInTheDocument();
  });

  it("opens from the account menu", async () => {
    serveMockApi();
    const { user, router } = renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Account menu for anna" }));
    await user.click(await screen.findByRole("menuitem", { name: "Link a device" }));
    expect(await screen.findByRole("heading", { name: "Link a device" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/link");
  });

  it("signs a signed-out visitor in first, then comes back", async () => {
    const api = serveMockApi(scenarios.signedOut);
    const { user, router } = renderApp("/link?code=K7Q2M9");
    await user.type(await screen.findByRole("textbox", { name: "Username" }), "anna");
    await user.type(screen.getByLabelText("Password"), "a good password");
    api.signedIn = true;
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("heading", { name: "Allow Living Room to sign in as anna?" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/link?code=K7Q2M9");
  });
});
