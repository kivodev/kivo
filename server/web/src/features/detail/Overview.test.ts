import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cutOverview } from "./Overview";

describe("cutOverview", () => {
  // A line holds ten characters and the paragraph shows two lines.
  beforeEach(() => {
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockImplementation(function (this: HTMLElement) {
      return Math.ceil((this.textContent ?? "").length / 10) * 20;
    });
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(40);
  });
  afterEach(() => vi.restoreAllMocks());

  const paragraph = () => {
    const parent = document.createElement("div");
    const el = document.createElement("p");
    parent.append(el);
    document.body.append(parent);
    return el;
  };

  it("keeps a text that fits", () => {
    expect(cutOverview(paragraph(), "Short one", "More")).toBeUndefined();
  });

  it("ends a long text at a word with an ellipsis, leaving room for More", () => {
    const el = paragraph();
    expect(cutOverview(el, "A spy, a thief and a pilot rob the moon", "More")).toBe("A spy, a thief…");
    // Its hidden copy is gone.
    expect(el.parentElement?.childElementCount).toBe(1);
  });
});
