import { i18n } from "@lingui/core";
import { display } from "./display";

// Locale-aware formatting of numbers that the API sends as integers.
// Durations and positions are in milliseconds, sizes in bytes, and
// bitrates in bits per second. Bitrates, sizes, runtimes, time left,
// episode codes and quality choices take their words from the shared
// display table. They follow the same rules as the native apps, and the
// shared vectors test both. Each value is rounded half up, and a decimal
// shows only under 10.

const minuteMs = 60_000;
const numberFormats = new Map<string, Intl.NumberFormat>();

function numberFormat(options?: Intl.NumberFormatOptions): Intl.NumberFormat {
  const key = JSON.stringify([i18n.locale, options]);
  let formatter = numberFormats.get(key);
  if (!formatter) {
    formatter = new Intl.NumberFormat(i18n.locale, options);
    numberFormats.set(key, formatter);
  }
  return formatter;
}

/** The words with each `{name}` replaced by its value. */
function fill(words: string, values: Record<string, string>): string {
  return words.replace(/\{(\w+)\}/g, (all, name: string) => values[name] ?? all);
}

/** `n` over `d`, rounded half up. */
function rounded(n: number, d: number): number {
  return Math.floor((n + d / 2) / d);
}

/** A number in the locale's digits, grouped by thousands: "1,234", "9.8". */
function number(n: number, fractionDigits = 0): string {
  return numberFormat({ maximumFractionDigits: fractionDigits }).format(n);
}

/** `value` over `unit`, at least zero, with a decimal under 10, such as "9.8" or "20". */
function scaled(value: number, unit: number): string {
  const v = Math.max(0, value);
  return v < 10 * unit ? number(rounded(v, unit / 10) / 10, 1) : number(rounded(v, unit));
}

/** Whole minutes, at least one. */
function minutesOf(ms: number): number {
  return Math.max(1, rounded(Math.max(0, ms), minuteMs));
}

/**
 * Whole minutes, at least one. Under an hour it is in minutes. Otherwise
 * it is in hours and the remaining minutes.
 */
function inHours(ms: number, inMinutes: string, inWholeHours: string, inHoursMinutes: string): string {
  const total = minutesOf(ms);
  const hours = number(Math.floor(total / 60));
  const minutes = number(total % 60);
  if (total < 60) return fill(inMinutes, { minutes });
  if (total % 60 === 0) return fill(inWholeHours, { hours });
  return fill(inHoursMinutes, { hours, minutes });
}

/** A runtime, such as "2h 46m", "48m" or "2h". */
export function formatRuntime(ms: number): string {
  const words = display.durations;
  return inHours(ms, words.minutes, words.hours, words.hoursMinutes);
}

/** Time left, such as "18 min left", "1 hr 34 min left" or "2 hr left". */
export function formatTimeLeft(ms: number): string {
  const words = display.durations;
  return inHours(ms, words.minutesLeft, words.hoursLeft, words.hoursMinutesLeft);
}

/**
 * How long ago something happened, in the largest whole unit, such as
 * "5 minutes ago", "2 hours ago" or "yesterday".
 */
export function formatTimeAgo(ms: number): string {
  const minutes = Math.max(1, Math.floor(ms / minuteMs));
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);
  const [value, u]: [number, Intl.RelativeTimeFormatUnit] =
    minutes < 60
      ? [minutes, "minute"]
      : hours < 24
        ? [hours, "hour"]
        : days < 31
          ? [days, "day"]
          : days < 365
            ? [Math.floor(days / 30), "month"]
            : [Math.floor(days / 365), "year"];
  return new Intl.RelativeTimeFormat(i18n.locale, { numeric: "auto" }).format(-value, u);
}

/**
 * When something happened, from an ISO timestamp, as seen at `now`. Within
 * a week it says how long ago ("3 hours ago"). Otherwise it gives the date
 * ("Oct 14, 2026").
 */
export function formatWhen(iso: string, now: number): string {
  const ms = now - Date.parse(iso);
  return ms < 7 * 24 * 60 * minuteMs ? formatTimeAgo(ms) : i18n.date(iso, { dateStyle: "medium" });
}

/** A playback position, at least zero, such as "1:12:03" or "4:05". */
export function formatClock(ms: number): string {
  const total = Math.floor(Math.max(0, ms) / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const two = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${two(m)}:${two(s)}` : `${m}:${two(s)}`;
}

/** A file size, such as "58 GB", "5.1 GB", "700 MB" or "300 KB". */
export function formatSize(bytes: number): string {
  const sizes = display.sizes;
  if (bytes >= 1e9) return fill(sizes.gigabytes, { n: scaled(bytes, 1e9) });
  if (bytes >= 1e6) return fill(sizes.megabytes, { n: scaled(bytes, 1e6) });
  return fill(sizes.kilobytes, { n: scaled(bytes, 1e3) });
}

/** A bitrate, such as "8 Mb/s" or "9.8 Mb/s". */
export function formatBitrate(bps: number): string {
  return fill(display.bitrate, { n: scaled(bps, 1e6) });
}

/** An episode's place, such as "S3 · E4", or "S1 · E1 to E2" for a file holding several. */
export function formatEpisodeCode(episode: { seasonNumber: number; number: number; endNumber?: number }): string {
  const { seasonNumber, number, endNumber } = episode;
  const words = display.episodeCode;
  const place = { season: String(seasonNumber), episode: String(number) };
  return endNumber !== undefined && endNumber !== number
    ? fill(words.range, { ...place, end: String(endNumber) })
    : fill(words.one, place);
}

type Rung = { bitrateBps: number; height: number };

/** A rung of the quality ladder, by its height and bitrate, such as "1080p · 8 Mb/s". */
export function formatRung({ bitrateBps, height }: Rung): string {
  return fill(display.quality.rung, { height: String(height), bitrate: formatBitrate(bitrateBps) });
}

/**
 * A choice in the quality menu. `quality` is Auto, Original or a limit's
 * bitrate.
 *  - Auto is "Auto".
 *  - Original is "Original", with the file's bitrate when given
 *    ("Original · 20 Mb/s").
 *  - A limit is its rung ("1080p · 8 Mb/s"), or just its bitrate when the
 *    ladder has no rung for it.
 */
export function formatQuality(quality: "auto" | "original" | number, rung?: Rung, sourceBps?: number): string {
  const words = display.quality;
  if (quality === "auto") return words.auto;
  if (quality === "original") {
    return sourceBps === undefined ? words.original : fill(words.originalAt, { bitrate: formatBitrate(sourceBps) });
  }
  return rung ? formatRung(rung) : formatBitrate(quality);
}

/** A number with grouping, such as "3,100". */
export function formatCount(n: number): string {
  return numberFormat().format(n);
}

/** A community rating out of ten, such as "8.2". */
export function formatCommunityRating(n: number): string {
  return numberFormat({ minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(n);
}

/** A calendar date the API sends as YYYY-MM-DD, such as "Jan 9, 2018". */
export function formatDate(date: string): string {
  const d = new Date(`${date}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return date;
  return new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium", timeZone: "UTC" }).format(d);
}

/** A moment from an ISO timestamp, in the viewer's zone, such as "Jan 9, 2018, 9:41 PM". */
export function formatDateTime(iso: string): string {
  return i18n.date(iso, { dateStyle: "medium", timeStyle: "short" });
}

/** A day from an ISO timestamp, in the viewer's zone, such as "Oct 31". */
export function formatDay(iso: string): string {
  return i18n.date(iso, { month: "short", day: "numeric" });
}

/** A time of day from an ISO timestamp, in the viewer's zone, such as "9:41 PM". */
export function formatTimeOfDay(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(i18n.locale, { timeStyle: "short" }).format(d);
}

/**
 * A time of day, from a moment as from Date.now(), in the browser's own
 * locale and clock, such as "9:42 PM" or "21:42".
 */
export function formatEndTime(ms: number): string {
  return new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(new Date(ms));
}

/**
 * A future time, from an ISO timestamp, as seen at `now` in the viewer's
 * zone. Within a day it is the time of day ("2:00 AM"). Within a week it
 * adds the weekday ("Fri 2:00 AM"). Otherwise it adds the date ("Oct 30,
 * 2:00 AM").
 */
export function formatUpcoming(iso: string, now: number): string {
  const ms = Date.parse(iso) - now;
  const day = 24 * 60 * minuteMs;
  const date: Intl.DateTimeFormatOptions =
    ms < day ? {} : ms < 7 * day ? { weekday: "short" } : { month: "short", day: "numeric" };
  return new Intl.DateTimeFormat(i18n.locale, { ...date, hour: "numeric", minute: "2-digit" }).format(new Date(iso));
}

/** A multiple, such as "1.8×". */
export function formatTimes(n: number): string {
  return `${numberFormat({ maximumFractionDigits: 1 }).format(n)}×`;
}
