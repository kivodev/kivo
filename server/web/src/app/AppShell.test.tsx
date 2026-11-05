import { within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderApp, screen } from "@/test/render";
import { serveMockApi } from "@/test/server";

describe("AppShell", () => {
  it("names an entry of the sidebar collapsed to its icons in a tooltip as the keyboard focuses it", async () => {
    // Only the collapsed sidebar's query matches, as for a window between
    // the phone layout and lg.
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query.includes("64rem"),
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    serveMockApi();
    const { user } = renderApp("/");
    const sidebar = await screen.findByRole("navigation", { name: "Main" });
    const search = within(sidebar).getByRole("link", { name: "Search" });
    while (document.activeElement !== search) await user.tab();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Search");
    expect(search).toHaveAccessibleName("Search");
    vi.unstubAllGlobals();
  });
});
