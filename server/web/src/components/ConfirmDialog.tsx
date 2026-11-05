import { Trans } from "@lingui/react/macro";
import type { ReactNode } from "react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import { isTv } from "@/tv/flag";
import { TvFocusGroup } from "@/tv/focus";
import { TvDialogButton, useTvDialog } from "@/tv/forms/dialog";
import { Button } from "./Button";

type Props = {
  isOpen: boolean;
  title: string;
  /** What saying yes does, under the question. */
  children?: ReactNode;
  confirmLabel: string;
  /** `danger` for a yes that can't be undone, such as deleting. */
  tone?: "default" | "danger";
  onConfirm: () => void;
  onCancel: () => void;
};

/**
 * A yes-or-no question. Focus starts on Cancel, and returns to the opener on
 * close. On TV focus stays inside it, and Back cancels.
 */
export function ConfirmDialog({ isOpen, title, children, confirmLabel, tone = "default", onConfirm, onCancel }: Props) {
  useTvDialog(isOpen, onCancel);
  return (
    <ModalOverlay
      isOpen={isOpen}
      onOpenChange={(open) => !open && onCancel()}
      isDismissable
      className="fixed inset-x-0 top-0 z-50 flex h-(--visual-viewport-height) items-center justify-center bg-scrim-soft p-4"
    >
      <Modal className="w-full max-w-form rounded-lg border border-border bg-surface-raised p-6 shadow-overlay tv:max-w-prose tv:p-10">
        <Dialog role="alertdialog" className="outline-none">
          <Heading slot="title" className="text-lg font-semibold">
            {title}
          </Heading>
          {children && <p className="mt-3 text-text-muted">{children}</p>}
          {isTv ? (
            <TvFocusGroup focusKey="tv:confirm" trap part="confirm-actions" className="mt-10 flex justify-end gap-4">
              <TvDialogButton focusKey="tv:confirm:cancel" autoFocus onPress={onCancel}>
                <Trans>Cancel</Trans>
              </TvDialogButton>
              <TvDialogButton focusKey="tv:confirm:yes" tone={tone} onPress={onConfirm}>
                {confirmLabel}
              </TvDialogButton>
            </TvFocusGroup>
          ) : (
            <div className="mt-6 flex justify-end gap-2">
              <Button autoFocus onPress={onCancel}>
                <Trans>Cancel</Trans>
              </Button>
              <Button variant={tone === "danger" ? "danger" : "primary"} onPress={onConfirm}>
                {confirmLabel}
              </Button>
            </div>
          )}
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}
