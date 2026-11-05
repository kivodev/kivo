import { describePlayFailure, describePlayStopped } from "@/i18n/problems";
import type { PlayerFailure } from "./reducer";

/** What stopped the play, in a sentence. */
export function useFailureText(failure: PlayerFailure): string;
export function useFailureText(failure: PlayerFailure | undefined): string | undefined;
export function useFailureText(failure: PlayerFailure | undefined): string | undefined {
  if (!failure) return undefined;
  if (failure.kind === "ended") return describePlayStopped(failure.message);
  return describePlayFailure(failure);
}
