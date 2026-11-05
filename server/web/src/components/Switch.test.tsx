import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Switch } from "./Switch";

describe("Switch", () => {
  it("is a switch that flips on press and from the keyboard", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = render(<Switch aria-label="Wi-Fi only" isSelected={false} onChange={onChange} />);
    const toggle = screen.getByRole("switch", { name: "Wi-Fi only" });
    expect(toggle).not.toBeChecked();
    await user.click(toggle);
    expect(onChange).toHaveBeenLastCalledWith(true);

    rerender(<Switch aria-label="Wi-Fi only" isSelected onChange={onChange} />);
    expect(toggle).toBeChecked();
    toggle.focus();
    await user.keyboard(" ");
    expect(onChange).toHaveBeenLastCalledWith(false);
  });

  it("does nothing while disabled", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Switch aria-label="Wi-Fi only" isSelected={false} isDisabled onChange={onChange} />);
    await user.click(screen.getByRole("switch", { name: "Wi-Fi only" }));
    expect(onChange).not.toHaveBeenCalled();
  });
});
