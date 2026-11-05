// Keyboard and remote keys. Media keys and the TV remote's keys arrive as
// KeyboardEvent.key values. A keyboard also has the usual letters, and its
// arrows skip and change the volume. A remote's arrows scrub and move
// through the controls instead.

export type KeyCommand =
  | "togglePlay"
  | "play"
  | "pause"
  /** Show controls and pause. This is Enter on a keyboard while the controls are hidden. */
  | "revealAndPause"
  /** Back 10 seconds, or the scrub target back as far when there is one. */
  | "skipBack"
  /** Forward 30 seconds, or the scrub target forward as far when there is one. */
  | "skipForward"
  /** Show controls with focus on the scrub bar and move its target back a step. */
  | "scrubBack"
  /** Show controls with focus on the scrub bar and move its target forward a step. */
  | "scrubForward"
  /** OK on the scrub bar. It seeks to the target, or plays or pauses when there is none. */
  | "scrubOk"
  /** Move focus to the control beside the focused one, in its row. */
  | "focusLeft"
  | "focusRight"
  /** Move focus to the row above or below the focused control. */
  | "focusUp"
  | "focusDown"
  | "previous"
  | "next"
  /** Show controls with focus on play or pause. */
  | "showControls"
  /** Skip to the end of the intro or credits, leaving the controls hidden. */
  | "skip"
  | "hideControls"
  /** Close the open menu, stream info or dialog. */
  | "closeMenu"
  | "exit"
  /** Open the stream info, or close it when it's open. */
  | "info"
  | "fullscreen"
  | "mute"
  | "volumeUp"
  | "volumeDown";

export type KeyContext = {
  /** A TV's remote, whose arrows scrub and move through the controls rather than skip and change the volume. */
  remote: boolean;
  controlsVisible: boolean;
  loading: boolean;
  /** A menu, the stream info or a dialog is open over the player. */
  menuOpen: boolean;
  /** The button over the picture, Skip or Up next's button. A remote reaches it while the controls are hidden. */
  overlay?: "skip" | "upNext";
  /**
   * Focus is in a control that handles this key itself, such as a field, a
   * button's Enter and Space, or a slider's arrows.
   */
  handled: boolean;
  onScrubBar: boolean;
};

/** Back on a remote: 461 on webOS and 10009 on Tizen. These, and Escape on a TV's keyboard, become GoBack. */
const backKeyCodes = new Set([461, 10009]);

/**
 * The media keys and Info on webOS and Tizen remotes, by key code. Both
 * report most of these as "Unidentified" or with no key name, and Tizen's
 * previous, next and Play/Pause are 10232, 10233 and 10252.
 */
const remoteKeyCodes: Record<number, string> = {
  19: "MediaPause",
  412: "MediaRewind",
  413: "MediaStop",
  415: "MediaPlay",
  417: "MediaFastForward",
  457: "Info",
  10232: "MediaTrackPrevious",
  10233: "MediaTrackNext",
  10252: "MediaPlayPause",
};

/**
 * A key event's key, with every Back key named GoBack. On a remote, the
 * media keys and Info are named by their key codes. On a computer, Escape
 * keeps its own name.
 */
export function keyName({ key, keyCode }: { key: string; keyCode: number }, remote: boolean): string {
  if (key === "BrowserBack" || key === "XF86Back" || backKeyCodes.has(keyCode)) return "GoBack";
  if (remote && remoteKeyCodes[keyCode]) return remoteKeyCodes[keyCode];
  if (key === "Escape" && remote) return "GoBack";
  return key;
}

/**
 * The command for a key named by `keyName`, or undefined to let the key do
 * its default. Media keys, Info and Back always go to the player. Back
 * closes an open menu, leaves while the video loads, hides shown controls,
 * and leaves when they are hidden. An open menu takes every other key.
 */
export function keyCommand(key: string, context: KeyContext): KeyCommand | undefined {
  const player = playerKey(key, context);
  if (player !== null) return player;
  if (context.menuOpen) return undefined;
  if (context.remote) {
    const command = remoteCommand(key, context);
    if (command !== null) return command;
  }
  return context.handled ? undefined : keyboardCommand(key, context.controlsVisible);
}

/**
 * The command for a play, pause, rewind, fast forward, previous or next key,
 * which the video player and the docked book both take.
 */
export function mediaKeyCommand(key: string): KeyCommand | undefined {
  switch (key) {
    case "MediaPlayPause":
      return "togglePlay";
    case "MediaPlay":
      return "play";
    case "MediaPause":
    case "MediaStop":
      return "pause";
    case "MediaRewind":
      return "skipBack";
    case "MediaFastForward":
      return "skipForward";
    case "MediaTrackPrevious":
      return "previous";
    case "MediaTrackNext":
      return "next";
    default:
      return undefined;
  }
}

/** Media keys, Info, Back and a computer's Escape. It returns null for any other key. */
function playerKey(key: string, { menuOpen, loading, controlsVisible }: KeyContext): KeyCommand | undefined | null {
  const media = mediaKeyCommand(key);
  if (media) return media;
  switch (key) {
    case "Info":
      return "info";
    case "GoBack":
      if (menuOpen) return "closeMenu";
      return loading || !controlsVisible ? "exit" : "hideControls";
    // Escape closes and hides but never leaves. With nothing to close, the browser handles it, for example to leave fullscreen.
    case "Escape":
      if (menuOpen) return "closeMenu";
      return controlsVisible ? "hideControls" : undefined;
    default:
      return null;
  }
}

/**
 * A remote's OK and arrows. Null leaves the key to the keyboard's map.
 *
 * With the controls hidden, OK, Up and Down show them with focus on play or
 * pause, and Left and Right show them and scrub. Over a Skip button, OK
 * skips and Left and Right still scrub. Over Up next, OK, Left and Right go
 * to its buttons.
 *
 * With the controls shown, the scrub bar takes Left and Right to scrub and
 * OK to seek, or to play or pause. Elsewhere the arrows move focus and OK
 * presses the focused button.
 */
function remoteCommand(
  key: string,
  { controlsVisible, overlay, onScrubBar }: KeyContext,
): KeyCommand | undefined | null {
  switch (key) {
    case "Enter":
      if (controlsVisible) return onScrubBar ? "scrubOk" : undefined;
      if (overlay === "skip") return "skip";
      return overlay === "upNext" ? undefined : "showControls";
    case "ArrowLeft":
    case "ArrowRight": {
      const left = key === "ArrowLeft";
      const scrubs = controlsVisible ? onScrubBar : overlay !== "upNext";
      if (scrubs) return left ? "scrubBack" : "scrubForward";
      return left ? "focusLeft" : "focusRight";
    }
    case "ArrowUp":
      return controlsVisible ? "focusUp" : "showControls";
    case "ArrowDown":
      return controlsVisible ? "focusDown" : "showControls";
    default:
      return null;
  }
}

/** A keyboard's keys, with the player's letters. */
function keyboardCommand(key: string, controlsVisible: boolean): KeyCommand | undefined {
  switch (key) {
    case " ":
    case "k":
      return "togglePlay";
    case "Enter":
      return controlsVisible ? undefined : "revealAndPause";
    case "ArrowLeft":
    case "j":
      return "skipBack";
    case "ArrowRight":
    case "l":
      return "skipForward";
    case "ArrowUp":
      return "volumeUp";
    case "ArrowDown":
      return "volumeDown";
    case "i":
      return "info";
    case "f":
      return "fullscreen";
    case "m":
      return "mute";
    default:
      return undefined;
  }
}

/** How far one Left or Right moves the scrub target. A press moves 10 seconds, and holding the key moves 30 and then 60. */
export function scrubStepMs(repeatCount: number): number {
  if (repeatCount >= 15) return 60_000;
  if (repeatCount >= 5) return 30_000;
  return 10_000;
}

/**
 * Whether the focused element handles a key itself, named as
 * KeyboardEvent.key names it, so the player and the TV's spatial navigation
 * leave the key to it. A menu or list box keeps every key, and a slider its
 * arrows. A field keeps every key but Up and Down, which leave it. A
 * checkbox or radio button keeps Enter and Space, so the arrows move focus
 * rather than the choice, and so does anything else that can be pressed.
 */
export function focusHandles(target: EventTarget | null, key: string): boolean {
  if (!(target instanceof Element)) return false;
  if (target.closest("[role=menu], [role=listbox]")) return true;
  if (target.closest("[role=slider], input[type=range]")) return key.startsWith("Arrow");
  const press = key === "Enter" || key === " ";
  if (target.closest("input[type=checkbox], input[type=radio]")) return press;
  if (target.closest("input, textarea, select, [contenteditable=true]"))
    return key !== "ArrowUp" && key !== "ArrowDown";
  return press && target.closest("a[href], button, [role=button]") !== null;
}
