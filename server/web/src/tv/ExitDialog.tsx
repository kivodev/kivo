import { Trans, useLingui } from "@lingui/react/macro";
import { type ReactNode, useEffect, useState } from "react";
import { Button, Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import { buttonClass } from "@/components/Button";
import { useTvBack, useTvShellBack } from "./back";
import { exitApp } from "./exit";
import { isTv } from "./flag";
import { TvFocusGroup, useTvFocusable } from "./focus";
import { currentTvFocus, focusTv } from "./spatial";

const exitButtonClass = `${buttonClass("secondary")} h-auto px-8 py-3 text-base`;

/** "Exit Kivo?" with No focused. No, or Back, returns focus to where it was. */
export function ExitDialog({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const { t } = useLingui();
  const [returnTo, setReturnTo] = useState<string>();
  const [wasOpen, setWasOpen] = useState(isOpen);
  if (isOpen !== wasOpen) {
    setWasOpen(isOpen);
    if (isOpen) setReturnTo(currentTvFocus());
  }
  const close = () => {
    onClose();
    if (returnTo !== undefined) focusTv(returnTo);
  };
  useTvBack(() => {
    close();
    return true;
  }, isOpen);
  return (
    <ModalOverlay
      isOpen={isOpen}
      onOpenChange={(open) => !open && close()}
      isDismissable
      className="fixed inset-0 z-50 flex items-center justify-center bg-scrim"
    >
      <Modal className="w-full max-w-form rounded-lg bg-surface-raised p-10 shadow-overlay">
        <Dialog role="alertdialog" aria-label={t`Exit Kivo?`} data-part="tv-exit" className="outline-none">
          <Heading slot="title" className="text-xl font-semibold">
            <Trans>Exit Kivo?</Trans>
          </Heading>
          <TvFocusGroup focusKey="tv:exit" trap className="mt-10 flex justify-end gap-4">
            <ExitButton focusKey="tv:exit:no" onPress={close} autoFocus>
              <Trans>No</Trans>
            </ExitButton>
            <ExitButton focusKey="tv:exit:yes" onPress={exitApp}>
              <Trans>Exit</Trans>
            </ExitButton>
          </TvFocusGroup>
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}

function ExitButton({
  focusKey,
  onPress,
  autoFocus = false,
  children,
}: {
  focusKey: string;
  onPress: () => void;
  autoFocus?: boolean;
  children: ReactNode;
}) {
  const { ref, attributes } = useTvFocusable<HTMLButtonElement>({ focusKey });
  useEffect(() => {
    if (autoFocus) focusTv(focusKey);
  }, [autoFocus, focusKey]);
  return (
    <Button
      ref={ref}
      {...attributes}
      data-tv-focus="button"
      data-tv-face=""
      onPress={onPress}
      className={exitButtonClass}
    >
      {children}
    </Button>
  );
}

/**
 * Back on a TV screen outside the signed-in shell, such as sign-in or setup,
 * when the screen doesn't handle it: it asks to exit, as Back at Home does.
 * Back must do something on the first screen, and the shell's own Back
 * isn't there to. Outside the TV build it renders nothing.
 */
export function TvExitOnBack() {
  const [exiting, setExiting] = useState(false);
  useTvShellBack(() => setExiting(true));
  if (!isTv) return null;
  return <ExitDialog isOpen={exiting} onClose={() => setExiting(false)} />;
}
