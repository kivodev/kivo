import { useLingui } from "@lingui/react/macro";
import type { RefObject } from "react";
import { Dialog, Popover } from "react-aria-components";
import type { PlayStarted, PlayTrack, QualityRequest, Version } from "@/api/schemas";
import { containerName, subtitleLine, trackLine } from "@/components/trackLabels";
import { formatBitrate } from "@/i18n/format";
import { describeReason, methodLabel, mismatchLabel, playMethod } from "@/i18n/playback";

type Row = { label: string; value: string; why?: string };

/** Why a track isn't played from the file as it is, with the capability it failed. Repackaging is said once, on its own row. */
function whyOf(track: PlayTrack): string | undefined {
  if (!track.reason || track.reason === "containerUnsupported") return undefined;
  const why = describeReason(track.reason);
  return track.mismatch ? `${why} (${mismatchLabel(track.mismatch)})` : why;
}

type Props = {
  play: PlayStarted;
  version: Version | undefined;
  /** The quality the viewer picked. */
  quality: QualityRequest | undefined;
  /** The subtitle choice showing now, which a choice in the player can change without a new play. */
  subtitle: PlayTrack | undefined;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  /** The Playback Settings button, which the overlay opens from. */
  anchorRef: RefObject<Element | null>;
};

/**
 * StreamInfo is the stream info overlay. The headline is the play's
 * method, its video's, and every line that isn't direct says why.
 */
export function StreamInfo({ play, version, quality, subtitle, isOpen, onOpenChange, anchorRef }: Props) {
  const { t } = useLingui();
  const sourceOf = (track: PlayTrack) => version?.tracks.find((tr) => tr.id === track.trackId);
  const names: Record<string, string> = { video: t`Video`, audio: t`Audio`, subtitle: t`Subtitles` };
  const selected = play.tracks.filter((tr) => tr.selected);
  const method = playMethod(play.tracks);

  const rows: Row[] = method ? [{ label: t`Playback`, value: methodLabel(method) }] : [];
  // Repackaging the file is due to the container. It is said once on its
  // own row, not as the reason on every copied track.
  const container = containerName(version?.container);
  if (play.delivery === "stream" && container) {
    const repackaged = selected.some((tr) => tr.reason === "containerUnsupported");
    rows.push({
      label: t`Container`,
      value: t`${container} → HLS stream`,
      why: repackaged ? t`This device can't play ${container} files.` : undefined,
    });
  }
  for (const track of selected) {
    if (track.type === "subtitle") continue;
    rows.push({
      label: names[track.type] ?? track.type,
      value: trackLine(track, sourceOf(track)),
      why: track.delivery === "file" ? undefined : whyOf(track),
    });
  }
  // The subtitle showing now, as the play's tracks say it shows.
  const showing = subtitle?.delivery && subtitle.delivery !== "none" ? subtitle : undefined;
  if (!showing) rows.push({ label: names.subtitle ?? "", value: t`Off` });
  else rows.push({ label: names.subtitle ?? "", value: subtitleLine(showing, sourceOf(showing)), why: whyOf(showing) });
  // A limit the file is already under plays the original. Say so, or the
  // menu choice seems ignored.
  const limited = selected.some((tr) => tr.reason === "clientQualityLimit");
  const cap = quality?.mode === "limit" ? quality.maxBitrateBps : undefined;
  if (cap) {
    const source = version?.bitrateBps;
    rows.push({
      label: t`Quality`,
      value:
        !limited && source && source <= cap
          ? t`Original (${formatBitrate(source)} is under the ${formatBitrate(cap)} limit)`
          : t`Limited to ${formatBitrate(cap)}`,
    });
  }
  rows.push({
    label: t`Delivery`,
    value: [play.delivery === "stream" ? "HLS" : t`File`, play.local ? t`Local` : t`Remote`].join(" · "),
  });
  // The stream's bitrate. Now playing shows the same figure.
  if (play.bitrateBps) rows.push({ label: t`Bitrate`, value: formatBitrate(play.bitrateBps) });

  return (
    <Popover
      triggerRef={anchorRef}
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      placement="top end"
      className="w-(--layout-player-panel-width) max-w-full rounded-md bg-player-panel p-4 shadow-overlay outline-none"
    >
      <Dialog data-part="stream-info" aria-label={t`Stream info`} className="outline-none">
        <dl className="flex flex-col gap-2 text-sm">
          {rows.map((row) => (
            <div key={row.label} className="flex gap-4">
              <dt className="w-24 shrink-0 text-on-player-muted">{row.label}</dt>
              <dd className="min-w-0 text-on-player">
                {row.value}
                {row.why && <span className="block text-on-player-muted">{t`Why: ${row.why}`}</span>}
              </dd>
            </div>
          ))}
        </dl>
      </Dialog>
    </Popover>
  );
}
