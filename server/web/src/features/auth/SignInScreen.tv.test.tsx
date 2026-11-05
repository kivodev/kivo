import { fireEvent, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderApp, screen } from "@/test/render";
import { scenarios, sent, serveMockApi } from "@/test/server";
import { exitApp } from "@/tv/exit";
import { focusTv } from "@/tv/spatial";
import { startTv, stopTv } from "@/tv/start";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn(), changeServer: vi.fn() }));

// Each test starts the TV build, which the one before it stopped.
describe("Sign in on TV", () => {
  /** Opens the password form from the code, with focus on the username. */
  async function openPasswordForm() {
    serveMockApi(scenarios.signedOut);
    renderApp("/");
    expect(await screen.findByText("K7Q-2M9")).toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "signin:password"));
    fireEvent.keyDown(document.activeElement ?? window, { key: "Enter" });
    await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "signin:username"));
  }

  it("types the password form's fields on the TV's own keyboard, with none of its own", {
    timeout: 10_000,
  }, async () => {
    startTv();
    try {
      await openPasswordForm();
      expect(document.querySelector('[data-part="tv-keyboard"]')).toBeNull();
      expect(screen.queryByRole("button", { name: "a" })).not.toBeInTheDocument();
      // OK on a field focuses its input, which opens the TV's keyboard.
      fireEvent.keyDown(document.activeElement ?? window, { key: "Enter" });
      expect(document.activeElement).toBe(screen.getByLabelText("Username"));
    } finally {
      stopTv();
    }
  });

  it("labels the TV keyboard's action Next on Username and Go on Password", { timeout: 10_000 }, async () => {
    startTv();
    try {
      await openPasswordForm();
      expect(screen.getByLabelText("Username")).toHaveAttribute("enterkeyhint", "next");
      expect(screen.getByLabelText("Password")).toHaveAttribute("enterkeyhint", "go");
    } finally {
      stopTv();
    }
  });

  it("goes on from Username to Password with the TV keyboard's Enter, then signs in", { timeout: 10_000 }, async () => {
    startTv();
    try {
      await openPasswordForm();
      const username = screen.getByLabelText("Username");
      const password = screen.getByLabelText("Password");
      fireEvent.keyDown(document.activeElement ?? window, { key: "Enter" });
      fireEvent.change(username, { target: { value: "anna" } });
      fireEvent.keyDown(username, { key: "Enter" });
      await waitFor(() => expect(document.activeElement).toBe(password));
      fireEvent.change(password, { target: { value: "secret" } });
      fireEvent.keyDown(password, { key: "Enter" });
      await waitFor(() => expect(sent("POST", "/sessions")).toHaveLength(1));
      expect(sent("POST", "/sessions")[0]?.body).toMatchObject({ username: "anna", password: "secret" });
    } finally {
      stopTv();
    }
  });

  it("returns to an empty Username from Password's Go without leaving it on the way", {
    timeout: 10_000,
  }, async () => {
    startTv();
    try {
      await openPasswordForm();
      const username = screen.getByLabelText("Username");
      const password = screen.getByLabelText("Password");
      focusTv("signin:password");
      await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "signin:password"));
      fireEvent.keyDown(document.activeElement ?? window, { key: "Enter" });
      fireEvent.change(password, { target: { value: "secret" } });
      let blurs = 0;
      username.addEventListener("blur", () => blurs++);
      fireEvent.keyDown(password, { key: "Enter" });
      await waitFor(() => expect(document.activeElement).toBe(username));
      // Its keyboard opens once, rather than opening, closing and opening.
      await new Promise((r) => setTimeout(r, 300));
      expect(document.activeElement).toBe(username);
      expect(blurs).toBe(0);
      expect(username).toHaveAttribute("aria-invalid", "true");
      expect(sent("POST", "/sessions")).toHaveLength(0);
    } finally {
      stopTv();
    }
  });

  it("takes Tizen's keyboard Done as Enter, going on from Username to Password", { timeout: 10_000 }, async () => {
    startTv();
    try {
      await openPasswordForm();
      const username = screen.getByLabelText("Username");
      fireEvent.keyDown(document.activeElement ?? window, { key: "Enter" });
      fireEvent.change(username, { target: { value: "anna" } });
      fireEvent.keyDown(username, { key: "Unidentified", keyCode: 65376 });
      await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText("Password")));
    } finally {
      stopTv();
    }
  });

  it("asks to exit on Back in the TV build, and Exit leaves Kivo", { timeout: 10_000 }, async () => {
    startTv();
    try {
      serveMockApi(scenarios.signedOut);
      renderApp("/");
      expect(await screen.findByText("K7Q-2M9")).toBeInTheDocument();
      await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "signin:password"));
      fireEvent.keyDown(window, { key: "XF86Back" });
      await screen.findByRole("alertdialog", { name: "Exit Kivo?" });
      await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "tv:exit:no"));
      // Back closes it, back to where focus was.
      fireEvent.keyDown(window, { key: "XF86Back" });
      await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
      await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "signin:password"));
      fireEvent.keyDown(window, { key: "XF86Back" });
      await screen.findByRole("alertdialog", { name: "Exit Kivo?" });
      focusTv("tv:exit:yes");
      await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "tv:exit:yes"));
      fireEvent.keyDown(document.activeElement ?? window, { key: "Enter" });
      fireEvent.keyUp(document.activeElement ?? window, { key: "Enter" });
      expect(exitApp).toHaveBeenCalledTimes(1);
    } finally {
      stopTv();
    }
  });
});
