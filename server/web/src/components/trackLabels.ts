import { i18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import aliases from "@shared/display/languages/aliases.json";
import type { PlayTrack, Track, Version } from "@/api/schemas";
import { display, lookup, media } from "@/i18n/display";
import { formatSize } from "@/i18n/format";
import { methodLabel, subtitleLabel, subtitleSource, trackMethod } from "@/i18n/playback";
import type { Option } from "./Select";

const separator = " · ";

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
 * A language tag in words, in English, as the shared tables name it.
 * A dialect uses its own name ("American English"). Otherwise the name is
 * the language with its script and region ("Chinese (Traditional,
 * Taiwan)"). An ISO 639-2 code is read as its language ("eng" is English).
 * A missing tag, und or "null" is the display table's unknown language. A
 * tag that Intl has no name for shows as written.
 */
export function languageName(tag: string | undefined): string {
  const unknown = display.tracks.unknownLanguage;
  if (!hasLanguage(tag)) return unknown;
  const [first = "", ...rest] = tag.split(/[-_]/);
  const language = first.toLowerCase();
  const alias = lookup(aliases, language);
  const canonical = alias ? (rest.length > 0 ? [alias.split("-")[0], ...rest] : [alias]) : [language, ...rest];
  if (canonical[0] === "und") return unknown;
  try {
    return new Intl.DisplayNames(["en"], { type: "language", fallback: "none" }).of(canonical.join("-")) ?? tag;
  } catch {
    // A malformed tag, which Intl rejects, shows as written too.
    return tag;
  }
}

/** "Mono", "Stereo", "5.1" or "7.1" for a channel count, or else the count in words. */
export function channelLayout(channels: number): string {
  return lookup(media.channels, String(channels)) ?? display.tracks.channels.replace("{channels}", String(channels));
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
  const flags = display.tracks.flags;
  const language = languageName(t.language);
  const parts = [t.original ? `${language} [${flags.original}]` : language];
  if (t.hearingImpaired) parts.push(flags.hearingImpaired);
  if (t.forced) parts.push(flags.forced);
  if (t.commentary) parts.push(flags.commentary);
  return parts;
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
  const what = [source?.fullName, outputLabel(track, source)].filter(Boolean).join(" → ");
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

/** The tracks of one kind as a picker lists them: each by its name, with its title under it. */
export function trackOptions(tracks: readonly Track[]): Option[] {
  return tracks.map((t) => ({ id: t.id, label: t.name, detail: t.title?.trim() || undefined }));
}

/** A version as the picker lists it: "Final Cut · 4K HDR" over "HEVC · 58 GB". */
export function versionOption(version: Version): Option {
  const video = version.tracks.find((t) => t.type === "video");
  const detail = [video ? codecName(video.codec) : undefined, formatSize(version.sizeBytes)]
    .filter(Boolean)
    .join(" · ");
  const name = version.name;
  return { id: version.id, label: version.available ? name : i18n._(msg`${name} (missing)`), detail };
}
