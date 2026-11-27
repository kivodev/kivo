import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { mockIds, onRequest, sent, serveMockApi } from "@/test/server";

const path = "/mnt/user/Movies/Missing Reel (1999)/Missing Reel.mkv";
const page = `/admin/libraries/${mockIds.movies}/missing-files`;

describe("Missing files", () => {
  it("lists a library's gone files, each with its title, last path and when it went missing", async () => {
    serveMockApi();
    renderApp(page);
    expect(await screen.findByText(path)).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Missing files in Movies" })).toBeInTheDocument();
    expect(screen.getByText("Missing Reel")).toBeInTheDocument();
    expect(screen.getByText(/^Missing since /)).toBeInTheDocument();
  });

  it("lists none of another library's", async () => {
    serveMockApi();
    renderApp(`/admin/libraries/${mockIds.shows}/missing-files`);
    expect(await screen.findByRole("heading", { name: "No missing files" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Forget all" })).not.toBeInTheDocument();
  });

  it("links a work a library still has", async () => {
    serveMockApi((api) => {
      api.missingFiles = api.missingFiles.map((f) => ({ ...f, workId: mockIds.missingReel }));
    });
    renderApp(page);
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
    const { user } = renderApp(page);
    await user.click(await screen.findByRole("button", { name: `Forget ${path}` }));
    const confirm = await screen.findByRole("alertdialog");
    await user.click(within(confirm).getByRole("button", { name: "Forget" }));
    expect(await screen.findByRole("heading", { name: "No missing files" })).toBeInTheDocument();
    await waitFor(() =>
      expect(sent("DELETE", "/files/{id}").map((r) => r.url.pathname)).toEqual([`/api/v1/files/${file?.id}`]),
    );
  });

  it("forgets every missing file of the library after asking", async () => {
    const api = serveMockApi();
    onRequest("delete", "/files", () => {
      api.missingFiles = [];
    });
    const { user } = renderApp(page);
    await user.click(await screen.findByRole("button", { name: "Forget all" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Forget every missing file in Movies?" });
    await user.click(within(confirm).getByRole("button", { name: "Forget all" }));
    expect(await screen.findByRole("heading", { name: "No missing files" })).toBeInTheDocument();
    const params = sent("DELETE", "/files")[0]?.url.searchParams;
    expect([params?.get("status"), params?.get("libraryId")]).toEqual(["gone", mockIds.movies]);
  });
});
