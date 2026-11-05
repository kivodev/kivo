import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { WorkDetail } from "@/api/schemas";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockId, mockIds, onRequest, scenarios, sent, serveMockApi } from "@/test/server";

const section = (name: string) => screen.getByRole("heading", { name }).closest("section") as HTMLElement;

describe("Unmatched", () => {
  it("lists each unmatched root work under its library with its year and what it was parsed from", async () => {
    serveMockApi(scenarios.busy);
    renderApp("/admin/unmatched");
    expect(await screen.findByText("/mnt/user/Movies/Holiday 2019 Final.mkv")).toBeInTheDocument();
    const movies = section("Movies");
    expect(within(movies).getByRole("link", { name: "Holiday 2019 Final" })).toHaveAttribute(
      "href",
      `/works/${mockIds.holiday}`,
    );
    expect(within(movies).getByText("2019")).toBeInTheDocument();
    const shows = section("TV Shows");
    expect(within(shows).getByRole("link", { name: "the.night.shift" })).toHaveAttribute(
      "href",
      `/works/${mockIds.nightShift}`,
    );
    expect(within(shows).getByText("/mnt/user/TV/the.night.shift.2024")).toBeInTheDocument();
    expect(within(movies).queryByText("the.night.shift")).not.toBeInTheDocument();
    expect(document.querySelectorAll('[data-part="unmatched-title"]')).toHaveLength(2);
  });

  it("fixes a match in the dialog, and the matched root work leaves the list while the page stays", async () => {
    const api = serveMockApi(scenarios.busy);
    // The match is another work, Dune, which the root work's versions move to.
    onRequest("post", "/works/{id}/match", () => {
      api.works[mockIds.holiday] = api.works[mockIds.dune] as WorkDetail;
      api.unmatched = api.unmatched.filter((w) => w.id !== mockIds.holiday);
    });
    const { router, user } = renderApp("/admin/unmatched");
    await user.click(await screen.findByRole("button", { name: "Fix match for Holiday 2019 Final" }));
    const dialog = await screen.findByRole("dialog", { name: "Fix match: Holiday 2019 Final" });
    expect(within(dialog).getByRole("textbox", { name: "Title" })).toHaveValue("Holiday 2019 Final");
    await user.click(await within(dialog).findByRole("button", { name: "Choose Holiday (2019)" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.queryByText("Holiday 2019 Final")).not.toBeInTheDocument());
    expect(screen.queryByRole("heading", { name: "Movies" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "the.night.shift" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/admin/unmatched");
    expect(sent("POST", `/works/${mockIds.holiday}/match`)[0]?.body).toEqual({ matchId: "tmdb-1" });
  });

  it("says when nothing is unmatched", async () => {
    serveMockApi(scenarios.quiet);
    renderApp("/admin/unmatched");
    expect(await screen.findByRole("heading", { name: "Nothing unmatched" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
  });

  it("shows more a page at a time", async () => {
    serveMockApi((api) => {
      const [holiday] = api.unmatched;
      if (!holiday) throw new Error("the mock has an unmatched root work");
      api.unmatched = Array.from({ length: 150 }, (_, i) => ({
        ...holiday,
        id: mockId(80_000 + i),
        title: `untitled.${i}`,
        path: `/mnt/user/Movies/untitled.${i}.mkv`,
      }));
    });
    const { user } = renderApp("/admin/unmatched");
    expect(await screen.findByText("untitled.99")).toBeInTheDocument();
    expect(screen.queryByText("untitled.100")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Show more" }));
    expect(await screen.findByText("untitled.149")).toBeInTheDocument();
    expect(document.querySelectorAll('[data-part="unmatched-title"]')).toHaveLength(150);
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    expect(sent("GET", "/unmatched-works").map((r) => r.url.searchParams.get("after"))).toEqual([null, "100"]);
  });
});
