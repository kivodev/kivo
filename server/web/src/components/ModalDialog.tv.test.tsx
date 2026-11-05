import { I18nProvider } from "@lingui/react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/i18n";
import { TvFocusGroup } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { focusTv, TV_CONTENT } from "@/tv/spatial";
import { startTv, stopTv } from "@/tv/start";
import { ModalDialog, useDismissal } from "./ModalDialog";

// React Aria hands focus back to the opener by itself, so the test watches
// where the dialog asks Norigin to put it.
vi.mock("@/tv/spatial", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/tv/spatial")>();
  return { ...actual, focusTv: vi.fn(actual.focusTv) };
});

const focused = () => document.activeElement?.getAttribute("data-tv-focus-key");
const press = (key: string) => {
  fireEvent.keyDown(document.activeElement ?? window, { key });
  fireEvent.keyUp(document.activeElement ?? window, { key });
};
const back = () => fireEvent.keyDown(window, { key: "XF86Back" });

// A form that has something to lose, as an edit dialog does.
function EditForm({ onClose }: { onClose: () => void }) {
  useDismissal("ask");
  return (
    <>
      <TvControl focusKey="edit:name">
        <input aria-label="Name" />
      </TvControl>
      <TvControl focusKey="edit:close" onPress={onClose}>
        <button type="button">Close</button>
      </TvControl>
    </>
  );
}

function Screen() {
  const [open, setOpen] = useState(false);
  return (
    <TvFocusGroup focusKey={TV_CONTENT}>
      <TvControl focusKey="opener" onPress={() => setOpen(true)}>
        <button type="button">Edit</button>
      </TvControl>
      <ModalDialog isOpen={open} onClose={() => setOpen(false)} title="Edit">
        <EditForm onClose={() => setOpen(false)} />
      </ModalDialog>
    </TvFocusGroup>
  );
}

describe("a dialog on TV", () => {
  beforeEach(() => {
    startTv();
  });
  afterEach(() => {
    stopTv();
  });

  it("returns focus to its opener after the discard question was asked and cancelled", async () => {
    render(
      <I18nProvider i18n={i18n}>
        <Screen />
      </I18nProvider>,
    );
    focusTv("opener");
    await waitFor(() => expect(focused()).toBe("opener"));
    press("Enter");
    await screen.findByRole("dialog", { name: "Edit" });
    focusTv("edit:name");
    await waitFor(() => expect(focused()).toBe("edit:name"));

    // Back asks before discarding, with Cancel focused.
    back();
    await screen.findByRole("alertdialog", { name: "Discard changes?" });
    await waitFor(() => expect(focused()).toBe("tv:confirm:cancel"));

    // Cancel returns to the field, the dialog stays, and the question is gone.
    press("Enter");
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    await waitFor(() => expect(focused()).toBe("edit:name"));
    expect(screen.getByRole("dialog", { name: "Edit" })).toBeInTheDocument();

    // Closing the dialog sends focus to what opened it, not to the question's Cancel, which is gone.
    focusTv("edit:close");
    await waitFor(() => expect(focused()).toBe("edit:close"));
    vi.mocked(focusTv).mockClear();
    press("Enter");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(focused()).toBe("opener"));
    expect(vi.mocked(focusTv).mock.calls.map(([key]) => key)).toEqual(["opener"]);
  });
});
