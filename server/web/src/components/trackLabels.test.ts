import { describe, expect, it } from "vitest";
import type { Track, Version } from "@/api/schemas";
import { languageName, output, trackDetail, trackNames, versionName } from "./trackLabels";

type Flags = Partial<Pick<Track, "forced" | "hearingImpaired" | "commentary" | "original" | "default">> & {
  profile?: NonNullable<Track["audio"]>["profile"];
};

function audio(
  id: string,
  codec: string,
  language: string | undefined,
  title: string | undefined,
  channels: number,
  layout?: string,
  { profile, ...flags }: Flags = {},
): Track {
  return {
    id,
    type: "audio",
    codec,
    default: false,
    ...flags,
    language,
    title,
    audio: { channels, layout, profile },
  };
}

function subtitle(id: string, codec: string, language: string | undefined, title?: string, flags: Flags = {}): Track {
  return {
    id,
    type: "subtitle",
    codec,
    default: false,
    ...flags,
    language,
    title,
    subtitle: { source: "embedded" },
  };
}

const names = (tracks: Track[]) => [...trackNames(tracks).values()];

// The tracks of a Blu-ray rip of Tremors (1990): five English audio tracks
// and eighteen subtitle tracks.
const tremorsAudio = [
  audio("a1", "eac3", "eng", "E-AC3 DD+5.1 1024kbs", 6, "5.1", { default: true }),
  audio("a2", "eac3", "eng", "E-AC3 DD+4.0 1024kbs Surround Mix", 4, "4.0"),
  audio("a3", "flac", "eng", "Flac 2.0 Original Stereo Audio 627 kb/s", 2, "2.0"),
  audio(
    "a4",
    "ac3",
    "eng",
    "Commentary by director Ron Underwood, and writers/producers Brent Maddock and S.S. Wilson",
    2,
    "2.0",
    { commentary: true },
  ),
  audio(
    "a5",
    "ac3",
    "eng",
    "Commentary by Jonathan Melville (author of Seeking Perfection: The Unofficial Guide to Tremors)",
    2,
    "2.0",
    { commentary: true },
  ),
];

const pgs = "hdmv_pgs_subtitle";
const tremorsSubtitles = [
  subtitle("s01", "subrip", "eng", "Full .srt"),
  subtitle("s02", "subrip", "eng", "SDH .srt", { hearingImpaired: true }),
  subtitle("s03", pgs, "jpn"),
  subtitle("s04", pgs, "fre"),
  subtitle("s05", pgs, "ita"),
  subtitle("s06", pgs, "ger"),
  subtitle("s07", pgs, "spa", "Castilian"),
  subtitle("s08", pgs, "dut"),
  subtitle("s09", pgs, "chi", "Simplified"),
  subtitle("s10", pgs, "dan"),
  subtitle("s11", pgs, "fin"),
  subtitle("s12", pgs, "gre"),
  subtitle("s13", pgs, "nor"),
  subtitle("s14", pgs, "ice"),
  subtitle("s15", pgs, "kor"),
  subtitle("s16", pgs, "por", "Portuguese (Portugal)"),
  subtitle("s17", pgs, "swe"),
  subtitle("s18", pgs, "chi", "Traditional"),
];

describe("trackNames", () => {
  it("names audio by its language, flags and channels, with the file's title under it", () => {
    expect(names(tremorsAudio)).toEqual([
      { name: "English · 5.1", note: "E-AC3 DD+5.1 1024kbs" },
      { name: "English · 4.0", note: "E-AC3 DD+4.0 1024kbs Surround Mix" },
      { name: "English · 2.0 · FLAC", note: "Flac 2.0 Original Stereo Audio 627 kb/s" },
      {
        name: "English · Commentary · 2.0",
        note: "Commentary by director Ron Underwood, and writers/producers Brent Maddock and S.S. Wilson",
      },
      {
        name: "English · Commentary · 2.0",
        note: "Commentary by Jonathan Melville (author of Seeking Perfection: The Unofficial Guide to Tremors)",
      },
    ]);
  });

  it("names subtitles by language and flags, reading ISO 639-2 codes, with the file's title under it", () => {
    expect(names(tremorsSubtitles)).toEqual([
      { name: "English", note: "Full .srt" },
      { name: "English · SDH", note: "SDH .srt" },
      { name: "Japanese" },
      { name: "French" },
      { name: "Italian" },
      { name: "German" },
      { name: "Spanish", note: "Castilian" },
      { name: "Dutch" },
      { name: "Chinese", note: "Simplified" },
      { name: "Danish" },
      { name: "Finnish" },
      { name: "Greek" },
      { name: "Norwegian" },
      { name: "Icelandic" },
      { name: "Korean" },
      { name: "Portuguese", note: "Portuguese (Portugal)" },
      { name: "Swedish" },
      { name: "Chinese", note: "Traditional" },
    ]);
  });

  it("always names a lossless or premium format, and other codecs only to tell tracks apart", () => {
    expect(
      names([
        audio("t", "truehd", "eng", undefined, 8, "7.1", { profile: "atmos" }),
        audio("m", "dts", "eng", "DTS-HD MA 5.1", 6, "5.1", { profile: "dtsHdMa" }),
        audio("x", "dts", "eng", undefined, 8, "7.1", { profile: "dtsX" }),
        audio("e", "eac3", "eng", undefined, 6, "5.1", { profile: "atmos" }),
        audio("d", "eac3", "eng", undefined, 6, "5.1"),
        audio("a", "ac3", "eng", undefined, 6, "5.1"),
      ]).map((n) => n.name),
    ).toEqual([
      "English · 7.1 · TrueHD Atmos",
      "English · 5.1 · DTS-HD MA",
      "English · 7.1 · DTS:X",
      "English · 5.1 · E-AC-3 Atmos",
      "English · 5.1 · E-AC-3",
      "English · 5.1 · AC-3",
    ]);
  });

  it("marks the original beside a dub", () => {
    expect(
      names([
        audio("j", "aac", "jpn", undefined, 2, "2.0", { original: true }),
        audio("e", "aac", "eng", undefined, 2, "2.0"),
      ]),
    ).toEqual([{ name: "Japanese [Original] · 2.0" }, { name: "English · 2.0" }]);
  });

  it("names a track from its facts alone and shows its title as written", () => {
    const shown = (track: Track) => names([track])[0];
    expect(shown(subtitle("f", "subrip", "eng", "English [Forced]", { forced: true }))).toEqual({
      name: "English · Forced",
      note: "English [Forced]",
    });
    expect(
      shown(subtitle("l", "subrip", "spa", "  Español (Latinoamérica) (SDH) ", { hearingImpaired: true })),
    ).toEqual({
      name: "Spanish · SDH",
      note: "Español (Latinoamérica) (SDH)",
    });
    expect(shown(audio("a", "dts", "eng", "English DTS 5.1 @ 1509 Kbps", 6, "5.1"))).toEqual({
      name: "English · 5.1",
      note: "English DTS 5.1 @ 1509 Kbps",
    });
    expect(shown(subtitle("s", "subrip", "eng", "Forced"))).toEqual({ name: "English", note: "Forced" });
    expect(shown(subtitle("w", "subrip", "eng", "   "))).toEqual({ name: "English" });
    expect(shown(audio("m", "aac", "eng", undefined, 1, "1.0"))).toEqual({ name: "English · Mono" });
  });

  it("cuts a long title at a word", () => {
    const credits = `Audio commentary by ${"Director James Cameron, Producer Gale Anne Hurd, ".repeat(4)}`;
    expect(names([audio("c", "aac", "eng", credits, 2, "2.0", { commentary: true })])[0]?.note).toBe(
      "Audio commentary by Director James Cameron, Producer Gale Anne Hurd, Director James Cameron,…",
    );
  });

  it("tells an embedded subtitle from a file beside it, then numbers what's left", () => {
    const external: Track = { ...subtitle("x", "subrip", "eng"), subtitle: { source: "external" } };
    expect(names([subtitle("e", "subrip", "eng"), external]).map((n) => n.name)).toEqual([
      "English",
      "English · External",
    ]);
    expect(
      names([subtitle("x", "subrip", "fre"), subtitle("y", "subrip", "fre"), subtitle("z", "ass", "fre")]).map(
        (n) => n.name,
      ),
    ).toEqual(["French · SRT · 1", "French · SRT · 2", "French · ASS"]);
  });

  it("names where a downloaded subtitle came from, with its release under it", () => {
    const downloaded = (id: string, title?: string, flags: Partial<Track> = {}): Track => ({
      ...subtitle(id, "subrip", "eng", title),
      ...flags,
      subtitle: { source: "downloaded" },
    });
    expect(
      names([subtitle("e", "subrip", "eng", "Full"), downloaded("d", "Modern.Family.S03E16.480p.BluRay.x264-mSD")]),
    ).toEqual([
      { name: "English", note: "Full" },
      { name: "English · OpenSubtitles", note: "Modern.Family.S03E16.480p.BluRay.x264-mSD" },
    ]);
    expect(
      names([
        downloaded("s", "Dune.2021.SDH", { hearingImpaired: true }),
        downloaded("f", "Dune.2021", { forced: true }),
      ]),
    ).toEqual([
      { name: "English · SDH · OpenSubtitles", note: "Dune.2021.SDH" },
      { name: "English · Forced · OpenSubtitles", note: "Dune.2021" },
    ]);
    expect(names([downloaded("a"), downloaded("b")])).toEqual([
      { name: "English · OpenSubtitles · 1" },
      { name: "English · OpenSubtitles · 2" },
    ]);
    const ass: Track = { ...subtitle("d", "ass", "eng"), subtitle: { source: "downloaded" } };
    expect(
      names([downloaded("a"), downloaded("b", undefined, { hearingImpaired: true }), downloaded("c"), ass]).map(
        (n) => n.name,
      ),
    ).toEqual([
      "English · OpenSubtitles · SRT · 1",
      "English · SDH · OpenSubtitles",
      "English · OpenSubtitles · SRT · 2",
      "English · OpenSubtitles · ASS",
    ]);
    expect(names([subtitle("e", "subrip", "eng"), downloaded("d")]).map((n) => n.name)).toEqual([
      "English",
      "English · OpenSubtitles",
    ]);
  });

  it("tells tracks apart only when their titles are the same too", () => {
    expect(names([subtitle("a", "subrip", "eng", "Signs"), subtitle("b", "ass", "eng", "Dialogue")])).toEqual([
      { name: "English", note: "Signs" },
      { name: "English", note: "Dialogue" },
    ]);
    expect(names([subtitle("a", "subrip", "eng", "Signs"), subtitle("b", "ass", "eng", "Signs")])).toEqual([
      { name: "English · SRT", note: "Signs" },
      { name: "English · ASS", note: "Signs" },
    ]);
  });

  it("names a track without a language", () => {
    expect(names([subtitle("u", "subrip", undefined)])[0]?.name).toBe("Unknown language");
    expect(names([subtitle("n", "subrip", "null")])[0]?.name).toBe("Unknown language");
    expect(names([audio("c", "aac", "und", "Commentary", 2, "2.0", { commentary: true })])[0]).toEqual({
      name: "Unknown language · Commentary · 2.0",
      note: "Commentary",
    });
    expect(names([subtitle("t", "subrip", undefined, "English")])[0]).toEqual({
      name: "Unknown language",
      note: "English",
    });
  });
});

describe("trackDetail", () => {
  it("describes tracks in full", () => {
    expect(trackDetail(tremorsAudio[0] as Track)).toBe("English · E-AC-3 5.1 · Default · “E-AC3 DD+5.1 1024kbs”");
    expect(trackDetail(tremorsAudio[2] as Track)).toBe(
      "English · FLAC 2.0 · “Flac 2.0 Original Stereo Audio 627 kb/s”",
    );
    expect(trackDetail(audio("t", "truehd", "eng", undefined, 8, "7.1", { profile: "atmos" }))).toBe(
      "English · TrueHD Atmos 7.1",
    );
    expect(trackDetail(subtitle("s", "subrip", "eng", "SDH .srt", { hearingImpaired: true }))).toBe(
      "English · SDH · SRT · Embedded · “SDH .srt”",
    );
    const video: Track = {
      id: "v",
      type: "video",
      codec: "hevc",
      default: true,
      video: { width: 3840, height: 1600, resolution: "4k", profile: "Main 10", hdr: "dolbyVision" },
    };
    expect(trackDetail(video)).toBe("HEVC Main 10 4K Dolby Vision");
  });

  it("falls back to the channel count without a layout", () => {
    expect(trackDetail(audio("a", "truehd", "eng", undefined, 8))).toBe("English · TrueHD 7.1");
    expect(trackDetail(audio("a", "aac", "eng", undefined, 3))).toBe("English · AAC 3 channels");
  });
});

describe("output", () => {
  it("names a transcode's video by resolution and its audio by channels", () => {
    expect(output({ codec: "h264", resolution: "1080p", width: 1920, height: 1080 })).toBe("H.264 1080p");
    expect(output({ codec: "aac", channels: 6 })).toBe("AAC 5.1");
    expect(output({ codec: "aac", channels: 2 })).toBe("AAC Stereo");
  });
});

describe("versionName", () => {
  const video: Track = { id: "v", type: "video", codec: "hevc", default: true, video: { width: 3840, height: 2160 } };
  const version = (v: Partial<Version>, tracks: Track[]): Version => ({
    id: "1",
    available: true,
    durationMs: 1,
    sizeBytes: 1,
    parts: [],
    workIds: [],
    tracks,
    ...v,
  });

  it("names a version by resolution and HDR", () => {
    expect(versionName(version({ resolution: "4k", hdr: "hdr10" }, [video]))).toBe("4K HDR");
    expect(versionName(version({ resolution: "4k", hdr: "hlg" }, [video]))).toBe("4K HDR");
    expect(versionName(version({ resolution: "4k", hdr: "dolbyVision" }, [video]))).toBe("4K Dolby Vision");
    expect(versionName(version({ resolution: "1080p" }, [video]))).toBe("1080p");
  });

  it("names a version without video Audio, and one with video but no resolution not at all", () => {
    expect(versionName(version({}, [audio("a", "aac", "eng", undefined, 2)]))).toBe("Audio");
    expect(versionName(version({}, [video]))).toBe("");
  });
});

describe("languageName", () => {
  it("reads BCP 47 tags and ISO 639-2 codes", () => {
    expect(languageName("zh-Hant-TW")).toBe("Chinese (Traditional, Taiwan)");
    expect(languageName("en-US")).toBe("American English");
    const codes = { chi: "Chinese", ger: "German", fre: "French", dut: "Dutch", gre: "Greek", ice: "Icelandic" };
    const more = { per: "Persian", rum: "Romanian", slo: "Slovak", baq: "Basque", eng: "English" };
    for (const [code, name] of Object.entries({ ...codes, ...more })) expect(languageName(code)).toBe(name);
    expect(languageName("und")).toBe("Unknown language");
  });
});
