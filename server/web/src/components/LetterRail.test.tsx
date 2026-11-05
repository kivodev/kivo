import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LetterRail } from "./LetterRail";

function renderRail() {
  const user = userEvent.setup();
  render(
    <I18nProvider i18n={i18n}>
      <button type="button">Before</button>
      <LetterRail letters={[]} onJump={vi.fn()} />
    </I18nProvider>,
  );
  const rail = screen.getByRole("slider", { name: "Jump to letter" });
  rail.setPointerCapture = () => {};
  rail.hasPointerCapture = () => true;
  return { user, rail };
}

describe("LetterRail", () => {
  it("shows its focus ring for the keyboard, and not after a scrub by mouse or finger", async () => {
    const { user, rail } = renderRail();
    // Scrubbed by the mouse, then by a finger. The rail is focused, with no
    // ring, during and after.
    for (const button of ["MouseLeft", "TouchA"]) {
      await user.pointer({ keys: `[${button}>]`, target: rail, coords: { clientY: 5 } });
      expect(rail).toHaveFocus();
      expect(rail).not.toHaveAttribute("data-focus-visible");
      await user.pointer({ keys: `[/${button}]`, target: rail, coords: { clientY: 40 } });
      expect(rail).toHaveFocus();
      expect(rail).not.toHaveAttribute("data-focus-visible");
      rail.blur();
    }

    // Tabbed to, it shows the ring.
    screen.getByRole("button", { name: "Before" }).focus();
    await user.tab();
    expect(rail).toHaveFocus();
    expect(rail).toHaveAttribute("data-focus-visible");

    // Pressed again, the ring goes. An arrow key brings it back.
    await user.pointer({ keys: "[MouseLeft]", target: rail, coords: { clientY: 5 } });
    expect(rail).not.toHaveAttribute("data-focus-visible");
    await user.keyboard("{ArrowDown}");
    expect(rail).toHaveAttribute("data-focus-visible");
  });
});
