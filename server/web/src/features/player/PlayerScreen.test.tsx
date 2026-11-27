import { fireEvent, waitFor, within } from "@testing-library/react";
import { HttpResponse } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { liveQueries } from "@/api/live";
import type { MockApi } from "@/api/mocks/api";
import type { PlayStarted, PlayTrack, Track } from "@/api/schemas";
import { formatEndTime } from "@/i18n/format";
import { playTo, playToEnd } from "@/test/media";
import { heartbeats, minute, playing, plays, resumeMs, stops, video } from "@/test/player";
import { locationOf, renderApp, screen } from "@/test/render";
import { http, mockIds, onRequest, problem, scenarios, sent, serveMockApi, server } from "@/test/server";

// The ASS and PGS renderers need workers, which jsdom lacks.
vi.mock("./renderers", () => ({ drawAss: () => () => {}, drawPgs: () => () => {} }));

/** Puts the video in picture-in-picture, or takes it out, as the browser says and tells it. */
function pictureInPicture(on: boolean) {
  Object.defineProperty(document, "pictureInPictureElement", { configurable: true, value: on ? video() : null });
  video().dispatchEvent(new Event(on ? "enterpictureinpicture" : "leavepictureinpicture"));
}

/** How long Dune: Part Two plays: the time left at the resume position is −1:33:57. */
const dunePartTwoMs = resumeMs + 93 * minute + 57_000;

/** The play that Dune: Part Two starts with. It plays the 1080p version directly. */
const dunePlay = (api: MockApi) => api.starts[mockIds.dunePartTwo] as PlayStarted;

/** An embedded subtitle track, named as the server names it. */
function embedded(id: string, codec: string, language: string, name: string): Track {
  return {
    id,
    type: "subtitle",
    codec,
    language,
    default: false,
    name,
    fullName: name,
    subtitle: { source: "embedded" },
  };
}

/** Gives Dune's 1080p version these tracks too. Its play lists them, each taking a new play. */
function withTracks(api: MockApi, tracks: Track[]) {
  const version = api.works[mockIds.dunePartTwo]?.versions[1];
  if (version) version.tracks = [...version.tracks, ...tracks];
  deliver(api, ...tracks.map((t): PlayTrack => ({ trackId: t.id, type: t.type })));
}

/** Sets how the play shows these choices, in place of what it said of them. */
function deliver(api: MockApi, ...tracks: PlayTrack[]) {
  const play = dunePlay(api);
  const ids = new Set(tracks.map((t) => t.trackId));
  play.tracks = [...play.tracks.filter((t) => !ids.has(t.trackId)), ...tracks];
}

/** Answers the next plays with this subtitle track selected and shown as `delivery`, as the server would for a play that asks for it. */
function nextPlaysShow(api: MockApi, trackId: string, delivery: "stream" | "burn") {
  const play = dunePlay(api);
  play.tracks = play.tracks.map((t) => (t.type === "subtitle" ? { trackId: t.trackId, type: t.type } : t));
  // While a subtitle is burned in, every other subtitle choice takes a new play.
  const off: PlayTrack =
    delivery === "burn" ? { trackId: "off", type: "subtitle" } : { trackId: "off", type: "subtitle", delivery: "none" };
  deliver(
    api,
    { trackId, type: "subtitle", selected: true, delivery, codec: delivery === "stream" ? "webvtt" : undefined },
    off,
  );
}

/** A subtitle the play shows from its own WebVTT file. */
const external = (trackId: string, selected?: true): PlayTrack => ({
  trackId,
  type: "subtitle",
  selected,
  delivery: "external",
  codec: "webvtt",
  url: `plays/play/subtitles/${trackId}.vtt`,
});

type User = ReturnType<typeof renderApp>["user"];

/** Opens Audio & Subtitles from the speech bubble, and returns its menu. */
async function tracksMenu(user: User) {
  await user.click(screen.getByRole("button", { name: "Audio & Subtitles" }));
  return screen.findByRole("menu", { name: "Audio & Subtitles" });
}

/** Opens Playback Settings from the gear, and returns its menu. */
async function settingsMenu(user: User) {
  await user.click(screen.getByRole("button", { name: "Playback Settings" }));
  return screen.findByRole("menu", { name: "Playback Settings" });
}

/** Opens one of Playback Settings' submenus, and returns it. */
async function settingsSubmenu(user: User, name: "Quality" | "Version" | "Speed") {
  await user.click(within(await settingsMenu(user)).getByRole("menuitem", { name: new RegExp(`^${name}`) }));
  return screen.findByRole("menu", { name: new RegExp(`^${name}`) });
}

/** Opens the stream info from Playback Settings. */
async function openStreamInfo(user: User) {
  await user.click(within(await settingsMenu(user)).getByRole("menuitem", { name: "Stream info" }));
}

describe("Player", () => {
  it("resumes a file at the play's position, seeking once its metadata is in", async () => {
    serveMockApi();
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    expect(plays()).toMatchObject([{ workId: mockIds.dunePartTwo }]);
    expect(video().getAttribute("src")).toMatch(/\/api\/v1\/plays\/[\w-]+\/files\/0$/);
    expect(video().currentTime).toBe(resumeMs / 1000);
    expect(screen.getByRole("slider", { name: "Seek" })).toBeInTheDocument();
    expect(screen.getByText("1:12:03")).toBeInTheDocument();
    expect(screen.getByText("−1:33:57")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Dune: Part Two" })).toBeInTheDocument();
  });

  it("starts at this browser's local or remote quality until another is picked, with its subtitle look", async () => {
    const api = serveMockApi();
    // The server applies the local quality to this local play.
    Object.assign(dunePlay(api), { quality: { mode: "limit", maxBitrateBps: 2_000_000 } });
    localStorage.setItem("kivo.quality.local", "2000000");
    localStorage.setItem("kivo.quality.remote", "1000000");
    localStorage.setItem("kivo.subtitleSize", "large");
    localStorage.setItem("kivo.subtitleBackground", "box");
    try {
      const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
      await playing();
      expect(plays()[0]?.settings).toMatchObject({
        localQuality: { mode: "limit", maxBitrateBps: 2_000_000 },
        remoteQuality: { mode: "limit", maxBitrateBps: 1_000_000 },
      });
      const player = document.querySelector('[data-part="player"]');
      expect(player).toHaveAttribute("data-subtitle-size", "large");
      expect(player).toHaveAttribute("data-subtitle-background", "box");

      await settingsSubmenu(user, "Quality");
      expect(await screen.findByRole("menuitemradio", { name: "480p · 2 Mb/s" })).toBeChecked();
      await user.click(screen.getByRole("menuitemradio", { name: /^Auto/ }));
      await waitFor(() => expect(plays()).toHaveLength(2));
      expect(plays()[1]?.settings).toMatchObject({ localQuality: { mode: "auto" }, remoteQuality: { mode: "auto" } });
    } finally {
      localStorage.clear();
    }
  });

  it("offers the qualities over the one picked", async () => {
    const api = serveMockApi();
    // A play at a picked 2 Mb/s. The play's cap is the pick, not the server's.
    Object.assign(dunePlay(api), { quality: { mode: "limit", maxBitrateBps: 2_000_000 } });
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await settingsSubmenu(user, "Quality");
    expect(await screen.findByRole("menuitemradio", { name: /^Original/ })).not.toHaveAttribute("aria-disabled");
    expect(screen.getByRole("menuitemradio", { name: "1080p · 8 Mb/s" })).not.toHaveAttribute("aria-disabled");
  });

  it("offers no quality over the server's limit", async () => {
    const api = serveMockApi();
    Object.assign(dunePlay(api), {
      quality: { mode: "limit", maxBitrateBps: 2_000_000 },
      limitBps: 4_000_000,
    });
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await settingsSubmenu(user, "Quality");
    expect(await screen.findByRole("menuitemradio", { name: /^Original/ })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("menuitemradio", { name: /^1080p · 8 Mb\/s/ })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("menuitemradio", { name: "720p · 4 Mb/s" })).not.toHaveAttribute("aria-disabled");
  });

  it("starts Auto with the bandwidth this browser measured within the hour", async () => {
    serveMockApi();
    localStorage.setItem("kivo.bandwidth", JSON.stringify({ bps: 20_000_000, at: Date.now() - 59 * 60_000 }));
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    expect(plays()[0]?.settings).toMatchObject({
      localQuality: { mode: "auto", bandwidthBps: 20_000_000 },
      remoteQuality: { mode: "auto", bandwidthBps: 20_000_000 },
    });
    expect(sent("GET", "/bandwidth-test")).toHaveLength(0);
  });

  it("measures the bandwidth once before a first remote play, and keeps it", async () => {
    serveMockApi();
    server.use(
      http.untyped.get(
        "*/api/v1/bandwidth-test",
        () => new HttpResponse(new Uint8Array(100_000), { headers: { "Content-Type": "application/octet-stream" } }),
      ),
    );
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    const tests = sent("GET", "/bandwidth-test");
    expect(tests).toHaveLength(1);
    expect(tests[0]?.url.searchParams.get("size")).toBe("5000000");
    const measured = plays()[0]?.settings?.localQuality?.bandwidthBps;
    expect(measured).toBeGreaterThan(0);
    expect(plays()[0]?.settings?.remoteQuality?.bandwidthBps).toBe(measured);
    expect(JSON.parse(localStorage.getItem("kivo.bandwidth") ?? "null")?.bps).toBe(measured);
  });

  it("measures again once the last measurement is an hour old", async () => {
    serveMockApi();
    localStorage.setItem("kivo.bandwidth", JSON.stringify({ bps: 20_000_000, at: Date.now() - 61 * 60_000 }));
    server.use(
      http.untyped.get(
        "*/api/v1/bandwidth-test",
        () => new HttpResponse(new Uint8Array(100_000), { headers: { "Content-Type": "application/octet-stream" } }),
      ),
    );
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    expect(sent("GET", "/bandwidth-test")).toHaveLength(1);
    expect(plays()[0]?.settings?.localQuality?.bandwidthBps).not.toBe(20_000_000);
  });

  it("starts Auto without a bandwidth on the local network", async () => {
    serveMockApi();
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    expect(sent("GET", "/bandwidth-test")).toHaveLength(1);
    expect(plays()[0]?.settings).toMatchObject({ localQuality: { mode: "auto" }, remoteQuality: { mode: "auto" } });
    expect(plays()[0]?.settings?.localQuality?.bandwidthBps).toBeUndefined();
    expect(localStorage.getItem("kivo.bandwidth")).toBeNull();
  });

  it("starts over when asked to", async () => {
    const api = serveMockApi();
    dunePlay(api).startMs = 0;
    renderApp(`/watch/${mockIds.dunePartTwo}?from=start`);
    await playing();
    expect(plays()[0]?.positionMs).toBe(0);
    expect(screen.getByText("0:00")).toBeInTheDocument();
    expect(screen.getByText("−2:46:00")).toBeInTheDocument();
  });

  it("changes the volume a tenth at a time with Up and Down", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.keyboard("{ArrowDown}{ArrowDown}");
    expect(video().volume).toBe(0.8);
    await user.keyboard("{ArrowUp}");
    expect(video().volume).toBe(0.9);
  });

  it("seeks with the keys, reporting the seek in a heartbeat", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.keyboard("{ArrowRight}");
    await waitFor(() => expect(heartbeats().at(-1)?.seekToFirstFrameMs).toBeTypeOf("number"));
    expect(heartbeats().at(-1)?.positionMs).toBe(resumeMs + 30_000);
    await user.keyboard("{ArrowLeft}");
    await waitFor(() => expect(heartbeats().at(-1)?.positionMs).toBe(resumeMs + 20_000));
  });

  it("heartbeats on pause, and stops the play at its position on Back", async () => {
    serveMockApi();
    const { router, user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(await screen.findByRole("link", { name: "Resume 1:12:03" }));
    await playing();
    playTo(video(), 5000);
    await user.click(screen.getByRole("button", { name: "Pause" }));
    await waitFor(() => expect(heartbeats().at(-1)).toMatchObject({ state: "paused", positionMs: 5_000_000 }));

    await user.click(screen.getByRole("button", { name: "Back" }));
    await waitFor(() => expect(locationOf(router)).toBe(`/works/${mockIds.dunePartTwo}`));
    await waitFor(() => expect(stops()).toMatchObject([{ positionMs: 5_000_000 }]));
  });

  it("keeps playing in a hidden browser tab", async () => {
    serveMockApi();
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    await new Promise((r) => setTimeout(r, 50));
    expect(stops()).toHaveLength(0);
    expect(video().hasAttribute("src")).toBe(true);
  });

  it("stops with a request that outlives the page when the tab closes", async () => {
    serveMockApi();
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    window.dispatchEvent(new Event("pagehide"));
    await waitFor(() => expect(stops()).toHaveLength(1));
    expect(video().hasAttribute("src")).toBe(false);
  });

  it("retries twice as a transcode after a decode error, and offers recovery when those fail too", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    Object.defineProperty(video(), "error", { configurable: true, value: { code: 3, message: "decode" } });
    video().dispatchEvent(new Event("error"));
    // The retries are HLS, which nothing plays here: the third failure shows
    // the error without asking again.
    const alert = await screen.findByRole("alert");
    expect(sent("POST", "/plays/{id}/fallback").map((r) => r.body)).toEqual([
      { code: "mediaDecodeError", positionMs: resumeMs, detail: expect.any(String) },
      { code: "unsupported", positionMs: resumeMs, detail: expect.any(String) },
    ]);
    // The stop says what failed, so the server logs the failure that ended playback.
    await waitFor(() => expect(stops()).toHaveLength(1));
    expect(stops()[0]).toEqual({ positionMs: resumeMs, code: expect.any(String), detail: expect.any(String) });
    expect(within(alert).getByRole("heading")).toHaveTextContent("Couldn't play this.");
    expect(within(alert).getByText("Can't play on this device, even with transcoding.")).toBeInTheDocument();
    expect(within(alert).getByRole("button", { name: "Play another version" })).toBeInTheDocument();

    await user.click(within(alert).getByRole("button", { name: "View diagnostics" }));
    expect(within(alert).getByText("no engine plays stream here")).toBeInTheDocument();
    expect(within(alert).getByText("noFallback")).toBeInTheDocument();
    expect(sent("GET", "/plays/{id}")).toHaveLength(0);

    // Trying again is a new play, which may fall back again.
    await user.click(within(alert).getByRole("button", { name: "Try again" }));
    await playing();
    expect(plays()).toHaveLength(2);
  });

  it("shows the error when the server has nothing left to fall back to", async () => {
    serveMockApi();
    server.use(http.post("/api/v1/plays/{id}/fallback", () => problem(409, "noFallback")));
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    Object.defineProperty(video(), "error", { configurable: true, value: { code: 3, message: "decode" } });
    video().dispatchEvent(new Event("error"));
    const alert = await screen.findByRole("alert");
    expect(sent("POST", "/plays/{id}/fallback")).toHaveLength(1);
    expect(within(alert).getByText("Can't play on this device, even with transcoding.")).toBeInTheDocument();
  });

  it("keeps web error actions keyboard-accessible and Escape on the player", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    Object.defineProperty(video(), "error", { configurable: true, value: { code: 3, message: "decode" } });
    video().dispatchEvent(new Event("error"));
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByRole("button", { name: "Try again" })).toHaveFocus();
    expect(alert.querySelector("[data-tv-focusable]")).toBeNull();
    await user.keyboard("{Escape}");
    expect(locationOf(router)).toBe(`/watch/${mockIds.dunePartTwo}`);
    expect(alert).toBeInTheDocument();
    await user.tab();
    expect(within(alert).getByRole("button", { name: "Play another version" })).toHaveFocus();
    await user.click(within(alert).getByRole("button", { name: "Back" }));
    await waitFor(() => expect(locationOf(router)).toBe(`/works/${mockIds.dunePartTwo}`));
  });

  it("reads a failed transcode's diagnostics for admins only", async () => {
    serveMockApi(scenarios.viewer);
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    Object.defineProperty(video(), "error", { configurable: true, value: { code: 3, message: "decode" } });
    video().dispatchEvent(new Event("error"));
    const alert = await screen.findByRole("alert");
    await waitFor(() => expect(stops()).toHaveLength(1));
    expect(within(alert).getByText("Can't play on this device, even with transcoding.")).toBeInTheDocument();
    expect(sent("GET", "/plays/{id}")).toEqual([]);
  });

  it("shows a refusal with its reason, and hides diagnostics from viewers", async () => {
    serveMockApi(scenarios.viewer);
    server.use(http.post("/api/v1/plays", () => problem(409, "notPlayable", { reason: "transcodeFailed" })));
    const { user, router } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("The server couldn't convert this video.")).toBeInTheDocument();
    expect(within(alert).queryByRole("button", { name: "View diagnostics" })).not.toBeInTheDocument();
    await user.click(within(alert).getByRole("button", { name: "Play another version" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ version: expect.any(String) }));
  });

  it("stops when the play is ended elsewhere", async () => {
    const api = serveMockApi();
    const { queryClient } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    queryClient.setQueryData(liveQueries.play(dunePlay(api).id).queryKey, { ended: true });
    expect(await screen.findByText("Playback was stopped.")).toBeInTheDocument();
  });

  it("shows the message of an admin who stopped it", async () => {
    const api = serveMockApi();
    const { queryClient } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    queryClient.setQueryData(liveQueries.play(dunePlay(api).id).queryKey, {
      ended: true,
      message: "Server maintenance",
    });
    expect(await screen.findByText("An admin stopped it: “Server maintenance”")).toBeInTheDocument();
  });

  it("plays on when the play a switch replaced ends", async () => {
    const api = serveMockApi();
    let started = 0;
    server.use(
      http.post("/api/v1/plays", () =>
        HttpResponse.json({ ...dunePlay(api), id: `play-${++started}` }, { status: 201 }),
      ),
    );
    const { user, queryClient } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(within(await tracksMenu(user)).getByRole("menuitemcheckbox", { name: /^Dialogue boost/ }));
    // The switch stops the first play, and the server says it ended.
    queryClient.setQueryData(liveQueries.play("play-1").queryKey, { ended: true });
    await waitFor(() => expect(started).toBe(2));
    expect(await screen.findByRole("menu", { name: "Audio & Subtitles" })).toBeInTheDocument();
    expect(screen.queryByText("Playback was stopped.")).not.toBeInTheDocument();
  }, 15_000);

  it("lists audio and subtitles by name, the file's title under it, the playing track ticked", async () => {
    const api = serveMockApi();
    withTracks(api, [
      {
        id: "audio-commentary",
        type: "audio",
        codec: "ac3",
        language: "eng",
        title: "Commentary by director Ron Underwood",
        commentary: true,
        default: false,
        name: "English · Commentary · 2.0",
        fullName: "English · AC-3 2.0 · “Commentary by director Ron Underwood”",
        audio: { channels: 2, layout: "2.0" },
      },
      embedded("t3", "subrip", "chi", "Chinese"),
    ]);
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    const menu = await tracksMenu(user);
    expect(within(menu).getByRole("menuitemradio", { name: "English · 2.0" })).toBeChecked();
    const commentary = within(menu).getByRole("menuitemradio", { name: /^English · Commentary/ });
    expect(commentary).not.toBeChecked();
    expect(commentary).toHaveTextContent("Commentary by director Ron Underwood");
    expect(within(menu).getByRole("menuitemradio", { name: "Chinese" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitemradio", { name: "Off" })).toBeChecked();
  });

  it("switches audio by starting a new play where playback was", async () => {
    const api = serveMockApi();
    withTracks(api, [
      {
        id: "audio-fr",
        type: "audio",
        codec: "aac",
        language: "fr",
        default: false,
        name: "French · 2.0",
        fullName: "French · AAC 2.0",
        audio: { channels: 2, layout: "2.0" },
      },
    ]);
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await tracksMenu(user);
    await user.click(await screen.findByRole("menuitemradio", { name: /French/ }));
    await waitFor(() => expect(plays()).toHaveLength(2));
    // The new play stays on the play's version, because the track belongs to it.
    expect(plays()[1]).toMatchObject({
      audioTrackId: "audio-fr",
      versionId: dunePlay(api).versionId,
      positionMs: resumeMs,
    });
    await waitFor(() => expect(stops()).toEqual([{ positionMs: resumeMs }]));
  });

  it("plays another audio track of the file without a new play when the play says it can, and reports it", async () => {
    const api = serveMockApi();
    withTracks(api, [
      {
        id: "audio-fr",
        type: "audio",
        codec: "aac",
        language: "fr",
        default: false,
        name: "French · 2.0",
        fullName: "French · AAC 2.0",
        audio: { channels: 2, layout: "2.0" },
      },
    ]);
    deliver(api, { trackId: "audio-fr", type: "audio", delivery: "file", codec: "aac", index: 2 });
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await tracksMenu(user);
    await user.click(await screen.findByRole("menuitemradio", { name: /French/ }));
    await waitFor(() => expect(heartbeats().at(-1)).toMatchObject({ audioTrackId: "audio-fr" }));
    expect(plays()).toHaveLength(1);
    expect(within(await tracksMenu(user)).getByRole("menuitemradio", { name: /French/ })).toBeChecked();
  });

  it("starts a new play at the position for a subtitle choice without a delivery", async () => {
    const api = serveMockApi();
    withTracks(api, [embedded("t3", "subrip", "en", "English")]);
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    nextPlaysShow(api, "t3", "stream");
    await tracksMenu(user);
    await user.click(
      within(await screen.findByRole("menu", { name: "Audio & Subtitles" })).getByRole("menuitemradio", {
        name: /^English/,
      }),
    );
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(plays()[1]).toMatchObject({ subtitleTrackId: "t3", positionMs: resumeMs });
    await waitFor(() => expect(stops()).toEqual([{ positionMs: resumeMs }]));
    expect(sent("GET", "/plays/{id}/subtitles/{name}")).toEqual([]);
  }, 15_000);

  it("burns in a subtitle through a new play, and takes another new play to turn it off", async () => {
    const api = serveMockApi();
    withTracks(api, [embedded("t3", "dvd_subtitle", "en", "English")]);
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    nextPlaysShow(api, "t3", "burn");
    await tracksMenu(user);
    const dvd = await within(await screen.findByRole("menu", { name: "Audio & Subtitles" })).findByRole(
      "menuitemradio",
      {
        name: /^English/,
      },
    );
    await user.click(dvd);
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(plays()[1]).toMatchObject({ subtitleTrackId: "t3", positionMs: resumeMs });
    await playing();

    // While it's burned in, off has no delivery either.
    await user.click(within(await tracksMenu(user)).getByRole("menuitemradio", { name: "Off" }));
    await waitFor(() => expect(plays()).toHaveLength(3));
    expect(plays()[2]).toMatchObject({ subtitleTrackId: "off" });
  }, 15_000);

  it("lets a viewer search a video without subtitles, and plays the subtitle it downloads", async () => {
    const api = serveMockApi(scenarios.viewer);
    const downloaded = api.addedSubtitle.id;
    onRequest("post", "/files/{id}/subtitles", () => {
      const play = dunePlay(api);
      play.tracks = play.tracks.map((t) =>
        t.trackId === "off" ? { trackId: "off", type: "subtitle", delivery: "none" } : t,
      );
      deliver(api, external(downloaded, true));
    });
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await tracksMenu(user);
    await user.click(await screen.findByRole("menuitemradio", { name: "Search subtitles…" }));
    const dialog = await screen.findByRole("dialog", { name: "Search subtitles" });
    expect(await within(dialog).findByText("English · Exact match")).toBeInTheDocument();
    await user.click(within(dialog).getAllByRole("button", { name: "Download" })[0] as HTMLElement);
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(sent("POST", "/files/{id}/subtitles")[0]?.body).toEqual({ resultId: "os-1" });
    expect(plays()[1]?.subtitleTrackId).toBe(downloaded);
    await waitFor(() =>
      expect(video().querySelector("track")?.getAttribute("src")).toMatch(`/subtitles/${downloaded}.vtt`),
    );
  });

  it("words a failed download from the server's facts", async () => {
    serveMockApi(scenarios.viewer);
    server.use(
      http.post("/api/v1/files/{id}/subtitles", () => problem(503, "quotaUsed", { resetsAt: "2099-01-01T16:00:00Z" })),
    );
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await tracksMenu(user);
    await user.click(await screen.findByRole("menuitemradio", { name: "Search subtitles…" }));
    const dialog = await screen.findByRole("dialog", { name: "Search subtitles" });
    await user.click((await within(dialog).findAllByRole("button", { name: "Download" }))[0] as HTMLElement);
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      /^The server's subtitle downloads are used up\. More at \d{1,2}:\d{2}/,
    );
    expect(plays()).toHaveLength(1);
  });

  it("plays and pauses on a click, goes full screen on a double-click, and a tap hides the controls", async () => {
    serveMockApi();
    const requestFullscreen = vi.fn(() => Promise.resolve());
    document.documentElement.requestFullscreen = requestFullscreen;
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(video());
    expect(await screen.findByRole("button", { name: "Play" })).toBeInTheDocument();
    await user.click(video());
    await playing();

    // A double-click's two clicks leave play as it was.
    await user.dblClick(video());
    expect(requestFullscreen).toHaveBeenCalledTimes(1);
    await playing();

    const player = document.querySelector('[data-part="player"]') as HTMLElement;
    expect(player.dataset.controls).toBe("shown");
    await user.pointer({ keys: "[TouchA]", target: video() });
    expect(player.dataset.controls).toBe("hidden");
    await user.pointer({ keys: "[TouchA]", target: video() });
    expect(player.dataset.controls).toBe("shown");
    await playing();
  }, 15_000);

  it("puts the video full screen where the page can't go full screen, as on the iPhone", async () => {
    serveMockApi();
    Reflect.deleteProperty(document.documentElement, "requestFullscreen");
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    const webkitEnterFullscreen = vi.fn();
    Object.assign(video(), { webkitEnterFullscreen });
    await user.click(screen.getByRole("button", { name: "Full screen" }));
    expect(webkitEnterFullscreen).toHaveBeenCalledTimes(1);
  });

  it("mutes from its button, and this browser keeps the volume for the next video", async () => {
    serveMockApi();
    localStorage.removeItem("kivo.volume");
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(screen.getByRole("button", { name: "Mute" }));
    expect(video().muted).toBe(true);
    expect(await screen.findByRole("button", { name: "Unmute" })).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("kivo.volume") ?? "{}")).toMatchObject({ muted: true });
    await user.click(screen.getByRole("button", { name: "Unmute" }));
    expect(video().muted).toBe(false);
  });

  it("shows each track's method in the stream info", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await openStreamInfo(user);
    const info = await screen.findByRole("dialog", { name: "Stream info" });
    expect(within(info).getByText("Direct Play")).toBeInTheDocument();
    expect(within(info).getByText("Direct Play · H.264 High 1080p")).toBeInTheDocument();
    expect(within(info).getByText("Direct Play · English · AAC 2.0 · Default")).toBeInTheDocument();
    expect(within(info).getByText("File · Local")).toBeInTheDocument();
  });

  it("draws a subtitle chosen during playback from its file, and shows it in the stream info", async () => {
    const api = serveMockApi();
    withTracks(api, [embedded("t3", "subrip", "en", "English")]);
    deliver(api, external("t3"));
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await tracksMenu(user);
    await user.click(
      await within(await screen.findByRole("menu", { name: "Audio & Subtitles" })).findByRole("menuitemradio", {
        name: /^English/,
      }),
    );
    await waitFor(() => expect(video().querySelector("track")).not.toBeNull());
    expect(plays()).toHaveLength(1);
    // A heartbeat reports it at once, for the server to remember.
    await waitFor(() => expect(heartbeats().at(-1)).toMatchObject({ subtitleTrackId: "t3" }));
    await openStreamInfo(user);
    const info = await screen.findByRole("dialog", { name: "Stream info" });
    expect(within(info).getByText("English · Plain (SRT) · Embedded")).toBeInTheDocument();
    await user.keyboard("{Escape}");

    // Off shows nothing, at once.
    await user.click(within(await tracksMenu(user)).getByRole("menuitemradio", { name: "Off" }));
    await waitFor(() => expect(video().querySelector("track")).toBeNull());
    await waitFor(() => expect(heartbeats().at(-1)).toMatchObject({ subtitleTrackId: "off" }));
    expect(plays()).toHaveLength(1);
  }, 15_000);

  it("says why a track isn't direct in the stream info, with the capability it failed", async () => {
    const api = serveMockApi();
    deliver(api, {
      trackId: "1080-audio-en",
      type: "audio",
      selected: true,
      delivery: "stream",
      codec: "aac",
      reason: "audioChannelsUnsupported",
      mismatch: { field: "maxChannels", declared: "2", actual: "6" },
      transcode: { channels: 2 },
    });
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await openStreamInfo(user);
    const info = await screen.findByRole("dialog", { name: "Stream info" });
    expect(
      within(info).getByText("Why: The audio has more channels than this device plays. (maxChannels: 6, declared 2)"),
    ).toBeInTheDocument();
  });

  it("asks for no external subtitles in picture-in-picture, starting a new play for one shown and another on leaving if it was burned in", async () => {
    const api = serveMockApi();
    withTracks(api, [embedded("t3", "ass", "ja", "Japanese")]);
    deliver(api, { ...external("t3"), codec: "ass", url: "plays/play/subtitles/t3.ass" });
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(within(await tracksMenu(user)).getByRole("menuitemradio", { name: /^Japanese/ }));
    await waitFor(() => expect(heartbeats().at(-1)).toMatchObject({ subtitleTrackId: "t3" }));
    expect(plays()[0]?.capabilities.subtitles.some((e) => e.delivery.includes("external"))).toBe(true);

    // Entering picture-in-picture with an external subtitle showing starts a play without external subtitles, which burns it in.
    nextPlaysShow(api, "t3", "burn");
    pictureInPicture(true);
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(plays()[1]).toMatchObject({ subtitleTrackId: "t3", positionMs: resumeMs });
    expect(plays()[1]?.capabilities.subtitles.some((e) => e.delivery.includes("external"))).toBe(false);
    await playing();

    // Leaving it starts a play with external subtitles again.
    pictureInPicture(false);
    await waitFor(() => expect(plays()).toHaveLength(3));
    expect(plays()[2]?.capabilities.subtitles.some((e) => e.delivery.includes("external"))).toBe(true);
    Reflect.deleteProperty(document, "pictureInPictureElement");
  }, 20_000);

  it("keeps the play on entering picture-in-picture when its subtitle isn't delivered external", async () => {
    serveMockApi();
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    pictureInPicture(true);
    await new Promise((r) => setTimeout(r, 50));
    expect(plays()).toHaveLength(1);
    Reflect.deleteProperty(document, "pictureInPictureElement");
  });
});

describe("Player menus", () => {
  afterEach(() => localStorage.clear());

  it("has one Audio & Subtitles button and one Playback Settings gear, whose rows come in order", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    for (const gone of ["Audio", "Subtitles", "Quality", "Stream info"]) {
      expect(screen.queryByRole("button", { name: gone })).not.toBeInTheDocument();
    }
    const rows = within(await settingsMenu(user)).getAllByRole("menuitem");
    expect(rows.map((r) => r.textContent)).toEqual([
      "Audio & Subtitles",
      "QualityAuto",
      "Version1080p",
      "SpeedNormal",
      "Stream info",
    ]);
  });

  it("opens Audio & Subtitles from its row in Playback Settings", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(within(await settingsMenu(user)).getByRole("menuitem", { name: "Audio & Subtitles" }));
    const menu = await screen.findByRole("menu", { name: "Audio & Subtitles" });
    expect(within(menu).getByRole("menuitemcheckbox", { name: /^Dialogue boost/ })).toBeInTheDocument();
    expect(screen.queryByRole("menu", { name: "Playback Settings" })).not.toBeInTheDocument();
  });

  it("switches the dialogue boost with a new play where playback was, kept for this browser", async () => {
    const api = serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    expect(plays()[0]?.dialogueBoost).toBe(false);
    const boost = within(await tracksMenu(user)).getByRole("menuitemcheckbox", { name: /^Dialogue boost/ });
    expect(boost).not.toBeChecked();
    await user.click(boost);
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(plays()[1]).toMatchObject({ dialogueBoost: true, versionId: dunePlay(api).versionId, positionMs: resumeMs });
    expect(localStorage.getItem("kivo.dialogueBoost")).toBe("on");
    // The menu stays open, with the switch on as the new play says.
    await waitFor(() => expect(screen.getByRole("menuitemcheckbox", { name: /^Dialogue boost/ })).toBeChecked());
  });

  it("starts with this browser's dialogue boost", async () => {
    serveMockApi();
    localStorage.setItem("kivo.dialogueBoost", "on");
    renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    expect(plays()[0]?.dialogueBoost).toBe(true);
  });

  it("keeps a show's tracks without saying so in Audio & Subtitles", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.expanseReload}`);
    await playing();
    const menu = await tracksMenu(user);
    expect(within(menu).getByRole("menuitemcheckbox", { name: /^Dialogue boost/ })).toBeInTheDocument();
    expect(screen.queryByText("Remembered for this show")).not.toBeInTheDocument();
  });

  it("offers no dialogue boost on a play without one", async () => {
    const api = serveMockApi();
    delete dunePlay(api).dialogueBoost;
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    // With one audio track and no subtitles, only Search subtitles is offered, so Audio & Subtitles shows only Subtitles.
    const menu = await tracksMenu(user);
    expect(within(menu).queryByRole("menuitemcheckbox")).not.toBeInTheDocument();
    expect(within(menu).queryByText("Audio")).not.toBeInTheDocument();
  });

  it("plays at the speed chosen, for this video, without a new play", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(within(await settingsSubmenu(user, "Speed")).getByRole("menuitemradio", { name: "1.5×" }));
    expect(video().playbackRate).toBe(1.5);
    expect(video().defaultPlaybackRate).toBe(1.5);
    expect(plays()).toHaveLength(1);
    expect(within(await settingsMenu(user)).getByRole("menuitem", { name: /^Speed/ })).toHaveTextContent("1.5×");
  }, 15_000);

  it("says under the time left when the play ends, at the speed chosen", async () => {
    serveMockApi();
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    const left = dunePartTwoMs - resumeMs;
    // The end time as of now, or of a moment ago if the minute turned since.
    const endsAt = (rate: number) => {
      const now = Date.now();
      return [now - 2000, now].map((at) => `Ends at ${formatEndTime(at + left / rate)}`);
    };
    const bar = document.querySelector('[data-part="player-controls"]') as HTMLElement;
    expect(endsAt(1)).toContain(within(bar).getByText(/^Ends at /).textContent);
    await user.click(within(await settingsSubmenu(user, "Speed")).getByRole("menuitemradio", { name: "2×" }));
    await waitFor(() => expect(endsAt(2)).toContain(within(bar).getByText(/^Ends at /).textContent));
  }, 15_000);

  it("plays another version from the position, its tracks the server's pick", async () => {
    const api = serveMockApi();
    const other = api.works[mockIds.dunePartTwo]?.versions[0];
    const { user } = renderApp(`/watch/${mockIds.dunePartTwo}`);
    await playing();
    await user.click(
      within(await settingsSubmenu(user, "Version")).getByRole("menuitemradio", {
        name: /^4K HDR/,
      }),
    );
    await waitFor(() => expect(plays()).toHaveLength(2));
    expect(plays()[1]).toMatchObject({ versionId: other?.id, positionMs: resumeMs });
    expect(plays()[1]).not.toHaveProperty("audioTrackId");
    expect(plays()[1]).not.toHaveProperty("subtitleTrackId");
  }, 15_000);
});

describe("Player markers and up next", () => {
  /** Plays of The Expanse's "Reload" start at startMs. */
  const reloadAt = (startMs: number) => (api: MockApi) => {
    const play = api.starts[mockIds.expanseReload];
    if (play) play.startMs = startMs;
  };

  it("offers Skip Intro inside the intro, skipping to its end", async () => {
    serveMockApi(reloadAt(65_000));
    const { user } = renderApp(`/watch/${mockIds.expanseReload}`);
    await user.click(await screen.findByRole("button", { name: "Skip Intro" }));
    await waitFor(() => expect(video().currentTime).toBe(150));
    await waitFor(() => expect(heartbeats().at(-1)?.positionMs).toBe(150_000));
    expect(screen.queryByRole("button", { name: "Skip Intro" })).not.toBeInTheDocument();
  });

  /** Answers a play of the episode after "Reload" as one of "Reload". */
  const playableNext = (api: MockApi) => {
    const next = api.works[mockIds.expanseReload]?.episode?.nextId ?? "";
    api.starts[next] = { ...(api.starts[mockIds.expanseReload] as PlayStarted), workId: next };
    return next;
  };

  it("brings up the next episode 35 seconds before the end of a 44-minute one, and plays it", async () => {
    const api = serveMockApi(reloadAt(43 * minute));
    const next = playableNext(api);
    const { user, router } = renderApp(`/watch/${mockIds.expanseReload}`);
    await playing();
    // In the credits, but before the card's time.
    expect(screen.queryByRole("region", { name: "Up next" })).not.toBeInTheDocument();
    playTo(video(), 43 * 60 + 25);
    const card = await screen.findByRole("region", { name: "Up next" });
    expect(await within(card).findByText("S3 · E5 “Episode 5”")).toBeInTheDocument();
    expect(within(card).getByText("Playing in 35 s")).toBeInTheDocument();
    // Skip Credits gives way to the card.
    expect(screen.queryByRole("button", { name: "Skip Credits" })).not.toBeInTheDocument();
    await user.click(within(card).getByRole("button", { name: "Play now" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${next}`));
    await waitFor(() => expect(plays().at(-1)?.workId).toBe(next));
  });

  it("comes at final credits, where Watch credits hides it and the credits play on", async () => {
    const api = serveMockApi((api: MockApi) => {
      reloadAt(42 * minute)(api);
      const play = api.starts[mockIds.expanseReload];
      play?.markers?.forEach((m) => {
        if (m.kind === "credits") m.final = true;
      });
    });
    playableNext(api);
    const { user } = renderApp(`/watch/${mockIds.expanseReload}`);
    await playing();
    expect(screen.queryByRole("region", { name: "Up next" })).not.toBeInTheDocument();
    // The credits start 90 seconds before the end of the 44-minute episode.
    playTo(video(), 42 * 60 + 30);
    const card = await screen.findByRole("region", { name: "Up next" });
    expect(within(card).getByText("Playing in 10 s")).toBeInTheDocument();
    await user.click(within(card).getByRole("button", { name: "Watch credits" }));
    expect(screen.queryByRole("region", { name: "Up next" })).not.toBeInTheDocument();
  });

  it("hides on Hide, keeping Skip Credits, and still plays the next episode at the end", async () => {
    const api = serveMockApi(reloadAt(43 * minute + 30_000));
    const next = playableNext(api);
    const { user, router } = renderApp(`/watch/${mockIds.expanseReload}`);
    const card = await screen.findByRole("region", { name: "Up next" });
    await user.click(within(card).getByRole("button", { name: "Hide" }));
    expect(await screen.findByRole("button", { name: "Skip Credits" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Up next" })).not.toBeInTheDocument();
    playToEnd(video());
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${next}`));
  });

  /**
   * Plays of "Reload" and the episode after it start 30 seconds before
   * their end, and each one's next episode is the other.
   */
  const backAndForth = (api: MockApi) => {
    reloadAt(43 * minute + 30_000)(api);
    const next = playableNext(api);
    const episode = api.works[next]?.episode;
    if (episode) episode.nextId = mockIds.expanseReload;
    return next;
  };

  /** Plays the current episode to its end, and returns the next episode's card as its player starts. */
  async function playsOn(router: ReturnType<typeof renderApp>["router"], workId: string) {
    const ended = video();
    playToEnd(ended);
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${workId}`));
    await waitFor(() => expect(video()).not.toBe(ended));
    return screen.findByRole("region", { name: "Up next" });
  }

  /** Plays on three episodes in a row from "Reload", until the episode after it asks "Still watching?". */
  async function playOnThrice(router: ReturnType<typeof renderApp>["router"], next: string) {
    const card = await screen.findByRole("region", { name: "Up next" });
    expect(within(card).getByText("Playing in 30 s")).toBeInTheDocument();
    for (const workId of [next, mockIds.expanseReload]) {
      expect(within(await playsOn(router, workId)).getByText("Playing in 30 s")).toBeInTheDocument();
    }
    return playsOn(router, next);
  }

  it("asks after three episodes played on without input, and waits at the end", async () => {
    const api = serveMockApi();
    const next = backAndForth(api);
    const { router } = renderApp(`/watch/${mockIds.expanseReload}`);
    const card = await playOnThrice(router, next);
    expect(within(card).getByText("Still watching?")).toBeInTheDocument();
    expect(within(card).queryByText(/Playing in/)).not.toBeInTheDocument();
    playToEnd(video());
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(router.state.location.pathname).toBe(`/watch/${next}`);
    expect(screen.getByRole("region", { name: "Up next" })).toBeInTheDocument();
  });

  it("plays on again after input, though the card still asks", async () => {
    const api = serveMockApi();
    const next = backAndForth(api);
    const { router } = renderApp(`/watch/${mockIds.expanseReload}`);
    await playOnThrice(router, next);
    fireEvent.pointerMove(document.querySelector('[data-part="player"]') as Element);
    expect(within(await playsOn(router, mockIds.expanseReload)).getByText("Playing in 30 s")).toBeInTheDocument();
  });

  it("keeps the run across each episode's player, and starts a new one after leaving the player", async () => {
    const api = serveMockApi();
    const next = backAndForth(api);
    const { router } = renderApp(`/watch/${mockIds.expanseReload}`);
    expect(within(await playOnThrice(router, next)).getByText("Still watching?")).toBeInTheDocument();
    await router.navigate({ to: "/" });
    await waitFor(() => expect(document.querySelector("video")).toBeNull());
    await router.navigate({ to: "/watch/$workId", params: { workId: next } });
    const card = await screen.findByRole("region", { name: "Up next" });
    expect(within(card).getByText("Playing in 30 s")).toBeInTheDocument();
  });

  it("with autoplay off, shows the next episode without a countdown and waits at the end", async () => {
    const api = serveMockApi(reloadAt(43 * minute + 30_000));
    api.currentUser.playback.autoplayNext = false;
    const next = playableNext(api);
    const { user, router } = renderApp(`/watch/${mockIds.expanseReload}`);
    const card = await screen.findByRole("region", { name: "Up next" });
    expect(within(card).getByText("Up next")).toBeInTheDocument();
    expect(within(card).queryByText(/Playing in/)).not.toBeInTheDocument();
    playToEnd(video());
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(router.state.location.pathname).toBe(`/watch/${mockIds.expanseReload}`);
    await user.click(within(screen.getByRole("region", { name: "Up next" })).getByRole("button", { name: "Play now" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${next}`));
  });

  it("ends the last episode on its series' page, with no card", async () => {
    const api = serveMockApi(reloadAt(43 * minute + 30_000));
    const episode = api.works[mockIds.expanseReload]?.episode;
    if (episode) delete episode.nextId;
    const { router } = renderApp(`/watch/${mockIds.expanseReload}`);
    await playing();
    expect(screen.queryByRole("region", { name: "Up next" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Skip Credits" })).toBeInTheDocument();
    playToEnd(video());
    await waitFor(() => expect(router.state.location.pathname).toBe(`/works/${episode?.seriesId}`));
  });
});
