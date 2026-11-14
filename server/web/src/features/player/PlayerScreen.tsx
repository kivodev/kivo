import { useSuspenseQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { type RefObject, useEffect, useEffectEvent, useRef, useState } from "react";
import type { QualityRequest, WorkDetail } from "@/api/schemas";
import { currentUserQueries } from "@/api/users";
import { workQueries } from "@/api/works";
import { useDocumentTitle } from "@/components/documentTitle";
import { useEndsAt } from "@/components/endsAt";
import { labelledTitle } from "@/features/browse/WorkCard";
import { deviceSettings, saveDeviceSettings } from "@/lib/deviceSettings";
import { isTv } from "@/tv/flag";
import { AudioSubtitlesMenu, audioSubtitles } from "./AudioSubtitlesMenu";
import { CastButton } from "./CastButton";
import { castLink, type LocalPlay } from "./castSession";
import type { PlayerController } from "./controller";
import { playsAtSpeed } from "./engine";
import { KeyDispatcher, type KeyInput } from "./keyDispatcher";
import { focusHandles, type KeyCommand, keyCommand } from "./keys";
import type { SkipMarker } from "./markers";
import { useMediaSession } from "./mediaSession";
import { PlaybackSettingsMenu } from "./PlaybackSettingsMenu";
import { PlayerError } from "./PlayerError";
import { LoadingSpinner, PictureButtons, TopBar, WindowButtons } from "./PlayerOverlays";
import { PlayerVideo } from "./PlayerVideo";
import { isLive } from "./reducer";
import { ScrubBar } from "./ScrubBar";
import { StreamInfo } from "./StreamInfo";
import { SubtitleSearch } from "./SubtitleSearch";
import { backMs, forwardMs } from "./skips";
import { Transport } from "./Transport";
import { useAudioChoice } from "./useAudioChoice";
import { useControlsVisibility } from "./useControlsVisibility";
import { useEndedElsewhere } from "./useEndedElsewhere";
import { togglePictureInPicture, useFullscreen } from "./useFullscreen";
import { usePlaybackSpeed } from "./usePlaybackSpeed";
import { usePlayer } from "./usePlayer";
import { usePlayerKeys } from "./usePlayerKeys";
import { usePlayerMenus } from "./usePlayerMenus";
import { useRemoteFocus } from "./useRemoteFocus";
import { useScrubTarget } from "./useScrubTarget";
import { useSkipMarker } from "./useSkipMarker";
import { useSubtitleChoice } from "./useSubtitleChoice";
import { useSurfacePointer } from "./useSurfacePointer";
import { useUpNext } from "./useUpNext";
import { VolumeControl } from "./VolumeControl";
import { useWatchNavigation } from "./watchSearch";

const watchRoute = getRouteApi("/authed/watch/$workId");

type Props = {
  /**
   * Input comes from a TV's remote. Its arrows scrub and move through the
   * controls. Play or pause takes focus as the controls appear, or the scrub
   * bar when the key that showed them scrubs.
   * The volume and window buttons are left out, because the TV has its own.
   */
  remote?: boolean;
  /** How many episodes have played on in a row without input. The page keeps it across episodes. */
  autoplayRun: number;
  onAutoplayRun: (run: number) => void;
};

/**
 * PlayerScreen is the player. It shows the video, controls that hide while
 * playing, track and quality menus, stream info, Skip Intro and Credits, up
 * next for episodes, and the error state with its recovery actions.
 */
export function PlayerScreen({ remote = false, autoplayRun, onAutoplayRun }: Props) {
  const { workId } = watchRoute.useParams();
  const search = watchRoute.useSearch();
  const { exit, playWork, showSeries } = useWatchNavigation();
  const work = useSuspenseQuery(workQueries.detail(workId)).data;
  const episode = work.episode;
  const nextId = episode?.nextId;
  const previousId = episode?.previousId;
  const autoplayNext = useSuspenseQuery(currentUserQueries.current()).data.playback.autoplayNext;

  const { state, videoRef, run } = usePlayer(workId, search, playEnded);
  const play = state.play;
  const version = work.versions.find((v) => v.id === play?.versionId);
  const live = isLive(state.status);
  useEndedElsewhere(play?.id, (id, message) => run((c) => c.playEnded(id, message)));

  // The play's quality comes from this browser's home and remote settings until one is picked here.
  const [device] = useState(deviceSettings);
  const [picked, setPicked] = useState<{ quality: QualityRequest | undefined }>();
  const quality = picked ? picked.quality : play?.quality;

  // keys dispatches every key press (see usePlayerKeys). The menus and a remote's focus tell it when they move focus.
  const [keys] = useState(() => new KeyDispatcher());

  // Controls show on any input and hide after 3 seconds while playing, 5 with a remote. Pausing shows them.
  const menus = usePlayerMenus(keys);
  const { menu, searching } = menus;
  const { visible, reveal, hide } = useControlsVisibility(state.status === "playing" && !state.paused && !menu, {
    remote,
  });
  // A pause shows them however it came: from the controls, the browser's own media keys or the TV.
  const showPaused = useEffectEvent(reveal);
  useEffect(() => {
    if (state.paused) showPaused();
  }, [state.paused]);
  const control = (command: (c: PlayerController) => void) => {
    reveal();
    run(command);
  };
  const closeMenu = () => {
    reveal();
    menus.close();
  };

  // A track choice the play can't show as it is needs a new play at the
  // same position.
  const {
    subtitle,
    shown,
    textTrackUrl,
    track: subtitleTrack,
    choose: chooseSubtitle,
    pick: pickSubtitle,
  } = useSubtitleChoice(play, version, videoRef, run, (trackId) =>
    control((c) => void c.switchTracks({ subtitleTrackId: trackId })),
  );
  const { audioTrackId, choose: chooseAudio } = useAudioChoice(play, run, (trackId) =>
    control((c) => void c.switchTracks({ audioTrackId: trackId, subtitleTrackId: subtitle })),
  );

  const upNext = useUpNext({
    nextId,
    play,
    live,
    positionMs: state.positionMs,
    counting: state.status === "playing" && !state.paused && !menu && !searching,
    autoplayNext,
    run: autoplayRun,
    onRunChange: onAutoplayRun,
  });
  // At its end, an episode goes on to up next, and the last episode goes to
  // its series' page. Anything else goes back to where playback started.
  function playEnded() {
    if (nextId) upNext.ended();
    else if (episode) showSeries(episode.seriesId, episode.seasonId);
    else exit();
  }
  const { marker, skip: skipMarker } = useSkipMarker(play, live, state.positionMs, {
    controls: visible,
    upNext: upNext.shows,
  });
  const skipPast = (m: SkipMarker, via: (command: (c: PlayerController) => void) => void) => {
    skipMarker();
    via((c) => c.seek(m.endMs));
  };
  // The button over the picture. A remote focuses it while the controls are hidden.
  const overlay = upNext.shows ? "upNext" : marker ? "skip" : undefined;

  // Hiding the controls drops a remote's scrub target.
  const scrubbing = useScrubTarget({ play, live, positionMs: state.positionMs, reveal, run });
  const hideControls = () => {
    hide();
    scrubbing.drop();
  };
  const { fullscreen, toggle: toggleFullscreen } = useFullscreen(videoRef);
  const { thumbRef, playRef, transportRef, overlayRef, ...focus } = useRemoteFocus(
    remote,
    {
      ready: play !== undefined,
      visible,
      live,
      overlay,
      stillWatching: upNext.stillWatching,
      menuOpen: menu !== undefined || searching,
    },
    keys,
  );
  // Scrubbing shows hidden controls with focus on the scrub bar.
  const scrub = (direction: -1 | 1, repeatCount: number) => {
    if (!visible) focus.revealOnBar();
    scrubbing.scrub(direction, repeatCount);
  };

  const commands: Record<KeyCommand, (repeatCount: number) => void> = {
    togglePlay: () => control((c) => c.togglePlay()),
    play: () => control((c) => c.resume()),
    pause: () => control((c) => c.pause()),
    revealAndPause: () => control((c) => c.pause()),
    skipBack: () => scrubbing.skipBy(-backMs),
    skipForward: () => scrubbing.skipBy(forwardMs),
    scrubBack: (repeatCount) => scrub(-1, repeatCount),
    scrubForward: (repeatCount) => scrub(1, repeatCount),
    scrubOk: () => {
      // Once the video has ended, OK on the bar doesn't start it over.
      if (scrubbing.commit() || state.ended) return reveal();
      control((c) => c.togglePlay());
    },
    // With the controls hidden, Up next keeps them hidden.
    focusLeft: () => {
      if (visible) reveal();
      focus.along(-1);
    },
    focusRight: () => {
      if (visible) reveal();
      focus.along(1);
    },
    focusUp: () => {
      reveal();
      focus.across(-1);
    },
    focusDown: () => {
      reveal();
      focus.across(1);
    },
    previous: () => previousId && playWork(previousId),
    next: upNext.playNow,
    showControls: reveal,
    skip: () => marker && skipPast(marker, run),
    hideControls,
    closeMenu,
    exit,
    info: () => {
      reveal();
      menus.open(menu === "info" ? undefined : "info");
    },
    fullscreen: toggleFullscreen,
    mute: () => {
      const video = videoRef.current;
      if (video) video.muted = !video.muted;
    },
    volumeUp: () => stepVolume(videoRef, 0.1, reveal),
    volumeDown: () => stepVolume(videoRef, -0.1, reveal),
  };
  const perform = (command: KeyCommand, repeatCount: number) => commands[command](repeatCount);
  usePlayerKeys(
    keys,
    remote,
    ({ key, target }: KeyInput) => {
      upNext.input();
      if (state.status === "error") return remote && key === "GoBack" ? "exit" : undefined;
      const command = keyCommand(key, {
        remote,
        controlsVisible: visible,
        loading: !live,
        menuOpen: menu !== undefined || searching,
        overlay,
        handled: focusHandles(target, key),
        onScrubBar: focus.onScrubBar(),
      });
      // OK, Left and Right on Up next over hidden controls leave them hidden.
      if (!command && (!remote || visible)) reveal();
      return command;
    },
    perform,
  );

  // Any input means someone is watching, so up next may play on again.
  const onInput = () => {
    upNext.input();
    reveal();
  };
  const surface = useSurfacePointer({
    visible,
    menuOpen: menu !== undefined,
    failed: state.status === "error",
    paused: state.paused,
    onInput,
    onTogglePlay: () => perform("togglePlay", 0),
    onHideControls: hideControls,
    onFullscreen: toggleFullscreen,
  });

  const [chosenSpeed, setSpeed] = usePlaybackSpeed(videoRef, workId);
  // AVPlay plays only at normal speed, so its play offers no other.
  const atSpeed = play === undefined || playsAtSpeed(play.delivery);
  const speed = atSpeed ? chosenSpeed : 1;
  const endsAt = useEndsAt(play && play.durationMs - state.positionMs, speed, 1000);
  const tracks =
    play &&
    audioSubtitles(play, version, {
      canSearch: version?.parts[0] !== undefined,
    });
  // The stream info opens over the menus' buttons, from the gear or from the Info key.
  const menusRef = useRef<HTMLDivElement>(null);
  const boostDialogue = (on: boolean) => {
    saveDeviceSettings({ ...deviceSettings(), dialogueBoost: on });
    control((c) => void c.boostDialogue(on));
  };

  // A Cast session that starts during this play carries the play on to the TV from here.
  const local = useRef<LocalPlay | undefined>(undefined);
  useEffect(() => {
    local.current = play && {
      workId,
      versionId: play.versionId,
      audioTrackId,
      subtitleTrackId: subtitle,
      positionMs: state.positionMs,
    };
  });
  useEffect(() => {
    // A TV casts nothing (read directly, as useCast is, so the build drops it).
    if (import.meta.env.VITE_KIVO_TV === "1") return;
    castLink.setLocal(() => local.current);
    return () => castLink.setLocal(undefined);
  }, []);

  // On TV the browser draws text subtitles at the foot of the picture,
  // under the controls. While they show, the subtitles lift clear of them
  // (index.css) by this much: from the screen's foot to the top of the scrub
  // bar, below the controls' gradient.
  const controlsRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const controls = controlsRef.current;
    const player = controls?.closest<HTMLElement>('[data-part="player"]');
    if (!isTv || !controls || !player) return;
    const observer = new ResizeObserver(() => {
      const top = controls.getBoundingClientRect().top + Number.parseFloat(getComputedStyle(controls).paddingTop);
      player.style.setProperty("--player-cue-lift", `${Math.max(0, player.getBoundingClientRect().bottom - top)}px`);
    });
    observer.observe(controls);
    return () => observer.disconnect();
  }, []);

  const otherVersion = work.versions.find((v) => v.available && v.id !== (play?.versionId ?? search.version));
  const title = playerTitle(work);
  useDocumentTitle(title);

  useMediaSession(
    sessionMetadata(work),
    {
      play: () => run((c) => c.resume()),
      pause: () => run((c) => c.pause()),
      seekBy: (deltaMs) => run((c) => c.skip(deltaMs)),
      seekTo: (positionMs) => run((c) => c.seek(positionMs)),
      previous: previousId ? () => playWork(previousId) : undefined,
      next: nextId ? upNext.playNow : undefined,
    },
    play ? { positionMs: state.positionMs, durationMs: play.durationMs, paused: state.paused } : undefined,
  );

  return (
    // On TV the shell gives the player every key while it's here and not
    // failed (data-failed), so the remote reaches the error's actions.
    // Clicking the picture is a pointer shortcut. The keyboard equivalents
    // are the player's own keys: Space and k play or pause, f goes full screen.
    // biome-ignore lint/a11y/noStaticElementInteractions: see above
    // biome-ignore lint/a11y/useKeyWithClickEvents: see above
    <div
      data-part="player"
      data-failed={state.status === "error" ? "" : undefined}
      data-controls={visible ? "shown" : "hidden"}
      data-subtitle-size={device.subtitleSize}
      data-subtitle-background={device.subtitleBackground}
      className={`fixed inset-0 z-50 overflow-hidden bg-player text-on-player ${visible ? "" : "cursor-none"}`}
      onPointerMove={surface.onPointerMove}
      onPointerDown={surface.onPointerDown}
      onClick={surface.onClick}
      onDoubleClick={surface.onDoubleClick}
    >
      <PlayerVideo
        videoRef={videoRef}
        subtitle={subtitle}
        textTrackUrl={textTrackUrl}
        subtitleTrack={subtitleTrack}
        shown={shown}
      />

      {(state.status === "loading" || state.status === "buffering" || state.status === "seeking") && <LoadingSpinner />}

      <div
        inert={!visible}
        data-surface
        className={`absolute inset-0 flex flex-col justify-between pr-(--layout-safe-area-right) pb-(--layout-safe-area-bottom) pl-(--layout-safe-area-left) transition-opacity tv:px-(--layout-tv-overscan-x) tv:py-(--layout-tv-overscan-y) ${visible ? "opacity-100" : "opacity-0"}`}
      >
        <TopBar remote={remote} title={title} onExit={exit} />

        <div
          ref={controlsRef}
          data-part="player-controls"
          className="flex flex-col gap-2 bg-linear-to-t from-player-scrim to-transparent px-4 pt-16 pb-4"
        >
          {/* Until the play arrives, an empty row holds the scrub bar's height. */}
          {play ? (
            <ScrubBar
              play={play}
              positionMs={state.positionMs}
              target={scrubbing.target}
              thumbRef={thumbRef}
              endsAt={endsAt}
              onSeek={(ms) => control((c) => c.seek(ms))}
            />
          ) : (
            <div aria-hidden="true" className="h-6" />
          )}
          {/*
            The transport in the middle: the volume to its left, the track
            menus and the window's buttons to its right. On a phone the
            transport has a row of its own.
          */}
          <div ref={transportRef} className="flex flex-wrap items-center gap-y-2">
            <div className="flex flex-1 basis-0 items-center">{!remote && <VolumeControl videoRef={videoRef} />}</div>
            <Transport
              episode={episode !== undefined}
              hasPrevious={previousId !== undefined}
              hasNext={nextId !== undefined}
              live={live}
              paused={state.paused}
              playRef={playRef}
              onPrevious={() => previousId && playWork(previousId)}
              onSkip={(deltaMs) => control((c) => c.skip(deltaMs))}
              onTogglePlay={() => control((c) => c.togglePlay())}
              onNext={upNext.playNow}
            />
            <div className="flex flex-1 basis-0 items-center justify-end gap-1">
              {play && (
                <div ref={menusRef} data-part="track-menus" className="flex items-center gap-1">
                  {tracks && (
                    <AudioSubtitlesMenu
                      choices={tracks}
                      audioTrackId={audioTrackId}
                      subtitle={subtitle}
                      keepOpen={remote}
                      onAudio={(trackId) => {
                        reveal();
                        chooseAudio(trackId);
                      }}
                      onSubtitle={chooseSubtitle}
                      onSearchSubtitles={() => menus.setSearching(true)}
                      onDialogueBoost={boostDialogue}
                      isOpen={menu === "tracks"}
                      onOpenChange={menus.openChange("tracks")}
                    />
                  )}
                  <PlaybackSettingsMenu
                    play={play}
                    version={version}
                    versions={work.versions}
                    quality={quality}
                    speed={speed}
                    hasTracks={tracks !== undefined}
                    onTracks={() => menus.open("tracks")}
                    onQuality={(q) => {
                      setPicked({ quality: q });
                      // The pick lasts for this video, at home or away.
                      control((c) => void c.switchTo({ localQuality: q, remoteQuality: q, subtitleTrackId: subtitle }));
                    }}
                    onVersion={(versionId) => {
                      // A version has its own tracks, so the server picks them again.
                      control(
                        (c) => void c.switchTo({ versionId, audioTrackId: undefined, subtitleTrackId: undefined }),
                      );
                    }}
                    onSpeed={atSpeed ? setSpeed : undefined}
                    onStreamInfo={() => menus.open("info")}
                    isOpen={menu === "settings"}
                    onOpenChange={menus.openChange("settings")}
                  />
                  <StreamInfo
                    play={play}
                    version={version}
                    quality={quality}
                    subtitle={shown}
                    isOpen={menu === "info"}
                    onOpenChange={menus.openChange("info")}
                    anchorRef={menusRef}
                  />
                </div>
              )}
              {!remote && <CastButton />}
              {!remote && (
                <WindowButtons
                  fullscreen={fullscreen}
                  live={live}
                  onFullscreen={toggleFullscreen}
                  onPictureInPicture={() => togglePictureInPicture(videoRef)}
                />
              )}
            </div>
          </div>
        </div>
      </div>

      <PictureButtons
        overlayRef={overlayRef}
        upNext={
          upNext.card && nextId ? { nextId, card: upNext.card, onPlay: upNext.playNow, onHide: upNext.hide } : undefined
        }
        marker={marker}
        onSkip={(m) => skipPast(m, control)}
      />

      {version?.parts[0] && (
        <SubtitleSearch
          fileId={version.parts[0].fileId}
          isOpen={searching}
          onClose={() => menus.setSearching(false)}
          onDownloaded={(trackId) => {
            menus.setSearching(false);
            pickSubtitle(trackId);
            control((c) => void c.switchTracks({ subtitleTrackId: trackId }));
          }}
        />
      )}

      {state.status === "error" && state.failure && (
        <PlayerError
          failure={state.failure}
          play={play}
          detail={state.detail}
          onRetry={() => run((c) => void c.retry())}
          onPlayOther={otherVersion ? () => playWork(workId, { version: otherVersion.id }) : undefined}
          onBack={exit}
        />
      )}
    </div>
  );
}

/** Turns the volume up or down a tenth at a time and unmutes. The volume control follows the element. */
function stepVolume(videoRef: RefObject<HTMLVideoElement | null>, delta: number, reveal: () => void) {
  const video = videoRef.current;
  if (!video) return;
  reveal();
  video.muted = false;
  video.volume = Math.min(1, Math.max(0, Math.round((video.volume + delta) * 10) / 10));
}

/** The title over the player and in the tab. An episode's title includes its series, season and number. */
function playerTitle(work: WorkDetail): string {
  const episode = work.episode;
  return episode
    ? `${episode.seriesTitle} · S${episode.seasonNumber} E${episode.number} “${work.title}”`
    : labelledTitle(work);
}

/** What the lock screen and the OS's media controls show for the work. */
function sessionMetadata(work: WorkDetail) {
  const artwork = work.images.still ?? work.images.poster;
  return {
    title: work.title,
    artist: work.episode?.seriesTitle,
    artwork,
  };
}
