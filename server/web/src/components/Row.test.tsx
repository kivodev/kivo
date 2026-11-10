import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { render } from "@testing-library/react";
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
});
