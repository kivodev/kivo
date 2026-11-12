import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { render, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Row } from "./Row";

describe("Row", () => {
  it("goes back to its start when its first card changes, and stays put when cards join after it", () => {
    const row = (items: string[]) => (
      <I18nProvider i18n={i18n}>
        <Row title="Shelf" shape="poster" items={items} itemKey={(item) => item} renderItem={(item) => item} />
      </I18nProvider>
    );
    const { container, rerender } = render(row(["a", "b", "c"]));
    const cards = container.querySelector("[data-part='row-cards']") as HTMLElement;
    cards.scrollLeft = 300;
    rerender(row(["a", "b", "c", "d"]));
    expect(cards.scrollLeft).toBe(300);
    rerender(row(["z", "a", "b", "c", "d"]));
    expect(cards.scrollLeft).toBe(0);
  });

  it("draws a staged row's first cards at once and the rest after", async () => {
    const items = Array.from({ length: 20 }, (_, i) => `w${i}`);
    const row = (staged: boolean) => (
      <I18nProvider i18n={i18n}>
        <Row
          title="Shelf"
          shape="poster"
          items={items}
          itemKey={(item) => item}
          renderItem={(item) => item}
          staged={staged}
        />
      </I18nProvider>
    );
    const whole = render(row(false));
    expect(whole.container.querySelectorAll("li")).toHaveLength(20);
    whole.unmount();

    const { container } = render(row(true));
    // Seven 152 px posters, 16 px apart, fit across the 1024 px window, and one more is drawn.
    expect(container.querySelectorAll("li")).toHaveLength(8);
    await waitFor(() => expect(container.querySelectorAll("li")).toHaveLength(20));
  });
});
