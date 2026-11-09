import { afterEach, describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { mockIds, sent, serveMockApi } from "@/test/server";
import { stopTv } from "@/tv/start";

afterEach(() => stopTv());

describe("Smart lists on TV", () => {
  it("lists a smart list with the others, with no builder", async () => {
    serveMockApi();
    renderApp("/lists");
    const row = await screen.findByRole("link", { name: /Sci-fi/ });
    expect(row).toHaveTextContent("Smart list, 2 titles");
    expect(screen.queryByRole("heading", { name: "Smart Lists" })).toBeNull();
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
