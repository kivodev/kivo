/** A key event as the dispatcher reads it. Its key is named by `keyName`. */
export type KeyInput = {
  type: "keydown" | "keyup";
  key: string;
  repeat: boolean;
  target: EventTarget | null;
};

/** What a key press does. It runs with the press's repeat count, which is 0 for the first keydown and the release. */
export type KeyAction = (repeatCount: number) => void;

type Press = { action: KeyAction | undefined; focusMoves: number; repeatCount: number };

/**
 * KeyDispatcher owns each key press in the player, from its first keydown
 * to its keyup. At the first keydown it asks the registered `decide`, once,
 * what the press does. The answer is an action, or undefined to leave the
 * press to the focused control. With no `decide` registered, every press
 * goes to the control. The rest of the press follows that answer and
 * reaches nothing else.
 *
 * OK (Enter) and Back run their action on release. Any other key runs it on
 * keydown. When a key is held, only the arrows, fast forward and rewind run
 * the action again on each repeat. A press left to a control stops reaching
 * it once the player moves focus away from it (`focusMoved`). Events of a
 * press whose first keydown the dispatcher didn't see are dropped.
 */
export class KeyDispatcher {
  #decide: ((input: KeyInput) => KeyAction | undefined) | undefined;
  readonly #presses = new Map<string, Press>();
  #focusMoves = 0;

  /** Makes `decide` the one asked at each first keydown. It returns a function that removes it. */
  register(decide: (input: KeyInput) => KeyAction | undefined): () => void {
    this.#decide = decide;
    return () => {
      if (this.#decide === decide) this.#decide = undefined;
    };
  }

  /** Handles a key event. It returns whether the player takes the event, so the page and the focused control don't see it. */
  dispatch(input: KeyInput): boolean {
    const { key } = input;
    if (input.type === "keydown" && !input.repeat) {
      const action = this.#decide?.(input);
      this.#presses.set(key, { action, focusMoves: this.#focusMoves, repeatCount: 0 });
      if (action && !actsOnRelease(key)) action(0);
      return action !== undefined;
    }
    const press = this.#presses.get(key);
    if (!press) return true;
    const up = input.type === "keyup";
    if (up) this.#presses.delete(key);
    else press.repeatCount++;
    if (!press.action) return press.focusMoves !== this.#focusMoves;
    if (up ? actsOnRelease(key) : actsOnRepeat(key)) press.action(up ? 0 : press.repeatCount);
    return true;
  }

  /** Records that the player moved focus itself. A press under way no longer reaches the control it began on. */
  focusMoved() {
    this.#focusMoves++;
  }
}

const actsOnRelease = (key: string) => key === "Enter" || key === "GoBack";

const actsOnRepeat = (key: string) => key.startsWith("Arrow") || key === "MediaFastForward" || key === "MediaRewind";
