import { waitFor, within } from "@testing-library/react";
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
        {
          id: mockId(9100),
          name: "Films",
          kind: "movies",
          pinned: true,
          workCount: 0,
          unmatchedCount: 0,
          folders: [{ id: mockId(9101), path: "/mnt/user/Movies", reachable: true }],
        },
      ];
    });
    const { router, user } = renderApp("/");

    // Any page shows setup until the admin user exists.
    expect(await screen.findByRole("heading", { name: "Create the admin account" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/setup");
    await user.type(screen.getByLabelText("Username"), "alice");
    await user.type(screen.getByLabelText(/^Password/), "a long password");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    // Libraries. With none yet, going on is skipping, and Add library opens
    // the form in a dialog, so the step has no form to leave half filled.
    expect(await screen.findByRole("heading", { name: "Add libraries" })).toBeInTheDocument();
    expect(await screen.findByText("No libraries yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Skip for now" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Name")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add library" }));
    const dialog = await screen.findByRole("dialog", { name: "Add a library" });
    // The name follows the kind until the admin types one.
    expect(screen.getByLabelText("Name")).toHaveValue("Movies");
    await user.click(screen.getByRole("radio", { name: "Shows" }));
    expect(screen.getByLabelText("Name")).toHaveValue("Shows");
    await user.click(screen.getByRole("radio", { name: "Movies" }));
    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Films");
    await user.click(screen.getByRole("button", { name: "Add folder" }));
    await user.click(await screen.findByRole("button", { name: "mnt" }));
    await user.click(await screen.findByRole("button", { name: "user" }));
    await user.click(await screen.findByRole("button", { name: "Movies" }));
    await user.click(await screen.findByRole("button", { name: "Use this folder" }));
    expect(screen.getByRole("button", { name: "Remove /mnt/user/Movies" })).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Add library" }));
    // The dialog closes, and the step lists the library with its folder.
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByText("Films")).toBeInTheDocument();
    expect(screen.getByText("/mnt/user/Movies")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add another library" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Skip for now" })).not.toBeInTheDocument();
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
    expect(await screen.findByText(/^Scanning libraries\./)).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Go to Home" }));
    // Home shows what's there. The first scan's progress is in Admin.
    expect(await screen.findByRole("heading", { name: "Nothing here yet" })).toBeInTheDocument();
    expect(locationOf(router)).toBe("/");
  });

  it("says there's no library yet when the libraries step was skipped", async () => {
    const api = serveMockApi(scenarios.setup);
    onRequest("post", "/setup", () => {
      api.setup.required = false;
      api.signedIn = true;
    });
    const { user } = renderApp("/");
    await user.type(await screen.findByLabelText("Username"), "alice");
    await user.type(screen.getByLabelText(/^Password/), "a long password");
    await user.click(screen.getByRole("button", { name: "Create account" }));
    await user.click(await screen.findByRole("button", { name: "Skip for now" }));
    await user.click(await screen.findByRole("button", { name: "Next" }));
    await user.click(await screen.findByRole("button", { name: "Set up later" }));
    expect(await screen.findByText("No libraries yet. Add one from Home when you're ready.")).toBeInTheDocument();
    expect(screen.queryByText(/Scanning libraries/)).not.toBeInTheDocument();
  });

  it("is gone once the server is set up", async () => {
    serveMockApi();
    const { router } = renderApp("/setup");
    await screen.findByRole("heading", { name: "Continue Watching" });
    expect(locationOf(router)).toBe("/");
  });
});
