import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Row } from "./Row";
import { revealTvCard } from "./tvLayout";

// A row's layout, which the test page doesn't have: each card is 90 px wide
// on a 100 px pitch, and the row shows 300 px of them.
const pitch = 100;
const across = 300;

function layOut() {
  const index = (el: HTMLElement) => (el.parentElement ? [...el.parentElement.children].indexOf(el) : 0);
  vi.spyOn(HTMLElement.prototype, "offsetLeft", "get").mockImplementation(function (this: HTMLElement) {
    return this.tagName === "LI" ? index(this) * pitch : 0;
  });
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(function (this: HTMLElement) {
    return this.tagName === "LI" ? pitch - 10 : 0;
  });
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (this: HTMLElement) {
    return this.tagName === "UL" ? across : 0;
  });
}

const ids = (n: number) => Array.from({ length: n }, (_, i) => `w${i}`);

function row(items: string[]) {
  return (
    <I18nProvider i18n={i18n}>
      <Row
        title="Row"
        shape="poster"
        items={items}
        itemKey={(id) => id}
        renderItem={(id) => <span>{id}</span>}
        id="row"
      />
    </I18nProvider>
  );
}

const trackOf = (container: HTMLElement) => container.querySelector('[data-part="row-cards"]') as HTMLElement;
const cardOf = (track: HTMLElement, id: string) =>
  [...track.querySelectorAll("li")].find((li) => li.textContent === id) as HTMLElement;

describe("a TV row whose cards change", () => {
  beforeEach(layOut);
  afterEach(() => vi.restoreAllMocks());

  it("moves its cards back once fewer remain, rather than showing empty track", () => {
    const view = render(row(ids(12)));
    const track = trackOf(view.container);
    revealTvCard(track, cardOf(track, "w11"));
    expect(track.style.transform).toBe("translateX(-890px)");
    view.rerender(row(ids(5)));
    // The five cards end at 490 px, so the track shows their last 300.
    expect(track.style.transform).toBe("translateX(-190px)");
    expect(track.style.transitionDuration).toBe("0s");
    view.rerender(row(ids(2)));
    expect(track.style.transform).toBe("translateX(0px)");
    expect(track.parentElement?.hasAttribute("data-tv-scrolled")).toBe(false);
  });

  it("shows the focused card where its row put it after a change", () => {
    const view = render(row(ids(12)));
    const track = trackOf(view.container);
    const card = cardOf(track, "w2");
    card.tabIndex = -1;
    card.focus();
    revealTvCard(track, card);
    expect(track.style.transform).toBe("");
    // Live data puts the focused work last: the track goes to it at once.
    view.rerender(row([...ids(12).filter((id) => id !== "w2"), "w2"]));
    expect(track.style.transform).toBe("translateX(-890px)");
    expect(track.style.transitionDuration).toBe("0s");
  });
});
