import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createPortal } from "react-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { goBack, useTvBack } from "./back";
import { TvFocusable, TvFocusGroup } from "./focus";
import { expectTvScreen, rememberTvFocus } from "./focusMemory";
import { focusTv, pauseTv, TV_CONTENT, TV_DOCK, TV_RAIL } from "./spatial";
import { startTv, stopTv } from "./start";

// jsdom lays nothing out, so each item gets a place on a 1080p screen by
// its focus key: [left, top, width, height].
function layOut(places: Record<string, readonly [number, number, number, number]>) {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
    const el = this as HTMLElement;
    const place = places[el.dataset.tvFocusKey ?? el.dataset.tvGroup ?? ""] ?? [0, 0, 1920, 1080];
    const [x, y, width, height] = place;
    return { x, y, left: x, top: y, width, height, right: x + width, bottom: y + height, toJSON: () => ({}) };
  });
}

const focused = () => document.activeElement?.getAttribute("data-tv-focus-key");
const press = (key: string, init: Partial<KeyboardEventInit> = {}) => {
  fireEvent.keyDown(document.activeElement ?? window, { key, ...init });
  fireEvent.keyUp(document.activeElement ?? window, { key, ...init });
};

// Two rows of three posters, the second scrolled one card along.
const rows = {
  "row:a": [200, 100, 1600, 300],
  "a:1": [200, 100, 200, 300],
  "a:2": [450, 100, 200, 300],
  "a:3": [700, 100, 200, 300],
  "row:b": [200, 500, 1600, 300],
  "b:1": [-50, 500, 200, 300],
  "b:2": [200, 500, 200, 300],
  "b:3": [450, 500, 200, 300],
} as const;

function Rows({ onPlay }: { onPlay?: () => void }) {
  return (
    <TvFocusGroup focusKey={TV_CONTENT}>
      {(["a", "b"] as const).map((row) => (
        <TvFocusGroup key={row} focusKey={`row:${row}`} as="section" label={`Row ${row}`}>
          {[1, 2, 3].map((n) => (
            <TvFocusable key={n} focusKey={`${row}:${n}`} kind="card">
              <button type="button" onClick={row === "a" && n === 2 ? onPlay : undefined}>
                {row}
                {n}
              </button>
            </TvFocusable>
          ))}
        </TvFocusGroup>
      ))}
    </TvFocusGroup>
  );
}

describe("TV focus", () => {
  beforeEach(() => {
    startTv();
    layOut(rows);
  });
  afterEach(() => {
    stopTv();
  });

  it("gives a screen opened anew its first item, and moves along a row with Left and Right", async () => {
    render(<Rows />);
    expectTvScreen("entry-1");
    await waitFor(() => expect(focused()).toBe("a:1"));
    expect(document.activeElement).toHaveAttribute("data-tv-focus", "card");
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("a:2"));
    press("ArrowLeft");
    await waitFor(() => expect(focused()).toBe("a:1"));
  });

  it("moves Up and Down to the card directly above or below as the rows are scrolled, not the one the row last had", async () => {
    render(<Rows />);
    focusTv("b:3");
    await waitFor(() => expect(focused()).toBe("b:3"));
    press("ArrowUp");
    // b:3 sits under a:2.
    await waitFor(() => expect(focused()).toBe("a:2"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("a:3"));
    press("ArrowDown");
    await waitFor(() => expect(focused()).toBe("b:3"));
  });

  it("measures only the rows that may hold the card above or below, not every card on the screen", async () => {
    // Six rows of three, each 400 pixels below the last, the fifth with no
    // box of its own, as with display: contents, so its cards are measured.
    const places: Record<string, [number, number, number, number]> = {};
    for (let r = 0; r < 6; r++) {
      const top = 100 + 400 * r;
      places[`row:${r}`] = r === 4 ? [0, 0, 0, 0] : [200, top, 1600, 300];
      for (let n = 0; n < 3; n++) places[`${r}:${n}`] = [200 + 250 * n, top, 200, 300];
    }
    layOut(places);
    render(
      <TvFocusGroup focusKey={TV_CONTENT}>
        {[0, 1, 2, 3, 4, 5].map((r) => (
          <TvFocusGroup key={r} focusKey={`row:${r}`}>
            {[0, 1, 2].map((n) => (
              <TvFocusable key={n} focusKey={`${r}:${n}`} kind="card">
                <button type="button">{`${r}${n}`}</button>
              </TvFocusable>
            ))}
          </TvFocusGroup>
        ))}
      </TvFocusGroup>,
    );
    focusTv("1:2");
    await waitFor(() => expect(focused()).toBe("1:2"));
    const spy = vi.mocked(Element.prototype.getBoundingClientRect);
    spy.mockClear();
    press("ArrowDown");
    await waitFor(() => expect(focused()).toBe("2:2"));
    const measured = new Set(spy.mock.contexts.map((el) => (el as HTMLElement).dataset.tvFocusKey).filter(Boolean));
    expect(measured).toEqual(new Set(["1:0", "1:1", "1:2", "2:0", "2:1", "2:2", "4:0", "4:1", "4:2"]));

    press("ArrowUp");
    await waitFor(() => expect(focused()).toBe("1:2"));
    press("ArrowUp");
    await waitFor(() => expect(focused()).toBe("0:2"));
  });

  // The row's group is measured afresh before Norigin moves within it
  // (spatial.ts), which then trusts what was just measured: each card is
  // measured once, not twice.
  it("measures each of a row's cards once for Right", async () => {
    render(<Rows />);
    focusTv("a:1");
    await waitFor(() => expect(focused()).toBe("a:1"));
    await new Promise((resolve) => setTimeout(resolve, 50));
    const spy = vi.mocked(Element.prototype.getBoundingClientRect);
    spy.mockClear();
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("a:2"));
    const times = (key: string) => spy.mock.contexts.filter((el) => (el as HTMLElement).dataset.tvFocusKey === key);
    // The card focus moves to is measured again as it takes focus.
    expect(times("a:3")).toHaveLength(1);
    expect(times("a:2")).toHaveLength(2);
  });

  it("moves along a line by its order, measuring neither it nor, for Up and Down, the focused card's own line", async () => {
    render(
      <TvFocusGroup focusKey={TV_CONTENT}>
        {(["a", "b"] as const).map((row) => (
          <TvFocusGroup key={row} focusKey={`row:${row}`} line>
            {[1, 2, 3].map((n) => (
              <TvFocusable key={n} focusKey={`${row}:${n}`} kind="card">
                <button type="button">{`${row}${n}`}</button>
              </TvFocusable>
            ))}
          </TvFocusGroup>
        ))}
      </TvFocusGroup>,
    );
    focusTv("b:1");
    await waitFor(() => expect(focused()).toBe("b:1"));
    const spy = vi.mocked(Element.prototype.getBoundingClientRect);
    const measured = () =>
      new Set(spy.mock.contexts.map((el) => (el as HTMLElement).dataset.tvFocusKey).filter(Boolean));
    spy.mockClear();
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("b:2"));
    expect(measured()).not.toContain("b:3");
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("b:3"));
    press("ArrowLeft");
    await waitFor(() => expect(focused()).toBe("b:2"));

    spy.mockClear();
    press("ArrowUp");
    // b:2 sits under a:1, as row b is scrolled one card along.
    await waitFor(() => expect(focused()).toBe("a:1"));
    expect(measured()).toEqual(new Set(["b:2", "a:1", "a:2", "a:3"]));
  });

  it("returns into a row at the card it left from", async () => {
    render(<Rows />);
    focusTv("a:3");
    await waitFor(() => expect(focused()).toBe("a:3"));
    focusTv("b:1");
    await waitFor(() => expect(focused()).toBe("b:1"));
    focusTv("row:a");
    await waitFor(() => expect(focused()).toBe("a:3"));
  });

  it("presses the button inside a card with OK", async () => {
    const onPlay = vi.fn();
    render(<Rows onPlay={onPlay} />);
    focusTv("a:2");
    await waitFor(() => expect(focused()).toBe("a:2"));
    press("Enter");
    await waitFor(() => expect(onPlay).toHaveBeenCalledOnce());
  });

  it("restores focus to the card a screen was left from when Back returns to it", async () => {
    const view = render(<Rows />);
    focusTv("b:2");
    await waitFor(() => expect(focused()).toBe("b:2"));
    rememberTvFocus("entry-1");
    view.unmount();
    render(<Rows />);
    expectTvScreen("entry-1");
    await waitFor(() => expect(focused()).toBe("b:2"));
  });

  it("restores focus before the screen is drawn when the card is there as Back arrives, rather than a task later", async () => {
    const view = render(<Rows />);
    focusTv("b:2");
    await waitFor(() => expect(focused()).toBe("b:2"));
    rememberTvFocus("entry-1");
    // The work's page it opened had focus on its Play.
    view.rerender(
      <TvFocusGroup focusKey={TV_CONTENT}>
        <TvFocusable focusKey="play">
          <button type="button">Play</button>
        </TvFocusable>
      </TvFocusGroup>,
    );
    focusTv("play");
    await waitFor(() => expect(focused()).toBe("play"));
    view.rerender(<Rows />);
    vi.useFakeTimers({ toFake: ["setTimeout", "requestAnimationFrame"] });
    try {
      expectTvScreen("entry-1");
      // Norigin focuses in promises, which settle before the next frame; no task or frame runs.
      for (let i = 0; i < 20; i++) await Promise.resolve();
      expect(focused()).toBe("b:2");
    } finally {
      vi.useRealTimers();
    }
  });

  it("doesn't take focus back from the rail for a screen that arrives after the rail was opened", async () => {
    const rail = (
      <TvFocusGroup focusKey={TV_RAIL}>
        <TvFocusable focusKey="rail:home">
          <button type="button">Home</button>
        </TvFocusable>
      </TvFocusGroup>
    );
    const view = render(rail);
    expectTvScreen("entry-1");
    focusTv("rail:home");
    await waitFor(() => expect(focused()).toBe("rail:home"));
    view.rerender(
      <>
        {rail}
        <Rows />
      </>,
    );
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(focused()).toBe("rail:home");
  });

  it("moves Down from the last row to the dock, and Up from the dock back to the card it came from", async () => {
    layOut({ ...rows, "dock:play": [200, 950, 80, 80], "dock:stop": [300, 950, 80, 80] });
    render(
      <>
        <Rows />
        <TvFocusGroup focusKey={TV_DOCK}>
          {["play", "stop"].map((name) => (
            <TvFocusable key={name} focusKey={`dock:${name}`}>
              <button type="button">{name}</button>
            </TvFocusable>
          ))}
        </TvFocusGroup>
      </>,
    );
    focusTv("b:3");
    await waitFor(() => expect(focused()).toBe("b:3"));
    press("ArrowDown");
    await waitFor(() => expect(focused()).toMatch(/^dock:/));
    press("ArrowUp");
    await waitFor(() => expect(focused()).toBe("b:3"));
  });

  it("keeps Up and Down in a column, going on from its last row to the dock and back", async () => {
    // A key beside the rows, a line nearer below row a than row b is, as
    // Search's keyboard is beside its results.
    layOut({
      ...rows,
      "row:b": [200, 600, 1600, 300],
      "b:1": [-50, 600, 200, 300],
      "b:2": [200, 600, 200, 300],
      "b:3": [450, 600, 200, 300],
      key: [0, 410, 150, 60],
      "dock:play": [200, 950, 80, 80],
      "dock:stop": [300, 950, 80, 80],
    });
    render(
      <>
        <TvFocusGroup focusKey={TV_CONTENT}>
          <TvFocusable focusKey="key">
            <button type="button">key</button>
          </TvFocusable>
          <TvFocusGroup focusKey="results" column>
            {(["a", "b"] as const).map((row) => (
              <TvFocusGroup key={row} focusKey={`row:${row}`}>
                {[1, 2, 3].map((n) => (
                  <TvFocusable key={n} focusKey={`${row}:${n}`} kind="card">
                    <button type="button">{`${row}${n}`}</button>
                  </TvFocusable>
                ))}
              </TvFocusGroup>
            ))}
          </TvFocusGroup>
        </TvFocusGroup>
        <TvFocusGroup focusKey={TV_DOCK}>
          {["play", "stop"].map((name) => (
            <TvFocusable key={name} focusKey={`dock:${name}`}>
              <button type="button">{name}</button>
            </TvFocusable>
          ))}
        </TvFocusGroup>
      </>,
    );
    focusTv("a:1");
    await waitFor(() => expect(focused()).toBe("a:1"));
    press("ArrowUp");
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(focused()).toBe("a:1");
    press("ArrowDown");
    // a:1 sits over b:2, as row b is scrolled.
    await waitFor(() => expect(focused()).toBe("b:2"));
    press("ArrowDown");
    await waitFor(() => expect(focused()).toMatch(/^dock:/));
    press("ArrowUp");
    await waitFor(() => expect(focused()).toBe("b:2"));
  });

  it("moves focus with the arrows even where a widget stops them, and leaves a field its Left and Right", async () => {
    render(
      <TvFocusGroup focusKey={TV_CONTENT}>
        <TvFocusGroup focusKey="row:a">
          <TvFocusable focusKey="a:1">
            {/* biome-ignore lint/a11y/noStaticElementInteractions: a stand-in for a widget that keeps its arrows. */}
            <div onKeyDown={(e) => e.stopPropagation()}>
              <input aria-label="Name" />
            </div>
          </TvFocusable>
          <TvFocusable focusKey="a:2">
            <button type="button">a2</button>
          </TvFocusable>
        </TvFocusGroup>
      </TvFocusGroup>,
    );
    focusTv("a:1");
    await waitFor(() => expect(focused()).toBe("a:1"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("a:2"));

    focusTv("a:1");
    await waitFor(() => expect(focused()).toBe("a:1"));
    const input = screen.getByRole("textbox", { name: "Name" });
    input.focus();
    const right = new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true, cancelable: true });
    input.dispatchEvent(right);
    expect(right.defaultPrevented).toBe(false);
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(document.activeElement).toBe(input);
  });

  it("leaves the arrows to a grid that moves focus itself, and moves on with those it doesn't take", async () => {
    const onKeyDown = vi.fn((e: React.KeyboardEvent) => {
      if (e.key === "ArrowRight") e.preventDefault();
    });
    render(
      <TvFocusGroup focusKey={TV_CONTENT}>
        <TvFocusGroup focusKey="row:a" ownKeys>
          {/* biome-ignore lint/a11y/noStaticElementInteractions: a stand-in for the grid's own arrow keys. */}
          <div onKeyDown={onKeyDown}>
            {[1, 2, 3].map((n) => (
              <TvFocusable key={n} focusKey={`a:${n}`}>
                <button type="button">a{n}</button>
              </TvFocusable>
            ))}
          </div>
        </TvFocusGroup>
      </TvFocusGroup>,
    );
    focusTv("a:2");
    await waitFor(() => expect(focused()).toBe("a:2"));
    press("ArrowRight");
    expect(onKeyDown).toHaveBeenCalled();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(focused()).toBe("a:2");
    press("ArrowLeft");
    await waitFor(() => expect(focused()).toBe("a:1"));
  });

  it("hands focus that returns to a control inside an item, as from a closed popover, to the item", async () => {
    render(<Rows />);
    focusTv("a:2");
    await waitFor(() => expect(focused()).toBe("a:2"));
    screen.getByRole("button", { name: "b1" }).focus();
    await waitFor(() => expect(focused()).toBe("b:1"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toBe("b:2"));
  });

  it("leaves focus in a popover the item opened, which React renders in a portal inside it", async () => {
    render(
      <TvFocusGroup focusKey={TV_CONTENT}>
        <TvFocusable focusKey="opener">
          <button type="button">Filter</button>
          {createPortal(
            <TvFocusGroup focusKey="popover" trap>
              {[1, 2].map((n) => (
                <TvFocusable key={n} focusKey={`option:${n}`}>
                  <button type="button">option {n}</button>
                </TvFocusable>
              ))}
            </TvFocusGroup>,
            document.body,
          )}
        </TvFocusable>
      </TvFocusGroup>,
    );
    focusTv("option:1");
    await waitFor(() => expect(focused()).toBe("option:1"));
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(focused()).toBe("option:1");
  });

  it("leaves the arrows to the player while it is paused", async () => {
    render(<Rows />);
    focusTv("a:1");
    await waitFor(() => expect(focused()).toBe("a:1"));
    pauseTv(true);
    const right = new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true, cancelable: true });
    document.activeElement?.dispatchEvent(right);
    expect(right.defaultPrevented).toBe(false);
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(focused()).toBe("a:1");
    pauseTv(false);
  });
});

function Dialog({ onClose }: { onClose: () => boolean | undefined }) {
  useTvBack(onClose);
  return <p>Dialog</p>;
}

describe("TV Back", () => {
  beforeEach(() => {
    startTv();
  });
  afterEach(() => {
    stopTv();
  });

  it.each([
    ["webOS's Back", { key: "Unidentified", keyCode: 461 }],
    ["Tizen's Back", { key: "Unidentified", keyCode: 10009 }],
    ["XF86Back", { key: "XF86Back" }],
    ["BrowserBack", { key: "BrowserBack" }],
    ["Escape", { key: "Escape" }],
  ])("takes %s as Back", (_name, init) => {
    const onBack = vi.fn(() => true);
    render(<Dialog onClose={onBack} />);
    const event = new KeyboardEvent("keydown", { ...init, bubbles: true, cancelable: true });
    window.dispatchEvent(event);
    expect(onBack).toHaveBeenCalledOnce();
    expect(event.defaultPrevented).toBe(true);
  });

  it("gives Back to the newest handler first, which can pass it on", () => {
    const order: string[] = [];
    render(
      <>
        <Dialog
          onClose={() => {
            order.push("screen");
            return true;
          }}
        />
        <Dialog
          onClose={() => {
            order.push("dialog");
            return false;
          }}
        />
      </>,
    );
    goBack();
    expect(order).toEqual(["dialog", "screen"]);
  });

  it("ignores a held Back's repeats, and Back while the player has the keys", () => {
    const onBack = vi.fn(() => true);
    render(<Dialog onClose={onBack} />);
    fireEvent.keyDown(window, { key: "XF86Back", repeat: true });
    pauseTv(true);
    fireEvent.keyDown(window, { key: "XF86Back" });
    pauseTv(false);
    expect(onBack).not.toHaveBeenCalled();
    expect(screen.getByText("Dialog")).toBeInTheDocument();
  });
});
