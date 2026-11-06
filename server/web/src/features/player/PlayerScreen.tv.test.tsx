import { fireEvent, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { playTo } from "@/test/media";
import { heartbeats, minute, playing, plays, resumeMs, stops, video } from "@/test/player";
import { locationOf, renderApp, screen } from "@/test/render";
import { mockIds, serveMockApi } from "@/test/server";

describe("Player on a TV remote", () => {
  it("starts a failure on Try again and moves along its recovery actions", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    Object.defineProperty(video(), "error", { configurable: true, value: { code: 3, message: "decode" } });
    video().dispatchEvent(new Event("error"));
    const alert = await screen.findByRole("alert");
    const action = (name: string) => within(alert).getByRole("button", { name }).closest("[data-tv-focusable]");
    await waitFor(() => expect(action("Try again")).toHaveFocus(), { timeout: 1000 });
    await user.keyboard("{ArrowRight}");
    expect(action("Play another version")).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(action("Back")).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(action("View diagnostics")).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(within(alert).getByText("no engine plays stream here")).toBeInTheDocument();
    await user.keyboard("{ArrowLeft}{ArrowLeft}{ArrowLeft}");
    expect(action("Try again")).toHaveFocus();
    await user.keyboard("{Enter}");
    await playing();
    await waitFor(() => expect(screen.getByRole("button", { name: "Pause" })).toHaveFocus());
  });

  it("leaves a failure with Back and restores navigation on the detail screen", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    Object.defineProperty(video(), "error", { configurable: true, value: { code: 3, message: "decode" } });
    video().dispatchEvent(new Event("error"));
    await screen.findByRole("alert");
    await user.keyboard("{GoBack}");
    await waitFor(() => expect(locationOf(router)).toBe(`/works/${mockIds.dunePartTwo}`), { timeout: 1000 });
    await waitFor(() => expect(document.activeElement).toHaveAttribute("data-tv-focus-key", "play"));
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).not.toHaveAttribute("data-tv-focus-key", "play");
  });

  const setVisibility = (state: DocumentVisibilityState) => {
    vi.spyOn(document, "visibilityState", "get").mockReturnValue(state);
    document.dispatchEvent(new Event("visibilitychange"));
  };

  it("stops at its position when the TV hides the app, and plays on from there on return", async () => {
    serveMockApi();
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    playTo(video(), 5000);
    setVisibility("hidden");
    await waitFor(() => expect(stops()).toMatchObject([{ positionMs: 5_000_000 }]));
    expect(video().hasAttribute("src")).toBe(false);

    setVisibility("visible");
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(plays()[1]).toMatchObject({ workId: mockIds.dunePartTwo, positionMs: 5_000_000 });
    await playing();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("comes back paused to a video paused before the TV hid the app", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(screen.getByRole("button", { name: "Pause" }));
    setVisibility("hidden");
    await waitFor(() => expect(stops()).toHaveLength(1));
    setVisibility("visible");
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(await screen.findByRole("button", { name: "Play" })).toBeInTheDocument();
    expect(video().paused).toBe(true);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  const seekBar = () => screen.getByRole("slider", { name: "Seek" });
  const pause = () => screen.getByRole("button", { name: "Pause" });
  const preview = () => document.querySelector('[data-part="scrub-preview"]');
  const controls = () => document.querySelector('[data-part="player"]')?.getAttribute("data-controls");

  it("scrubs with Left and Right from hidden controls, seeking to the target a second after the last press", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    expect(screen.queryByRole("slider", { name: "Volume" })).not.toBeInTheDocument();
    await waitFor(() => expect(pause()).toHaveFocus());
    await user.keyboard("{GoBack}");
    expect(controls()).toBe("hidden");

    await user.keyboard("{ArrowRight}{ArrowRight}{ArrowLeft}{ArrowRight}");
    expect(controls()).toBe("shown");
    expect(seekBar()).toHaveFocus();
    expect(preview()).toHaveTextContent("1:12:23");
    expect(heartbeats().some((h) => h.positionMs === resumeMs + 20_000)).toBe(false);
    await waitFor(() => expect(heartbeats().at(-1)?.positionMs).toBe(resumeMs + 20_000), { timeout: 2000 });
    expect(preview()).toBeNull();
  });

  it("scrubs further as Right is held", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await waitFor(() => expect(pause()).toHaveFocus());
    await user.keyboard("{ArrowUp}");
    expect(seekBar()).toHaveFocus();
    fireEvent.keyDown(seekBar(), { key: "ArrowRight" });
    for (let i = 0; i < 5; i++) fireEvent.keyDown(seekBar(), { key: "ArrowRight", repeat: true });
    // Five steps of 10 seconds, then one of 30.
    expect(preview()).toHaveTextContent("1:13:23");
  });

  it("seeks at once on OK, plays or pauses on OK with no target, and Back drops a target", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await waitFor(() => expect(pause()).toHaveFocus());
    await user.keyboard("{ArrowUp}");
    expect(seekBar()).toHaveFocus();
    await user.keyboard("{ArrowRight}{Enter}");
    await waitFor(() => expect(heartbeats().at(-1)?.positionMs).toBe(resumeMs + 10_000));
    expect(preview()).toBeNull();

    await user.keyboard("{Enter}");
    expect(await screen.findByRole("button", { name: "Play" })).toBeInTheDocument();

    await user.keyboard("{ArrowRight}{GoBack}");
    expect(controls()).toBe("hidden");
    expect(preview()).toBeNull();
    await new Promise((resolve) => setTimeout(resolve, 1200));
    expect(heartbeats().some((h) => h.positionMs === resumeMs + 20_000)).toBe(false);
  });

  it("starts on play or pause, goes up to the scrub bar and back down, and along the buttons, with nothing above the bar", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await waitFor(() => expect(pause()).toHaveFocus());
    await user.keyboard("{ArrowUp}");
    expect(seekBar()).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("button", { name: "Pause" })).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: "Forward 30 seconds" })).toHaveFocus();
    await user.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(screen.getByRole("button", { name: "Back 10 seconds" })).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(seekBar()).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(seekBar()).toHaveFocus();
    expect(screen.queryByRole("button", { name: "Back" })).not.toBeInTheDocument();
  });

  it("opens Playback Settings' lists in its place, under a row of the list's name that goes back, as Left and Back do", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await waitFor(() => expect(pause()).toHaveFocus());
    await user.keyboard("{ArrowRight}{ArrowRight}{ArrowRight}{Enter}");
    const settings = await screen.findByRole("menu", { name: "Playback Settings" });
    await waitFor(() => expect(within(settings).getByRole("menuitem", { name: "Audio & Subtitles" })).toHaveFocus());
    await user.keyboard("{ArrowDown}{Enter}");
    // One menu, the list in Playback Settings' place, on its choice.
    const quality = await screen.findByRole("menu", { name: "Quality" });
    expect(screen.getAllByRole("menu")).toHaveLength(1);
    await waitFor(() => expect(within(quality).getByRole("menuitemradio", { name: /^Auto/ })).toHaveFocus());
    await user.keyboard("{ArrowLeft}");
    const back = await screen.findByRole("menu", { name: "Playback Settings" });
    await waitFor(() => expect(within(back).getByRole("menuitem", { name: /^Quality/ })).toHaveFocus());

    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowRight}");
    const speed = await screen.findByRole("menu", { name: "Speed" });
    await waitFor(() => expect(within(speed).getByRole("menuitemradio", { name: "Normal" })).toHaveFocus());
    await user.keyboard("{GoBack}");
    await waitFor(() =>
      expect(
        within(screen.getByRole("menu", { name: "Playback Settings" })).getByRole("menuitem", { name: /^Speed/ }),
      ).toHaveFocus(),
    );
    // OK on the list's first row goes back too.
    await user.keyboard("{ArrowUp}{Enter}");
    const version = await screen.findByRole("menu", { name: "Version" });
    const chosen = within(version).getByRole("menuitemradio", { checked: true });
    await waitFor(() => expect(chosen).toHaveFocus());
    const above = within(version).getAllByRole("menuitemradio").indexOf(chosen) + 1;
    await user.keyboard(`${"{ArrowUp}".repeat(above)}{Enter}`);
    await waitFor(() =>
      expect(
        within(screen.getByRole("menu", { name: "Playback Settings" })).getByRole("menuitem", { name: /^Version/ }),
      ).toHaveFocus(),
    );

    // Choosing from a list applies it and closes the menu.
    await user.keyboard("{ArrowDown}{Enter}");
    await screen.findByRole("menu", { name: "Speed" });
    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");
    expect(video().playbackRate).toBe(1.5);
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  }, 15_000);

  it("moves between Up next's buttons over hidden controls", async () => {
    serveMockApi((api) => {
      const play = api.starts[mockIds.expanseReload];
      if (play) play.startMs = 43 * minute + 30_000;
    });
    const { user } = renderApp(`/watch/${mockIds.expanseReload}`);
    const card = await screen.findByRole("region", { name: "Up next" });
    await user.keyboard("{GoBack}");
    expect(controls()).toBe("hidden");
    expect(within(card).getByRole("button", { name: "Play now" })).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(within(card).getByRole("button", { name: "Hide" })).toHaveFocus();
    expect(controls()).toBe("hidden");
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("button", { name: "Skip Credits" })).toHaveFocus();
    expect(controls()).toBe("hidden");
    await user.keyboard("{ArrowUp}");
    expect(controls()).toBe("shown");
    expect(pause()).toHaveFocus();
  });
});
