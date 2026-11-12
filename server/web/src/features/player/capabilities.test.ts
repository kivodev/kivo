import { describe, expect, it } from "vitest";
import type { Capabilities } from "@/api/schemas";
import { type Browser, browserOf } from "@/lib/browser";
import { capChannels, outputChannels, type Probe, report, withoutExternal } from "./capabilities";

const chromeUa =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";
const edgeUa = `${chromeUa} Edg/141.0.0.0`;
const androidUa =
  "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36";
const firefoxUa = "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0";
const firefoxMacUa = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:143.0) Gecko/20100101 Firefox/143.0";
const safariUa =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15";
const iphoneUa =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1";

const chrome = browserOf(chromeUa, "Google Inc.");
const edge = browserOf(edgeUa, "Google Inc.");
const android = browserOf(androidUa, "Google Inc.");
const firefox = browserOf(firefoxUa, "");
const firefoxMac = browserOf(firefoxMacUa, "");
const safari = browserOf(safariUa, "Apple Computer, Inc.");
const iphone = browserOf(iphoneUa, "Apple Computer, Inc.");
const tizenUa =
  "Mozilla/5.0 (SMART-TV; LINUX; Tizen 6.5) AppleWebKit/537.36 (KHTML, like Gecko) 85.0.4183.93/6.5 TV Safari/537.36";
const tizen = browserOf(tizenUa, "");
const tizen8 = browserOf(tizenUa.replaceAll("6.5", "8.0").replace("85.0.4183.93", "108.0.5359.1"), "");

/** A probe whose elements play the listed types, and the types whose codecs start with one listed. */
function probe(plays: string[], options: Partial<Probe> = {}): Probe {
  const can = (type: string) => plays.some((p) => type === p || type.includes(`"${p}`));
  return {
    canPlay: can,
    canPlayAudio: can,
    hls: true,
    audioTracks: false,
    assRenderer: true,
    tv: false,
    videoOnlyFullscreen: false,
    packaged: false,
    fullHdPanel: false,
    hdrPanel: undefined,
    avplay: false,
    ...options,
  };
}

const codecs = (caps: Capabilities) => caps.video.map((v) => v.codec);
const entry = (caps: Capabilities, codec: string) => caps.video.find((v) => v.codec === codec);
const audio = (caps: Capabilities, codec: string) => caps.audio.find((a) => a.codec === codec);

describe("report", () => {
  const h264 = ['video/mp4; codecs="avc1.42E01E, mp4a.40.2"', 'video/mp4; codecs="avc1.640029, mp4a.40.2"'];

  it("plays H.264 from a file and a stream, at level 4.2 unless a higher one answers", () => {
    const caps = report(probe([...h264, "video/mp4"]), chrome);
    expect(entry(caps, "h264")).toEqual({
      codec: "h264",
      delivery: ["file", "stream"],
      containers: ["mp4"],
      profiles: ["High", "Main", "Baseline", "Constrained Baseline"],
      maxLevel: 4.2,
    });
    expect(entry(report(probe([...h264, "avc1.640833"]), chrome), "h264")?.maxLevel).toBe(5.1);
    expect(entry(report(probe([...h264, "avc1.640834"]), chrome), "h264")?.maxLevel).toBe(5.2);
  });

  it("adds High 10 only where it plays and the browser isn't one that fails it", () => {
    const plays = [...h264, "avc1.6e0033"];
    expect(entry(report(probe(plays), chrome), "h264")?.profiles).toContain("High 10");
    for (const p of [safari, edge, android])
      expect(entry(report(probe(plays), p), "h264")?.profiles).not.toContain("High 10");
  });

  it("finds HEVC's profiles and level from the highest Main 10 level that answers", () => {
    const main = entry(report(probe(["hvc1.1.L120"]), chrome), "hevc");
    expect(main).toMatchObject({ profiles: ["Main"], maxLevel: 4 });
    const main10 = entry(report(probe(["hev1.1.L120", "hvc1.2.4.L123", "hev1.2.4.L153"]), chrome), "hevc");
    expect(main10).toMatchObject({ profiles: ["Main", "Main 10"], maxLevel: 5.1 });
  });

  it("streams HEVC only on the browsers that play it in fragmented MP4 HLS", () => {
    const plays = ["hvc1.1.L120"];
    expect(entry(report(probe(plays), chrome), "hevc")?.delivery).toEqual(["file", "stream"]);
    expect(
      entry(report(probe(plays), browserOf(androidUa.replace("Chrome/141", "Chrome/104"), "")), "hevc")?.delivery,
    ).toEqual(["file"]);
    expect(entry(report(probe(plays), firefox), "hevc")?.delivery).toEqual(["file", "stream"]);
    expect(
      entry(report(probe(plays), browserOf(firefoxUa.replace("Firefox/143", "Firefox/133"), "")), "hevc")?.delivery,
    ).toEqual(["file"]);
  });

  it("limits Safari's HEVC to the hvc1 and dvh1 tags, at 60 frames a second", () => {
    expect(entry(report(probe(["hvc1.1.L120"]), safari), "hevc")).toMatchObject({
      tags: ["hvc1", "dvh1"],
      maxFrameRate: 60,
    });
    expect(entry(report(probe(["hvc1.1.L120"]), chrome), "hevc")?.tags).toBeUndefined();
  });

  it("declares HDR on the browsers that tone-map it themselves", () => {
    const plays = ["hvc1.1.L120"];
    for (const p of [chrome, safari, iphone, firefoxMac, edge]) {
      expect(entry(report(probe(plays), p), "hevc")?.hdr).toEqual(["hdr10", "hdr10plus", "hlg"]);
    }
    for (const p of [android, firefox, browserOf(edgeUa.replace("Edg/141", "Edg/120"), "")]) {
      expect(entry(report(probe(plays), p), "hevc")?.hdr).toBeUndefined();
    }
  });

  it("declares the Dolby Vision profiles that answer, only on Safari", () => {
    const plays = ["hvc1.1.L120", "dvh1.05.06", "dvh1.08.06", "av01.0.15M.08", "av01.0.15M.10", "dav1.10.06"];
    const caps = report(probe(plays), safari);
    expect(entry(caps, "hevc")?.dolbyVision).toEqual([{ profile: 5 }, { profile: 8 }]);
    expect(entry(caps, "av1")?.dolbyVision).toEqual([{ profile: 10 }]);
    expect(entry(report(probe(plays), chrome), "hevc")?.dolbyVision).toBeUndefined();
  });

  it("plays AV1 at the highest level that answers, and streams it on no mobile browser but Safari", () => {
    const av1 = ["av01.0.15M.08", "av01.0.15M.10"];
    expect(entry(report(probe(av1), chrome), "av1")).toMatchObject({ delivery: ["file", "stream"], maxLevel: 5.3 });
    expect(entry(report(probe([...av1, "av01.0.17M"]), chrome), "av1")?.maxLevel).toBe(6.1);
    expect(entry(report(probe(av1), android), "av1")?.delivery).toEqual(["file"]);
    expect(entry(report(probe(av1), iphone), "av1")?.delivery).toEqual(["file", "stream"]);
  });

  it("plays VP9 in MP4 except on iOS and Firefox on a Mac, and WebM except on Safari 17 and later", () => {
    const vp9 = ['video/webm; codecs="vp9"'];
    expect(entry(report(probe(vp9), chrome), "vp9")?.containers).toEqual(["mp4", "webm"]);
    expect(entry(report(probe(vp9), iphone), "vp9")?.containers).toEqual([]);
    expect(entry(report(probe(vp9), firefoxMac), "vp9")?.containers).toEqual(["webm"]);
    expect(entry(report(probe(vp9), safari), "vp9")?.containers).toEqual(["mp4"]);
  });

  it("plays MKV where the element says so, never on Firefox, and always on Edge", () => {
    const plays = [...h264, "video/x-matroska"];
    expect(entry(report(probe(plays), chrome), "h264")?.containers).toEqual(["mp4", "mkv"]);
    expect(entry(report(probe(plays), firefox), "h264")?.containers).toEqual(["mp4"]);
    expect(entry(report(probe(h264), edge), "h264")?.containers).toEqual(["mp4", "mkv"]);
  });

  it("streams only the audio that plays in fragmented MP4 HLS", () => {
    const plays = [...h264, "avc1.640029, mp4a.69", "ac-3", "ec-3"];
    const caps = report(probe(plays), safari);
    expect(audio(caps, "aac")?.delivery).toEqual(["file", "stream"]);
    expect(audio(caps, "mp3")?.delivery).toEqual(["file"]);
    expect(audio(caps, "ac3")?.delivery).toEqual(["file"]);
    expect(audio(caps, "eac3")?.delivery).toEqual(["file"]);
    expect(audio(report(probe(plays), edge), "eac3")?.delivery).toEqual(["file", "stream"]);
    const native = report(probe([...plays, 'application/vnd.apple.mpegURL; codecs="avc1.42E01E, ac-3"']), safari);
    expect(audio(native, "ac3")?.delivery).toEqual(["file", "stream"]);
  });

  it("plays AAC's other profiles only, where the browser can't decode HE-AAC", () => {
    expect(audio(report(probe(h264), chrome), "aac")?.profiles).toEqual(["LC", "Main", "LTP"]);
    const he = report(probe([...h264, 'video/mp4; codecs="avc1.640029, mp4a.40.5"']), chrome);
    expect(audio(he, "aac")?.profiles).toBeUndefined();
  });

  it("plays Opus in stereo on Safari 17 and later, through CAF", () => {
    expect(audio(report(probe(['audio/x-caf; codecs="opus"']), safari), "opus")).toMatchObject({ maxChannels: 2 });
    expect(audio(report(probe(['audio/ogg; codecs="opus"']), chrome), "opus")).toMatchObject({ maxChannels: 8 });
  });

  it("plays ALAC on Apple's browsers, and MP2 where MP3 plays on the desktop", () => {
    expect(audio(report(probe([]), iphone), "alac")).toBeDefined();
    expect(audio(report(probe([]), chrome), "alac")).toBeUndefined();
    expect(audio(report(probe(["avc1.640029, mp4a.69"]), chrome), "mp2")).toBeDefined();
    expect(audio(report(probe(["avc1.640029, mp4a.69"]), android), "mp2")).toBeUndefined();
  });

  it("lists file delivery only where the browser plays no HLS, and no WebVTT stream", () => {
    const caps = report(probe([...h264], { hls: false }), chrome);
    expect(entry(caps, "h264")?.delivery).toEqual(["file"]);
    expect(caps.subtitles.find((s) => s.codec === "webvtt")?.delivery).toEqual(["external"]);
  });

  it("draws ASS only where JASSUB runs, and only WebVTT on a TV or where fullscreen shows only the video", () => {
    const drawn = (options: Partial<Probe>) => report(probe([], options), chrome).subtitles.map((s) => s.codec);
    expect(drawn({})).toEqual(["webvtt", "ass", "hdmv_pgs_subtitle"]);
    expect(drawn({ assRenderer: false })).toEqual(["webvtt", "hdmv_pgs_subtitle"]);
    expect(drawn({ tv: true })).toEqual(["webvtt"]);
    expect(drawn({ videoOnlyFullscreen: true })).toEqual(["webvtt"]);
  });

  it("switches a file's audio tracks where the element can, except on Firefox", () => {
    expect(report(probe([], { audioTracks: true }), chrome).file?.trackSelection).toBe(true);
    expect(report(probe([], { audioTracks: true }), firefox).file?.trackSelection).toBe(false);
  });

  it("plays files in its codecs' containers, and books in the audio types that answer", () => {
    expect(report(probe([...h264, 'video/webm; codecs="vp9"', "audio/mpeg"]), chrome).file?.containers).toEqual([
      "mp4",
      "webm",
      "m4a",
      "mp3",
    ]);
    expect(codecs(report(probe([]), chrome))).toEqual([]);
  });
});

describe("report on a Samsung TV", () => {
  const h264 = ['video/mp4; codecs="avc1.42E01E, mp4a.40.2"', 'video/mp4; codecs="avc1.640029, mp4a.40.2"'];

  it("plays what Jellyfin's Tizen rules say, whatever canPlayType answers", () => {
    const caps = report(probe(h264, { tv: true }), tizen);
    expect(codecs(caps)).toEqual(["h264", "h264", "hevc", "av1", "vc1", "mpeg2video"]);
    expect(entry(caps, "h264")).toMatchObject({ containers: ["mp4", "ts", "m2ts", "mkv"], maxLevel: 5.1 });
    expect(entry(caps, "hevc")).toMatchObject({
      delivery: ["file", "stream"],
      containers: ["mp4", "ts", "m2ts", "mkv"],
      hdr: ["hdr10", "hdr10plus", "hlg"],
      doviFallback: true,
    });
    expect(audio(caps, "eac3")?.delivery).toEqual(["file", "stream"]);
    expect(audio(caps, "flac")).toMatchObject({ delivery: ["file"], containers: ["flac"] });
    expect(caps.audio.map((a) => a.codec)).toEqual(
      expect.arrayContaining(["ac3", "mp2", "pcm_s16le", "pcm_s24le", "aac_latm", "vorbis"]),
    );
    expect(audio(caps, "dts")).toBeUndefined();
  });

  it("plays H.264 at level 5.2 in the packaged app from Tizen 5, and AV1 from Tizen 5.5", () => {
    expect(entry(report(probe(h264, { packaged: true }), tizen), "h264")?.maxLevel).toBe(5.2);
    expect(codecs(report(probe(h264), browserOf("Mozilla/5.0 (SMART-TV; Tizen 5.0) TV", "")))).not.toContain("av1");
  });

  it("caps video at 20 Mb/s on a 1080p panel", () => {
    const caps = report(probe(h264, { fullHdPanel: true }), tizen);
    expect(caps.video.filter((v) => v.maxBitrateBps === 20_000_000).map((v) => v.codec)).toEqual([
      "h264",
      "h264",
      "hevc",
      "av1",
    ]);
    expect(entry(report(probe(h264), tizen), "h264")?.maxBitrateBps).toBeUndefined();
  });

  it("plays Opus files only in MKV and WebM, and switches audio tracks only from Tizen 5.5 to 7", () => {
    expect(audio(report(probe(['audio/ogg; codecs="opus"']), tizen), "opus")?.containers).toEqual(["mkv", "webm"]);
    expect(report(probe([], { audioTracks: true }), tizen).file?.trackSelection).toBe(true);
    const eight = browserOf("Mozilla/5.0 (SMART-TV; Tizen 8.0) TV", "");
    expect(report(probe([], { audioTracks: true }), eight).file?.trackSelection).toBe(false);
  });
});

describe("report on a Samsung TV from Tizen 8", () => {
  const caps = report(probe([], { tv: true, packaged: true, avplay: true }), tizen8);
  // The web client in Samsung's browser, which plays through the video element.
  const browser = report(probe([], { tv: true }), tizen8);
  const h264 = (d: string) => caps.video.filter((v) => v.codec === "h264" && v.delivery.includes(d as "file"));

  it("with AVPlay, plays HEVC Main 10 to level 5.1 by Samsung's specification, whatever canPlayType answers", () => {
    expect(entry(caps, "hevc")).toMatchObject({
      delivery: ["file", "stream"],
      containers: ["mp4", "ts", "m2ts", "mkv"],
      profiles: ["Main", "Main 10"],
      maxLevel: 5.1,
      hdr: ["hdr10", "hdr10plus", "hlg"],
    });
    const higher = report(probe(["hvc1.2.4.L156"], { packaged: true, avplay: true }), tizen8);
    expect(entry(higher, "hevc")?.maxLevel).toBe(5.2);
  });

  it("in the video element, plays the HEVC and VP9 canPlayType answers for", () => {
    expect(entry(browser, "hevc")).toMatchObject({ profiles: ["Main"], maxLevel: 4 });
    expect(entry(browser, "vp9")).toBeUndefined();
    const answered = report(probe(["hev1.2.4.L153", 'video/webm; codecs="vp9"'], { tv: true }), tizen8);
    expect(entry(answered, "hevc")).toMatchObject({ profiles: ["Main", "Main 10"], maxLevel: 5.1 });
    expect(entry(answered, "vp9")?.profiles).toEqual(["Profile 0", "Profile 2"]);
  });

  it("plays H.264 to 4K from a file, and to 1080p in a stream", () => {
    expect(h264("file")).toEqual([expect.objectContaining({ delivery: ["file"], maxLevel: 5.2 })]);
    expect(h264("stream")).toEqual([
      expect.objectContaining({ delivery: ["stream"], maxLevel: 4.2, maxWidth: 1920, maxHeight: 1080 }),
    ]);
    expect(h264("stream")[0]?.containers).toBeUndefined();
    const noHls = report(probe([], { hls: false }), tizen8).video.filter((v) => v.codec === "h264");
    expect(noHls).toEqual([expect.objectContaining({ delivery: ["file"], maxLevel: 5.1 })]);
  });

  it("caps both H.264 entries at 20 Mb/s on a 1080p panel", () => {
    const fhd = report(probe([], { fullHdPanel: true }), tizen8);
    expect(fhd.video.filter((v) => v.codec === "h264").map((v) => v.maxBitrateBps)).toEqual([20_000_000, 20_000_000]);
  });

  it("with AVPlay, plays AV1 and VP9 profiles 0 and 2", () => {
    expect(entry(caps, "av1")).toMatchObject({ profiles: ["Main"], maxLevel: 5.3 });
    expect(entry(caps, "vp9")).toMatchObject({ delivery: ["file"], profiles: ["Profile 0", "Profile 2"] });
  });

  it("declares HDR unless the panel says it shows none", () => {
    const sdr = report(probe([], { hdrPanel: false }), tizen8);
    for (const codec of ["hevc", "av1", "vp9"]) {
      expect(entry(sdr, codec)?.hdr).toBeUndefined();
      expect(entry(sdr, codec)?.doviFallback).toBeUndefined();
    }
    expect(entry(report(probe([], { hdrPanel: true }), tizen8), "hevc")?.hdr).toEqual(["hdr10", "hdr10plus", "hlg"]);
  });

  it("plays Samsung's audio, E-AC-3 and Vorbis in 5.1 at most, and no DTS or TrueHD", () => {
    expect(audio(caps, "aac")).toMatchObject({ delivery: ["file", "stream"], maxChannels: 8 });
    expect(audio(caps, "aac")?.profiles).toBeUndefined();
    expect(audio(caps, "mp3")?.delivery).toEqual(["file"]);
    expect(audio(caps, "eac3")).toMatchObject({ delivery: ["file", "stream"], maxChannels: 6 });
    expect(audio(caps, "vorbis")?.maxChannels).toBe(6);
    expect(audio(caps, "flac")).toMatchObject({ delivery: ["file"], containers: ["flac"] });
    expect(audio(caps, "dts")).toBeUndefined();
    expect(audio(caps, "truehd")).toBeUndefined();
  });

  it("with AVPlay, switches a file's audio tracks and plays Opus in any container", () => {
    const native = report(probe([], { packaged: true, avplay: true }), tizen8);
    expect(native.file).toMatchObject({ trackSelection: true });
    expect(native.file?.containers).toEqual(expect.arrayContaining(["mkv", "mp4", "ts", "m2ts", "webm"]));
    expect(native.audio.filter((a) => a.codec === "opus")).toEqual([
      { codec: "opus", delivery: ["file"], maxChannels: 8 },
    ]);
    const streamed = report(probe(['audio/ogg; codecs="opus"'], { avplay: true }), tizen8);
    expect(streamed.audio.filter((a) => a.codec === "opus").map((a) => a.delivery)).toEqual([["file"], ["stream"]]);
    expect(browser.file?.trackSelection).toBe(false);
    expect(audio(browser, "opus")).toBeUndefined();
  });
});

describe("report on an LG TV", () => {
  const webos = browserOf(
    "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.6834.207 Safari/537.36 WebAppManager",
    "Google Inc.",
  );
  // What a 2024 LG's video element answers: no MKV and no DTS, though it plays both.
  const plays = [
    'video/mp4; codecs="avc1.42E01E, mp4a.40.2"',
    'video/mp4; codecs="avc1.640029, mp4a.40.2"',
    "hvc1.1.L120",
    "hvc1.2.4.L153",
    'audio/mp4; codecs="ac-3"',
    'audio/mp4; codecs="ec-3"',
  ];
  const tv = (options: Partial<Probe> = {}) => report(probe(plays, { tv: true, packaged: true, ...options }), webos);

  it("plays MKV files, which its video element doesn't answer for", () => {
    const caps = tv();
    expect(entry(caps, "h264")?.containers).toEqual(["mp4", "mkv"]);
    expect(entry(caps, "hevc")?.containers).toEqual(["mp4", "mkv"]);
    expect(caps.file?.containers).toContain("mkv");
  });

  it("plays DTS from a file, which its video element doesn't answer for", () => {
    expect(audio(tv(), "dts")).toEqual({ codec: "dts", delivery: ["file"], maxChannels: 8 });
    expect(audio(report(probe(plays), chrome), "dts")).toBeUndefined();
  });

  it("switches between a file's audio tracks", () => {
    expect(tv({ audioTracks: true }).file?.trackSelection).toBe(true);
  });

  it("declares HDR unless the panel says it shows none", () => {
    const hdr = ["hdr10", "hdr10plus", "hlg"];
    expect(entry(tv({ hdrPanel: true }), "hevc")?.hdr).toEqual(hdr);
    expect(entry(tv(), "hevc")?.hdr).toEqual(hdr);
    expect(entry(tv({ hdrPanel: false }), "hevc")?.hdr).toBeUndefined();
  });
});

describe("withoutExternal", () => {
  it("leaves out external subtitles, and any entry with nothing left", () => {
    const caps: Capabilities = {
      video: [],
      audio: [],
      subtitles: [
        { codec: "webvtt", delivery: ["stream", "external"] },
        { codec: "ass", delivery: ["external"] },
      ],
    };
    expect(withoutExternal(caps).subtitles).toEqual([{ codec: "webvtt", delivery: ["stream"] }]);
  });
});

describe("outputChannels", () => {
  const caps = (...names: string[]): Capabilities => ({
    file: { containers: ["mp4"] },
    video: [],
    audio: names.map((codec) => ({ codec, delivery: ["file", "stream"], maxChannels: 6 })),
    subtitles: [],
  });
  const other: Browser = { ...chrome, chrome: false };

  it("takes the browser's setting first", () => {
    expect(outputChannels(2, caps("aac", "eac3"), safari, false, 8)).toBe(2);
    expect(outputChannels(6, caps("aac"), chrome, false, 2)).toBe(6);
    expect(outputChannels(8, caps("aac", "eac3"), tizen8, true, 2)).toBe(8);
  });

  it("counts a surround browser that plays AC-3 or E-AC-3 as at least 5.1", () => {
    expect(outputChannels(undefined, caps("aac", "eac3"), safari, false, 2)).toBe(6);
    expect(outputChannels(undefined, caps("aac", "ac3"), chrome, false, 8)).toBe(8);
    expect(outputChannels(undefined, caps("aac", "ac3"), other, false, 2)).toBe(2);
  });

  it("then takes the speakers, stereo where the browser can't output more, and 5.1 where it says nothing", () => {
    expect(outputChannels(undefined, caps("aac"), chrome, false, 2)).toBe(2);
    expect(outputChannels(undefined, caps("aac"), firefox, false, 6)).toBe(6);
    expect(outputChannels(undefined, caps("aac"), other, false, 6)).toBe(2);
    expect(outputChannels(undefined, caps("aac"), chrome, false, 0)).toBe(6);
    expect(outputChannels(undefined, caps("aac"), other, false, 0)).toBe(2);
    expect(outputChannels(undefined, caps("aac"), other, true, 0)).toBe(6);
  });
});

describe("capChannels", () => {
  it("caps every audio entry at the browser's channels, never raising one", () => {
    const e = (codec: string, maxChannels: number) => ({ codec, delivery: ["file" as const], maxChannels });
    const capped = capChannels({ video: [], audio: [e("aac", 6), e("eac3", 6), e("mp3", 2)], subtitles: [] }, 2);
    expect(capped.audio).toEqual([e("aac", 2), e("eac3", 2), e("mp3", 2)]);
    expect(capChannels(capped, 6).audio.map((a) => a.maxChannels)).toEqual([2, 2, 2]);
  });
});
