import { waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { onRequest, sent, serveMockApi } from "@/test/server";

describe("Problems", () => {
  it("shows each open problem with what to do about it", async () => {
    serveMockApi();
    renderApp("/admin/problems");
    expect(
      await screen.findByText(
        "/mnt/user/TV can't be reached (no such file or directory). Nothing in it has been removed.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Check that the disk or share is mounted, then scan the library again." }),
    ).toHaveAttribute("href", "/admin/libraries");
    // An unavailable provider clears itself, so it has no action.
    const unavailable = screen.getByText(/^TMDb is not responding/).closest('[data-part="problem"]');
    expect(unavailable?.querySelector("p")).toBeNull();
  });

  it("dismisses a problem", async () => {
    const api = serveMockApi();
    onRequest("delete", "/problems/{id}", (_, { id }) => {
      api.problems = api.problems.filter((p) => p.id !== id);
    });
    const { user } = renderApp("/admin/problems");
    await user.click(await screen.findByRole("button", { name: /^Dismiss TMDb is not responding/ }));
    await waitFor(() => expect(screen.queryByText(/TMDb is not responding/)).not.toBeInTheDocument());
    expect(sent("DELETE", "/problems/{id}").map((r) => decodeURIComponent(r.url.pathname))).toEqual([
      "/api/v1/problems/metadata:tmdbUnavailable",
    ]);
  });

  it("says when there are none", async () => {
    const api = serveMockApi();
    api.problems = [];
    renderApp("/admin/problems");
    expect(await screen.findByRole("heading", { name: "No problems" })).toBeInTheDocument();
  });
});
