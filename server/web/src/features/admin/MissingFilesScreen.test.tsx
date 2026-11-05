import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { mockIds, onRequest, sent, serveMockApi } from "@/test/server";

const path = "/mnt/user/Movies/Missing Reel (1999)/Missing Reel.mkv";

describe("Missing files", () => {
  it("lists each gone file under its library with its title, last path and when it went missing", async () => {
    serveMockApi();
    renderApp("/admin/missing-files");
    expect(await screen.findByText(path)).toBeInTheDocument();
    const movies = screen.getByRole("heading", { name: "Movies" }).closest("section") as HTMLElement;
    expect(within(movies).getByText("Missing Reel")).toBeInTheDocument();
    expect(within(movies).getByText(/^Missing since /)).toBeInTheDocument();
  });

  it("links a work a library still has", async () => {
    serveMockApi((api) => {
      api.missingFiles = api.missingFiles.map((f) => ({ ...f, workId: mockIds.missingReel }));
    });
    renderApp("/admin/missing-files");
    expect(await screen.findByRole("link", { name: "Missing Reel" })).toHaveAttribute(
      "href",
      `/works/${mockIds.missingReel}`,
    );
  });

  it("forgets a file after asking", async () => {
    const api = serveMockApi();
    const [file] = api.missingFiles;
    onRequest("delete", "/files/{id}", () => {
      api.missingFiles = [];
    });
    const { user } = renderApp("/admin/missing-files");
    await user.click(await screen.findByRole("button", { name: `Forget ${path}` }));
    const confirm = await screen.findByRole("alertdialog");
    await user.click(within(confirm).getByRole("button", { name: "Forget" }));
    expect(await screen.findByRole("heading", { name: "No missing files" })).toBeInTheDocument();
    await waitFor(() =>
      expect(sent("DELETE", "/files/{id}").map((r) => r.url.pathname)).toEqual([`/api/v1/files/${file?.id}`]),
    );
  });

  it("forgets every missing file after asking", async () => {
    const api = serveMockApi();
    onRequest("delete", "/files", () => {
      api.missingFiles = [];
    });
    const { user } = renderApp("/admin/missing-files");
    await user.click(await screen.findByRole("button", { name: "Forget all" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Forget every missing file?" });
    await user.click(within(confirm).getByRole("button", { name: "Forget all" }));
    expect(await screen.findByRole("heading", { name: "No missing files" })).toBeInTheDocument();
    expect(sent("DELETE", "/files")[0]?.url.searchParams.get("status")).toBe("gone");
  });
});
