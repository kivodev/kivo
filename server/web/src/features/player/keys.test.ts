import { describe, expect, it } from "vitest";
import { focusHandles, type KeyContext, keyCommand, keyName, scrubStepMs } from "./keys";

const hidden: KeyContext = {
  remote: true,
  controlsVisible: false,
  loading: false,
  menuOpen: false,
  handled: false,
  onScrubBar: false,
};
const shown: KeyContext = { ...hidden, controlsVisible: true };
/** Controls shown with focus on the scrub bar. Its slider claims the arrows. */
const onBar = (key: string): KeyContext => ({ ...shown, onScrubBar: true, handled: key.startsWith("Arrow") });
/** Controls shown with focus on a button. The button claims OK. */
const onButton = (key: string): KeyContext => ({ ...shown, handled: key === "Enter" || key === " " });

describe("keyCommand on a remote", () => {
  it("shows hidden controls with OK, Up and Down, and scrubs with Left and Right", () => {
    expect(keyCommand("Enter", hidden)).toBe("showControls");
    expect(keyCommand("ArrowLeft", hidden)).toBe("scrubBack");
    expect(keyCommand("ArrowRight", hidden)).toBe("scrubForward");
    expect(keyCommand("ArrowUp", hidden)).toBe("showControls");
    expect(keyCommand("ArrowDown", hidden)).toBe("showControls");
  });

  it("scrubs on the scrub bar, where OK seeks or plays and Up and Down only move focus", () => {
    expect(keyCommand("ArrowLeft", onBar("ArrowLeft"))).toBe("scrubBack");
    expect(keyCommand("ArrowRight", onBar("ArrowRight"))).toBe("scrubForward");
    expect(keyCommand("Enter", onBar("Enter"))).toBe("scrubOk");
    expect(keyCommand("ArrowDown", onBar("ArrowDown"))).toBe("focusDown");
    expect(keyCommand("ArrowUp", onBar("ArrowUp"))).toBe("focusUp");
  });

  it("moves focus among the buttons, and leaves OK to them", () => {
    expect(keyCommand("ArrowLeft", onButton("ArrowLeft"))).toBe("focusLeft");
    expect(keyCommand("ArrowRight", onButton("ArrowRight"))).toBe("focusRight");
    expect(keyCommand("ArrowUp", onButton("ArrowUp"))).toBe("focusUp");
    expect(keyCommand("ArrowDown", onButton("ArrowDown"))).toBe("focusDown");
    expect(keyCommand("Enter", onButton("Enter"))).toBeUndefined();
  });

  it("skips with OK over a Skip button and scrubs with Left and Right, with the controls hidden", () => {
    const overSkip: KeyContext = { ...hidden, overlay: "skip", handled: true };
    expect(keyCommand("Enter", overSkip)).toBe("skip");
    expect(keyCommand("ArrowLeft", overSkip)).toBe("scrubBack");
    expect(keyCommand("ArrowRight", overSkip)).toBe("scrubForward");
    expect(keyCommand("ArrowDown", overSkip)).toBe("showControls");
  });

  it("leaves OK to Up next's buttons and moves between them with Left and Right", () => {
    const overUpNext: KeyContext = { ...hidden, overlay: "upNext", handled: true };
    expect(keyCommand("Enter", overUpNext)).toBeUndefined();
    expect(keyCommand("ArrowLeft", overUpNext)).toBe("focusLeft");
    expect(keyCommand("ArrowRight", overUpNext)).toBe("focusRight");
    expect(keyCommand("ArrowUp", overUpNext)).toBe("showControls");
  });

  it("leaves OK and the arrows to an open menu", () => {
    const menu: KeyContext = { ...shown, menuOpen: true, handled: true };
    for (const key of ["Enter", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", " "]) {
      expect(keyCommand(key, menu)).toBeUndefined();
    }
  });
});

describe("keyCommand's player keys", () => {
  const contexts: KeyContext[] = [
    hidden,
    shown,
    { ...shown, menuOpen: true, handled: true },
    { ...hidden, overlay: "skip" },
    { ...shown, remote: false, handled: true },
  ];

  it("takes the media keys and Info whatever shows and has focus", () => {
    for (const context of contexts) {
      expect(keyCommand("MediaPlayPause", context)).toBe("togglePlay");
      expect(keyCommand("MediaPlay", context)).toBe("play");
      expect(keyCommand("MediaPause", context)).toBe("pause");
      expect(keyCommand("MediaFastForward", context)).toBe("skipForward");
      expect(keyCommand("MediaRewind", context)).toBe("skipBack");
      expect(keyCommand("MediaTrackPrevious", context)).toBe("previous");
      expect(keyCommand("MediaTrackNext", context)).toBe("next");
      expect(keyCommand("Info", context)).toBe("info");
    }
  });

  it("closes a menu on Back, then hides the controls, then exits", () => {
    expect(keyCommand("GoBack", { ...shown, menuOpen: true })).toBe("closeMenu");
    expect(keyCommand("GoBack", shown)).toBe("hideControls");
    expect(keyCommand("GoBack", hidden)).toBe("exit");
    expect(keyCommand("GoBack", { ...hidden, overlay: "skip" })).toBe("exit");
    expect(keyCommand("GoBack", { ...hidden, overlay: "upNext" })).toBe("exit");
  });

  it("exits on Back while the video loads", () => {
    expect(keyCommand("GoBack", { ...shown, loading: true })).toBe("exit");
  });
});

describe("keyName", () => {
  it("names every Back GoBack", () => {
    for (const event of [
      { key: "GoBack", keyCode: 0 },
      { key: "BrowserBack", keyCode: 166 },
      { key: "Escape", keyCode: 27 },
      { key: "XF86Back", keyCode: 10009 },
      { key: "Unidentified", keyCode: 10009 },
      { key: "Unidentified", keyCode: 461 },
    ]) {
      expect(keyName(event, true)).toBe("GoBack");
    }
    expect(keyName({ key: "Enter", keyCode: 13 }, true)).toBe("Enter");
  });

  it("names a remote's media keys and Info by their key codes", () => {
    const named = (keyCode: number) => keyName({ key: "Unidentified", keyCode }, true);
    expect([19, 412, 413, 415, 417, 457, 10232, 10233, 10252].map(named)).toEqual([
      "MediaPause",
      "MediaRewind",
      "MediaStop",
      "MediaPlay",
      "MediaFastForward",
      "Info",
      "MediaTrackPrevious",
      "MediaTrackNext",
      "MediaPlayPause",
    ]);
    expect(keyName({ key: "Pause", keyCode: 19 }, false)).toBe("Pause");
  });

  it("keeps a computer's Escape apart from Back", () => {
    expect(keyName({ key: "Escape", keyCode: 27 }, false)).toBe("Escape");
    expect(keyName({ key: "BrowserBack", keyCode: 166 }, false)).toBe("GoBack");
  });
});

describe("Escape on a computer", () => {
  const keyboard = { ...shown, remote: false };

  it("closes a menu or hides the controls, and never leaves the player", () => {
    expect(keyCommand("Escape", { ...keyboard, menuOpen: true })).toBe("closeMenu");
    expect(keyCommand("Escape", keyboard)).toBe("hideControls");
    expect(keyCommand("Escape", { ...keyboard, controlsVisible: false })).toBeUndefined();
  });
});

describe("keyCommand on a keyboard", () => {
  const keyboard = { ...shown, remote: false };

  it("skips with Left and Right and changes the volume with Up and Down", () => {
    expect(keyCommand("ArrowLeft", { ...keyboard, controlsVisible: false })).toBe("skipBack");
    expect(keyCommand("ArrowRight", keyboard)).toBe("skipForward");
    expect(keyCommand("Enter", { ...keyboard, controlsVisible: false })).toBe("revealAndPause");
    expect(keyCommand("Enter", keyboard)).toBeUndefined();
    expect(keyCommand("ArrowUp", keyboard)).toBe("volumeUp");
    expect(keyCommand("ArrowDown", { ...keyboard, controlsVisible: false })).toBe("volumeDown");
    expect(keyCommand("ArrowUp", { ...keyboard, handled: true })).toBeUndefined();
  });

  it("adds the web's letters and leaves keys a focused control handles", () => {
    expect(keyCommand(" ", keyboard)).toBe("togglePlay");
    expect(keyCommand("f", keyboard)).toBe("fullscreen");
    expect(keyCommand("ArrowLeft", { ...keyboard, handled: true })).toBeUndefined();
    expect(keyCommand("x", keyboard)).toBeUndefined();
  });
});

describe("scrubStepMs", () => {
  it("steps 10 seconds a press, then 30 and 60 as the key is held", () => {
    expect(scrubStepMs(0)).toBe(10_000);
    expect(scrubStepMs(4)).toBe(10_000);
    expect(scrubStepMs(5)).toBe(30_000);
    expect(scrubStepMs(14)).toBe(30_000);
    expect(scrubStepMs(15)).toBe(60_000);
  });
});

describe("focusHandles", () => {
  it("gives arrows to sliders, and Enter and Space to buttons", () => {
    document.body.innerHTML = `<div role="slider"><input type="range" /></div><button>Play</button><div id="plain"></div>`;
    const range = document.querySelector("input");
    const button = document.querySelector("button");
    expect(focusHandles(range, "ArrowLeft")).toBe(true);
    expect(focusHandles(range, " ")).toBe(false);
    expect(focusHandles(button, "Enter")).toBe(true);
    expect(focusHandles(button, "ArrowLeft")).toBe(false);
    expect(focusHandles(document.getElementById("plain"), " ")).toBe(false);
    expect(focusHandles(null, " ")).toBe(false);
  });

  it("gives a field every key but Up and Down, a checkbox only Enter and Space, and a menu everything", () => {
    document.body.innerHTML = `<input id="text" /><input id="check" type="checkbox" /><div role="menu"><div id="item" role="menuitem"></div></div>`;
    const text = document.getElementById("text");
    const check = document.getElementById("check");
    expect(focusHandles(text, "ArrowLeft")).toBe(true);
    expect(focusHandles(text, "f")).toBe(true);
    expect(focusHandles(text, "ArrowDown")).toBe(false);
    expect(focusHandles(check, " ")).toBe(true);
    expect(focusHandles(check, "ArrowRight")).toBe(false);
    expect(focusHandles(document.getElementById("item"), "ArrowDown")).toBe(true);
  });
});
