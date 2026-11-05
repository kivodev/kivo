import { i18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import type { Track } from "@/api/schemas";
import { type Option, Select } from "./Select";
import { trackNames, trackOptions } from "./trackLabels";

/** The choice that leaves the track to the server, which picks by the user's settings. */
export const automaticTrack = "automatic";

type Choices = {
  kind: "audio" | "subtitle";
  /** The tracks of this kind to choose from. */
  tracks: readonly Track[];
  /** The ID of an Off option, listed before the tracks. Subtitles have one. */
  offId?: string;
  /**
   * The track a play starts with when none is chosen, or `offId`. It is
   * offered as an Automatic option labelled with that track's name.
   */
  automatic?: string;
  /** Says a choice is kept for the rest of the show. */
  remembered?: boolean;
  /** Options after the tracks, such as Search subtitles. */
  extra?: readonly Option[];
};

/** An audio or subtitle picker's heading, note and options. */
export type TrackChoices = {
  label: string;
  /** "Remembered for this show", on an episode whose choices are kept. */
  description: string | undefined;
  options: Option[];
};

/**
 * What the audio or subtitle picker offers, on the detail page and in the
 * player. It lists the tracks by their short names, Off for subtitles, and
 * Automatic when nothing's chosen yet. It is undefined when there's no
 * choice, which is with fewer than two tracks or with Off alone.
 */
export function trackChoices({
  kind,
  tracks,
  offId,
  automatic,
  remembered,
  extra = [],
}: Choices): TrackChoices | undefined {
  const choices: Option[] = [
    ...(offId !== undefined ? [{ id: offId, label: i18n._(msg`Off`) }] : []),
    ...trackOptions(tracks),
    ...extra,
  ];
  if (choices.length < 2) return undefined;
  const plays =
    automatic === undefined
      ? undefined
      : automatic === offId
        ? i18n._(msg`Off`)
        : trackNames(tracks).get(automatic)?.name;
  const automaticLabel = plays ? i18n._(msg`Automatic (${plays})`) : i18n._(msg`Automatic`);
  return {
    label: kind === "audio" ? i18n._(msg`Audio`) : i18n._(msg`Subtitles`),
    description: remembered ? i18n._(msg`Remembered for this show`) : undefined,
    options: [...(automatic !== undefined ? [{ id: automaticTrack, label: automaticLabel }] : []), ...choices],
  };
}

type Props = Choices & {
  /** A track's ID, `offId`, or `automaticTrack`. */
  selected: string;
  onChange: (id: string) => void;
};

/**
 * The detail page's audio or subtitle picker, as a labelled dropdown. The
 * player lists the same choices in its menu.
 */
export function TrackPicker({ selected, onChange, ...choices }: Props) {
  const picker = trackChoices(choices);
  if (!picker) return null;
  return (
    <Select
      variant="inline"
      part={`${choices.kind}-picker`}
      label={picker.label}
      options={picker.options}
      selected={selected}
      onChange={onChange}
      description={picker.description}
    />
  );
}
