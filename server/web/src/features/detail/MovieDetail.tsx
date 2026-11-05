import type { WorkDetail } from "@/api/schemas";
import { DetailHeader } from "./DetailHeader";
import { PlayActions, PlaybackPickers, useResumeParts } from "./PlaybackControls";
import { WorkRows } from "./WorkRows";

type Props = { work: WorkDetail };

/** A movie, other video or extra, which is anything with versions of its own. */
export function MovieDetail({ work }: Props) {
  const resume = useResumeParts(work);
  return (
    <div data-part="detail" data-kind={work.kind}>
      <DetailHeader
        work={work}
        primary={<PlayActions work={work} />}
        playing={resume.playing}
        onPlayFromStart={resume.onPlayFromStart}
      >
        <PlaybackPickers work={work} />
      </DetailHeader>
      <WorkRows work={work} />
    </div>
  );
}
