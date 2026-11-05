import type { PlayTrack } from "@/api/schemas";

/**
 * Whether the play needs replacing, at the same position, for its subtitle
 * to show as picture-in-picture starts or ends. Nothing drawn over the
 * video shows in picture-in-picture, so a play started there asks for no
 * external subtitles. Entering it replaces a play whose subtitle showing is
 * delivered external. Leaving it replaces a play started there whose subtitle is
 * burned in, which an external file could show again. Any other subtitle shows the
 * same either way.
 */
export function pictureInPictureNeedsNewPlay(
  inPictureInPicture: boolean,
  playInPictureInPicture: boolean,
  subtitle: Pick<PlayTrack, "delivery"> | undefined,
): boolean {
  if (inPictureInPicture === playInPictureInPicture) return false;
  return subtitle?.delivery === (inPictureInPicture ? "external" : "burn");
}
