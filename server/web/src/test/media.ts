// jsdom has no media pipeline, so a video element never loads, plays or
// fires media events. This stands in for an element whose media loads at
// once, so the player's real engines and controls run in tests. Events
// fire on a later task, as a browser's events do.

type MediaState = { time: number; paused: boolean; ended: boolean };

const states = new WeakMap<HTMLMediaElement, MediaState>();
const textTrackLists = new WeakMap<HTMLMediaElement, TextTrackList>();

function stateOf(element: HTMLMediaElement): MediaState {
  let state = states.get(element);
  if (!state) {
    state = { time: 0, paused: true, ended: false };
    states.set(element, state);
  }
  return state;
}

function fire(element: HTMLMediaElement, ...types: string[]) {
  setTimeout(() => {
    for (const type of types) element.dispatchEvent(new Event(type));
  });
}

export function installMediaElement() {
  const proto = HTMLMediaElement.prototype;
  Object.defineProperties(proto, {
    src: {
      configurable: true,
      get(this: HTMLMediaElement) {
        return this.getAttribute("src") ?? "";
      },
      set(this: HTMLMediaElement, value: string) {
        this.setAttribute("src", value);
        this.load();
      },
    },
    currentTime: {
      configurable: true,
      get(this: HTMLMediaElement) {
        return stateOf(this).time;
      },
      set(this: HTMLMediaElement, value: number) {
        const state = stateOf(this);
        state.time = value;
        state.ended = false;
        fire(this, "seeking", "seeked", "timeupdate");
      },
    },
    paused: {
      configurable: true,
      get(this: HTMLMediaElement) {
        return stateOf(this).paused;
      },
    },
    ended: {
      configurable: true,
      get(this: HTMLMediaElement) {
        return stateOf(this).ended;
      },
    },
    // jsdom's text track list isn't an event target, as a browser's is. An
    // empty one stands in, as no media here carries text tracks.
    textTracks: {
      configurable: true,
      get(this: HTMLMediaElement) {
        let list = textTrackLists.get(this);
        if (!list) {
          list = Object.assign(new EventTarget(), { length: 0 }) as unknown as TextTrackList;
          textTrackLists.set(this, list);
        }
        return list;
      },
    },
  });
  proto.load = function (this: HTMLMediaElement) {
    states.set(this, { time: 0, paused: true, ended: false });
    if (this.getAttribute("src")) fire(this, "loadedmetadata", "canplay");
  };
  proto.play = function (this: HTMLMediaElement) {
    const state = stateOf(this);
    if (state.paused) {
      state.paused = false;
      fire(this, "play", "playing");
    }
    return Promise.resolve();
  };
  proto.pause = function (this: HTMLMediaElement) {
    const state = stateOf(this);
    if (!state.paused) {
      state.paused = true;
      fire(this, "pause");
    }
  };
}

/** Plays a media element on to a time, in seconds, as frames would. */
export function playTo(element: HTMLMediaElement, seconds: number) {
  stateOf(element).time = seconds;
  element.dispatchEvent(new Event("timeupdate"));
}

/** Plays a media element to its end. */
export function playToEnd(element: HTMLMediaElement) {
  const state = stateOf(element);
  state.paused = true;
  state.ended = true;
  element.dispatchEvent(new Event("pause"));
  element.dispatchEvent(new Event("ended"));
}
