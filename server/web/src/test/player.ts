// What the players' tests read: the video and audio, the mini player, the
// Pause button, and the play requests the app sent.
import type { PlayCreate, PlayUpdate } from "@/api/schemas";
import { screen } from "./render";
import { sent } from "./server";

export const minute = 60_000;
/** Dune: Part Two's saved position, 1:12:03, where its play starts. */
export const resumeMs = 72 * minute + 3000;

export const video = () => document.querySelector("video") as HTMLVideoElement;
export const audio = () => document.querySelector("audio") as HTMLAudioElement;
export const miniPlayer = () => document.querySelector('[data-part="mini-player"]') as HTMLElement | null;
export const playing = () => screen.findByRole("button", { name: "Pause" });

/** The play requests sent, oldest first. */
export const plays = () => sent("POST", "/plays").map((r) => r.body as PlayCreate);
export const stops = () => sent("POST", "/plays/{id}/stop").map((r) => r.body as { positionMs?: number });
export const heartbeats = () => sent("PATCH", "/plays/{id}").map((r) => r.body as PlayUpdate);
