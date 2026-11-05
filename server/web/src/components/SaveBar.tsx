import { Trans } from "@lingui/react/macro";
import { Button } from "./Button";
import { FormError } from "./FormError";

type Props = {
  /** Whether the page differs from what's saved. The bar shows only then. */
  isDirty: boolean;
  isPending: boolean;
  /** Something stops Save, such as a required choice not made yet. */
  isSaveDisabled?: boolean;
  /** The problem that made the last save fail. Nothing shows while it's null. */
  error?: unknown;
  onDiscard: () => void;
};

/**
 * A bar for a page's unsaved changes, with "Unsaved changes", Discard and
 * Save. Save submits the form the bar is in. The bar stays in view at the
 * bottom of the window while the page scrolls, above the bottom bar and
 * the mini player when they show. It goes away once the page matches
 * what's saved, and keeps showing a failed save's error.
 */
export function SaveBar({ isDirty, isPending, isSaveDisabled = false, error, onDiscard }: Props) {
  if (!isDirty) return null;
  return (
    <div
      data-part="save-bar"
      className="sticky bottom-0 z-30 max-w-(--layout-settings-width) py-3 group-data-listening/main:bottom-(--layout-mini-player-height) phone:bottom-(--layout-bottom-bar-inset) phone:group-data-listening/main:bottom-(--layout-bottom-bar-and-mini-player-height)"
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border bg-surface-raised px-4 py-3 shadow-overlay">
        <div className="flex min-w-0 grow basis-32 flex-col gap-0.5">
          <p className="text-sm font-medium text-text">
            <Trans>Unsaved changes</Trans>
          </p>
          <FormError error={error} />
        </div>
        <div className="flex gap-2">
          <Button variant="ghost" isDisabled={isPending} onPress={onDiscard}>
            <Trans>Discard</Trans>
          </Button>
          <Button type="submit" variant="primary" isDisabled={isSaveDisabled} isPending={isPending}>
            <Trans>Save</Trans>
          </Button>
        </div>
      </div>
    </div>
  );
}
