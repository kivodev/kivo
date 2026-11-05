import { useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import { imageUrl } from "@/api/images";
import type { WorkDetail } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { Icon } from "@/components/Icon";
import { playerButton } from "@/components/PlayerMenu";
import { deviceSettings, saveDeviceSettings } from "@/lib/deviceSettings";
import { AudioSubtitlesMenu, audioSubtitles } from "./AudioSubtitlesMenu";
import { CastButton, CastGlyph, useCast } from "./CastButton";
import { type CastRequest, castMessages } from "./castProtocol";
import { castLink } from "./castSession";
import { PlaybackSettingsMenu } from "./PlaybackSettingsMenu";
import { ScrubBar } from "./ScrubBar";
import { Transport } from "./Transport";
import { UpNextCard } from "./UpNextCard";
import { upNextShows } from "./upNext";
import { useWatchNavigation } from "./watchSearch";

const watchRoute = getRouteApi("/authed/watch/$workId");

/**
 * CastRemote is the player while casting. It shows the work's art with
 * "Casting to Living Room TV", the name and its line, the scrub bar, the
 * transport, and up next as the TV shows it. Audio & Subtitles and Playback
 * Settings act on the TV. The TV's volume is in the Cast menu and on the
 * keyboard.
 *
 * A spinner replaces play while the TV loads or buffers. When the cast
 * can't go on, the reason and Try again replace the scrub bar and the
 * transport. Back leaves the remote, and the cast plays on.
 */
export function CastRemote() {
  const { t } = useLingui();
  const { workId } = watchRoute.useParams();
  const search = watchRoute.useSearch();
  // When another work plays on the TV, by a choice here or by the TV playing on, the remote follows it.
  const { exit, playWork } = useWatchNavigation();
  const work = useSuspenseQuery(workQueries.detail(workId)).data;
  const cast = useCast();
  const { media, status, loading, failure } = cast;
  const episode = work.episode;
  const title = episode?.seriesTitle ?? work.title;
  const line = episode ? `S${episode.seasonNumber} · E${episode.number} · ${work.title}` : movieLine(work);
  useDocumentTitle(work.title);
  const art = work.images.poster ?? work.images.still ?? work.images.backdrop;
  const image = art && imageUrl(art, 480, 1);

  // Cast the work once, when the TV doesn't already play it. It starts from
  // where it played here, or else as the route asks.
  const request = useRef<CastRequest | undefined>(undefined);
  useEffect(() => {
    const now = castLink.getSnapshot();
    const onTv = now.media?.workId ?? now.status?.workId;
    const handoff = castLink.takeHandoff(work.id);
    const { localQuality, remoteQuality, dialogueBoost } = deviceSettings();
    request.current = {
      workId: work.id,
      versionId: handoff?.versionId ?? search.version,
      audioTrackId: handoff?.audioTrackId ?? search.audio,
      subtitleTrackId: handoff?.subtitleTrackId ?? search.subtitle,
      positionMs: handoff?.positionMs ?? (search.from === "start" ? 0 : search.at),
      dialogueBoost,
      localQuality: localQuality ?? { mode: "auto" },
      remoteQuality: remoteQuality ?? { mode: "auto" },
    };
    if (onTv === work.id || now.loading === work.id) return;
    void castLink.cast(request.current, { title: work.title, image });
  }, [work.id, work.title, image, search]);

  // When the TV plays on to another episode, the remote follows.
  const onTv = media?.workId;
  useEffect(() => {
    if (onTv && onTv !== work.id && !loading && !failure) playWork(onTv);
  }, [onTv, work.id, loading, failure, playWork]);

  const ours = media?.workId === work.id ? media : undefined;
  const waiting = !ours ? loading === work.id : ours.state === "loading" || ours.state === "buffering";
  const durationMs = ours && ours.durationMs > 0 ? ours.durationMs : (work.durationMs ?? 0);
  const version = work.versions.find((v) => v.id === status?.versionId);
  const tracks = status?.versionId
    ? audioSubtitles({ dialogueBoost: status.dialogueBoost }, version, {
        canSearch: false,
      })
    : undefined;
  const subtitle = status?.subtitleTrackId ?? "off";
  const problem = failure ?? (status?.error === "serverUnreachable" ? t`Can't reach the server.` : undefined);
  const [menu, setMenu] = useState<"tracks" | "settings">();
  const tracksRef = useRef<HTMLButtonElement>(null);
  const playRef = useRef<HTMLButtonElement>(null);
  const next = status?.nextWorkId;

  return (
    <div data-part="cast-remote" className="fixed inset-0 z-50 overflow-auto bg-player text-on-player">
      {work.images.backdrop && (
        <div aria-hidden="true" className="pointer-events-none fixed inset-0 opacity-40 blur-3xl saturate-150">
          <Artwork
            image={work.images.backdrop}
            title=""
            shape="landscape"
            width={320}
            corners="none"
            className="size-full"
          />
        </div>
      )}
      <header className="relative flex items-center gap-2 p-4">
        <AriaButton aria-label={t`Back`} className={playerButton.icon} onPress={exit}>
          <Icon name="arrowLeft" />
        </AriaButton>
        <span className="flex-1" />
        <CastButton
          title={work.title}
          // Stop casting carries on here from where the TV was.
          onStopCasting={(carry) =>
            carry &&
            playWork(carry.workId, {
              version: carry.versionId,
              audio: carry.audioTrackId,
              subtitle: carry.subtitleTrackId,
              at: carry.positionMs === undefined ? undefined : Math.round(carry.positionMs),
            })
          }
        />
      </header>

      <main className="relative mx-auto flex w-full max-w-(--layout-cast-remote-width) flex-col gap-6 px-4 pb-8">
        <Artwork
          image={work.images.still ?? work.images.backdrop}
          title={work.title}
          shape="landscape"
          width={640}
          className="shadow-overlay"
        />
        <p className="flex items-center gap-2 font-medium text-accent text-sm">
          <span className="size-4">
            <CastGlyph connection="connected" />
          </span>
          {t`Casting to ${cast.device ?? ""}`}
        </p>
        <div>
          <h1 className="font-bold text-2xl">{title}</h1>
          {line && <p className="text-on-player-muted text-sm">{line}</p>}
        </div>

        {problem ? (
          <div role="alert" className="flex flex-col items-center gap-3 py-4 text-center">
            <Icon name="alert" className="size-7 text-on-player-muted" />
            <p>{problem}</p>
            <Button
              variant="primary"
              onPress={() =>
                request.current && castLink.retry({ request: request.current, label: { title: work.title, image } })
              }
            >
              {t`Try again`}
            </Button>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            <ScrubBar
              play={{ durationMs, chapters: version?.chapters ?? [] }}
              positionMs={ours?.positionMs ?? 0}
              onSeek={(ms) => castLink.seek(ms)}
            />
            <div className="relative flex justify-center">
              <Transport
                episode={episode !== undefined}
                hasPrevious={episode?.previousId !== undefined}
                hasNext={episode?.nextId !== undefined}
                live={ours !== undefined && !waiting}
                paused={ours?.state !== "playing"}
                playRef={playRef}
                onPrevious={() => episode?.previousId && playWork(episode.previousId)}
                onSkip={(deltaMs) => ours && castLink.seek(ours.positionMs + deltaMs)}
                onTogglePlay={() => castLink.togglePlay()}
                onNext={() => episode?.nextId && playWork(episode.nextId)}
              />
              {waiting && (
                <div role="status" className="pointer-events-none absolute inset-0 flex items-center justify-center">
                  <Icon name="spinner" className="size-10 animate-spin" />
                  <span className="sr-only">{t`Loading`}</span>
                </div>
              )}
            </div>
          </div>
        )}

        {status && next && upNextShows(status.upNext) && (
          <UpNextCard
            nextId={next}
            upNext={status.upNext}
            onPlay={() => castLink.send(castMessages.playNext())}
            onHide={() => castLink.send(castMessages.hideUpNext())}
          />
        )}

        {status?.versionId && (
          <div className="flex items-center justify-center gap-6">
            {tracks && (
              <AudioSubtitlesMenu
                choices={tracks}
                audioTrackId={status.audioTrackId}
                subtitle={subtitle}
                onAudio={(audioTrackId) => castLink.send(castMessages.setTracks({ audioTrackId }))}
                onSubtitle={(subtitleTrackId) => castLink.send(castMessages.setTracks({ subtitleTrackId }))}
                onSearchSubtitles={() => {}}
                onDialogueBoost={(on) => {
                  saveDeviceSettings({ ...deviceSettings(), dialogueBoost: on });
                  castLink.send(castMessages.setDialogueBoost(on));
                }}
                isOpen={menu === "tracks"}
                onOpenChange={(open) => setMenu(open ? "tracks" : undefined)}
                triggerRef={tracksRef}
              />
            )}
            <PlaybackSettingsMenu
              play={{ versionId: status.versionId, limitBps: status.limitBps }}
              version={version}
              versions={work.versions}
              quality={status.quality}
              speed={ours?.rate ?? 1}
              hasTracks={tracks !== undefined}
              onTracks={() => setMenu("tracks")}
              onQuality={(q) => castLink.send(castMessages.setQuality(q, q))}
              onVersion={(versionId) => castLink.send(castMessages.setVersion(versionId))}
              onSpeed={(rate) => castLink.setRate(rate)}
              isOpen={menu === "settings"}
              onOpenChange={(open) => setMenu(open ? "settings" : undefined)}
              triggerRef={tracks ? undefined : tracksRef}
            />
          </div>
        )}
      </main>
    </div>
  );
}

/** A movie's "1990 · PG-13". */
function movieLine(work: WorkDetail): string | undefined {
  const parts = [work.year?.toString(), work.contentRating].filter(Boolean);
  return parts.length > 0 ? parts.join(" · ") : undefined;
}
