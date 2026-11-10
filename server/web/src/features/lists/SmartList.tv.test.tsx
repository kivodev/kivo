import { within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { mockIds, sent, serveMockApi } from "@/test/server";
import { stopTv } from "@/tv/start";

afterEach(() => stopTv());

describe("Smart lists on TV", () => {
  it("shelves a smart list with the others, each ending in See all, with no builder", async () => {
    serveMockApi();
    renderApp("/lists");
    const heading = await screen.findByRole("heading", { name: /^Sci-fi/, level: 2 });
    expect(heading).toHaveTextContent("Sci-fiSmart list2 titles");
    expect(heading.querySelector("a")).toBeNull();
    const shelf = heading.closest("[data-part='row']") as HTMLElement;
    expect(within(shelf).getByRole("link", { name: "See all" })).toHaveAttribute("href", `/lists/${mockIds.sciFi}`);
    // An empty list's placeholder opens it, and it has no See all.
    const weekend = screen.getByRole("heading", { name: /^Weekend/ }).closest("[data-part='row']") as HTMLElement;
    expect(within(weekend).getByRole("link", { name: "Nothing in this list yet" })).toHaveAttribute(
      "href",
      `/lists/${mockIds.weekend}`,
    );
    expect(within(weekend).queryByRole("link", { name: "See all" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Create" })).toBeNull();
  });

  it("opens its grid by the paged read, with no Edit and no read of the field catalog", async () => {
    serveMockApi();
    renderApp(`/lists/${mockIds.sciFi}`);
    expect(await screen.findByRole("heading", { name: "Sci-fi", level: 1 })).toBeInTheDocument();
    expect(await screen.findByText("2 titles")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(sent("GET", "/work-fields")).toHaveLength(0);
    expect(sent("GET", `/lists/${mockIds.sciFi}/works`).length).toBeGreaterThan(0);
  });
});
