import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { http, mockId, serveMockApi, server } from "@/test/server";

const sidebar = () => screen.findByRole("navigation", { name: "Admin" });

/** Holds every answer from one GET route until released. */
function hold(path: "/api/v1/settings" | "/api/v1/play-events"): () => void {
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  server.use(http.get(path, () => held));
  return release;
}

/** Whatever shows the page loading, which is a placeholder or Loading… for a screen reader. */
const placeholders = () => document.querySelectorAll('[aria-busy="true"], .animate-pulse, .sr-only');

describe("Admin layout", () => {
  it("marks a section current on its sub-pages and filtered views", async () => {
    serveMockApi();
    renderApp(`/admin/users/${mockId(7101)}`);
    expect(within(await sidebar()).getByRole("link", { name: "Users" })).toHaveAttribute("aria-current", "page");
    expect(within(await sidebar()).getByRole("link", { name: "Overview" })).not.toHaveAttribute("aria-current");
  });

  it("marks Tasks current on a task's page", async () => {
    serveMockApi();
    renderApp("/admin/tasks/make-previews");
    expect(within(await sidebar()).getByRole("link", { name: "Tasks" })).toHaveAttribute("aria-current", "page");
  });

  it("names the page on the phones' button, which opens the pages as a sheet", async () => {
    serveMockApi();
    const { user, router } = renderApp("/admin/tasks");
    const button = await screen.findByRole("button", { name: "Admin pages" });
    expect(button).toHaveTextContent("AdminTasks");
    await user.click(button);
    const sheet = await screen.findByRole("dialog", { name: "Admin" });
    await user.click(within(sheet).getByRole("link", { name: "Logs" }));
    await waitFor(() => expect(locationOf(router)).toBe("/admin/logs"));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByRole("heading", { name: "Logs" })).toBeInTheDocument();
  });

  it("shows the sidebar beside nothing until a page's data is here", async () => {
    serveMockApi();
    const release = hold("/api/v1/settings");
    renderApp("/admin/settings/general");
    await sidebar();
    expect(screen.queryByRole("heading", { level: 1 })).not.toBeInTheDocument();
    expect(placeholders()).toHaveLength(0);
    release();
    expect(await screen.findByRole("heading", { name: "General", level: 1 })).toBeInTheDocument();
  });

  it("shows nothing in place of the server-wide history's placeholders", async () => {
    serveMockApi();
    const release = hold("/api/v1/play-events");
    renderApp("/admin/history");
    await sidebar();
    expect(screen.queryByRole("heading", { name: "Watch history" })).not.toBeInTheDocument();
    expect(placeholders()).toHaveLength(0);
    release();
    expect(await screen.findByRole("heading", { name: "Watch history" })).toBeInTheDocument();
  });
});
