import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Select } from "./Select";

describe("Select", () => {
  it("shows the choice and picks another from the list", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <I18nProvider i18n={i18n}>
        <Select
          label="Subtitle size"
          options={[
            { id: "small", label: "Small" },
            { id: "medium", label: "Medium" },
            { id: "large", label: "Large" },
          ]}
          selected="medium"
          onChange={onChange}
        />
      </I18nProvider>,
    );
    const trigger = screen.getByRole("button", { name: /Subtitle size/ });
    expect(trigger).toHaveAccessibleName(/Medium/);
    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: "Large" }));
    expect(onChange).toHaveBeenCalledWith("large");
  });

  it("shows a compact select's short words, and lists each option with its detail", async () => {
    const user = userEvent.setup();
    render(
      <I18nProvider i18n={i18n}>
        <Select
          variant="compact"
          label="Sort"
          options={[
            { id: "title", label: "Title", detail: "From A to Z" },
            { id: "addedAt", label: "Date added" },
          ]}
          selected="title"
          value="A–Z"
          onChange={() => {}}
        />
      </I18nProvider>,
    );
    const trigger = screen.getByRole("button", { name: /Sort/ });
    expect(trigger).toHaveTextContent("A–Z");
    await user.click(trigger);
    expect(await screen.findByRole("option", { name: "Title" })).toHaveAccessibleDescription("From A to Z");
  });

  it("keeps a disabled option from being chosen", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <I18nProvider i18n={i18n}>
        <Select
          variant="form"
          label="Version"
          options={[
            { id: "a", label: "4K" },
            { id: "b", label: "1080p", disabled: true },
          ]}
          selected="a"
          onChange={onChange}
        />
      </I18nProvider>,
    );
    expect(screen.getByText("Version")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Version/ }));
    await user.click(await screen.findByRole("option", { name: "1080p" }));
    expect(onChange).not.toHaveBeenCalled();
  });
});
