import { useLingui } from "@lingui/react/macro";
import type { RefObject } from "react";
import { Button as AriaButton } from "react-aria-components";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { playerButton } from "@/components/PlayerMenu";
import type { SkipMarker } from "./markers";
import { UpNextCard } from "./UpNextCard";
import type { UpNextShown } from "./upNext";

/** The spinner over the picture while the media loads, buffers or seeks. */
export function LoadingSpinner() {
  const { t } = useLingui();
  return (
    <div role="status" className="pointer-events-none absolute inset-0 flex items-center justify-center">
      <Icon name="spinner" className="size-12 animate-spin text-on-player" />
      <span className="sr-only">{t`Loading`}</span>
    </div>
  );
}

/**
 * TopBar shows Back, the name of what plays, and when it ends. A remote has
 * its own Back, so it finds nothing to focus up here.
 */
export function TopBar({
  remote,
  title,
  endsAt,
  onExit,
}: {
  remote: boolean;
  title: string;
  /** "Ends at 9:42 PM", once the play's length is known. */
  endsAt: string | undefined;
  onExit: () => void;
}) {
  const { t } = useLingui();
  return (
    <header
      data-part="player-top-bar"
      className="flex items-center gap-2 bg-linear-to-b from-player-scrim to-transparent p-4"
    >
      {!remote && (
        <AriaButton aria-label={t`Back`} className={playerButton.icon} onPress={onExit}>
          <Icon name="arrowLeft" />
        </AriaButton>
      )}
      <h1 className="min-w-0 flex-1 truncate text-lg font-semibold">{title}</h1>
      {endsAt && (
        <p data-part="ends-at" className="shrink-0 text-sm text-on-player-muted">
          {endsAt}
        </p>
      )}
    </header>
  );
}

/** Full screen, and picture in picture where the browser has it. */
export function WindowButtons({
  fullscreen,
  live,
  onFullscreen,
  onPictureInPicture,
}: {
  fullscreen: boolean;
  live: boolean;
  onFullscreen: () => void;
  onPictureInPicture: () => void;
}) {
  const { t } = useLingui();
  return (
    <>
      <AriaButton
        aria-label={fullscreen ? t`Exit full screen` : t`Full screen`}
        className={playerButton.icon}
        onPress={onFullscreen}
      >
        <Icon name={fullscreen ? "exitFullscreen" : "fullscreen"} />
      </AriaButton>
      {document.pictureInPictureEnabled && (
        <AriaButton
          aria-label={t`Picture in picture`}
          className={`${playerButton.icon} max-sm:hidden`}
          isDisabled={!live}
          onPress={onPictureInPicture}
        >
          <Icon name="pip" />
        </AriaButton>
      )}
    </>
  );
}

/** The buttons over the picture: up next, and Skip Intro, Recap or Credits. A remote reaches them while the controls are hidden. */
export function PictureButtons({
  overlayRef,
  upNext,
  marker,
  onSkip,
}: {
  overlayRef: RefObject<HTMLDivElement | null>;
  upNext: { nextId: string; card: UpNextShown; onPlay: () => void; onHide: () => void } | undefined;
  marker: SkipMarker | undefined;
  onSkip: (marker: SkipMarker) => void;
}) {
  const { t } = useLingui();
  return (
    <div ref={overlayRef} className="absolute right-4 bottom-36 flex flex-col items-end gap-2 max-sm:bottom-44">
      {upNext && (
        <UpNextCard nextId={upNext.nextId} upNext={upNext.card} onPlay={upNext.onPlay} onHide={upNext.onHide} />
      )}
      {marker && (
        <Button variant="primary" data-part="skip-marker" data-tv-face="" onPress={() => onSkip(marker)}>
          {marker.kind === "intro" ? t`Skip Intro` : marker.kind === "recap" ? t`Skip Recap` : t`Skip Credits`}
        </Button>
      )}
    </div>
  );
}
