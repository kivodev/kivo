import { i18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import { display, lookup } from "./display";

// How playback went, in words from the shared display table. This covers
// how each track of a play is delivered, and the reason code the server
// gives whenever a track isn't played from the file as it is.

type Method = "directPlay" | "directStream" | "transcode";

/** What these words need of a play's track (`PlayTrack`). */
type Delivered = { type: string; selected?: boolean; delivery?: string; codec?: string; transcode?: object };

/** How a track plays: Transcode when it's encoded, else Direct Play from the file, else Direct Stream. */
export function trackMethod(track: Pick<Delivered, "delivery" | "transcode">): Method {
  if (track.transcode) return "transcode";
  return track.delivery === "file" ? "directPlay" : "directStream";
}

/** How a play plays: its video's method, or its audio's when it has no video. */
export function playMethod(tracks: readonly Delivered[]): Method | undefined {
  const main =
    tracks.find((t) => t.selected && t.type === "video") ?? tracks.find((t) => t.selected && t.type === "audio");
  return main && trackMethod(main);
}

/** "Styled", "Plain", "Image" or "Burned in", with the source's codec when given: "Styled (ASS)". */
export function subtitleLabel(track: Pick<Delivered, "delivery" | "codec">, codec?: string): string {
  const kinds = display().subtitleKinds;
  const kind =
    track.delivery === "burn"
      ? kinds.burn
      : track.codec === "ass"
        ? kinds.ass
        : track.codec === "hdmv_pgs_subtitle" || track.codec === "pgs"
          ? kinds.pgs
          : kinds.vtt;
  return codec ? i18n._(msg`${kind} (${codec})`) : kind;
}

/** A play's track in words: the kind for a subtitle, or else the method. */
export function trackLabel(track: Delivered): string {
  return track.type === "subtitle" ? subtitleLabel(track) : methodLabel(trackMethod(track));
}

/** "Direct Play", "Direct Stream" or "Transcode". Unknown methods show as sent. */
export function methodLabel(method: string): string {
  return lookup(display().methods, method) ?? method;
}

/** A sentence for a playback reason code, in the active locale. */
export function describeReason(code: string): string {
  const d = display();
  return lookup(d.reasons, code) ?? d.unknownReason;
}

/** The capability a track failed, for diagnostics: "maxLevel: 5.1, declared 4.1". */
export function mismatchLabel({
  field,
  declared,
  actual,
}: {
  field: string;
  declared?: string;
  actual: string;
}): string {
  return declared
    ? i18n._(msg`${field}: ${actual}, declared ${declared}`)
    : i18n._(msg`${field}: ${actual}, none declared`);
}

/** Where a subtitle track lives: "Embedded", "External" (a file beside the video) or "Downloaded". */
export function subtitleSource(source: string): string | undefined {
  return lookup(display().tracks.sources, source);
}
