import { useLingui } from "@lingui/react/macro";
import { type CSSProperties, useEffect, useRef, useState } from "react";
import { useFocusRing } from "react-aria";
import type { LetterStart } from "@/api/schemas";
import { isTv } from "@/tv/flag";
import { TvFocusable } from "@/tv/focus";

const railLetters = ["#", ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ"] as const;
export type RailLetter = (typeof railLetters)[number];

/** The letter of the grid's item at index. That is the last letter to start at or before it. */
export function letterAt(letters: readonly LetterStart[], index: number): RailLetter | undefined {
  const at = letters.filter((l) => l.offset <= index).at(-1)?.letter;
  return railLetters.find((l) => l === at);
}

/**
 * Where a letter starts in the grid. A letter without works goes where
 * its works would be, which is the next letter's start or the last item.
 */
export function letterOffset(letters: readonly LetterStart[], letter: RailLetter, total: number): number {
  const next = railLetters.slice(railLetters.indexOf(letter));
  for (const l of next) {
    const start = letters.find((s) => s.letter === l);
    if (start) return start.offset;
  }
  return Math.max(total - 1, 0);
}

type Props = {
  /** Where each letter with works starts, so the arrow keys step between them. */
  letters: readonly LetterStart[];
  onJump: (letter: RailLetter) => void;
  /** Called as a finger or the pointer presses the rail and lets it go. */
  onScrub?: (scrubbing: boolean) => void;
  /** The letter of the first visible item, highlighted. */
  current?: RailLetter;
};

/**
 * Jumps a title-sorted grid by first letter. It's a scrubber: one control
 * that a finger or the pointer presses and drags along. The grid jumps as
 * the letter under the finger changes, and that letter shows large beside
 * it. The rail is fixed in the window, so the page scrolling under it
 * never moves it. A window too short for every letter shows no rail. The
 * arrow keys step through the letters that have works. The focus ring
 * shows for the keyboard only, not for a press, even though a press also
 * focuses the rail.
 *
 * On TV the rail is reached by pressing Right from the grid's last column.
 * Up and Down move a pick through the letters that have works, OK jumps to
 * it, and Left goes back to the grid.
 */
export function LetterRail({ letters, onJump, onScrub, current }: Props) {
  const { t } = useLingui();
  // The letter under the finger, and how far down the rail it is, while scrubbing.
  const [scrub, setScrub] = useState<{ letter: RailLetter; y: number }>();
  // Where the rail was when the finger went down, which the drag is measured against.
  const pressed = useRef<{ top: number; height: number }>(undefined);
  const jumped = useRef<RailLetter>(undefined);
  const frame = useRef(0);
  const { focusProps, isFocusVisible } = useFocusRing();
  // The letter a remote's Up and Down have picked, which OK jumps to.
  const [pick, setPick] = useState<RailLetter>();
  useEffect(() => () => cancelAnimationFrame(frame.current), []);

  // Jump at most once a frame, to the latest letter, and not at all while
  // the letter stays the same.
  const jump = (letter: RailLetter) => {
    if (letter === jumped.current) return;
    jumped.current = letter;
    cancelAnimationFrame(frame.current);
    frame.current = requestAnimationFrame(() => onJump(letter));
  };
  const scrubTo = (clientY: number) => {
    const rail = pressed.current;
    if (!rail || rail.height === 0) return;
    const y = Math.min(Math.max(clientY - rail.top, 0), rail.height - 1);
    const letter = railLetters[Math.floor((y / rail.height) * railLetters.length)] ?? "#";
    setScrub({ letter, y });
    jump(letter);
  };
  const end = () => {
    if (pressed.current) onScrub?.(false);
    pressed.current = undefined;
    setScrub(undefined);
    jumped.current = undefined;
  };
  const stepFrom = (from: RailLetter | undefined, by: 1 | -1): RailLetter | undefined => {
    const has = (l: RailLetter | undefined) => l === "#" || letters.some((s) => s.letter === l);
    let i = railLetters.indexOf(from ?? "#") + by;
    while (railLetters[i] !== undefined && !has(railLetters[i])) i += by;
    return railLetters[i];
  };
  const step = (by: 1 | -1) => {
    const letter = stepFrom(current, by);
    if (letter) onJump(letter);
  };
  const keys: Record<string, () => void> = {
    ArrowDown: () => step(1),
    ArrowRight: () => step(1),
    ArrowUp: () => step(-1),
    ArrowLeft: () => step(-1),
    Home: () => onJump("#"),
    End: () => onJump(railLetters.at(-1) ?? "#"),
  };

  const slider = (
    <div
      role="slider"
      tabIndex={isTv ? -1 : 0}
      data-part="letter-rail"
      aria-label={t`Jump to letter`}
      aria-orientation="vertical"
      aria-valuemin={0}
      aria-valuemax={railLetters.length - 1}
      aria-valuenow={railLetters.indexOf(current ?? "#")}
      aria-valuetext={current ?? "#"}
      data-scrubbing={scrub !== undefined || undefined}
      data-focus-visible={isFocusVisible || undefined}
      {...focusProps}
      onPointerDown={(e) => {
        if (e.button !== 0) return;
        // A drag selects no text and drags nothing. The press still
        // focuses the rail.
        e.preventDefault();
        e.currentTarget.focus({ preventScroll: true });
        e.currentTarget.setPointerCapture(e.pointerId);
        const rect = e.currentTarget.getBoundingClientRect();
        pressed.current = { top: rect.top, height: rect.height };
        onScrub?.(true);
        scrubTo(e.clientY);
      }}
      onPointerMove={(e) => {
        if (e.currentTarget.hasPointerCapture(e.pointerId)) scrubTo(e.clientY);
      }}
      onPointerUp={end}
      onPointerCancel={end}
      onLostPointerCapture={end}
      onKeyDown={(e) => {
        const key = keys[e.key];
        if (!key) return;
        e.preventDefault();
        key();
      }}
      // On TV the focusable around it is fixed in its place, and it fills that.
      className="fixed flex letter-rail-band w-(--layout-rail-width) cursor-pointer touch-none flex-col items-center rounded-sm text-xs outline-none select-none data-focus-visible:outline-2 data-focus-visible:outline-offset-2 data-focus-visible:outline-focus data-focus-visible:outline-solid pointer-coarse:w-11 tv:static tv:size-full"
    >
      {railLetters.map((letter) => (
        <span
          key={letter}
          aria-hidden="true"
          data-current={letter === current || undefined}
          data-picked={letter === pick || undefined}
          className="flex min-h-0 w-full flex-1 items-center justify-center font-semibold text-text-muted data-current:text-accent data-picked:rounded-sm data-picked:bg-accent data-picked:text-on-accent tv:mx-auto tv:w-10 tv:text-sm tv:leading-none tv:data-current:font-bold tv:data-picked:rounded-full tv:data-picked:bg-text tv:data-picked:text-bg"
        >
          {letter}
        </span>
      ))}
      {scrub && (
        <span
          aria-hidden="true"
          data-part="letter-rail-bubble"
          style={{ "--at": `${scrub.y}px` } as CSSProperties}
          className="pointer-events-none absolute top-(--at) right-full mr-2 flex size-12 -translate-y-1/2 items-center justify-center rounded-full bg-surface-raised text-2xl font-semibold text-text shadow-overlay"
        >
          {scrub.letter}
        </span>
      )}
    </div>
  );

  return (
    // This holds the rail's place in the row beside the grid. The rail
    // itself is fixed.
    <div className="w-(--layout-rail-width) shrink-0 pointer-coarse:-mr-3 pointer-coarse:w-11 rail-short:hidden tv:mr-0 tv:block tv:w-12">
      {isTv ? (
        // The rail runs from below the header's Filter button down to the
        // overscan, a line of the TV's smallest type for each letter, so it
        // reads from across a room; the pick is a pill within its line.
        <TvFocusable
          focusKey="letter-rail"
          scroll="none"
          part="letter-rail-focus"
          face="none"
          className="fixed top-(--layout-tv-letter-rail-top) bottom-(--layout-tv-overscan-y) w-12"
          onFocus={() => setPick(current)}
          onPress={() => pick && onJump(pick)}
          onArrow={(direction) => {
            if (direction === "up" || direction === "down") {
              setPick((p) => stepFrom(p ?? current, direction === "down" ? 1 : -1) ?? p);
              return false;
            }
            // Nothing is to the right of the rail.
            return direction !== "right";
          }}
        >
          {slider}
        </TvFocusable>
      ) : (
        slider
      )}
    </div>
  );
}
