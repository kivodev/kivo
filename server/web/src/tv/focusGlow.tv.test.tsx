import { render, screen } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useFocusGlow } from "./focusGlow";

// jsdom lays nothing out, so each card sits 300 pixels past the last, in
// its row.
beforeEach(() => {
  vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame"] });
  vi.spyOn(HTMLElement.prototype, "offsetParent", "get").mockImplementation(function (this: HTMLElement) {
    return this.closest("li")?.parentElement ?? null;
  });
  vi.spyOn(HTMLElement.prototype, "offsetLeft", "get").mockImplementation(function (this: HTMLElement) {
    return 300 * Number(this.dataset.n ?? 0);
  });
});
afterEach(() => {
  vi.useRealTimers();
});

function Cards() {
  const list = useRef<HTMLUListElement>(null);
  const { glows, onFocus, onBlur } = useFocusGlow(list);
  return (
    <ul ref={list} onFocus={onFocus} onBlur={onBlur}>
      {[0, 1, 2].map((n) => (
        <li key={n}>
          <button type="button" data-tv-focusable="">
            <div data-tv-ring="" data-n={n} />
            {`card ${n}`}
          </button>
        </li>
      ))}
      <li>{glows}</li>
    </ul>
  );
}

const glow = () => document.querySelector<HTMLElement>('[data-part="tv-glow"]') as HTMLElement;

it("shows on the card focus lands on without sliding there, as Back restores focus, and slides between cards after", () => {
  render(<Cards />);
  screen.getByRole("button", { name: "card 2" }).focus();
  expect(glow()).toHaveAttribute("data-shown");
  expect(glow().style.transform).toContain("translate(600px, 0px)");
  // Until a frame has drawn it there, only its fade-in transitions, not its place.
  expect(glow().style.transitionProperty).toBe("opacity");
  vi.advanceTimersToNextFrame();
  expect(glow().style.transitionProperty).toBe("opacity");
  vi.advanceTimersToNextFrame();
  expect(glow().style.transitionProperty).toBe("");
  // Moving along the row, it slides by the stylesheet's transition.
  screen.getByRole("button", { name: "card 1" }).focus();
  expect(glow().style.transform).toContain("translate(300px, 0px)");
  expect(glow().style.transitionProperty).toBe("");
});

it("shows afresh without sliding after focus left the cards, however soon", () => {
  render(
    <>
      <Cards />
      <button type="button">elsewhere</button>
    </>,
  );
  screen.getByRole("button", { name: "card 0" }).focus();
  vi.advanceTimersToNextFrame();
  screen.getByRole("button", { name: "elsewhere" }).focus();
  expect(glow()).not.toHaveAttribute("data-shown");
  screen.getByRole("button", { name: "card 2" }).focus();
  // The first showing's frames don't give it its transition back early.
  vi.advanceTimersToNextFrame();
  expect(glow().style.transitionProperty).toBe("opacity");
  vi.advanceTimersToNextFrame();
  expect(glow().style.transitionProperty).toBe("");
});
