import { i18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { type CSSProperties, type ReactNode, useEffect, useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import { useStopPlay } from "@/api/playback";
import type { ClientDevice, Play, PlayTrack, Track } from "@/api/schemas";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { usePlatformName } from "@/components/deviceCode";
import { Icon } from "@/components/Icon";
import { ModalDialog, useDismissal } from "@/components/ModalDialog";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { TranscodeDiagnostics } from "@/components/TranscodeDiagnostics";
import {
  codecName,
  hdrName,
  output,
  trackBrief,
  trackChange,
  trackDetail,
  transcodeOutput,
} from "@/components/trackLabels";
import { episodeCode } from "@/features/browse/WorkCard";
import { formatBitrate, formatClock, formatTimeOfDay, formatTimes } from "@/i18n/format";
import { describeReason, methodLabel, mismatchLabel, playMethod } from "@/i18n/playback";

// The Overview's play cards. Each shows the artwork with the play state,
// the method and the progress over it, then what is playing, who is
// watching and the stream, with Stop. A card opens its play's details,
// which also have Stop stream…. Both ask first, with an optional message
// to the viewer.

/** The play's source track a play track names. */
function sourceOf(play: Play, track: PlayTrack): Track | undefined {
  return play.sourceTracks?.find((t) => t.id === track.trackId);
}

/** The tracks the play shows. */
const selectedOf = (play: Play) => play.tracks.filter((t) => t.selected);

/**
 * The card's title and what follows it. A series has "S3 · E4", a film has
 * its year, and a book has its author.
 */
function titleOf(play: Play): { title: string; detail?: string } {
  const work = play.work;
  if (!work) return { title: play.title };
  const detail = work.episode ? episodeCode(work) : work.book ? work.book.authors[0]?.name : work.year?.toString();
  return { title: work.episode?.seriesTitle ?? work.title, detail };
}

/** The play's method, marking a video transcode on an accelerator: "Transcode (hw)". */
function methodBadge(play: Play): string {
  const method = playMethod(play.tracks);
  if (!method) return "";
  const label = methodLabel(method);
  return method === "transcode" && play.videoProcessing === "accelerator" ? i18n._(msg`${label} (hw)`) : label;
}

/** The first reason a track isn't played directly, for the card's hover line. */
function mainReason(play: Play): string | undefined {
  if (playMethod(play.tracks) === "directPlay") return undefined;
  const tracks = selectedOf(play);
  const d =
    tracks.find((t) => t.type !== "subtitle" && t.delivery !== "file" && t.reason) ?? tracks.find((t) => t.reason);
  return d?.reason ? describeReason(d.reason) : undefined;
}

/**
 * Video as it is sent, such as "H.264 1080p · 8.2 Mb/s". It describes the
 * output, or else the source in full. A book's audio is described the same
 * way.
 */
function streamOf(play: Play): string | undefined {
  const video = selectedOf(play).find((t) => t.type === "video");
  if (video) {
    const parts = [labelOf(play, video), play.bitrateBps ? formatBitrate(play.bitrateBps) : undefined];
    return parts.filter(Boolean).join(" · ") || undefined;
  }
  const audio = selectedOf(play).find((t) => t.type === "audio");
  if (!audio) return undefined;
  return labelOf(play, audio);
}

/** What a track is sent as: its output, or else its source in full. */
function labelOf(play: Play, track: PlayTrack): string | undefined {
  const out = transcodeOutput(track);
  if (out) return output(out);
  const source = sourceOf(play, track);
  return source && trackDetail(source);
}

/** How far a transcode has encoded ahead of the viewer, if at all. */
function aheadOf(play: Play): number | undefined {
  const encoded = play.transcode?.encodedMs;
  return encoded !== undefined && encoded > play.positionMs ? encoded - play.positionMs : undefined;
}

/** The user's initial in a circle, as the user menu shows it. */
function Avatar({ name }: { name: string }) {
  return (
    <span
      aria-hidden="true"
      className="flex size-5 shrink-0 items-center justify-center rounded-full bg-accent text-xs font-bold text-on-accent"
    >
      {name.charAt(0).toUpperCase()}
    </span>
  );
}

/**
 * The backdrop, or an episode's still, at 16:9. A book shows its square
 * cover over a blurred copy of itself.
 */
function PlayArt({ play, width }: { play: Play; width: number }) {
  const images = play.work?.images;
  const { title } = titleOf(play);
  const landscape = play.work?.episode ? (images?.still ?? images?.backdrop) : images?.backdrop;
  if (play.work?.kind !== "book" || !images?.poster) {
    return <Artwork image={landscape} title={title} shape="landscape" width={width} corners="none" />;
  }
  return (
    <div className="relative aspect-landscape overflow-hidden bg-placeholder">
      <div className="absolute inset-0 flex scale-125 items-center opacity-60 blur-xl">
        <Artwork image={images.poster} title="" shape="square" width={64} corners="none" className="w-full" />
      </div>
      <div className="absolute top-3 bottom-9 left-1/2 aspect-square -translate-x-1/2 shadow-card">
        <Artwork image={images.poster} title={title} shape="square" width={width / 2} />
      </div>
    </div>
  );
}

const progressStyles = {
  artwork: { bar: "absolute inset-x-0 bottom-0", encoded: "bg-player-track" },
  panel: { bar: "relative overflow-hidden rounded-full", encoded: "bg-text-subtle" },
} as const;

/**
 * How much has played, and for a transcode how far it has encoded ahead.
 * It shows along the artwork's bottom edge or in the details.
 */
function PlayProgress({ play, on }: { play: Play; on: keyof typeof progressStyles }) {
  const duration = play.durationMs ?? play.work?.durationMs;
  if (!duration) return null;
  const pct = (ms: number) => `${Math.min(100, Math.max(0, (ms / duration) * 100))}%`;
  const style = {
    "--played": pct(play.positionMs),
    "--encoded": pct(play.transcode?.encodedMs ?? 0),
  } as CSSProperties;
  return (
    <div
      role="progressbar"
      aria-valuenow={Math.round((play.positionMs / duration) * 100)}
      aria-valuemin={0}
      aria-valuemax={100}
      className={`h-1 bg-progress-track ${progressStyles[on].bar}`}
      style={style}
    >
      <div className={`absolute inset-y-0 left-0 w-(--encoded) ${progressStyles[on].encoded}`} />
      <div className="absolute inset-y-0 left-0 w-(--played) bg-progress" />
    </div>
  );
}

/** A play's card, which opens its details, with Stop in its corner. */
function PlayCard({ play, onOpen }: { play: Play; onOpen: () => void }) {
  const { t } = useLingui();
  const [stopping, setStopping] = useState(false);
  const { title } = titleOf(play);
  const user = play.username;
  return (
    <div className="relative transition duration-300 ease-standard hover:-translate-y-1">
      <CardButton play={play} onOpen={onOpen} />
      <Button
        size="sm"
        className="absolute right-3 bottom-2.5"
        aria-label={t`Stop ${title} for ${user}`}
        onPress={() => setStopping(true)}
      >
        <Trans>Stop</Trans>
      </Button>
      <StopDialog play={play} isOpen={stopping} onClose={() => setStopping(false)} />
    </div>
  );
}

type StopProps = {
  play: Play;
  isOpen: boolean;
  onClose: () => void;
  /** Called after the server stopped the play, once the dialog has closed. */
  onStopped?: () => void;
};

/**
 * "Stop <title> for <user>?", with an optional message the viewer's player
 * shows. The play keeps the position the viewer last reported.
 */
function StopDialog({ play, isOpen, onClose, onStopped }: StopProps) {
  const { t } = useLingui();
  const { title } = titleOf(play);
  const user = play.username;
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} title={t`Stop ${title} for ${user}?`} width="form">
      <StopForm
        play={play}
        onCancel={onClose}
        onStopped={() => {
          onClose();
          onStopped?.();
        }}
      />
    </ModalDialog>
  );
}

function StopForm({ play, onCancel, onStopped }: { play: Play; onCancel: () => void; onStopped: () => void }) {
  const { t } = useLingui();
  const stop = useStopPlay();
  const [message, setMessage] = useState("");
  useDismissal(message ? "ask" : "close");
  return (
    <SubmitForm
      onSubmit={() => {
        const text = message.trim();
        stop.mutate(
          { params: { path: { id: play.id } }, body: { message: text || undefined } },
          { onSuccess: onStopped },
        );
      }}
      submitLabel={<Trans>Stop stream</Trans>}
      tone="danger"
      isPending={stop.isPending}
      onCancel={onCancel}
      error={stop.error}
    >
      <p className="text-text-muted">
        <Trans>The viewer can resume from where it stopped.</Trans>
      </p>
      <TextField
        variant="form"
        label={t`Message to the viewer (optional)`}
        value={message}
        onChange={setMessage}
        maxLength={500}
        autoFocus
      />
    </SubmitForm>
  );
}

function CardButton({ play, onOpen }: { play: Play; onOpen: () => void }) {
  const { t } = useLingui();
  const { title, detail } = titleOf(play);
  const reason = mainReason(play);
  const ahead = aheadOf(play);
  const duration = play.durationMs ?? play.work?.durationMs;
  const stream = streamOf(play);
  const who = [play.username, play.clientName, play.local ? t`Local` : t`Remote`].filter(Boolean).join(" · ");
  return (
    <AriaButton
      data-part="play"
      data-method={playMethod(play.tracks)}
      aria-label={t`${title}, ${who}`}
      onPress={onOpen}
      className="group flex w-full cursor-pointer flex-col overflow-hidden rounded-lg border border-border bg-surface text-left transition duration-300 ease-standard outline-none hover:border-text-subtle hover:shadow-card data-focus-visible:outline-2 data-focus-visible:outline-offset-2 data-focus-visible:outline-focus data-focus-visible:outline-solid"
    >
      <div className="relative">
        <PlayArt play={play} width={480} />
        <div className="absolute inset-0 bg-linear-to-t from-player-scrim via-transparent to-transparent" />
        <span className="absolute top-2 left-2 flex size-7 items-center justify-center rounded-full bg-badge text-on-badge">
          <Icon name={play.state === "paused" ? "pause" : "play"} className="size-3.5" />
          <span className="sr-only">{play.state === "paused" ? t`Paused` : t`Playing`}</span>
        </span>
        <span
          data-part="play-summary"
          className="absolute top-2 right-2 max-w-3/4 truncate rounded-sm bg-badge px-1.5 py-0.5 text-xs font-semibold text-on-badge"
        >
          {methodBadge(play)}
        </span>
        <div className="absolute inset-x-0 bottom-0 flex flex-col gap-0.5 px-3 pb-2.5 text-xs text-on-player">
          {reason && (
            <p className="truncate font-medium opacity-0 transition-opacity group-hover:opacity-100 group-data-focus-visible:opacity-100">
              {methodLabel(playMethod(play.tracks) ?? "")}: {reason}
            </p>
          )}
          <p className="flex justify-between gap-2 tabular-nums">
            <span>
              {formatClock(play.positionMs)}
              {duration ? ` / ${formatClock(duration)}` : ""}
            </span>
            {ahead !== undefined && <span className="text-on-player-muted">{t`${formatClock(ahead)} ahead`}</span>}
          </p>
        </div>
        <PlayProgress play={play} on="artwork" />
      </div>
      {/* Room on the right for the card's Stop. */}
      <div className="flex min-w-0 flex-col gap-1 py-2.5 pr-20 pl-3">
        <p className="truncate font-semibold">
          {title}
          {detail && <span className="font-normal text-text-muted"> · {detail}</span>}
        </p>
        <p className="flex min-w-0 items-center gap-2 text-sm text-text-muted">
          <Avatar name={play.username} />
          <span className="truncate">{who}</span>
        </p>
        {stream && <p className="truncate text-sm text-text-muted">{stream}</p>}
      </div>
    </AriaButton>
  );
}

/** The details' thumbnail: the card's artwork, or a book's square cover. */
function PlayThumb({ play }: { play: Play }) {
  const poster = play.work?.images?.poster;
  if (play.work?.kind === "book" && poster) {
    return (
      <div className="w-14 shrink-0">
        <Artwork image={poster} title="" shape="square" width={56} />
      </div>
    );
  }
  return (
    <div className="w-24 shrink-0 overflow-hidden rounded-md">
      <PlayArt play={play} width={96} />
    </div>
  );
}

/**
 * The line under the details' title. For an episode it is the episode's
 * code and its own title. Otherwise it is what follows the title on the
 * card.
 */
function aboutOf(play: Play): string | undefined {
  const work = play.work;
  return work?.episode ? [episodeCode(work), work.title].join(" · ") : titleOf(play).detail;
}

/** Reasons that a subtitle gives, which a video converted to carry it shares. */
const subtitleReasons = ["subtitleCodecUnsupported", "burnSubtitlesEnabled", "subtitlePreparing"];

/**
 * Why a track isn't sent as it is. It uses a few words naming what the
 * device can't play, or else the reason in full.
 */
function useBriefReason() {
  const { t } = useLingui();
  return (code: string, track: PlayTrack, source: Track | undefined, burned: boolean): string => {
    const codec = source && codecName(source.codec);
    const hdr = source?.video?.hdr && hdrName(source.video.hdr);
    // Video converted to burn in subtitles says so. The subtitles' row says
    // why.
    if (track.type !== "subtitle" && burned && subtitleReasons.includes(code)) return t`Burned-in subtitles`;
    switch (code) {
      case "videoCodecUnsupported":
      case "audioCodecUnsupported":
        return codec ? t`Can't play ${codec}` : describeReason(code);
      case "streamCodecUnsupported":
        return codec ? t`Can't stream ${codec}` : describeReason(code);
      case "subtitleCodecUnsupported":
        return codec ? t`Can't show ${codec}` : describeReason(code);
      case "videoHdrUnsupported":
        return hdr ? t`Can't show ${hdr}` : describeReason(code);
      case "videoCodecTagUnsupported":
        return t`Can't play this codec tag`;
      case "videoProfileUnsupported":
      case "audioProfileUnsupported":
        return t`Can't play this profile`;
      case "videoLevelUnsupported":
        return t`Can't play this level`;
      case "videoBitDepthUnsupported":
      case "audioBitDepthUnsupported":
        return t`Can't play this bit depth`;
      case "videoResolutionUnsupported":
        return t`Larger than this device plays`;
      case "videoFrameRateUnsupported":
        return t`Can't play this frame rate`;
      case "videoBitrateUnsupported":
        return t`Bitrate higher than this device plays`;
      case "videoRefFramesUnsupported":
        return t`More reference frames than this device plays`;
      case "videoInterlacedUnsupported":
        return t`Can't play interlaced video`;
      case "videoAnamorphicUnsupported":
        return t`Can't play non-square pixels`;
      case "audioSampleRateUnsupported":
        return t`Can't play this sample rate`;
      case "audioChannelsUnsupported":
        return t`More channels than this device plays`;
      case "trackSelectionUnsupported":
        return t`Not the file's default track`;
      case "directPlayDisabled":
        return t`Direct Play is off on this device`;
      case "clientQualityLimit":
        return t`Quality set on this device`;
      case "remoteQualityLimit":
        return t`Remote quality limit`;
      case "bandwidthLimit":
        return t`Connection too slow`;
      case "dialogueBoost":
        return t`Dialogue boost`;
      default:
        return describeReason(code);
    }
  };
}

/**
 * Each track from its source to what's sent. It shows Same, or what the
 * track becomes, marked, with the reason under it.
 */
function PlayTracks({ play }: { play: Play }) {
  const { t } = useLingui();
  const brief = useBriefReason();
  const names: Record<string, string> = { video: t`Video`, audio: t`Audio`, subtitle: t`Subtitles` };
  // Off isn't a track the play sends.
  const tracks = selectedOf(play).filter((tr) => tr.delivery !== "none");
  const burned = tracks.some((tr) => tr.delivery === "burn");
  return (
    <table data-part="play-tracks" className="w-full table-fixed text-sm">
      <thead>
        <tr className="text-left text-xs text-text-subtle">
          <td className="w-21" />
          <th scope="col" className="px-3 pb-1 font-normal">
            <Trans>Source</Trans>
          </th>
          <th scope="col" className="pb-1 font-normal">
            <Trans>Sending</Trans>
          </th>
        </tr>
      </thead>
      <tbody>
        {tracks.map((track) => {
          const source = sourceOf(play, track);
          const change = trackChange(track, source);
          // A repackaged file's reason belongs to the panel, not to each
          // copied track. Subtitles burned in on request need no reason
          // beyond "Burned in".
          const quiet =
            track.reason === "containerUnsupported" ||
            (track.type === "subtitle" && track.reason === "burnSubtitlesEnabled");
          const why = track.delivery !== "file" && track.reason && !quiet ? track.reason : undefined;
          return (
            <tr key={`${track.type}-${track.trackId}`} className="align-baseline">
              <th scope="row" className="py-1 text-left font-normal text-text-muted">
                {names[track.type] ?? track.type}
              </th>
              <td className="px-3 py-1 break-words">{source && trackBrief(source)}</td>
              <td className="py-1 break-words">
                <span className={change ? "text-warning" : "text-text-muted"}>{change ?? t`Same`}</span>
                {why && <span className="block text-xs text-text-muted">{brief(why, track, source, burned)}</span>}
                {why && track.mismatch && (
                  <span className="block text-xs text-text-subtle">{mismatchLabel(track.mismatch)}</span>
                )}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

/**
 * The app and device a play came from, in brief: "Android TV · SHIELD
 * Android TV · OS 11 · kivo-android 1.4.0". A browser goes by its family
 * and version, as "Safari 17 · OS 17.5 · kivo-web".
 */
function useDeviceLine(device: ClientDevice | undefined): string | undefined {
  const { t } = useLingui();
  const platformName = usePlatformName();
  if (!device) return undefined;
  const platform = device.platform === "browser" ? undefined : platformName(device.platform);
  const app = device.app && [device.app, device.appVersion].filter(Boolean).join(" ");
  const version = device.osVersion;
  const os = version && t`OS ${version}`;
  return [platform, device.deviceModel, os, app].filter(Boolean).join(" · ") || undefined;
}

type Stat = { label: string; value: string; detail?: string };

/**
 * The transcoder's speed and how far ahead it is, and the bitrate with the
 * bandwidth available. Each shows when known.
 */
function usePlayStats(play: Play): Stat[] {
  const { t } = useLingui();
  const stats: Stat[] = [];
  const speed = play.transcode?.speed;
  if (speed !== undefined) {
    const ahead = aheadOf(play);
    const waiting = play.transcode?.state === "paused";
    const clock = ahead !== undefined ? formatClock(ahead) : undefined;
    stats.push({
      label: t`Transcoder`,
      value: t`${formatTimes(speed)} realtime`,
      detail:
        clock && waiting
          ? t`${clock} ahead, waiting for the player`
          : clock
            ? t`${clock} ahead`
            : waiting
              ? t`Waiting for the player`
              : undefined,
    });
  }
  const available = play.bandwidthBps ? t`${formatBitrate(play.bandwidthBps)} available` : undefined;
  if (play.bitrateBps) {
    stats.push({ label: t`Bandwidth`, value: formatBitrate(play.bitrateBps), detail: available });
  } else if (available) {
    stats.push({ label: t`Bandwidth`, value: available });
  }
  return stats;
}

function Stats({ stats }: { stats: Stat[] }) {
  return (
    <dl className="flex flex-col gap-2.5 text-sm">
      {stats.map((s) => (
        <div key={s.label} className="flex gap-3">
          <dt className="w-21 shrink-0 text-text-muted">{s.label}</dt>
          <dd className="flex min-w-0 flex-col gap-0.5">
            <span>{s.value}</span>
            {s.detail && <span className="text-xs text-text-muted">{s.detail}</span>}
          </dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * A play's details. They show who is playing, on what device and where.
 * Then come the progress with how far a transcode has encoded ahead, the
 * method, each track from source to what's sent with the reason, the
 * transcoder and the bandwidth, the diagnostics, and Stop stream.
 */
function PlayDetails({ play, onStopped }: { play: Play; onStopped: () => void }) {
  const { t } = useLingui();
  const [diagnostics, setDiagnostics] = useState(false);
  const [stopping, setStopping] = useState(false);
  const duration = play.durationMs ?? play.work?.durationMs;
  const method = playMethod(play.tracks);
  const repackaged = method === "directStream" && selectedOf(play).some((t) => t.reason === "containerUnsupported");
  const stats = usePlayStats(play);
  const device = useDeviceLine(play.device);
  const transcode = play.transcode;
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-2">
        <p className="flex justify-between gap-3 text-sm">
          <span className="min-w-0 truncate">{[play.username, play.clientName].filter(Boolean).join(" · ")}</span>
          <span className="shrink-0 text-text-muted">
            {[play.local ? t`Local` : t`Remote`, play.address].filter(Boolean).join(" · ")}
          </span>
        </p>
        {device && (
          <p data-part="play-device" className="truncate text-xs text-text-muted">
            {device}
          </p>
        )}
        <PlayProgress play={play} on="panel" />
        <p className="flex justify-between gap-3 text-xs text-text-muted tabular-nums">
          <span>
            {formatClock(play.positionMs)}
            {duration ? ` / ${formatClock(duration)}` : ""}
          </span>
          <span>{t`Started ${formatTimeOfDay(play.startedAt)}`}</span>
        </p>
      </div>
      <section className="flex flex-col gap-1.5">
        <p className="flex items-center gap-2 font-semibold">
          <span
            aria-hidden="true"
            className={`size-2 rounded-full ${method === "directPlay" ? "bg-success" : "bg-warning"}`}
          />
          {methodBadge(play)}
        </p>
        {repackaged && (
          <p className="text-sm text-text-muted">
            <Trans>Repackaged for this device</Trans>
          </p>
        )}
      </section>
      <PlayTracks play={play} />
      {(stats.length > 0 || transcode?.command) && (
        <section className="flex flex-col gap-3 border-t border-border pt-5">
          {stats.length > 0 && <Stats stats={stats} />}
          {transcode?.command && (
            <>
              <Button
                size="sm"
                variant="ghost"
                className="-ml-3 self-start"
                onPress={() => setDiagnostics((open) => !open)}
              >
                {diagnostics ? <Trans>Hide diagnostics</Trans> : <Trans>Show diagnostics</Trans>}
              </Button>
              {diagnostics && (
                <div className="text-sm text-text-muted">
                  <TranscodeDiagnostics transcode={transcode} />
                </div>
              )}
            </>
          )}
        </section>
      )}
      <footer className="mt-1 flex flex-col gap-2.5">
        <Button onPress={() => setStopping(true)}>
          <span className="text-danger">
            <Trans>Stop stream…</Trans>
          </span>
        </Button>
        <StopDialog play={play} isOpen={stopping} onClose={() => setStopping(false)} onStopped={onStopped} />
      </footer>
    </div>
  );
}

type Shown = { play: Play; leaving: boolean };

/** How long a card takes to leave, matching its duration-300 transition. */
const leaveMs = 300;

/** The cards shown. They are the current plays, and the plays that just ended, fading out where they were. */
function merge(shown: Shown[], plays: Play[]): Shown[] {
  const now = new Map(plays.map((s) => [s.id, s]));
  const known = new Set(shown.map((s) => s.play.id));
  return [
    ...shown.map(({ play }) => {
      const current = now.get(play.id);
      return current ? { play: current, leaving: false } : { play, leaving: true };
    }),
    ...plays.filter((s) => !known.has(s.id)).map((play) => ({ play, leaving: false })),
  ];
}

function useLeaving(plays: Play[]): Shown[] {
  const [source, setSource] = useState(plays);
  const [shown, setShown] = useState<Shown[]>(() => merge([], plays));
  if (source !== plays) {
    setSource(plays);
    setShown(merge(shown, plays));
  }
  const leaving = shown.some((s) => s.leaving);
  useEffect(() => {
    if (!leaving) return;
    const timer = setTimeout(() => setShown((list) => list.filter((s) => !s.leaving)), leaveMs);
    return () => clearTimeout(timer);
  }, [leaving]);
  return shown;
}

/**
 * Plays as cards in a grid of one to three columns. A card opens its
 * play's details beside the page. A play that ends fades out, and when
 * none are left, `empty` shows.
 */
export function PlayCards({ plays, empty }: { plays: Play[]; empty: ReactNode }) {
  const shown = useLeaving(plays);
  const [openId, setOpenId] = useState<string>();
  const open = plays.find((s) => s.id === openId);
  if (shown.length === 0) return empty;
  return (
    <>
      <ul data-part="plays" className="grid gap-4 pt-1 sm:grid-cols-2 2xl:grid-cols-3">
        {shown.map(({ play, leaving }) => (
          <li
            key={play.id}
            inert={leaving}
            className={`transition duration-300 ease-standard ${leaving ? "scale-95 opacity-0" : ""}`}
          >
            <PlayCard play={play} onOpen={() => setOpenId(play.id)} />
          </li>
        ))}
      </ul>
      <ModalDialog
        isOpen={open !== undefined}
        onClose={() => setOpenId(undefined)}
        title={open ? titleOf(open).title : ""}
        image={open && <PlayThumb play={open} />}
        subtitle={open && aboutOf(open)}
        width="form"
        placement="side"
        part="play-details"
      >
        {open && <PlayDetails play={open} onStopped={() => setOpenId(undefined)} />}
      </ModalDialog>
    </>
  );
}
