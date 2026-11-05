import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { GridKeyboard } from "./GridKeyboard";

const { lettersRendered } = vi.hoisted(() => ({ lettersRendered: vi.fn() }));
vi.mock("@/components/Button", async (original) => {
  const { Button } = await original<typeof import("@/components/Button")>();
  return {
    Button: (props: Parameters<typeof Button>[0]) => {
      if (typeof props.children === "string" && /^[a-z0-9]$/.test(props.children)) lettersRendered();
      return <Button {...props} />;
    },
  };
});

describe("TV keyboard rendering", () => {
  it("keeps unchanged character keys while its container changes, and uses updated callbacks", () => {
    const props = { name: "memo", onType: vi.fn(), onBackspace: vi.fn(), onClear: vi.fn() };
    const view = render(<GridKeyboard {...props} />, {
      wrapper: ({ children }) => <I18nProvider i18n={i18n}>{children}</I18nProvider>,
    });
    lettersRendered.mockClear();
    view.rerender(<GridKeyboard {...props} className="wide" />);
    expect(lettersRendered).not.toHaveBeenCalled();
    const next = vi.fn();
    view.rerender(<GridKeyboard {...props} onType={next} />);
    fireEvent.click(screen.getByRole("button", { name: "a" }));
    expect(next).toHaveBeenCalledExactlyOnceWith("a");
    expect(props.onType).not.toHaveBeenCalled();
  });
});
