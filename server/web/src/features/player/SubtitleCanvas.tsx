import { type RefObject, useEffect, useRef } from "react";
import { mediaUrl } from "@/api/playback";
import type { PlayTrack } from "@/api/schemas";
import { isTv } from "@/tv/flag";
import { canvasRenderer } from "./useSubtitleChoice";

type Props = {
  videoRef: RefObject<HTMLVideoElement | null>;
  /** How the chosen subtitle shows, as the play's tracks say. */
  shown: PlayTrack | undefined;
};

/**
 * SubtitleCanvas draws an ASS or PGS subtitle file over the video. A
 * renderer owns its canvas and hands it to a worker, so each renderer gets a
 * new canvas inside this layer and removes it when it stops. The renderers
 * load only when a play shows one of them.
 */
export function SubtitleCanvas({ videoRef, shown }: Props) {
  const layerRef = useRef<HTMLDivElement>(null);
  const kind = canvasRenderer(shown);
  const url = kind && shown?.url;
  const fonts = kind === "ass" ? (shown?.fonts ?? []).join("\n") : "";

  useEffect(() => {
    const video = videoRef.current;
    const layer = layerRef.current;
    // The TV build reports that it draws neither, so the server burns them in. The flag is a constant, so the TV build leaves out the renderers' chunk.
    if (isTv) return;
    if (!video || !layer || !kind || !url) return;
    const canvas = document.createElement("canvas");
    canvas.className = kind === "pgs" ? "absolute inset-0 size-full object-contain" : "absolute";
    layer.append(canvas);
    let stop: (() => void) | undefined;
    let stopped = false;
    void import("./renderers").then(({ drawAss, drawPgs }) => {
      if (stopped) return;
      stop =
        kind === "ass"
          ? drawAss(video, canvas, mediaUrl(url), fonts ? fonts.split("\n").map(mediaUrl) : [])
          : drawPgs(video, canvas, mediaUrl(url));
    });
    return () => {
      stopped = true;
      stop?.();
      canvas.remove();
    };
  }, [videoRef, kind, url, fonts]);

  return <div ref={layerRef} data-part="subtitle-layer" className="pointer-events-none absolute inset-0" />;
}
