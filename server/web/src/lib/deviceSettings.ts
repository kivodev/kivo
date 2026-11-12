import type { QualityRequest } from "@/api/schemas";
import { qualityKey, qualityOf } from "./quality";
import { readStored, writeStored } from "./storage";

// This device's settings, kept in the browser rather than on the server,
// because a phone and a TV signed in as the same user want different ones.
// Storage can be unavailable, as in private windows. Every setting then
// takes its default.

export type SubtitleSize = "small" | "medium" | "large";
export type SubtitleBackground = "none" | "shadow" | "box";
// The built-in themes are two token sets. Dark is the default, and the
// light set applies with data-theme="light" on the root.
export type ThemeName = "dark" | "light";

export type AudioChannels = 2 | 6 | 8;

export type DeviceSettings = {
  /** The quality the player starts with on the server's local networks. Undefined means Auto. */
  localQuality: QualityRequest | undefined;
  /** The quality the player starts with anywhere else. Undefined means Auto. */
  remoteQuality: QualityRequest | undefined;
  /**
   * The most audio channels this device outputs, stereo, 5.1 or 7.1.
   * Undefined means Auto, which uses what the browser reports.
   */
  maxAudioChannels: AudioChannels | undefined;
  /** Whether plays boost dialogue, as switched in the player. */
  dialogueBoost: boolean;
  /** How the text subtitles that the browser draws look. */
  subtitleSize: SubtitleSize;
  subtitleBackground: SubtitleBackground;
  theme: ThemeName;
};

const keys = {
  localQuality: "kivo.quality.local",
  remoteQuality: "kivo.quality.remote",
  maxAudioChannels: "kivo.audioChannels",
  dialogueBoost: "kivo.dialogueBoost",
  subtitleSize: "kivo.subtitleSize",
  subtitleBackground: "kivo.subtitleBackground",
  theme: "kivo.theme",
} as const;

function oneOf<T extends string>(value: string | undefined, choices: readonly T[], fallback: T): T {
  return choices.find((c) => c === value) ?? fallback;
}

export function deviceSettings(): DeviceSettings {
  return {
    localQuality: qualityOf(readStored(keys.localQuality)),
    remoteQuality: qualityOf(readStored(keys.remoteQuality)),
    maxAudioChannels: ([2, 6, 8] as const).find((n) => String(n) === readStored(keys.maxAudioChannels)),
    dialogueBoost: readStored(keys.dialogueBoost) === "on",
    subtitleSize: oneOf(readStored(keys.subtitleSize), ["small", "medium", "large"], "medium"),
    subtitleBackground: oneOf(readStored(keys.subtitleBackground), ["none", "shadow", "box"], "shadow"),
    theme: oneOf(readStored(keys.theme), ["dark", "light"], "dark"),
  };
}

/** Saves this device's settings and applies the theme. */
export function saveDeviceSettings(s: DeviceSettings): void {
  const quality = (q: QualityRequest | undefined) => (q && q.mode !== "auto" ? qualityKey(q) : undefined);
  writeStored(keys.localQuality, quality(s.localQuality));
  writeStored(keys.remoteQuality, quality(s.remoteQuality));
  writeStored(keys.maxAudioChannels, s.maxAudioChannels?.toString());
  writeStored(keys.dialogueBoost, s.dialogueBoost ? "on" : undefined);
  writeStored(keys.subtitleSize, s.subtitleSize);
  writeStored(keys.subtitleBackground, s.subtitleBackground);
  writeStored(keys.theme, s.theme);
  restoreTheme();
}

/** Puts this device's theme on the page, at start and when it changes. */
export function restoreTheme(): void {
  document.documentElement.dataset.theme = deviceSettings().theme;
}
