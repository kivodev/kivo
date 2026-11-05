import { describe, expect, it } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockId, onRequest, scenarios, sent, serveMockApi } from "@/test/server";

describe("First-run setup", () => {
  it("walks from the admin user through libraries and metadata to Home", async () => {
    const api = serveMockApi(scenarios.setup);
    onRequest("post", "/setup", () => {
      api.setup.required = false;
      api.signedIn = true;
    });
    onRequest("post", "/libraries", () => {
      api.libraries = [
        { id: mockId(9100), name: "Films", kind: "movies", pinned: true, workCount: 0, unmatchedCount: 0 },
      ];
    });
    const { router, user } = renderApp("/");

    // Any page shows setup until the admin user exists.
    expect(await screen.findByRole("heading", { name: "Create the admin account" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/setup");
    await user.type(screen.getByLabelText("Username"), "alice");
    await user.type(screen.getByLabelText(/^Password/), "a long password");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    // Libraries. Browse the server's folders and add one.
    expect(await screen.findByRole("heading", { name: "Add libraries" })).toBeInTheDocument();
    // The name follows the kind until the admin types one.
    expect(screen.getByLabelText("Name")).toHaveValue("Movies");
    await user.click(screen.getByRole("radio", { name: "Shows" }));
    expect(screen.getByLabelText("Name")).toHaveValue("Shows");
    await user.click(screen.getByRole("radio", { name: "Movies" }));
    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Films");
    await user.click(await screen.findByRole("button", { name: "mnt" }));
    await user.click(await screen.findByRole("button", { name: "user" }));
    await user.click(await screen.findByRole("button", { name: "Movies" }));
    await user.click(await screen.findByRole("button", { name: "Add this folder" }));
    expect(screen.getByRole("button", { name: "Remove /mnt/user/Movies" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add library" }));
    expect(await screen.findByText("Films")).toBeInTheDocument();
    expect(sent("POST", "/libraries").map((r) => r.body)).toMatchObject([
      { name: "Films", kind: "movies", folders: ["/mnt/user/Movies"] },
    ]);

    // Back returns to the user step, which then shows who is signed in.
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(await screen.findByText("Signed in as anna.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Next" }));
    await user.click(await screen.findByRole("button", { name: "Next" }));

    // Metadata defaults come from the browser locale.
    expect(await screen.findByRole("heading", { name: "Metadata" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("heading", { name: "Remote access" })).toBeInTheDocument();
    expect(sent("PATCH", "/settings").at(-1)?.body).toMatchObject({ contentRatingCountry: "US" });

    await user.click(screen.getByRole("button", { name: "Set up later" }));
    await user.click(await screen.findByRole("button", { name: "Go to Home" }));
    // Home shows what's there. The first scan's progress is in Admin.
    expect(await screen.findByRole("heading", { name: "Nothing here yet" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/");
  });

  it("is gone once the server is set up", async () => {
    serveMockApi();
    const { router } = renderApp("/setup");
    await screen.findByRole("heading", { name: "Continue Watching" });
    expect(locationOf(router)).toBe("/");
  });
});
