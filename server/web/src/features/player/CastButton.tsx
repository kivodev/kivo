import { useLingui } from "@lingui/react/macro";
import { type CSSProperties, useEffect, useSyncExternalStore } from "react";
import {
  Button as AriaButton,
  Dialog,
  DialogTrigger,
  Heading,
  Label,
  Popover,
  Slider,
  SliderThumb,
  SliderTrack,
} from "react-aria-components";
import { Icon } from "@/components/Icon";
import { playerButton, playerPanel } from "@/components/PlayerMenu";
import type { CastRequest } from "./castProtocol";
import { type CastConnection, castLink, unavailableCast } from "./castSession";

/**
 * The page's Cast session. It loads Google's sender library on first use.
 * A TV casts nothing, so its build has none; the flag is read directly
 * rather than isTv, which the bundler wouldn't drop the session for.
 */
export const useCast = import.meta.env.VITE_KIVO_TV === "1" ? useUnavailableCast : useWebCast;

function useUnavailableCast() {
  return unavailableCast;
}

function useWebCast() {
  useEffect(() => castLink.start(), []);
  return useSyncExternalStore(castLink.subscribe, castLink.getSnapshot);
}

type Props = {
  /** What is cast, for the menu's line. Absent when nothing is playing. */
  title?: string;
  /** Called on Stop casting. The TV stops, and what played there carries on here. */
  onStopCasting?: (carry: CastRequest | undefined) => void;
};

/**
 * CastButton uses Google's glyph and states. It is hidden when no Cast
 * device is found. It shows an outline when not connected, pulsing arcs
 * while connecting, and fills in the accent colour once connected. When not
 * connected, it opens Chrome's list of devices. When connected, it opens the
 * Cast menu with the device, the TV's volume, Stop casting and Disconnect.
 */
export const CastButton = import.meta.env.VITE_KIVO_TV === "1" ? () => null : WebCastButton;

function WebCastButton({ title, onStopCasting }: Props) {
  const { t } = useLingui();
  const cast = useCast();
  if (cast.connection === "unavailable") return null;
  if (cast.connection !== "connected") {
    return (
      <AriaButton
        aria-label={cast.connection === "connecting" ? t`Cast, connecting` : t`Cast`}
        className={playerButton.icon}
        onPress={() => castLink.showDevices()}
      >
        <CastGlyph connection={cast.connection} />
      </AriaButton>
    );
  }
  const state = cast.media?.state;
  return (
    <DialogTrigger>
      <AriaButton aria-label={t`Casting to ${cast.device ?? ""}`} className={`${playerButton.icon} text-accent`}>
        <CastGlyph connection="connected" />
      </AriaButton>
      <Popover placement="bottom end" className={`${playerPanel} w-72 p-3`}>
        <Dialog aria-label={cast.device ?? t`Cast`} className="flex flex-col gap-3 outline-none">
          <Heading slot="title" className="font-semibold text-on-player">
            {cast.device}
          </Heading>
          {cast.media && title && (
            <div className="flex items-center gap-2">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm text-on-player">{title}</p>
                <p className="text-on-player-muted text-xs">
                  {state === "playing" ? t`Playing` : state === "paused" ? t`Paused` : t`Loading`}
                </p>
              </div>
              <AriaButton
                aria-label={state === "playing" ? t`Pause` : t`Play`}
                className={playerButton.icon}
                onPress={() => castLink.togglePlay()}
              >
                <Icon name={state === "playing" ? "pause" : "play"} />
              </AriaButton>
            </div>
          )}
          <Slider
            aria-label={t`TV volume`}
            minValue={0}
            maxValue={1}
            step={0.05}
            value={cast.volume}
            onChange={(v) => castLink.setVolume(v)}
            className="flex items-center gap-2 text-on-player-muted"
          >
            <Label className="sr-only">{t`TV volume`}</Label>
            <Icon name="muted" className="size-4" />
            <SliderTrack className="relative flex h-6 flex-1 items-center">
              {({ state: slider }) => (
                <>
                  <div className="h-1 w-full rounded-full bg-player-track">
                    <div
                      className="h-1 w-(--level) rounded-full bg-on-player"
                      style={{ "--level": `${slider.getThumbPercent(0) * 100}%` } as CSSProperties}
                    />
                  </div>
                  <SliderThumb className="top-1/2 size-3.5 rounded-full bg-on-player data-focus-visible:outline-2 data-focus-visible:outline-focus" />
                </>
              )}
            </SliderTrack>
            <Icon name="volume" className="size-4" />
          </Slider>
          <div className="flex flex-col">
            <MenuRow
              label={t`Stop casting`}
              detail={t`Continue on this computer`}
              onPress={() => {
                const carry = castLink.end(true);
                onStopCasting?.(carry);
              }}
            />
            <MenuRow label={t`Disconnect`} detail={t`Keep playing on the TV`} onPress={() => castLink.end(false)} />
          </div>
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}

function MenuRow({ label, detail, onPress }: { label: string; detail: string; onPress: () => void }) {
  return (
    <AriaButton
      onPress={onPress}
      className="flex cursor-pointer flex-col items-start rounded-sm px-2 py-2 text-left outline-none hover:bg-player-track data-focus-visible:bg-player-track"
    >
      <span className="text-sm text-on-player">{label}</span>
      <span className="text-on-player-muted text-xs">{detail}</span>
    </AriaButton>
  );
}

/** Google's Cast glyph: a screen with signal arcs in its corner. The screen is filled once connected. */
export function CastGlyph({ connection }: { connection: CastConnection }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className="size-5" aria-hidden="true" focusable="false">
      <path d="M21 3H3c-1.1 0-2 .9-2 2v3h2V5h18v14h-7v2h7c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2z" />
      <g className={connection === "connecting" ? "animate-pulse" : undefined}>
        <path d="M1 18v3h3c0-1.66-1.34-3-3-3z" />
        <path d="M1 14v2c2.76 0 5 2.24 5 5h2c0-3.87-3.13-7-7-7z" />
        <path d="M1 10v2c4.97 0 9 4.03 9 9h2c0-6.08-4.93-11-11-11z" />
      </g>
      {connection === "connected" && <path d="M5 7v1.63c3.96 1.28 7.07 4.39 8.35 8.35H19V7H5z" />}
    </svg>
  );
}
