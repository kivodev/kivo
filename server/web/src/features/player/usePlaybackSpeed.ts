import { type RefObject, useEffect, useState } from "react";

/**
 * usePlaybackSpeed holds the speed the video plays at. Each work starts at
 * normal speed, and the speed is kept across new plays of the work, such as
 * a track or quality switch. A new play's media loads at the element's
 * default rate, which follows the speed.
 */
export function usePlaybackSpeed(videoRef: RefObject<HTMLVideoElement | null>, workId: string) {
  const [chosen, setChosen] = useState<{ workId: string; speed: number }>();
  const speed = chosen?.workId === workId ? chosen.speed : 1;
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.defaultPlaybackRate = speed;
    video.playbackRate = speed;
  }, [videoRef, speed]);
  return [speed, (s: number) => setChosen({ workId, speed: s })] as const;
}
