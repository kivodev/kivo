import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockId, mockIds, serveMockApi } from "@/test/server";

const rows = () => [...document.querySelectorAll<HTMLElement>('[data-part="activity-event"]')];
const days = () =>
  [...document.querySelectorAll<HTMLElement>('[data-part="activity-day"]')].map((d) => ({
    name: within(d).getByRole("heading").textContent,
    rows: d.querySelectorAll('[data-part="activity-event"]').length,
  }));

describe("Activity", () => {
  it("lists events newest first by day: today, yesterday, then dates", async () => {
    serveMockApi();
    renderApp("/admin/activity");
    expect(await screen.findByRole("heading", { name: "Activity", level: 1 })).toBeInTheDocument();
    await waitFor(() => expect(rows()).toHaveLength(14));
    const [today, yesterday, before, ...rest] = days();
    expect(today).toEqual({ name: "Today", rows: 9 });
    expect(yesterday).toEqual({ name: "Yesterday", rows: 3 });
    expect(before?.rows).toBe(2);
    expect(before?.name).toMatch(/^\w+day, \w+ \d+$/);
    expect(rest).toEqual([]);
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });

  it("says each event in words, with the client's address and links to failures and fixes", async () => {
    serveMockApi();
    renderApp("/admin/activity");
    await waitFor(() => expect(rows()).toHaveLength(14));
    const text = rows().map((r) => r.textContent?.replace(/^\d{1,2}:\d{2}\s?[AP]M/, ""));
    expect(text).toEqual([
      "ben started Dune: Part Two · Firefox on Windows · Remote",
      "anna started The Expanse S3 · E4 · Living Room TV",
      "3 episodes of The Expanse added to TV Shows",
      "carla finished Leviathan Wakes · Chrome on Android",
      "ben signed in on ben's iPhone · Remote · 203.0.113.24",
      "3 failed sign-ins for ben · 203.0.113.24",
      "Blade Runner added to Movies",
      "Preview thumbnails · 214 done · 2 failed",
      "Back up database · Backed up to kivo-backup.db",
      "dana joined from an invite",
      "Quick Sync stopped working · Fix",
      "anna finished The Expanse S3 · E3 · Living Room TV",
      "anna added carla",
      "Back up database failed · The backup folder is full.",
    ]);
    expect(within(rows()[7] as HTMLElement).getByRole("link", { name: "2 failed" })).toHaveAttribute(
      "href",
      "/admin/tasks/make-previews",
    );
    expect(within(rows()[10] as HTMLElement).getByRole("link", { name: "Fix" })).toHaveAttribute(
      "href",
      "/admin/settings/transcoding",
    );
  });

  it("shows one kind of event, kept in the address", async () => {
    serveMockApi();
    const { user, router } = renderApp("/admin/activity");
    await waitFor(() => expect(rows()).toHaveLength(14));
    await user.click(screen.getByRole("button", { name: /Show/ }));
    await user.click(await screen.findByRole("option", { name: "Sign-ins" }));
    await waitFor(() => expect(locationOf(router)).toBe("/admin/activity?show=signIns"));
    await waitFor(() => expect(rows()).toHaveLength(4));
    expect(rows().map((r) => r.dataset.kind)).toEqual(["signedIn", "signInFailed", "userJoined", "userJoined"]);

    await user.click(screen.getByRole("button", { name: /Show/ }));
    await user.click(await screen.findByRole("option", { name: "Everything" }));
    await waitFor(() => expect(locationOf(router)).toBe("/admin/activity"));
    await waitFor(() => expect(rows()).toHaveLength(14));
  });

  it("opens filtered, as Needs attention's link to failed sign-ins does", async () => {
    serveMockApi();
    renderApp("/admin/activity?show=plays");
    await waitFor(() => expect(rows()).toHaveLength(4));
    expect(new Set(rows().map((r) => r.dataset.kind))).toEqual(new Set(["playStarted", "playFinished"]));
  });

  it("shows more, a page at a time", async () => {
    const api = serveMockApi();
    const start = new Date().setHours(0, 0, 0, 0);
    api.activity = Array.from({ length: 60 }, (_, i) => ({
      id: mockId(9000 + i),
      occurredAt: new Date(start - i * 60_000).toISOString(),
      kind: "added" as const,
      added: {
        libraryName: "Movies",
        work: { workId: mockIds.dune, kind: "movie" as const, title: `Film ${i}` },
        count: 1,
      },
    }));
    const { user } = renderApp("/admin/activity");
    await waitFor(() => expect(rows()).toHaveLength(50));
    await user.click(screen.getByRole("button", { name: "Show more" }));
    await waitFor(() => expect(rows()).toHaveLength(60));
    expect(rows().at(-1)).toHaveTextContent("Film 59 added to Movies");
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });

  it("says when there's nothing yet", async () => {
    const api = serveMockApi();
    api.activity = [];
    renderApp("/admin/activity");
    expect(await screen.findByRole("heading", { name: "Nothing yet" })).toBeInTheDocument();
  });
});
