import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { Select } from "./Select";
import { SettingsCard, SettingsLayout, SettingsRow, SettingsSection } from "./SettingsLayout";
import { Switch } from "./Switch";
import { TextField } from "./TextField";

const renderI18n = (ui: ReactNode) => render(<I18nProvider i18n={i18n}>{ui}</I18nProvider>);

describe("settings layout", () => {
  it("titles a section with its description and heads its cards with their titles", () => {
    renderI18n(
      <SettingsLayout>
        <SettingsSection id="device" title="This device" description="Only in this browser.">
          <SettingsCard title="Quality" footer="Home is on the server's local network.">
            <SettingsRow label="Local network quality" />
          </SettingsCard>
        </SettingsSection>
      </SettingsLayout>,
    );
    const section = screen.getByRole("region", { name: "This device" });
    expect(section).toHaveAttribute("id", "device");
    expect(within(section).getByText("Only in this browser.")).toBeInTheDocument();
    expect(within(section).getByRole("heading", { name: "Quality", level: 3 })).toBeInTheDocument();
    expect(within(section).getByText("Local network quality")).toBeInTheDocument();
    expect(within(section).getByText("Home is on the server's local network.")).toBeInTheDocument();
  });

  it("names a row's switch, dropdown and text field by its label and describes them by its help", () => {
    renderI18n(
      <SettingsCard>
        <SettingsRow label="Remember choices per show" description="Each show keeps its tracks.">
          <Switch isSelected onChange={() => {}} />
        </SettingsRow>
        <SettingsRow label="Theme">
          <Select
            options={[
              { id: "dark", label: "Dark" },
              { id: "light", label: "Light" },
            ]}
            selected="dark"
            onChange={() => {}}
          />
        </SettingsRow>
        <SettingsRow label="Server name" description="Shown to apps." error="Too long.">
          <TextField value="Den" />
        </SettingsRow>
      </SettingsCard>,
    );
    const name = screen.getByRole("textbox", { name: "Server name" });
    expect(name).toHaveAccessibleDescription("Shown to apps. Too long.");
    expect(name).toBeInvalid();
    const toggle = screen.getByRole("switch", { name: "Remember choices per show" });
    expect(toggle).toHaveAccessibleDescription("Each show keeps its tracks.");
    expect(screen.getByRole("button", { name: /Theme/ })).toHaveAccessibleName(/Dark/);
  });

  it("shows a spinner while saving, and an error with Retry", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    const { rerender } = renderI18n(
      <SettingsRow label="Audio language" isPending>
        <span>control</span>
      </SettingsRow>,
    );
    expect(screen.getByText("Audio language").closest("[aria-busy]")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    rerender(
      <I18nProvider i18n={i18n}>
        <SettingsRow label="Audio language" error="Couldn't save. Something went wrong." onRetry={onRetry}>
          <span>control</span>
        </SettingsRow>
      </I18nProvider>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't save. Something went wrong.");
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });
});
