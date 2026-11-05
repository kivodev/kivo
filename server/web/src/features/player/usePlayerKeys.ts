import { useEffect, useEffectEvent } from "react";
import type { KeyDispatcher, KeyInput } from "./keyDispatcher";
import { type KeyCommand, keyName } from "./keys";

/**
 * usePlayerKeys sends every key press, from its first keydown to its keyup,
 * through `keys`, which gives it to the player or the focused control (see
 * keyDispatcher.ts). `decide` picks a press's command at its first keydown,
 * or none to leave the key to the focused control. `perform` runs the
 * command, its repeats and its release on the state as it is at that
 * moment. The listener runs before the focused control, so the player can
 * take a key from it. With `last`, it runs after the focused control and
 * everything else instead, and leaves a key one of them took (prevented its
 * default), as the docked book leaves the video player's keys.
 */
export function usePlayerKeys(
  keys: KeyDispatcher,
  remote: boolean,
  decide: (input: KeyInput) => KeyCommand | undefined,
  perform: (command: KeyCommand, repeatCount: number) => void,
  { last = false }: { last?: boolean } = {},
) {
  const decideNow = useEffectEvent(decide);
  const performLatest = useEffectEvent(perform);
  useEffect(() => {
    const unregister = keys.register((input) => {
      const command = decideNow(input);
      return command && ((repeatCount: number) => performLatest(command, repeatCount));
    });
    const listener = (e: KeyboardEvent) => {
      if (last && e.defaultPrevented) return;
      const input = { type: e.type as KeyInput["type"], key: keyName(e, remote), repeat: e.repeat, target: e.target };
      if (!keys.dispatch(input)) return;
      e.preventDefault();
      e.stopPropagation();
    };
    const capture = !last;
    window.addEventListener("keydown", listener, { capture });
    window.addEventListener("keyup", listener, { capture });
    return () => {
      unregister();
      window.removeEventListener("keydown", listener, { capture });
      window.removeEventListener("keyup", listener, { capture });
    };
  }, [keys, remote, last]);
}
