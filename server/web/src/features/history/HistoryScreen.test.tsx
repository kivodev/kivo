import { waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import type { WorkDetail } from "@/api/schemas";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockId, mockIds, sent, serveMockApi, summaryOf } from "@/test/server";

/** A play of Dune: Part Two by anna that stopped at 1:12:03. */
function played(api: MockApi) {
  api.history = [
    {
      id: "s1",
      work: summaryOf(api.works[mockIds.dunePartTwo] as WorkDetail),
      userId: mockId(7001),
      username: "anna",
      startedAt: "2026-10-01T20:00:00Z",
      positionMs: 4_323_000,
      completed: false,
    },
  ];
}

describe("Watch history", () => {
  it("lists your own plays with how far they got", async () => {
    serveMockApi(played);
    renderApp("/history");
    expect(await screen.findByRole("heading", { name: "Watch history" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Dune: Part Two" })).toHaveAttribute(
      "href",
      `/works/${mockIds.dunePartTwo}`,
    );
    expect(screen.getByText(/Stopped at 1:12:03/)).toBeInTheDocument();
    expect(screen.queryByText(/· anna$/)).not.toBeInTheDocument();
    expect(sent("GET", "/play-events")).toHaveLength(0);
  });

  it("shows admins everyone's, with who played each", async () => {
    serveMockApi(played);
    renderApp("/admin/history");
    expect(await screen.findByRole("heading", { name: "Watch history" })).toBeInTheDocument();
    expect(screen.getByText(/Stopped at 1:12:03/).closest("span")).toHaveTextContent(/Stopped at 1:12:03 · anna$/);
    expect(sent("GET", "/users/me/play-events")).toHaveLength(0);
  });

  it("filters the server-wide history by user, in the URL", async () => {
    serveMockApi(played);
    const { user, router } = renderApp("/admin/history");
    await user.click(await screen.findByRole("link", { name: "anna" }));
    await waitFor(() => expect(locationOf(router)).toBe(`/admin/history?userId=${mockId(7001)}`));
    await waitFor(() => expect(sent("GET", "/play-events").at(-1)?.url.searchParams.get("userId")).toBe(mockId(7001)));
    expect(await screen.findByRole("button", { name: /User/ })).toHaveTextContent("anna");
    await user.click(screen.getByRole("button", { name: /User/ }));
    await user.click(await screen.findByRole("option", { name: "All users" }));
    await waitFor(() => expect(locationOf(router)).toBe("/admin/history"));
  });

  it("says when nothing's been watched", async () => {
    serveMockApi();
    renderApp("/history");
    expect(await screen.findByRole("heading", { name: "No watch history" })).toBeInTheDocument();
  });
});
