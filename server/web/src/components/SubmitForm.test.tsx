import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SubmitForm } from "./SubmitForm";

describe("SubmitForm", () => {
  it("sends nothing more while a submit is pending, as a TV keyboard's Go pressed again would", () => {
    const onSubmit = vi.fn();
    const form = (isPending: boolean) => (
      <I18nProvider i18n={i18n}>
        <SubmitForm onSubmit={onSubmit} submitLabel="Sign in" isPending={isPending}>
          <input name="username" defaultValue="anna" />
        </SubmitForm>
      </I18nProvider>
    );
    const view = render(form(false));
    const element = screen.getByRole("textbox").closest("form");
    if (!element) throw new Error("no form");
    fireEvent.submit(element);
    view.rerender(form(true));
    fireEvent.submit(element);
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});
