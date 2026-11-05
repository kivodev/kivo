// Google's Cast sender library. It loads only where it can work: in Chrome
// and other Chromium browsers, on a secure page, which Chrome requires of a
// Cast sender. Only the parts Kivo uses are typed here.

import { isTv } from "@/tv/flag";

const senderUrl = "https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1";

export type CastState = "NO_DEVICES_AVAILABLE" | "NOT_CONNECTED" | "CONNECTING" | "CONNECTED";

export type SessionState =
  | "NO_SESSION"
  | "SESSION_STARTING"
  | "SESSION_STARTED"
  | "SESSION_START_FAILED"
  | "SESSION_ENDING"
  | "SESSION_ENDED"
  | "SESSION_RESUMED";

export interface MediaSession {
  media?: { contentId?: string; duration?: number | null } | null;
  playerState: string;
  idleReason?: string | null;
  playbackRate: number;
  customData?: unknown;
  mediaSessionId: number;
  getEstimatedTime(): number;
}

export interface CastSession {
  getCastDevice(): { friendlyName: string };
  getMediaSession(): MediaSession | null;
  loadMedia(request: unknown): Promise<unknown>;
  sendMessage(namespace: string, message: unknown): Promise<unknown>;
  addMessageListener(namespace: string, listener: (namespace: string, message: string) => void): void;
  removeMessageListener(namespace: string, listener: (namespace: string, message: string) => void): void;
  setVolume(level: number): Promise<unknown>;
  getVolume(): number | null;
  isMute(): boolean | null;
  endSession(stopCasting: boolean): void;
}

interface CastContext {
  setOptions(options: { receiverApplicationId: string; autoJoinPolicy: string }): void;
  addEventListener(
    type: string,
    listener: (event: { castState?: CastState; sessionState?: SessionState }) => void,
  ): void;
  getCastState(): CastState;
  getCurrentSession(): CastSession | null;
  requestSession(): Promise<unknown>;
}

export interface RemotePlayer {
  isConnected: boolean;
  isMediaLoaded: boolean;
  currentTime: number;
  duration: number;
  isPaused: boolean;
  playerState: string | null;
  volumeLevel: number;
  isMuted: boolean;
}

export interface RemotePlayerController {
  addEventListener(type: string, listener: () => void): void;
  playOrPause(): void;
  seek(): void;
  setVolumeLevel(): void;
}

/** What Google's notifications and Google Home show for a cast: its title, and the artwork's absolute URL. */
export type CastLabel = { title: string; image?: string };

/** What Kivo uses of `cast.framework` and `chrome.cast`. */
export interface CastFramework {
  context: CastContext;
  newPlayer(): { player: RemotePlayer; controller: RemotePlayerController };
  /** A LOAD request for the media described. */
  loadRequest(media: {
    contentId: string;
    entity: string;
    label: CastLabel;
    customData: unknown;
    currentTimeSec: number | undefined;
  }): unknown;
}

type Globals = {
  __onGCastApiAvailable?: (available: boolean) => void;
  chrome?: { cast?: ChromeCast };
  cast?: { framework?: Framework };
};

type ChromeCast = {
  AutoJoinPolicy: { ORIGIN_SCOPED: string };
  media: {
    MediaInfo: new (contentId: string, contentType: string) => Record<string, unknown>;
    MovieMediaMetadata: new () => Record<string, unknown>;
    Image: new (url: string) => unknown;
    LoadRequest: new (media: unknown) => Record<string, unknown>;
    StreamType: { BUFFERED: string };
  };
};

type Framework = {
  CastContext: { getInstance(): CastContext };
  RemotePlayer: new () => RemotePlayer;
  RemotePlayerController: new (player: RemotePlayer) => RemotePlayerController;
};

let loading: Promise<CastFramework | undefined> | undefined;

/** Whether this browser can be a Cast sender at all. */
function canCast(): boolean {
  // The TV build plays on the TV itself.
  if (isTv) return false;
  return typeof window !== "undefined" && window.isSecureContext && "chrome" in window;
}

/** Loads the sender library once, with Kivo's receiver set. It resolves to undefined where casting can't work. */
export function loadCastSdk(appId: string): Promise<CastFramework | undefined> {
  if (!canCast()) return Promise.resolve(undefined);
  loading ??= new Promise((resolve) => {
    const globals = window as unknown as Globals;
    globals.__onGCastApiAvailable = (available) => {
      const chromeCast = globals.chrome?.cast;
      const framework = globals.cast?.framework;
      if (!available || !chromeCast || !framework) return resolve(undefined);
      const context = framework.CastContext.getInstance();
      context.setOptions({ receiverApplicationId: appId, autoJoinPolicy: chromeCast.AutoJoinPolicy.ORIGIN_SCOPED });
      resolve({
        context,
        newPlayer: () => {
          const player = new framework.RemotePlayer();
          return { player, controller: new framework.RemotePlayerController(player) };
        },
        loadRequest: ({ contentId, entity, label, customData, currentTimeSec }) => {
          const info = new chromeCast.media.MediaInfo(contentId, "video/mp4");
          info.entity = entity;
          info.streamType = chromeCast.media.StreamType.BUFFERED;
          const metadata = new chromeCast.media.MovieMediaMetadata();
          metadata.title = label.title;
          if (label.image) metadata.images = [new chromeCast.media.Image(label.image)];
          info.metadata = metadata;
          info.customData = customData;
          const request = new chromeCast.media.LoadRequest(info);
          request.autoplay = true;
          if (currentTimeSec !== undefined) request.currentTime = currentTimeSec;
          return request;
        },
      });
    };
    const script = document.createElement("script");
    script.src = senderUrl;
    script.async = true;
    script.onerror = () => resolve(undefined);
    document.head.append(script);
  });
  return loading;
}
