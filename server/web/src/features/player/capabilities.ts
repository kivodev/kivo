import type { AudioEntry, Capabilities, CapabilityDelivery, SubtitleEntry, VideoEntry } from "@/api/schemas";
import { type Browser, currentBrowser } from "@/lib/browser";
import { deviceSettings } from "@/lib/deviceSettings";
import { readStoredJson, writeStoredJson } from "@/lib/storage";
import { isPackaged, isTv } from "@/tv/flag";
import { avplay, fullHdPanel, hdrPanel } from "@/tv/tizen";

// The capability report: what this browser plays, from the media element's
// canPlayType and the rules Jellyfin's web client keeps for the answers each
// browser gets wrong. It is computed once at idle after sign-in and cached
// per browser and app build, so starting playback never waits on it.

/** The browser's answers, behind an interface so tests can supply their own. */
export type Probe = {
  /** A video element's canPlayType, as a yes or no. */
  canPlay: (type: string) => boolean;
  /** An audio element's canPlayType, as a yes or no. */
  canPlayAudio: (type: string) => boolean;
  /** Whether the browser plays HLS, itself or through Media Source Extensions. */
  hls: boolean;
  /** Whether the media element has audioTracks, to switch between a file's audio tracks. */
  audioTracks: boolean;
  /** Whether JASSUB can run: WebAssembly, workers and OffscreenCanvas. */
  assRenderer: boolean;
  /** Whether this is the TV build, which draws no ASS or PGS because a TV is too small for it. */
  tv: boolean;
  /** Whether fullscreen shows only the video element, as on iOS, so nothing drawn over it shows. */
  videoOnlyFullscreen: boolean;
  /** Whether this is the packaged TV app, with the TV's own APIs. */
  packaged: boolean;
  /** Whether the TV says its panel is 1080p (the packaged app's productinfo). */
  fullHdPanel: boolean;
  /** Whether the TV says its panel shows HDR (the packaged app's avinfo); undefined where it can't say. */
  hdrPanel: boolean | undefined;
  /** Whether the packaged app plays files with Samsung's native player, AVPlay, rather than the video element. */
  avplay: boolean;
};

const mp4 = (codecs: string) => `video/mp4; codecs="${codecs}"`;
const hlsTypes = ["application/x-mpegurl", "application/vnd.apple.mpegURL"];

/**
 * The capability report from the browser's answers. Each codec and rule is
 * Jellyfin's web client's: what it probes, and where it overrides the
 * browser. The stream is HLS with fragmented MP4, so a codec plays in it
 * only where Jellyfin plays it in fMP4 HLS.
 */
export function report(probe: Probe, p: Browser): Capabilities {
  const can = probe.canPlay;
  const tizen = p.tizen > 0;
  const mkv = tizen || (!p.firefox && (can("video/x-matroska") || can("video/mkv") || p.edge));
  // WebM is unreliable from Safari 17.
  const webm = !p.safari || (p.version >= 15 && p.version < 17);
  const stream = (ok: boolean): CapabilityDelivery[] => (probe.hls && ok ? ["file", "stream"] : ["file"]);
  const files = (...containers: string[]) => [...containers, ...(mkv ? ["mkv"] : [])];

  // A Samsung TV whose panel says it shows no HDR is sent the SDR its
  // transcode makes, rather than HDR it would show washed out.
  const hdr: VideoEntry["hdr"] =
    (tizen && probe.hdrPanel !== false) ||
    (p.safari && (p.ios || p.mac)) ||
    (p.edge && p.version >= 121) ||
    (p.chrome && !p.mobile) ||
    (p.firefox && p.mac && p.version >= 100)
      ? ["hdr10", "hdr10plus", "hlg"]
      : undefined;
  const dolbyVision = p.safari && (p.ios || p.mac);
  // A Samsung TV plays no Dolby Vision, but plays its HDR10 fallback layer.
  const doviFallback = p.tizen >= 3 && hdr ? true : undefined;
  // It plays MPEG-TS files too, and caps video at 20 Mb/s on a 1080p panel.
  const ts = tizen ? ["ts", "m2ts"] : [];
  // Samsung's specifications for its Tizen 8 TVs (2024) and later, rather
  // than canPlayType, which answers some HEVC profiles and levels wrongly on
  // them. They're what AVPlay plays; Samsung's browser plays through the
  // video element, so there canPlayType decides.
  const tizen8 = probe.avplay && p.tizen >= 8;

  const video: VideoEntry[] = [];
  if (tizen || can(mp4("avc1.42E01E, mp4a.40.2"))) {
    const highTen =
      can(mp4("avc1.6e0033")) && !p.safari && !p.ios && !p.edge && !p.mobile && !probe.tv ? ["High 10"] : [];
    const h264: VideoEntry = {
      codec: "h264",
      delivery: stream(true),
      containers: files("mp4", ...ts),
      profiles: ["High", "Main", "Baseline", "Constrained Baseline", ...highTen],
      maxLevel:
        can(mp4("avc1.640834")) || (p.tizen >= 5 && probe.packaged)
          ? 5.2
          : can(mp4("avc1.640833")) || tizen
            ? 5.1
            : 4.2,
    };
    // A Samsung TV's Media Source Extensions play H.264 no larger than
    // 1080p, so a stream of it is held to 1080p's level and size, and only a
    // file plays 4K.
    if (tizen && h264.delivery.includes("stream")) {
      video.push({ ...h264, delivery: ["file"] });
      video.push({
        ...h264,
        delivery: ["stream"],
        containers: undefined,
        maxLevel: 4.2,
        maxWidth: 1920,
        maxHeight: 1080,
      });
    } else video.push(h264);
  }
  const hevc = tizen || ["hvc1.1.L120", "hev1.1.L120", "hvc1.1.0.L120", "hev1.1.0.L120"].some((c) => can(mp4(c)));
  if (hevc) {
    const at = (profile: number, level: string) =>
      can(mp4(`hvc1.${profile}.4.${level}`)) || can(mp4(`hev1.${profile}.4.${level}`));
    // From Tizen 8 Samsung's UHD TVs decode Main 10 to 4K at level 5.1, and
    // most to 5.2; a higher level that answers is taken.
    const main10Level = ["L186", "L183", "L156", "L153", "L123"].find((l) => at(2, l)) ?? (tizen8 ? "L153" : undefined);
    const level = main10Level ?? (at(1, "L123") ? "L123" : "L120");
    const profiles = main10Level ? ["Main", "Main 10"] : ["Main"];
    const dvProfiles = dolbyVision
      ? [can(mp4("dvh1.05.06")) && 5, can(mp4("dvh1.08.06")) && 8].filter((n): n is number => n !== false)
      : [];
    video.push({
      codec: "hevc",
      delivery: stream(
        p.edge ||
          p.safari ||
          tizen ||
          (p.chrome && (!p.android || p.version >= 105)) ||
          (p.firefox && p.version >= 134),
      ),
      // Samsung plays HEVC only in MKV, MP4 and MPEG-TS, of which M2TS is one.
      containers: files("mp4", ...ts),
      profiles,
      maxLevel: Number(level.slice(1)) / 30,
      hdr,
      dolbyVision: dvProfiles.length > 0 ? dvProfiles.map((profile) => ({ profile })) : undefined,
      doviFallback,
      // Safari plays only these tags, and no more than 60 frames a second.
      tags: p.safari ? ["hvc1", "dvh1"] : undefined,
      maxFrameRate: p.safari ? 60 : undefined,
    });
  }
  if (p.tizen >= 5.5 || (can(mp4("av01.0.15M.08")) && can(mp4("av01.0.15M.10")))) {
    const level = [19, 18, 17, 16].find((l) => can(mp4(`av01.0.${l}M.08`)) && can(mp4(`av01.0.${l}M.10`))) ?? 15;
    video.push({
      codec: "av1",
      // Mobile browsers other than Safari decode AV1 in software, too slowly.
      delivery: stream(p.safari || tizen || (!p.mobile && (p.edge || p.firefox || p.chrome))),
      containers: files("mp4", ...(webm ? ["webm"] : [])),
      profiles: ["Main"],
      maxLevel: av1Level(level),
      hdr,
      doviFallback,
      dolbyVision: dolbyVision && can(mp4("dav1.10.06")) ? [{ profile: 10 }] : undefined,
    });
  }
  if (tizen8 || can('video/webm; codecs="vp9"')) {
    // iOS fails VP9 in MP4, and Firefox on a Mac can't tell its HDR there.
    const inMp4 = !p.ios && !(p.firefox && p.mac);
    video.push({
      codec: "vp9",
      delivery: stream(p.safari || p.edge || p.chrome || p.firefox),
      containers: files(...(inMp4 ? ["mp4"] : []), ...(webm ? ["webm"] : [])),
      // Samsung lists profile 0 and profile 2, VP9's 10-bit HDR, and no other.
      profiles: tizen ? ["Profile 0", "Profile 2"] : undefined,
      hdr,
    });
  }
  const vp8 = can('video/webm; codecs="vp8"');
  if (vp8) video.push({ codec: "vp8", delivery: ["file"], containers: ["webm"] });
  if (tizen || can(mp4("vc-1"))) video.push({ codec: "vc1", delivery: ["file"], containers: files("mp4", ...ts) });
  if (tizen) video.push({ codec: "mpeg2video", delivery: ["file"], containers: files("mp4", ...ts) });
  if (tizen && probe.fullHdPanel) {
    for (const v of video) if (["h264", "hevc", "av1"].includes(v.codec)) v.maxBitrateBps = 20_000_000;
  }

  const audio: AudioEntry[] = [];
  const add = (codec: string, delivery: CapabilityDelivery[], extra: Partial<AudioEntry> = {}) =>
    audio.push({ codec, delivery, maxChannels: 8, ...extra });
  const inHls = (codecs: string) => hlsTypes.some((t) => can(`${t}; codecs="${codecs}"`));
  // A browser that can't decode HE-AAC plays AAC's other profiles, as
  // Jellyfin's web client probes it. Samsung lists HE-AAC and MP3.
  if (tizen || can(mp4("avc1.640029, mp4a.40.2"))) {
    add("aac", stream(true), tizen || can(mp4("avc1.640029, mp4a.40.5")) ? {} : { profiles: ["LC", "Main", "LTP"] });
  }
  const mp3 = tizen || ["mp4a.69", "mp4a.6B", "mp3"].some((c) => can(mp4(`avc1.640029, ${c}`)));
  // Safari plays MP3 in HLS only in MPEG-TS segments, not fragmented MP4.
  if (mp3) add("mp3", stream(inHls("avc1.64001E, mp4a.40.34")));
  const ac3 = tizen || can('audio/mp4; codecs="ac-3"');
  const ac3InHls = p.edge || tizen || inHls("avc1.42E01E, ac-3");
  if (ac3) add("ac3", stream(ac3InHls));
  // Samsung's TVs decode E-AC-3, Atmos's too, in 5.1 at most.
  if (ac3 && (tizen || can('audio/mp4; codecs="ec-3"'))) add("eac3", stream(ac3InHls), tizen ? { maxChannels: 6 } : {});
  if (tizen || (mp3 && (p.chrome || p.edge || (p.firefox && p.version >= 83)) && !p.android)) add("mp2", stream(true));
  // Samsung TVs from Tizen 4 play no DTS.
  if (p.tizen < 4 && (can(mp4("dts-")) || can(mp4("dts+")))) add("dts", ["file"]);
  if (tizen) for (const codec of ["pcm_s16le", "pcm_s24le", "aac_latm"]) add(codec, ["file"]);
  // A Samsung TV's video element plays Opus files only in MKV and WebM;
  // AVPlay plays it in any container Samsung lists it in.
  const opus = probe.canPlayAudio('audio/ogg; codecs="opus"');
  if (probe.avplay) {
    add("opus", ["file"]);
    if (opus && probe.hls) add("opus", ["stream"]);
  } else if (opus) add("opus", stream(true), tizen ? { containers: ["mkv", "webm"] } : {});
  // Safari plays Opus from 17, in stereo at most.
  else if (p.safari && p.version >= 17 && probe.canPlayAudio('audio/x-caf; codecs="opus"'))
    add("opus", stream(true), { maxChannels: 2 });
  // A Samsung TV plays FLAC late inside a video, so only in a FLAC file.
  // AVPlay doesn't, but Samsung lists FLAC only as a music file.
  if (tizen) add("flac", ["file"], { containers: ["flac"] });
  else if (probe.canPlayAudio("audio/flac")) add("flac", stream(true));
  if (p.ios || (p.mac && p.safari)) add("alac", stream(true));
  // Samsung's TVs decode Vorbis in 5.1 at most.
  if ((!p.safari && vp8) || tizen) add("vorbis", ["file"], tizen ? { maxChannels: 6 } : {});

  // A file plays in the containers its codecs list. The rest are for books.
  const audioFiles = Object.entries({ mp3: "audio/mpeg", aac: "audio/aac", flac: "audio/flac", ogg: "audio/ogg" })
    .filter(([name, type]) => probe.canPlayAudio(type) || (tizen && name === "flac"))
    .map(([name]) => name);
  const containers = [...new Set([...video.flatMap((v) => v.containers ?? []), "m4a", ...audioFiles])];

  // WebVTT is the browser's own; PGS is drawn on a canvas, which works
  // anywhere. A TV draws neither ASS nor PGS, because JASSUB and a canvas
  // over the video are too heavy for it, so the server burns those in. iOS
  // fullscreen shows only the video element, so a canvas over it can't be
  // seen; WebVTT is a text track of the video, which shows there too.
  const drawn =
    probe.tv || probe.videoOnlyFullscreen
      ? ["webvtt"]
      : ["webvtt", ...(probe.assRenderer ? ["ass"] : []), "hdmv_pgs_subtitle"];
  const subtitles: SubtitleEntry[] = drawn.map((codec) => ({
    codec,
    delivery: codec === "webvtt" && probe.hls ? ["stream", "external"] : ["external"],
  }));

  return {
    // Firefox sees only a file's first audio track, and a Samsung TV's
    // video element switches tracks only from Tizen 5.5 to 7. AVPlay
    // switches them on any.
    file: {
      containers,
      trackSelection: probe.avplay || (probe.audioTracks && !p.firefox && (!tizen || (p.tizen >= 5.5 && p.tizen < 8))),
    },
    video,
    audio,
    subtitles,
  };
}

/** An AV1 level as its specification writes it: index 15 is 5.3, 16 is 6.0. */
function av1Level(index: number): number {
  return 2 + Math.floor(index / 4) + (index % 4) / 10;
}

/**
 * The report for a play in picture-in-picture, where nothing drawn over the
 * video shows: its subtitles without their own files.
 */
export function withoutExternal(capabilities: Capabilities): Capabilities {
  return {
    ...capabilities,
    subtitles: capabilities.subtitles.flatMap((entry) => {
      const delivery = entry.delivery.filter((d) => d !== "external");
      return delivery.length > 0 ? [{ ...entry, delivery }] : [];
    }),
  };
}

/**
 * outputChannels is the most audio channels this browser outputs, by
 * Jellyfin's rule. The Maximum audio channels setting wins when set. A
 * browser that outputs surround and plays AC-3 or E-AC-3 gets at least 5.1.
 * Otherwise it's the speaker count the browser reports, stereo where it
 * can't output more, and 5.1 for a surround browser that reports none.
 */
export function outputChannels(
  setting: number | undefined,
  capabilities: Capabilities,
  p: Browser,
  tv: boolean,
  speakers: number,
): number {
  if (setting) return setting;
  const surround = p.safari || p.chrome || p.edge || p.firefox || tv;
  const dolby = capabilities.audio.some((a) => a.codec === "ac3" || a.codec === "eac3");
  if (dolby && surround) return Math.max(6, speakers);
  if (speakers > 2) return surround ? speakers : 2;
  if (speakers > 0) return speakers;
  return surround ? 6 : 2;
}

/**
 * capChannels caps every audio entry in the report at the browser's
 * channels. The server then mixes down what the browser can't output. The
 * browser's own downmix leaves dialogue too quiet.
 */
export function capChannels(capabilities: Capabilities, channels: number): Capabilities {
  return {
    ...capabilities,
    audio: capabilities.audio.map((a) => ({ ...a, maxChannels: Math.min(a.maxChannels, channels) })),
  };
}

/** The output channels the browser's audio reaches; 0 when it can't say. */
function speakerCount(): number {
  try {
    const context = new AudioContext();
    const channels = context.destination.maxChannelCount;
    void context.close();
    return channels;
  } catch {
    return 0;
  }
}

/** The real browser's answers. */
function browserProbe(): Probe {
  const video = document.createElement("video");
  const audio = document.createElement("audio");
  const mse = globalThis.MediaSource ?? (globalThis as { ManagedMediaSource?: typeof MediaSource }).ManagedMediaSource;
  const canPlay = (type: string) => video.canPlayType(type) !== "";
  return {
    canPlay,
    canPlayAudio: (type) => audio.canPlayType(type) !== "",
    hls: mse !== undefined || hlsTypes.some(canPlay),
    audioTracks: "audioTracks" in video,
    assRenderer:
      typeof WebAssembly === "object" && typeof Worker === "function" && typeof OffscreenCanvas === "function",
    tv: isTv,
    packaged: isPackaged,
    fullHdPanel: fullHdPanel(),
    hdrPanel: hdrPanel(),
    avplay: isPackaged && avplay() !== undefined,
    videoOnlyFullscreen: currentBrowser().ios,
  };
}

const storageKey = "kivo.capabilities";

// The cache is keyed by the user agent, which changes with every browser
// update, and by this module's URL, which carries the build's content hash.
// A build whose rules changed therefore asks again.
const cacheKey = () => `${navigator.userAgent}\n${import.meta.url}`;

function cached(): Capabilities | undefined {
  const stored = readStoredJson(storageKey) as { key: string; capabilities: Capabilities } | null | undefined;
  return stored?.key === cacheKey() ? stored.capabilities : undefined;
}

let reported: Capabilities | undefined;

/** What this browser plays: cached, or asked once per page. */
function browserReport(): Capabilities {
  reported ??= cached();
  if (!reported) {
    reported = report(browserProbe(), currentBrowser());
    writeStoredJson(storageKey, { key: cacheKey(), capabilities: reported });
  }
  return reported;
}

/**
 * capabilityReport is this browser's capability report, capped at the
 * channels it outputs. A play in picture-in-picture gets no external
 * subtitles, since nothing drawn over the video shows there. The speakers
 * and the setting are read on every call, so a change to either applies to
 * the next play.
 */
export async function capabilityReport({ pictureInPicture = false } = {}): Promise<Capabilities> {
  const caps = browserReport();
  const capped = capChannels(
    caps,
    outputChannels(deviceSettings().maxAudioChannels, caps, currentBrowser(), isTv, speakerCount()),
  );
  return pictureInPicture ? withoutExternal(capped) : capped;
}

/** Asks the browser when it's idle, so the first Play doesn't wait for it. */
export function reportWhenIdle(): void {
  const run = () => void browserReport();
  if (typeof requestIdleCallback === "function") requestIdleCallback(run, { timeout: 5000 });
  else setTimeout(run, 1000);
}
