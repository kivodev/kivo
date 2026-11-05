import { waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CastState, MediaSession, SessionState } from "./castSdk";

const token = { token: "secret", expiresAt: "2026-10-25T00:00:00Z", serverId: "srv", urls: ["http://kivo"] };
const createCastToken = vi.fn(async (_name: string) => token);
vi.mock("@/api/cast", () => ({ createCastToken: (name: string) => createCastToken(name) }));

/** A stand-in for Google's sender library: one TV, "Living Room TV". */
function fakeSdk() {
  const listeners: Record<string, ((e: { castState?: CastState; sessionState?: SessionState }) => void)[]> = {};
  let castState: CastState = "NOT_CONNECTED";
  let media: MediaSession | null = null;
  const messages: [string, unknown][] = [];
  const loads: unknown[] = [];
  let ended: boolean | undefined;
  const player = {
    isConnected: false,
    isMediaLoaded: false,
    currentTime: 0,
    duration: 0,
    isPaused: true,
    playerState: null,
    volumeLevel: 0.4,
    isMuted: false,
  };
  let anyChange = () => {};
  const session = {
    getCastDevice: () => ({ friendlyName: "Living Room TV" }),
    getMediaSession: () => media,
    loadMedia: vi.fn(async (request: unknown) => {
      loads.push(request);
    }),
    sendMessage: vi.fn(async (namespace: string, message: unknown) => {
      messages.push([namespace, message]);
    }),
    addMessageListener: vi.fn(),
    removeMessageListener: vi.fn(),
    setVolume: vi.fn(async () => {}),
    getVolume: () => 0.4,
    isMute: () => false,
    endSession: (stop: boolean) => {
      ended = stop;
    },
  };
  let current: typeof session | null = null;
  const emit = (type: string, e: { castState?: CastState; sessionState?: SessionState }) => {
    for (const l of listeners[type] ?? []) l(e);
  };
  const sdk = {
    context: {
      setOptions: () => {},
      addEventListener: (type: string, l: (e: { castState?: CastState; sessionState?: SessionState }) => void) => {
        listeners[type] = [...(listeners[type] ?? []), l];
      },
      getCastState: () => castState,
      getCurrentSession: () => current,
      requestSession: async () => null,
    },
    newPlayer: () => ({
      player,
      controller: {
        addEventListener: (_type: string, l: () => void) => {
          anyChange = l;
        },
        playOrPause: vi.fn(),
        seek: vi.fn(),
        setVolumeLevel: vi.fn(),
      },
    }),
    loadRequest: (m: unknown) => m,
  };
  return {
    sdk,
    messages,
    loads,
    ended: () => ended,
    connect() {
      current = session;
      castState = "CONNECTED";
      emit("caststatechanged", { castState });
      emit("sessionstatechanged", { sessionState: "SESSION_STARTED" });
    },
    /** Sends the receiver's media status, playing the work at a position with its custom data. */
    plays(workId: string, positionSec: number, customData?: unknown) {
      media = {
        media: { contentId: workId, duration: 3600 },
        playerState: "PLAYING",
        idleReason: null,
        playbackRate: 1,
        customData,
        mediaSessionId: 1,
        getEstimatedTime: () => positionSec,
      };
      Object.assign(player, { isMediaLoaded: true, currentTime: positionSec, duration: 3600 });
      anyChange();
    },
  };
}

let fake: ReturnType<typeof fakeSdk>;
vi.mock("./castSdk", () => ({ loadCastSdk: async () => fake.sdk }));

beforeEach(() => {
  fake = fakeSdk();
  createCastToken.mockClear();
});

async function started() {
  const { CastLink } = await import("./castSession");
  const link = new CastLink();
  link.start();
  await waitFor(() => expect(link.getSnapshot().connection).toBe("disconnected"));
  return link;
}

describe("CastLink", () => {
  it("connects to the device and asks the receiver for its status", async () => {
    const link = await started();
    fake.connect();
    expect(link.getSnapshot()).toMatchObject({ connection: "connected", device: "Living Room TV", volume: 0.4 });
    expect(fake.messages).toContainEqual(["urn:x-cast:com.mmcdole.kivo", { type: "kivo.getStatus" }]);
  });

  it("hands what plays here, as the session starts, to the remote for its work, once", async () => {
    const link = await started();
    link.setLocal(() => ({ workId: "w", positionMs: 61_000 }));
    fake.connect();
    link.setLocal(() => ({ workId: "w", positionMs: 99_000 }));
    expect(link.takeHandoff("other")).toBeUndefined();
    fake.connect();
    expect(link.takeHandoff("w")).toBeUndefined();
  });

  it("hands the play here over to the remote for its work", async () => {
    const link = await started();
    link.setLocal(() => ({ workId: "w", positionMs: 61_000 }));
    fake.connect();
    expect(link.takeHandoff("w")).toEqual({ workId: "w", positionMs: 61_000 });
    expect(link.takeHandoff("w")).toBeUndefined();
  });

  it("casts with a token for the device, and mirrors the TV's media and status", async () => {
    const link = await started();
    fake.connect();
    await link.cast({ workId: "w", positionMs: 30_000 }, { title: "Tremors", image: "https://kivo/i" });
    expect(createCastToken).toHaveBeenCalledWith("Living Room TV");
    expect(fake.loads[0]).toMatchObject({
      contentId: "w",
      currentTimeSec: 30,
      label: { title: "Tremors", image: "https://kivo/i" },
    });
    expect(link.getSnapshot().loading).toBe("w");

    fake.plays("w", 31, { status: { workId: "w", versionId: "v", audioTrackId: "a" } });
    const now = link.getSnapshot();
    expect(now.loading).toBeUndefined();
    expect(now.media).toEqual({ workId: "w", state: "playing", positionMs: 31_000, durationMs: 3_600_000, rate: 1 });
    expect(now.status).toMatchObject({ versionId: "v", audioTrackId: "a", subtitleTrackId: "off" });
  });

  it("says why when the receiver stops with an error, but keeps trying while the server is unreachable", async () => {
    const link = await started();
    fake.connect();
    fake.plays("w", 1, { status: { workId: "w", error: "serverUnreachable" } });
    expect(link.getSnapshot().failure).toBeUndefined();
    fake.plays("w", 1, { status: { workId: "w", error: "signedOut" } });
    expect(link.getSnapshot().failure).toBe("Signed out on the TV.");
  });

  it("Stop casting stops the TV and returns what played there, at the TV's position; Disconnect lets the TV play on", async () => {
    const link = await started();
    fake.connect();
    fake.plays("w", 90, { status: { workId: "w", versionId: "v", subtitleTrackId: "s" } });
    expect(link.end(true)).toEqual({
      workId: "w",
      versionId: "v",
      audioTrackId: undefined,
      subtitleTrackId: "s",
      positionMs: 90_000,
    });
    expect(fake.ended()).toBe(true);
    expect(link.getSnapshot()).toMatchObject({ connection: "disconnected", media: undefined });

    fake.connect();
    expect(link.end(false)).toBeUndefined();
    expect(fake.ended()).toBe(false);
  });
});
