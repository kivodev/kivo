import { useLingui } from "@lingui/react/macro";
import { useEffect, useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import type { PlayStarted } from "@/api/schemas";
import { useIsAdmin } from "@/api/users";
import { Button } from "@/components/Button";
import { playerButton } from "@/components/PlayerMenu";
import { mismatchLabel, trackLabel } from "@/i18n/playback";
import { isTv } from "@/tv/flag";
import { TvFocusGroup } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { focusTv } from "@/tv/spatial";
import type { PlayerFailure } from "./reducer";
import { useFailureText } from "./useFailureText";

type Props = {
  failure: PlayerFailure;
  /** The play that failed, for diagnostics. */
  play: PlayStarted | undefined;
  /** What the media element said, for diagnostics. */
  detail: string | undefined;
  onRetry: () => void;
  /** Present when another version can be tried. */
  onPlayOther: (() => void) | undefined;
  onBack: () => void;
};

const secondary = `${playerButton.text} border border-player-track`;

/**
 * PlayerError shows that playback stopped. It says what failed in one line
 * with the server's reason translated, then offers Try again, Play another
 * version and Back. Admins also see the diagnostics.
 */
export function PlayerError({ failure, play, detail, onRetry, onPlayOther, onBack }: Props) {
  const { t } = useLingui();
  const isAdmin = useIsAdmin();
  useEffect(() => {
    if (isTv) focusTv("player-error:retry");
  }, []);
  const [diagnostics, setDiagnostics] = useState(false);
  const limit = failure.kind === "problem" && failure.code === "streamLimit";

  const sentence = useFailureText(failure);
  let title = t`Couldn't play this.`;
  if (failure.kind === "ended") title = t`Playback was stopped.`;
  else if (limit) title = t`Stream limit reached`;

  return (
    <div
      data-part="player-error"
      role="alert"
      className="absolute inset-0 flex items-center justify-center overflow-auto bg-player-scrim p-4 tv:px-(--layout-tv-overscan-x) tv:py-(--layout-tv-overscan-y)"
    >
      <div className="flex max-w-prose flex-col items-center gap-3 text-center">
        <h2 className="text-xl font-semibold text-on-player">{title}</h2>
        <p className="text-on-player-muted">{sentence}</p>
        {limit && failure.kind === "problem" && failure.plays && failure.plays.length > 0 && (
          <ul className="flex flex-col gap-1 text-sm text-on-player">
            {failure.plays.map((s) => (
              <li key={s.id}>{s.clientName ? `${s.title} · ${s.clientName}` : s.title}</li>
            ))}
          </ul>
        )}
        <TvFocusGroup focusKey="player-error" preferredChild="player-error:retry" trap line>
          <div className="mt-2 flex flex-wrap justify-center gap-2">
            <TvControl focusKey="player-error:retry">
              <Button variant="primary" onPress={onRetry} autoFocus={!isTv}>
                {t`Try again`}
              </Button>
            </TvControl>
            {onPlayOther && (
              <TvControl focusKey="player-error:other">
                <AriaButton className={secondary} onPress={onPlayOther}>
                  {t`Play another version`}
                </AriaButton>
              </TvControl>
            )}
            <TvControl focusKey="player-error:back">
              <AriaButton className={secondary} onPress={onBack}>
                {t`Back`}
              </AriaButton>
            </TvControl>
            {isAdmin && (
              <TvControl focusKey="player-error:diagnostics">
                <AriaButton className={secondary} onPress={() => setDiagnostics((open) => !open)}>
                  {t`View diagnostics`}
                </AriaButton>
              </TvControl>
            )}
          </div>
        </TvFocusGroup>
        {diagnostics && (
          <dl className="mt-2 flex flex-col gap-1 text-left text-xs text-on-player-muted">
            {failure.kind === "problem" && (
              <div>
                <dt className="inline font-semibold">{t`Code`}: </dt>
                <dd className="inline">{failure.reason ? `${failure.code} (${failure.reason})` : failure.code}</dd>
              </div>
            )}
            {failure.kind === "problem" && failure.mismatch && (
              <div>
                <dt className="inline font-semibold">{t`Limit`}: </dt>
                <dd className="inline">{mismatchLabel(failure.mismatch)}</dd>
              </div>
            )}
            {play && (
              <>
                <div>
                  <dt className="inline font-semibold">{t`Play`}: </dt>
                  <dd className="inline">{play.id}</dd>
                </div>
                {play.tracks
                  .filter((tr) => tr.selected)
                  .map((tr) => (
                    <div key={`${tr.type}-${tr.trackId}`}>
                      <dt className="inline font-semibold">{`${tr.type} ${tr.trackId}`}: </dt>
                      <dd className="inline">
                        {[trackLabel(tr), tr.reason && `(${tr.reason})`, tr.mismatch && mismatchLabel(tr.mismatch)]
                          .filter(Boolean)
                          .join(" ")}
                      </dd>
                    </div>
                  ))}
              </>
            )}
            {detail && (
              <div>
                <dt className="inline font-semibold">{t`Player`}: </dt>
                <dd className="inline">{detail}</dd>
              </div>
            )}
          </dl>
        )}
      </div>
    </div>
  );
}
