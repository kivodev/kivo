import { within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockIds, scenarios, serveMockApi } from "@/test/server";

describe("Libraries", () => {
  it("lists Favorites, the pinned libraries, then More libraries, each with its count", async () => {
    serveMockApi(scenarios.unpinned);
    const { user, router } = renderApp("/libraries");
    expect(await screen.findByRole("heading", { name: "Libraries", level: 1 })).toBeInTheDocument();
    const favorites = screen.getByRole("list", { name: "Favorites" });
    // Favorites counts every kind, read from the server.
    expect(await within(favorites).findByRole("link", { name: /^Favorites\s*\d+$/ })).toBeInTheDocument();
    const pinned = screen.getByRole("list", { name: "Pinned libraries" });
    expect(within(pinned).getByRole("link", { name: /^Movies\s*238$/ })).toBeInTheDocument();
    const more = screen.getByRole("region", { name: "More libraries" });
    await user.click(within(more).getByRole("link", { name: /^Home Videos/ }));
    expect(await screen.findByRole("heading", { name: "Home Videos", level: 1 })).toBeInTheDocument();
    expect(locationOf(router)).toBe(`/libraries/${mockIds.videos}`);
  });

  it("titles a library in the phone layout with its name alone, after Back to the list", async () => {
    vi.stubGlobal("matchMedia", () => ({ matches: true, addEventListener: () => {}, removeEventListener: () => {} }));
    serveMockApi();
    const { user, router } = renderApp(`/libraries/${mockIds.movies}`);
    const title = await screen.findByRole("heading", { name: "Movies", level: 1 });
    expect(within(title).queryByRole("button")).not.toBeInTheDocument();
    await user.click(within(screen.getByRole("main")).getByRole("link", { name: "Libraries" }));
    expect(await screen.findByRole("heading", { name: "Libraries", level: 1 })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/libraries");
    vi.unstubAllGlobals();
  });
});
