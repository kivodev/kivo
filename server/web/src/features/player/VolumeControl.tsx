import { useLingui } from "@lingui/react/macro";
import { type CSSProperties, type RefObject, useEffect, useState } from "react";
import { Button, Slider, SliderThumb, SliderTrack } from "react-aria-components";
import { Icon } from "@/components/Icon";
import { playerButton } from "@/components/PlayerMenu";
import { readStoredJson, writeStoredJson } from "@/lib/storage";

// The volume is kept per browser, between videos.
const storageKey = "kivo.volume";

type Saved = { volume: number; muted: boolean };

/** The saved volume, or undefined for the browser's own. */
function load(): Saved | undefined {
  const saved = readStoredJson(storageKey) as Saved | null | undefined;
  return saved && typeof saved.volume === "number" ? saved : undefined;
}

const save = (saved: Saved) => writeStoredJson(storageKey, saved);

/** A device with a mouse shows a volume slider. A phone has volume buttons, so it shows only mute. */
const slider = () => globalThis.matchMedia?.("(hover: hover) and (pointer: fine)").matches ?? false;

/**
 * VolumeControl is mute and volume for the video's own volume. The `m` key
 * also mutes. The volume is restored for each video this browser plays.
 */
export function VolumeControl({ videoRef }: { videoRef: RefObject<HTMLVideoElement | null> }) {
  const { t } = useLingui();
  const [state, setState] = useState<Saved>({ volume: 1, muted: false });
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const saved = load();
    if (saved) {
      video.volume = saved.volume;
      video.muted = saved.muted;
    }
    const sync = () => {
      const next = { volume: video.volume, muted: video.muted };
      setState(next);
      save(next);
    };
    sync();
    video.addEventListener("volumechange", sync);
    return () => video.removeEventListener("volumechange", sync);
  }, [videoRef]);

  const silent = state.muted || state.volume === 0;
  const toggleMute = () => {
    const video = videoRef.current;
    if (!video) return;
    // Unmuting at zero volume would still be silent.
    if (silent && video.volume === 0) video.volume = 0.5;
    video.muted = !silent;
  };
  const shown = state.muted ? 0 : state.volume;

  return (
    <div className="flex items-center">
      <Button aria-label={silent ? t`Unmute` : t`Mute`} className={playerButton.icon} onPress={toggleMute}>
        <Icon name={silent ? "muted" : "volume"} />
      </Button>
      {slider() && (
        <Slider
          aria-label={t`Volume`}
          minValue={0}
          maxValue={1}
          step={0.05}
          value={shown}
          onChange={(volume) => {
            const video = videoRef.current;
            if (!video) return;
            video.volume = volume;
            video.muted = volume === 0;
          }}
          className="w-20 max-sm:hidden"
        >
          <SliderTrack className="relative flex h-6 w-full cursor-pointer items-center">
            <div className="relative h-1 w-full rounded-full bg-player-track">
              <div
                className="absolute inset-y-0 left-0 w-(--level) rounded-full bg-on-player"
                style={{ "--level": `${shown * 100}%` } as CSSProperties}
              />
            </div>
            <SliderThumb className="top-1/2 size-3 rounded-full bg-on-player shadow-overlay data-focus-visible:outline-2 data-focus-visible:outline-focus" />
          </SliderTrack>
        </Slider>
      )}
    </div>
  );
}
