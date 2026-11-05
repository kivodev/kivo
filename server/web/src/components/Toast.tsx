import { useLingui } from "@lingui/react/macro";
import {
  Button,
  Text,
  UNSTABLE_Toast as Toast,
  UNSTABLE_ToastContent as ToastContent,
  UNSTABLE_ToastQueue as ToastQueue,
  UNSTABLE_ToastRegion as ToastRegion,
} from "react-aria-components";
import { isTv } from "@/tv/flag";
import { Icon } from "./Icon";

/** A notice of what an action did, with a title and a line of detail. */
export type Notice = { title: string; detail?: string; tone?: "success" | "danger" };

const queue = new ToastQueue<Notice>({ maxVisibleToasts: 3 });

/**
 * Shows a notice in the corner. A success notice closes itself after a few
 * seconds. A failure stays until it's closed.
 */
export function toast(notice: Notice) {
  // A failure stays until closed, but the remote can't reach Close, so on
  // TV it times out too, longer than a success to give time to read it.
  const timeout = notice.tone === "danger" ? (isTv ? 10_000 : undefined) : 5000;
  queue.add(notice, timeout === undefined ? undefined : { timeout });
}

/** Where notices show, in the bottom right with the newest last. The app mounts it once. */
export function Toasts() {
  const { t } = useLingui();
  return (
    <ToastRegion
      queue={queue}
      className="fixed right-4 bottom-4 left-4 z-50 flex flex-col gap-2 outline-none sm:left-auto sm:w-80"
    >
      {({ toast: shown }) => (
        <Toast
          toast={shown}
          data-part="toast"
          data-tone={shown.content.tone ?? "success"}
          className="flex items-start gap-3 rounded-md border border-border bg-surface-raised p-3 text-sm shadow-overlay outline-none"
        >
          <Icon
            name={shown.content.tone === "danger" ? "alert" : "checkCircle"}
            className={`mt-0.5 size-4 shrink-0 ${shown.content.tone === "danger" ? "text-danger" : "text-text-muted"}`}
          />
          <ToastContent className="min-w-0 flex-1">
            <Text
              slot="title"
              className={`font-semibold ${shown.content.tone === "danger" ? "text-danger" : "text-text"}`}
            >
              {shown.content.title}
            </Text>
            {shown.content.detail && (
              <Text slot="description" className="mt-0.5 block break-words text-text-muted">
                {shown.content.detail}
              </Text>
            )}
          </ToastContent>
          {!isTv && (
            <Button
              slot="close"
              aria-label={t`Close`}
              className="shrink-0 rounded-sm text-text-muted outline-none hover:text-text data-focus-visible:ring-2"
            >
              <Icon name="close" className="size-4" />
            </Button>
          )}
        </Toast>
      )}
    </ToastRegion>
  );
}
