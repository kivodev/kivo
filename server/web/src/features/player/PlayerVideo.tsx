import { useLingui } from "@lingui/react/macro";
import type { RefObject } from "react";
import { mediaUrl } from "@/api/playback";
import type { PlayTrack, Track } from "@/api/schemas";
import { SubtitleCanvas } from "./SubtitleCanvas";

type Props = {
  videoRef: RefObject<HTMLVideoElement | null>;
  /** The chosen subtitle track's ID, or `off`. */
  subtitle: string | undefined;
  /** The chosen subtitle's WebVTT file, when the browser shows it as a text track. */
  textTrackUrl: string | undefined;
  subtitleTrack: Track | undefined;
  /** How the chosen subtitle shows, as the play's tracks say. */
  shown: PlayTrack | undefined;
  onPause: () => void;
};

/**
 * PlayerVideo is the picture: the video with the chosen subtitle as a text
 * track, or drawn over it by the renderers.
 */
export function PlayerVideo({ videoRef, subtitle, textTrackUrl, subtitleTrack, shown, onPause }: Props) {
  const { t } = useLingui();
  return (
    <>
      {/* biome-ignore lint/a11y/useMediaCaption: captions come from the chosen subtitle, shown as a text track, by the stream or by the renderers. */}
      <video
        ref={videoRef}
        playsInline
        data-surface
        onPause={onPause}
        className="absolute inset-0 size-full bg-player object-contain"
      >
        {textTrackUrl && (
          <track
            key={textTrackUrl}
            id={`subtitle-${subtitle}`}
            kind="subtitles"
            src={mediaUrl(textTrackUrl)}
            srcLang={subtitleTrack?.language}
            label={subtitleTrack?.title ?? subtitleTrack?.language ?? t`Subtitles`}
          />
        )}
      </video>
      <SubtitleCanvas videoRef={videoRef} shown={shown} />
    </>
  );
}
