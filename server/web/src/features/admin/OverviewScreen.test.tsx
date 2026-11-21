import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import { serverQueries } from "@/api/server";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockId, scenarios, sendEvent, serveMockApi } from "@/test/server";

/** The progress line whose label, on its left, starts with `label`. */
const lineOf = (label: string) =>
  waitFor(() => {
    const line = [...document.querySelectorAll<HTMLElement>('[data-part="step-progress"]')].find((l) =>
      l.textContent?.startsWith(label),
    );
    expect(line).toBeDefined();
    return line as HTMLElement;
  });

const section = (name: string) => screen.queryByRole("heading", { level: 2, name: new RegExp(`^${name}`) });

describe("Admin entry", () => {
  it("shows admins Admin, which opens the overview", async () => {
    serveMockApi();
    const { router, user } = renderApp("/");
    const admin = await screen.findByRole("link", { name: "Admin" });
    await user.click(admin);
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/admin");
    expect(
      within(screen.getByRole("navigation", { name: "Admin" })).getByRole("link", { name: "Overview" }),
    ).toHaveAttribute("aria-current", "page");
  });

  it("is hidden from viewers, and the admin area doesn't exist for them", async () => {
    serveMockApi(scenarios.viewer);
    renderApp("/admin");
    expect(await screen.findByRole("heading", { name: "This page doesn't exist." })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Admin/ })).not.toBeInTheDocument();
  });
});

describe("New release", () => {
  const out = (version: string) => (api: MockApi) => {
    api.server.newRelease = { version, url: `https://github.com/kivodev/kivo/releases/tag/v${version}` };
  };

  it("tells admins, links its release notes, and stays dismissed until the next one", async () => {
    const api = serveMockApi(out("1.1.0"));
    const first = renderApp("/admin");
    expect(await screen.findByText("Kivo 1.1.0 is available")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Release notes" })).toHaveAttribute(
      "href",
      "https://github.com/kivodev/kivo/releases/tag/v1.1.0",
    );
    await first.user.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText("Kivo 1.1.0 is available")).toBeNull();
    first.unmount();

    const second = renderApp("/admin");
    await waitFor(() => expect(second.queryClient.getQueryData(serverQueries.info().queryKey)).toBeDefined());
    expect(screen.queryByText(/is available$/)).toBeNull();
    second.unmount();

    out("1.2.0")(api);
    renderApp("/admin");
    expect(await screen.findByText("Kivo 1.2.0 is available")).toBeInTheDocument();
  });

  it("isn't shown to viewers, who have no admin area", async () => {
    serveMockApi(scenarios.viewer, out("1.1.0"));
    renderApp("/admin");
    expect(await screen.findByRole("heading", { name: "This page doesn't exist." })).toBeInTheDocument();
    expect(screen.queryByText(/is available$/)).toBeNull();
  });
});

describe("Overview", () => {
  it("on a busy evening shows the plays with their summary, the work going on and the latest activity", async () => {
    serveMockApi(scenarios.busy);
    renderApp("/admin");
    const playing = await waitFor(() => {
      const heading = section("Now playing");
      expect(heading).toHaveTextContent(/3 playing · 1 transcoding · \d+ Mb\/s$/);
      return heading as HTMLElement;
    });
    expect(within(playing).queryByRole("link")).toBeNull();
    expect(await screen.findByRole("button", { name: /^Dune: Part Two, ben/ })).toBeInTheDocument();

    expect(section("Working now")).toBeInTheDocument();
    expect(await lineOf("Scanning Movies · Reading files")).toHaveTextContent("340 of 812 · about 4m left");
    expect(await lineOf("Intro and credits · Analyzing")).toHaveTextContent("312 of 1,516");
    expect(document.querySelectorAll('[data-part="step-progress"]')).toHaveLength(2);

    const events = await waitFor(() => {
      const rows = document.querySelectorAll('[data-part="activity-event"]');
      expect(rows).toHaveLength(6);
      return rows;
    });
    expect(events[0]).toHaveTextContent(/ben started Dune: Part Two · Firefox on Windows · Remote$/);
    expect(events[1]).toHaveTextContent(/anna started The Expanse S3 · E4 · Living Room TV$/);
    expect(events[2]).toHaveTextContent(/3 episodes of The Expanse added to TV Shows$/);
    expect(screen.getByRole("link", { name: "See all" })).toHaveAttribute("href", "/admin/activity");
  });

  it("on a quiet day shows only what's playing", async () => {
    serveMockApi(scenarios.quiet);
    renderApp("/admin");
    expect(await screen.findByRole("button", { name: /^The Expanse, anna/ })).toBeInTheDocument();
    expect(section("Now playing")).toHaveTextContent(/^Now playing1 playing · 8\.2 Mb\/s$/);
    expect(screen.getAllByRole("heading", { level: 2 })).toHaveLength(1);
    expect(section("Working now")).toBeNull();
    expect(section("Activity")).toBeNull();
    // Nor the version, uptime or scans the Overview once had.
    expect(screen.queryByText(/^Kivo /)).toBeNull();
    expect(screen.queryByText("No scans running.")).toBeNull();
  });

  it("shows a library's scan matching on one line", async () => {
    serveMockApi(scenarios.quiet, scenarios.matching);
    renderApp("/admin");
    expect(await lineOf("Scanning Movies · Matching")).toHaveTextContent("120 of 768 · about 9m left");
    expect(document.querySelectorAll('[data-part="step-progress"]')).toHaveLength(1);
  });

  it("says when nothing is playing, and shows a play that starts, told by the server", async () => {
    const api = serveMockApi();
    renderApp("/admin");
    expect(await screen.findByText("Nothing playing")).toBeInTheDocument();
    scenarios.nowPlaying(api);
    sendEvent({ type: "event", kind: "playsChanged" });
    expect(await screen.findByRole("button", { name: /^Dune: Part Two, ben/ })).toBeInTheDocument();
    expect(screen.queryByText("Nothing playing")).toBeNull();
  });

  it("follows the activity as the server tells of changes", async () => {
    const api = serveMockApi();
    renderApp("/admin");
    api.activity.unshift({
      id: mockId(8999),
      occurredAt: new Date().toISOString(),
      kind: "userJoined",
      joined: { userId: mockId(7104), username: "erin" },
    });
    sendEvent({ type: "event", kind: "activityChanged" });
    await waitFor(
      () =>
        expect(document.querySelector('[data-part="activity-event"]')).toHaveTextContent(/erin joined from an invite$/),
      { timeout: 3000 },
    );
  });
});
