import { i18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import aliases from "@shared/display/languages/aliases.json";
import type { PlayTrack, Track, Version } from "@/api/schemas";
import { display, lookup, media } from "@/i18n/display";
import { formatSize } from "@/i18n/format";
import { methodLabel, subtitleLabel, subtitleSource, trackMethod } from "@/i18n/playback";
import type { Option } from "./Select";

/**
 * A track's name in a picker. `name` goes on one line. `note` is the file's
 * own title, shown dimmed under it.
 */
export type TrackName = { name: string; note?: string };

const separator = " · ";

/** The longest note shown whole. A longer title is cut at a word and ends in an ellipsis. */
const noteLength = 100;

/** A codec as people know it: "TrueHD", "SRT". */
export function codecName(codec: string): string {
  return lookup(media.codecs, codec) ?? codec.toUpperCase();
}

/** A container as people know it: "matroska,webm" is MKV. */
export function containerName(container: string | undefined): string | undefined {
  if (!container) return undefined;
  const c = container.split(",")[0] ?? container;
  return lookup(media.containers, c) ?? c.toUpperCase();
}

/**
 * A language tag in words, in `locale`, which defaults to the active one.
 * A dialect uses its own name ("American English"). Otherwise the name is
 * the language with its script and region ("Chinese (Traditional,
 * Taiwan)"). An ISO 639-2 code is read as its language ("eng" is English).
 * A missing tag, und or "null" is the display table's unknown language. A
 * tag that Intl has no name for shows as written.
 */
export function languageName(tag: string | undefined, locale: string = i18n.locale): string {
  const unknown = display().tracks.unknownLanguage;
  if (!hasLanguage(tag)) return unknown;
  const [first = "", ...rest] = tag.split(/[-_]/);
  const language = first.toLowerCase();
  const alias = lookup(aliases, language);
  const canonical = alias ? (rest.length > 0 ? [alias.split("-")[0], ...rest] : [alias]) : [language, ...rest];
  if (canonical[0] === "und") return unknown;
  try {
    return new Intl.DisplayNames([locale], { type: "language", fallback: "none" }).of(canonical.join("-")) ?? tag;
  } catch {
    // A malformed tag, which Intl rejects, shows as written too.
    return tag;
  }
}

/** "Mono", "Stereo", "5.1" or "7.1" for a channel count, or else the count in words. */
export function channelLayout(channels: number): string {
  return lookup(media.channels, String(channels)) ?? display().tracks.channels.replace("{channels}", String(channels));
}

/**
 * A channel layout as the server sends it, such as "2.0" or "5.1". "1.0"
 * reads as "Mono". Without a layout, the channel count's name is used.
 */
function channelsName(layout: string | undefined, count: number): string {
  return layout ? (lookup(media.layouts, layout) ?? layout) : channelLayout(count);
}

/** A lossless or premium audio format, which a track's name always shows: "TrueHD Atmos", "DTS-HD MA", "FLAC". */
function premiumAudio(t: Track): string | undefined {
  if (!t.audio) return undefined;
  return (
    (t.audio.profile && lookup(media.premiumAudio, `${t.codec} ${t.audio.profile}`)) ??
    lookup(media.premiumAudio, t.codec)
  );
}

/** A resolution class in words: `4k` is "4K" and `sd` is "SD". */
function resolutionName(resolution: string): string {
  return lookup(media.resolutions, resolution) ?? resolution;
}

/** An HDR format: "HDR10", "HLG", "Dolby Vision". */
export function hdrName(hdr: string): string {
  return lookup(media.hdr, hdr) ?? hdr;
}

/** The language, marked as the original beside a dub, and the flags that matter: "English", "Japanese [Original]", "English · SDH". */
function baseName(t: Track): string[] {
  const flags = display().tracks.flags;
  const language = languageName(t.language);
  const parts = [t.original ? `${language} [${flags.original}]` : language];
  if (t.hearingImpaired) parts.push(flags.hearingImpaired);
  if (t.forced) parts.push(flags.forced);
  if (t.commentary) parts.push(flags.commentary);
  return parts;
}

/**
 * Names for the tracks of one kind, as a picker lists them, keyed by ID. A
 * name is built from the track's facts alone, never its title. It has:
 *  - the language, marked when it's the original beside a dub
 *    ("Japanese [Original]");
 *  - the flags ("English · SDH");
 *  - for audio, the channel layout and any lossless or premium format
 *    ("English · 7.1 · TrueHD Atmos", "English · Commentary · 2.0");
 *  - for a downloaded subtitle, where it came from ("English · OpenSubtitles").
 *
 * The note is the file's own title as written. For a downloaded subtitle,
 * that is its release name. When tracks share both name and note, each
 * adds its codec, then External or Downloaded for a subtitle file. Tracks
 * still alike after that are numbered.
 */
export function trackNames(tracks: readonly Track[]): Map<string, TrackName> {
  const parts = new Map<string, string[]>();
  const notes = new Map<string, string>();
  for (const t of tracks) {
    parts.set(t.id, nameParts(t));
    const title = t.title?.trim();
    if (title) notes.set(t.id, capped(title));
  }
  const partsOf = (t: Track) => parts.get(t.id) as string[];
  const key = (t: Track) => `${partsOf(t).join(separator)}\n${notes.get(t.id) ?? ""}`;
  const alike = () => {
    const groups = new Map<string, Track[]>();
    for (const t of tracks) groups.set(key(t), [...(groups.get(key(t)) ?? []), t]);
    return [...groups.values()].filter((g) => g.length > 1);
  };

  const telling: ((t: Track) => string | undefined)[] = [
    (t) => (premiumAudio(t) || t.codec === "" ? undefined : codecName(t.codec)),
    (t) => (t.subtitle && t.subtitle.source !== "embedded" ? subtitleSource(t.subtitle.source) : undefined),
  ];
  for (const fact of telling) {
    for (const group of alike()) {
      if (new Set(group.map(fact)).size > 1) {
        for (const t of group) {
          const value = fact(t);
          if (value !== undefined) partsOf(t).push(value);
        }
      }
    }
  }
  for (const group of alike()) {
    for (const [i, t] of group.entries()) partsOf(t).push(String(i + 1));
  }
  return new Map(tracks.map((t) => [t.id, { name: partsOf(t).join(separator), note: notes.get(t.id) }]));
}

/**
 * The basic parts of a track's name, before anything is added to tell it
 * apart from another track. These are the language and the flags, then
 * the channels and premium format for audio, or where a downloaded
 * subtitle came from.
 */
function nameParts(t: Track): string[] {
  const parts = baseName(t);
  if (t.audio) {
    parts.push(channelsName(t.audio.layout, t.audio.channels));
    const premium = premiumAudio(t);
    if (premium) parts.push(premium);
  }
  if (t.subtitle?.source === "downloaded") parts.push(display().tracks.downloadedFrom);
  return parts;
}

/** A title cut to the note's length at a word, ending in an ellipsis. */
function capped(title: string): string {
  if (title.length <= noteLength) return title;
  const cut = title.slice(0, noteLength);
  const space = cut.lastIndexOf(" ");
  return `${(space > noteLength / 2 ? cut.slice(0, space) : cut).trimEnd()}…`;
}

/**
 * A track in full, for stream info, followed by the file's title as
 * written.
 *  - Video: codec, profile, resolution and HDR ("HEVC Main 10 4K Dolby Vision").
 *  - Audio: language, flags, codec or premium format, channels, and Default
 *    ("English · E-AC-3 5.1 · Default", "English · TrueHD Atmos 7.1").
 *  - Subtitle: language, flags, codec and where it lives
 *    ("English · SDH · SRT · External").
 */
export function trackDetail(track: Track): string {
  const video = track.video;
  if (video) {
    return [
      codecName(track.codec),
      video.profile,
      video.resolution && resolutionName(video.resolution),
      video.hdr && hdrName(video.hdr),
    ]
      .filter(Boolean)
      .join(" ");
  }
  const parts = baseName(track);
  if (track.audio) {
    const format = premiumAudio(track) ?? codecName(track.codec);
    parts.push(`${format} ${channelsName(track.audio.layout, track.audio.channels)}`);
  }
  if (track.subtitle) {
    parts.push(codecName(track.codec));
    const source = lookup(display().tracks.sources, track.subtitle.source);
    if (source) parts.push(source);
  }
  if (track.default) parts.push(display().tracks.default);
  const title = track.title?.trim();
  if (title) parts.push(`“${title}”`);
  return parts.join(separator);
}

/** What a transcoded track becomes: its codec, a video's resolution class and encoder, and audio's channels. */
export type TrackOutput = {
  codec: string;
  width?: number;
  height?: number;
  bitrateBps?: number;
  resolution?: string;
  channels?: number;
  encoder?: string;
};

/**
 * What a transcoded track becomes. For video that is the codec and
 * resolution ("H.264 1080p"). For audio it is the codec and channels
 * ("AAC 5.1").
 */
export function output(out: TrackOutput): string {
  const second = out.resolution
    ? resolutionName(out.resolution)
    : out.channels !== undefined
      ? channelLayout(out.channels)
      : undefined;
  return [codecName(out.codec), second].filter(Boolean).join(" ");
}

/**
 * A version's name: its edition label, then the video's resolution and HDR
 * ("Final Cut · 4K HDR", "1080p", "4K Dolby Vision"), or Audio for a
 * version without video. It is empty for an untagged version with video
 * whose resolution the server didn't send.
 */
export function versionName(version: Version): string {
  return [version.edition, quality(version)].filter(Boolean).join(" · ");
}

/** A version's resolution and HDR, or Audio for a version without video. */
function quality(version: Version): string {
  if (!version.resolution) {
    return version.tracks.some((t) => t.type === "video") ? "" : display().tracks.audioOnly;
  }
  const hdr = version.hdr && (lookup(media.versionHdr, version.hdr) ?? hdrName(version.hdr));
  return [resolutionName(version.resolution), hdr].filter(Boolean).join(" ");
}

/** Whether a tag names a language. It must not be missing, or the literal "null" that some files carry. */
function hasLanguage(tag: string | undefined): tag is string {
  const t = tag?.trim().toLowerCase();
  return !!t && t !== "null";
}

/** A video's resolution class by its width, as the server classes a version's: a widescreen film is shorter than its class's height. */
function resolutionOf(width: number): string {
  if (width >= 3200) return "4k";
  if (width >= 1700) return "1080p";
  if (width >= 1200) return "720p";
  return "sd";
}

/** What a play's transcoded track becomes, or undefined for a track sent as it is. */
export function transcodeOutput(track: PlayTrack): TrackOutput | undefined {
  const t = track.transcode;
  if (!t) return undefined;
  return { ...t, codec: track.codec ?? "", resolution: t.width ? resolutionOf(t.width) : undefined };
}

/**
 * What a transcoded track becomes. For video it also names the encoder:
 * "H.264 1080p · Intel Quick Sync (QSV)". Audio with fewer channels than
 * its source says it's mixed down: "AAC 5.1 (downmix)".
 */
function outputLabel(track: PlayTrack, source: Track | undefined): string | undefined {
  const out = transcodeOutput(track);
  if (!out) return undefined;
  const label = output(out);
  if (track.type === "video") return [label, out.encoder].filter(Boolean).join(" · ");
  const from = source?.audio?.channels;
  return from && out.channels && out.channels < from ? i18n._(msg`${label} (downmix)`) : label;
}

/** A video or audio track's method with its source and output: "Transcode · English · TrueHD 7.1 → AAC 5.1 (downmix)". */
export function trackLine(track: PlayTrack, source: Track | undefined): string {
  const method = methodLabel(trackMethod(track));
  const what = [source && trackDetail(source), outputLabel(track, source)].filter(Boolean).join(" → ");
  return what ? `${method} · ${what}` : method;
}

const styledCodecs = ["ass", "ssa"];

/** Whether a styled source shows as plain text, which names it "Plain" without its codec. */
function shownPlain(shown: Pick<PlayTrack, "delivery" | "codec">, source: Track | undefined): boolean {
  return !!source && styledCodecs.includes(source.codec) && shown.delivery !== "burn" && shown.codec !== "ass";
}

/**
 * A subtitle as it shows: its language, flags, kind and where it lives
 * ("English · SDH · Styled (ASS) · Embedded"). The source's codec is named
 * unless a styled track shows as plain, because "Plain (ASS)" would read
 * as styled.
 */
export function subtitleLine(shown: Pick<PlayTrack, "delivery" | "codec">, track: Track | undefined): string {
  const kind = subtitleLabel(shown, track && !shownPlain(shown, track) ? codecName(track.codec) : undefined);
  return [...(track ? baseName(track) : []), kind, track?.subtitle && subtitleSource(track.subtitle.source)]
    .filter(Boolean)
    .join(separator);
}

/**
 * A track in brief, for a play's source.
 *  - Video: codec, resolution and HDR ("H.264 720p").
 *  - Audio: codec or premium format, and channels ("AC-3 5.1").
 *  - Subtitle: language and codec, then flags ("English SRT", "French ASS · Forced").
 */
export function trackBrief(track: Track): string {
  const video = track.video;
  if (video) {
    return [
      codecName(track.codec),
      video.resolution && resolutionName(video.resolution),
      video.hdr && hdrName(video.hdr),
    ]
      .filter(Boolean)
      .join(" ");
  }
  if (track.audio) {
    return `${premiumAudio(track) ?? codecName(track.codec)} ${channelsName(track.audio.layout, track.audio.channels)}`;
  }
  const [language = "", ...flags] = baseName(track);
  return [`${language} ${codecName(track.codec)}`, ...flags].join(separator);
}

/**
 * What a play sends a track as when it isn't the source. That is a
 * transcode's output ("AAC Stereo"), a subtitle burned in ("Burned in"),
 * or a styled subtitle shown plain ("Plain"). It is undefined for a track
 * sent as it is.
 */
export function trackChange(track: PlayTrack, source: Track | undefined): string | undefined {
  if (track.type === "subtitle") {
    return track.delivery === "burn" || shownPlain(track, source) ? subtitleLabel(track) : undefined;
  }
  const out = transcodeOutput(track);
  return out && output(out);
}

/**
 * The tracks of one kind as a picker lists them. Each shows its short
 * name, with the rest of its title under it when the title is long.
 */
export function trackOptions(tracks: readonly Track[]): Option[] {
  const names = trackNames(tracks);
  return tracks.map((t) => {
    const { name, note } = names.get(t.id) ?? { name: "" };
    return { id: t.id, label: name, detail: note };
  });
}

/** A version as the picker lists it: "Final Cut · 4K HDR" over "HEVC · 58 GB". */
export function versionOption(version: Version): Option {
  const video = version.tracks.find((t) => t.type === "video");
  const detail = [video ? codecName(video.codec) : undefined, formatSize(version.sizeBytes)]
    .filter(Boolean)
    .join(" · ");
  // A version with no resolution to name it by goes by its codec and size alone.
  const name = versionName(version) || detail;
  return {
    id: version.id,
    label: version.available ? name : i18n._(msg`${name} (missing)`),
    detail: name !== detail ? detail : undefined,
  };
}
