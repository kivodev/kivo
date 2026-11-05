import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { bookIds } from "@/api/mocks/books";
import { audio, miniPlayer } from "@/test/player";
import { renderApp, screen } from "@/test/render";
import { http, mockIds, problem, sent, serveMockApi, server } from "@/test/server";

const minute = 60_000;

describe("Audio player", () => {
  it("plays a book from its page in the mini player, which stays while browsing", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/works/${bookIds.leviathan}`);
    await user.click(await screen.findByRole("button", { name: "Play" }));
    await waitFor(() => expect(sent("POST", "/plays")).toHaveLength(1));
    expect(sent("POST", "/plays")[0]?.body).toMatchObject({ workId: bookIds.leviathan });
    expect(sent("POST", "/plays")[0]?.body).not.toHaveProperty("positionMs");

    await waitFor(() =>
      expect(within(miniPlayer() as HTMLElement).getByRole("button", { name: "Pause" })).toBeEnabled(),
    );
    const dock = miniPlayer() as HTMLElement;
    expect(dock).toHaveTextContent("Leviathan Wakes");
    expect(dock.querySelector('[data-part="mini-player-chapter"]')).toHaveTextContent("01 - Opening Credits");
    expect(audio().getAttribute("src")).toMatch(/files\/0$/);

    await router.navigate({ to: "/" });
    await screen.findByRole("heading", { name: "Continue Listening" });
    expect(miniPlayer()).toHaveTextContent("Leviathan Wakes");
    expect(sent("POST", "/plays/{id}/stop")).toHaveLength(0);
  });

  it("resumes with the time left, and starts at a chapter from the book's page", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.hailMary}`);
    expect(await screen.findByRole("button", { name: "Resume" })).toBeInTheDocument();
    expect(screen.getByText("8h 5m left")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Play from Chapter 2" }));
    await waitFor(() => expect(sent("POST", "/plays")).toHaveLength(1));
    // Thirty chapters are spread over 970 minutes, so the third starts at 64m 40s.
    expect(sent("POST", "/plays")[0]?.body).toMatchObject({ workId: bookIds.hailMary, positionMs: 3_880_000 });
  });

  it("moves by chapter across a book's files and sets the user's speed in Now Playing", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.leviathan}`);
    await user.click(await screen.findByRole("button", { name: "Play" }));
    await waitFor(() =>
      expect(within(miniPlayer() as HTMLElement).getByRole("button", { name: "Pause" })).toBeEnabled(),
    );
    await user.click(screen.getByRole("button", { name: "Now Playing: Leviathan Wakes" }));

    const sheet = await screen.findByRole("dialog", { name: "Now Playing" });
    expect(within(sheet).getByText("Narrated by Jefferson Mays")).toBeInTheDocument();
    expect(within(sheet).getByText("Chapter 1 of 3")).toBeInTheDocument();
    // The seek bar spans the chapter playing, which is the two-minute opening credits.
    expect(within(sheet).getByRole("slider", { name: "Seek in the chapter" })).toHaveAttribute(
      "max",
      String(2 * minute),
    );

    // Next is the second file's start.
    await user.click(within(sheet).getByRole("button", { name: "Next chapter" }));
    await waitFor(() => expect(audio().getAttribute("src")).toMatch(/files\/1$/));
    await waitFor(() => expect(within(sheet).getByText("Chapter 2 of 3")).toBeInTheDocument());

    // A chapter from the list.
    await user.click(within(sheet).getByRole("button", { name: /03 - Chapter 2/ }));
    await waitFor(() => expect(audio().getAttribute("src")).toMatch(/files\/2$/));

    await user.click(within(sheet).getByRole("button", { name: /Speed/ }));
    await user.click(await screen.findByRole("option", { name: "1.5×" }));
    await waitFor(() => expect(sent("PATCH", "/users/me")).toHaveLength(1));
    expect(sent("PATCH", "/users/me")[0]?.body).toEqual({ playback: { bookSpeed: 1.5 } });
    expect(audio().playbackRate).toBe(1.5);
    expect(audio().defaultPlaybackRate).toBe(1.5);
  });

  it("says why a book can't play here", async () => {
    serveMockApi();
    server.use(http.post("/api/v1/plays", () => problem(409, "notPlayable", { reason: "fileUnplayable" })));
    const { user } = renderApp(`/works/${bookIds.churn}`);
    await user.click(await screen.findByRole("button", { name: "Play" }));
    expect(await within(miniPlayer() as HTMLElement).findByRole("alert")).toHaveTextContent(/can.t play the file/i);
  });

  it("stops the book, keeping its place, when a video plays", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/works/${bookIds.leviathan}`);
    await user.click(await screen.findByRole("button", { name: "Play" }));
    await waitFor(() =>
      expect(within(miniPlayer() as HTMLElement).getByRole("button", { name: "Pause" })).toBeEnabled(),
    );
    await router.navigate({ to: "/watch/$workId", params: { workId: mockIds.dunePartTwo } });
    await waitFor(() => expect(miniPlayer()).toBeNull());
    await waitFor(() => expect(sent("POST", "/plays/{id}/stop")).toHaveLength(1));
  });
});
