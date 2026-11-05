import { createContext, use } from "react";
import type { WorkDetail } from "@/api/schemas";
import type { PlayerController } from "./controller";
import type { PlayerState } from "./reducer";

export type BookPlayer = {
  book: WorkDetail | undefined;
  state: PlayerState;
  /** The user's speed for books. */
  speed: number;
  /**
   * Plays a book from the resume position, or from a position on its
   * timeline such as a chapter's start. If the book is already loaded, it
   * seeks there.
   */
  play: (book: WorkDetail, positionMs?: number) => void;
  /** Runs a command on the loaded book's controller. */
  run: (command: (controller: PlayerController) => void) => void;
  /** Sets the user's speed for books, on every client. */
  setSpeed: (speed: number) => void;
  /** Stops the book, saving its position, and closes the player. */
  close: () => void;
};

export const BookPlayerContext = createContext<BookPlayer | null>(null);

/** The book player. It must be used inside BookPlayerProvider. */
export function useBookPlayer(): BookPlayer {
  const player = use(BookPlayerContext);
  if (!player) throw new Error("useBookPlayer outside BookPlayerProvider");
  return player;
}
