import { useLingui } from "@lingui/react/macro";
import type { WorkDetail } from "@/api/schemas";
import { buttonClass } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { isTv } from "@/tv/flag";
import { DetailHeader } from "./DetailHeader";
import { PlayActions, PlaybackPickers, useResumeParts } from "./PlaybackControls";
import { WorkRows } from "./WorkRows";

type Props = { work: WorkDetail };

/** A movie, other video or extra, which is anything with versions of its own. */
export function MovieDetail({ work }: Props) {
  const { t } = useLingui();
  const resume = useResumeParts(work);
  // The packaged TV app never leaves its page and can't open YouTube.
  const trailer = isTv ? undefined : work.movie?.trailerUrl;
  return (
    <div data-part="detail" data-kind={work.kind}>
      <DetailHeader
        work={work}
        primary={<PlayActions work={work} />}
        playing={resume.playing}
        onPlayFromStart={resume.onPlayFromStart}
        trailing={
          trailer && (
            <a
              href={trailer}
              target="_blank"
              rel="noreferrer"
              aria-label={t`Trailer`}
              title={t`Trailer`}
              className={buttonClass("secondary", "icon")}
              data-part="trailer"
            >
              <Icon name="film" />
            </a>
          )
        }
      >
        <PlaybackPickers work={work} />
      </DetailHeader>
      <WorkRows work={work} />
    </div>
  );
}
