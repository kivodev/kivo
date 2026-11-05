import { useLingui } from "@lingui/react/macro";
import { createContext, type ReactNode, useContext, useEffect, useId, useState } from "react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import { TvFocusGroup } from "@/tv/focus";
import { useTvDialog } from "@/tv/forms/dialog";
import { Button } from "./Button";
import { ConfirmDialog } from "./ConfirmDialog";
import { Icon } from "./Icon";

type Props = {
  isOpen: boolean;
  onClose: () => void;
  title: ReactNode;
  /** A thumbnail beside the title. */
  image?: ReactNode;
  /** A muted line under the title. */
  subtitle?: ReactNode;
  /** `form` for a few fields, `prose` for a longer form, `page` for side-by-side content. */
  width?: "form" | "prose" | "page";
  /**
   * `center` shows the dialog over the page. `side` is for details beside
   * what opened them. It is a panel along the right edge, or on a phone a
   * sheet from the bottom, with a close button.
   */
  placement?: "center" | "side";
  /** Which dialog, for tests. */
  part?: string;
  children: ReactNode;
};

/**
 * What Escape and a press outside do to a dialog. `close` closes it. `ask`
 * asks before discarding what its form holds. `never` does nothing, for
 * something shown once that closes only with its own button.
 */
export type Dismissal = "close" | "ask" | "never";

const DismissalContext = createContext<(d: Dismissal) => void>(() => {});

/** Sets what Escape and a press outside do to the dialog around the caller, while it's mounted. */
export function useDismissal(dismissal: Dismissal) {
  const set = useContext(DismissalContext);
  useEffect(() => {
    set(dismissal);
    return () => set("close");
  }, [set, dismissal]);
}

const widths = { form: "max-w-form", prose: "max-w-prose", page: "max-w-page" } as const;

const placements = {
  center: {
    overlay: "items-center justify-center p-4",
    modal: "rounded-lg border",
    dialog: "",
  },
  side: {
    overlay: "items-end pt-12 sm:items-stretch sm:justify-end sm:pt-0",
    modal: "rounded-t-lg border-t sm:h-full sm:rounded-none sm:border-t-0 sm:border-l",
    // A sheet along the bottom keeps its content above the home indicator.
    dialog: "pb-(--layout-safe-area-bottom)",
  },
} as const;

/**
 * A titled dialog over the page. Escape and a press outside close it,
 * unless its content says otherwise with useDismissal. Focus returns to
 * whatever opened it.
 */
export function ModalDialog({
  isOpen,
  onClose,
  title,
  image,
  subtitle,
  width = "prose",
  placement = "center",
  part,
  children,
}: Props) {
  const { t } = useLingui();
  const [dismissal, setDismissal] = useState<Dismissal>("close");
  const [asking, setAsking] = useState(false);
  const p = placements[placement];
  const dismiss = () => {
    if (dismissal === "ask") setAsking(true);
    else if (dismissal === "close") onClose();
  };
  // On TV, Back dismisses as Escape does, and focus stays inside while it's
  // open. While the discard confirmation is up, Back is its.
  useTvDialog(isOpen, dismiss, !asking);
  const focusKey = `tv:dialog:${useId()}`;
  return (
    <ModalOverlay
      isOpen={isOpen}
      onOpenChange={(open) => !open && dismiss()}
      isDismissable
      className={`fixed inset-x-0 top-0 z-50 flex h-(--visual-viewport-height) bg-scrim-soft ${p.overlay}`}
    >
      <Modal
        className={`max-h-full w-full ${widths[width]} overflow-auto border-border bg-surface-raised p-6 shadow-overlay ${p.modal}`}
      >
        <Dialog data-part={part} className={`outline-none ${p.dialog}`}>
          <div className="mb-4 flex items-start justify-between gap-4">
            <div className="flex min-w-0 items-start gap-3.5">
              {image}
              <div className="flex min-w-0 flex-col gap-1">
                <Heading slot="title" className="min-w-0 text-xl font-semibold">
                  {title}
                </Heading>
                {subtitle && <p className="text-sm text-text-muted">{subtitle}</p>}
              </div>
            </div>
            {placement === "side" && (
              <Button variant="ghost" size="icon" aria-label={t`Close`} className="-mt-2 -mr-3" onPress={dismiss}>
                <Icon name="close" />
              </Button>
            )}
          </div>
          <DismissalContext value={setDismissal}>
            <TvFocusGroup focusKey={focusKey} trap>
              {children}
            </TvFocusGroup>
          </DismissalContext>
          <ConfirmDialog
            isOpen={asking}
            title={t`Discard changes?`}
            confirmLabel={t`Discard`}
            tone="danger"
            onCancel={() => setAsking(false)}
            onConfirm={() => {
              setAsking(false);
              onClose();
            }}
          />
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}
