import { Trans, useLingui } from "@lingui/react/macro";
import { Link, useNavigate } from "@tanstack/react-router";
import { lazy, type ReactNode, Suspense, useState } from "react";
import { subtitlesOff } from "@/api/playback";
import type { WorkDetail } from "@/api/schemas";
import { Button, buttonClass } from "@/components/Button";
import { useEndsAt } from "@/components/endsAt";
import { Icon } from "@/components/Icon";
import { Select } from "@/components/Select";
import { automaticTrack, TrackPicker, trackChoices } from "@/components/TrackPicker";
import { versionOption } from "@/components/trackLabels";
import type { WatchSearch } from "@/features/player/watchSearch";
import { formatClock, formatTimeLeft } from "@/i18n/format";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { type DetailSearch, useDetailSearch } from "./detailSearch";
import { selectedVersion } from "./versions";

/** The subtitle picker's last item, which opens Search subtitles. */
const searchSubtitles = "search";

/** Search subtitles, in its own chunk, loaded when the picker first offers it. */
const SubtitleSearch = lazy(() =>
  import("@/features/player/SubtitleSearch").then((m) => ({ default: m.SubtitleSearch })),
);

/** Whether the work, in the version selected, can play. */
function canPlay(work: WorkDetail, search: DetailSearch): boolean {
  return work.available && selectedVersion(work, search.version)?.available === true;
}

/**
 * Where the work resumes when the version selected plays. A position
 * belongs to the edition of the version it was saved on, so a version of
 * another edition starts from the beginning.
 */
function resumeAt(work: WorkDetail, search: DetailSearch): number | undefined {
  const position = work.userState?.positionMs;
  const saved = work.versions.find((v) => v.id === work.userState?.versionId);
  const selected = selectedVersion(work, search.version);
  return saved && selected && saved.edition !== selected.edition ? undefined : position;
}

/** The picks the player starts with: the version selected, and the tracks chosen. */
function playSearchOf(work: WorkDetail, search: DetailSearch): WatchSearch {
  return { version: selectedVersion(work, search.version)?.id, audio: search.audio, subtitle: search.subtitle };
}

/**
 * The line under the buttons, the time left once the work has started and
 * when it would end ("1 hr 34 min left · Ends at 9:42 PM"), and Play from
 * beginning once it has started.
 */
export function useResumeParts(work: WorkDetail) {
  const [search] = useDetailSearch();
  const navigate = useNavigate();
  const playable = canPlay(work, search);
  const position = playable ? resumeAt(work, search) : undefined;
  const remaining = playable && work.durationMs ? work.durationMs - (position ?? 0) : undefined;
  const endsAt = useEndsAt(remaining);
  const left = position && remaining !== undefined ? formatTimeLeft(remaining) : undefined;
  return {
    playing: [left, endsAt].filter(Boolean).join(" · ") || undefined,
    onPlayFromStart: position
      ? () =>
          void navigate({
            to: "/watch/$workId",
            params: { workId: work.id },
            search: { ...playSearchOf(work, search), from: "start" },
          })
      : undefined,
  };
}

/** Play from beginning, a round button beside Resume. */
export function PlayFromStartButton({ onPress }: { onPress: () => void }) {
  const { t } = useLingui();
  return (
    <TvFocusable focusKey="restart">
      <Button size="icon" aria-label={t`Play from beginning`} data-part="restart" onPress={onPress}>
        <Icon name="replay" />
      </Button>
    </TvFocusable>
  );
}

/**
 * Play, or Resume with the time ("Resume 1:12:03"), and the version, audio
 * and subtitle pickers. Choices live in the URL and carry into playback.
 */
export function PlayActions({ work }: { work: WorkDetail }) {
  const [search] = useDetailSearch();
  if (!canPlay(work, search)) {
    return (
      <p data-part="missing" role="status" className="flex items-center gap-2 font-semibold text-warning">
        <Icon name="alert" />
        <Trans>This file is missing</Trans>
      </p>
    );
  }
  return <PlayButton workId={work.id} positionMs={resumeAt(work, search)} search={playSearchOf(work, search)} />;
}

type PlayButtonProps = {
  workId: string;
  /** Where the work was left. When set, the button resumes from there. */
  positionMs: number | undefined;
  /** The version and tracks to play. */
  search?: WatchSearch;
  /** The words to show instead of Play when it starts from the beginning. */
  label?: ReactNode;
};

/**
 * Play, or Resume with the time ("Resume 1:12:03"), opening the player. On
 * TV it has focus as the page opens.
 */
export function PlayButton({ workId, positionMs, search = {}, label = <Trans>Play</Trans> }: PlayButtonProps) {
  return (
    <TvFocusable focusKey="play" autoFocus>
      <Link to="/watch/$workId" params={{ workId }} search={search} className={buttonClass("primary")} data-part="play">
        <Icon name="play" />
        {positionMs ? <Trans>Resume {formatClock(positionMs)}</Trans> : label}
      </Link>
    </TvFocusable>
  );
}

export function PlaybackPickers({ work }: { work: WorkDetail }) {
  const { t } = useLingui();
  const [search, setSearch] = useDetailSearch();
  const version = selectedVersion(work, search.version);
  const tracks = version?.tracks ?? [];
  // A version change resets track choices, because track IDs belong to one
  // file.
  const pickVersion = (id: string) => setSearch({ version: id });
  const pickTrack = (key: "audio" | "subtitle", id: string) =>
    setSearch({ ...search, [key]: id === automaticTrack ? undefined : id });
  const remembered = work.remembersTracks === true;
  const [searching, setSearching] = useState(false);
  const fileId = version?.parts[0]?.fileId;
  // Subtitle search's results are a web-only list for now, so the TV build
  // doesn't offer it rather than open a dialog the remote can't move in.
  const canSearch = !isTv && fileId !== undefined && tracks.some((tr) => tr.type === "video");
  const audio = {
    kind: "audio",
    tracks: tracks.filter((tr) => tr.type === "audio"),
    automatic: version?.audioTrackId,
    remembered,
  } as const;
  const subtitle = {
    kind: "subtitle",
    tracks: tracks.filter((tr) => tr.type === "subtitle"),
    offId: subtitlesOff,
    automatic: version?.subtitleTrackId ?? subtitlesOff,
    remembered,
    extra: canSearch ? [{ id: searchSubtitles, label: t`Search subtitles…` }] : [],
  } as const;

  return (
    <TvFocusGroup focusKey="pickers">
      <div data-part="pickers" className="flex flex-wrap gap-x-6 gap-y-1">
        {work.versions.length > 1 && (
          <TvFocusable focusKey="version" face="inside">
            <Select
              variant="inline"
              part="version-picker"
              label={t`Version`}
              options={work.versions.map(versionOption)}
              selected={version?.id ?? ""}
              onChange={pickVersion}
            />
          </TvFocusable>
        )}
        {trackChoices(audio) && (
          <TvFocusable focusKey="audio" face="inside">
            <TrackPicker
              {...audio}
              selected={search.audio ?? automaticTrack}
              onChange={(id) => pickTrack("audio", id)}
            />
          </TvFocusable>
        )}
        {trackChoices(subtitle) && (
          <TvFocusable focusKey="subtitle" face="inside">
            <TrackPicker
              {...subtitle}
              selected={search.subtitle ?? automaticTrack}
              onChange={(id) => (id === searchSubtitles ? setSearching(true) : pickTrack("subtitle", id))}
            />
          </TvFocusable>
        )}
        {canSearch && (
          <Suspense fallback={null}>
            <SubtitleSearch
              fileId={fileId}
              isOpen={searching}
              onClose={() => setSearching(false)}
              onDownloaded={(trackId) => {
                setSearching(false);
                pickTrack("subtitle", trackId);
              }}
            />
          </Suspense>
        )}
      </div>
    </TvFocusGroup>
  );
}
