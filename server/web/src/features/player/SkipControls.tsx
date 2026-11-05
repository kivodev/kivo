import { useLingui } from "@lingui/react/macro";
import type { ReactNode } from "react";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { backMs, forwardMs } from "./skips";

type Props = {
  /** Whether the book is playing or paused, so the skips work. */
  live: boolean;
  onSkip: (deltaMs: number) => void;
  /** Play or pause, placed between the two skips. */
  children: ReactNode;
};

/** A book's Back 10 seconds and Forward 30 seconds, on either side of play or pause. */
export function SkipControls({ live, onSkip, children }: Props) {
  const { t } = useLingui();
  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t`Back 10 seconds`}
        isDisabled={!live}
        onPress={() => onSkip(-backMs)}
      >
        <Icon name="replay" />
      </Button>
      {children}
      <Button
        variant="ghost"
        size="icon"
        aria-label={t`Forward 30 seconds`}
        isDisabled={!live}
        onPress={() => onSkip(forwardMs)}
      >
        <Icon name="forward" />
      </Button>
    </>
  );
}
