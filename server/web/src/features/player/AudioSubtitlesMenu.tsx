import { i18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";
import type { CSSProperties, KeyboardEvent, Ref } from "react";
import { Button, Menu, MenuSection, MenuTrigger, Popover } from "react-aria-components";
import { subtitlesOff } from "@/api/playback";
import type { DialogueBoost, PlayStarted, Version } from "@/api/schemas";
import { Icon } from "@/components/Icon";
import { MenuHeading, OptionItem, playerButton, SwitchItem } from "@/components/PlayerMenu";
import { type TrackChoices, trackChoices } from "@/components/TrackPicker";

/** The Subtitles section's last item. */
const searchSubtitles = "search";

const dialogueBoostId = "dialogueBoost";

/** What the Audio & Subtitles menu offers a play. Undefined parts aren't shown. */
export type AudioSubtitles = {
  audio: TrackChoices | undefined;
  /** The play's dialogue boost, where it offers one. */
  boost: DialogueBoost | undefined;
  subtitles: TrackChoices | undefined;
};

/**
 * What the menu offers a play. It offers the audio tracks when there is
 * more than one, and the dialogue boost when the play has one. It offers
 * the subtitles, with Search subtitles when the play's file can be searched
 * for. It returns undefined when there is nothing to offer.
 */
export function audioSubtitles(
  play: Pick<PlayStarted, "dialogueBoost">,
  version: Version | undefined,
  { canSearch }: { canSearch: boolean },
): AudioSubtitles | undefined {
  const tracks = version?.tracks ?? [];
  const choices = {
    audio: trackChoices({ kind: "audio", tracks: tracks.filter((tr) => tr.type === "audio") }),
    boost: play.dialogueBoost,
    subtitles: trackChoices({
      kind: "subtitle",
      tracks: tracks.filter((tr) => tr.type === "subtitle"),
      offId: subtitlesOff,
      extra: canSearch ? [{ id: searchSubtitles, label: i18n._(msg`Search subtitles…`) }] : [],
    }),
  };
  return choices.audio || choices.boost || choices.subtitles ? choices : undefined;
}

type Props = {
  choices: AudioSubtitles;
  audioTrackId: string | undefined;
  /** The subtitle track chosen, or `subtitlesOff`. */
  subtitle: string;
  onAudio: (trackId: string) => void;
  onSubtitle: (trackId: string) => void;
  onSearchSubtitles: () => void;
  onDialogueBoost: (on: boolean) => void;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  triggerRef?: Ref<HTMLButtonElement>;
  /** Whether choosing a track leaves the menu open, as on a TV. */
  keepOpen?: boolean;
};

/**
 * AudioSubtitlesMenu is the speech bubble's menu. It has two columns side
 * by side: Audio, with the dialogue boost over its tracks, and Subtitles.
 * Each column scrolls on its own and opens at its chosen track, and opened
 * from the keyboard or a remote, focus starts on the audio playing. Up and
 * Down move within a column and Left and Right between them, landing on
 * the other's chosen track. On a narrow screen the columns stack, and the
 * arrows go through them as one list. Choosing a track applies it at once
 * and closes the menu, unless `keepOpen`. The boost's switch starts a new
 * play with the boost and leaves the menu open.
 */
export function AudioSubtitlesMenu({
  choices: { audio, boost, subtitles },
  audioTrackId,
  subtitle,
  onAudio,
  onSubtitle,
  onSearchSubtitles,
  onDialogueBoost,
  isOpen,
  onOpenChange,
  triggerRef,
  keepOpen = false,
}: Props) {
  const { t } = useLingui();
  const hasAudio = audio !== undefined || boost !== undefined;
  const twoColumns = hasAudio && subtitles !== undefined;
  // In two columns, the boost sits over the audio tracks and Subtitles spans both rows beside them.
  const column = twoColumns ? "md:min-h-0 md:overflow-y-auto" : "";
  // Search subtitles opens a dialog in the menu's place, so it closes the menu.
  const closeOnSelect = (id: string) => (keepOpen && id !== searchSubtitles ? false : undefined);
  return (
    <MenuTrigger isOpen={isOpen} onOpenChange={onOpenChange}>
      <Button ref={triggerRef} aria-label={t`Audio & Subtitles`} data-tv-face="" className={playerButton.icon}>
        <Icon name="captions" />
      </Button>
      <Popover
        placement="top end"
        className={`flex flex-col rounded-md bg-player-panel p-1 shadow-overlay outline-none ${twoColumns ? "w-160 max-md:w-80" : "w-80"}`}
      >
        {/* The columns' own arrows go before the menu's, which would go through them as one list. */}
        <div className="contents" onKeyDownCapture={moveInColumns}>
          <Menu
            ref={openAtChosen}
            aria-label={t`Audio & Subtitles`}
            className={`grid max-h-112 min-h-0 flex-1 overflow-y-auto outline-none ${twoColumns ? "md:grid-cols-2 md:grid-rows-(--audio-subtitles-rows) md:gap-x-1 md:overflow-y-visible" : ""}`}
            // The boost takes the height it needs and the audio tracks the rest.
            style={{ "--audio-subtitles-rows": "auto minmax(0, 1fr)" } as CSSProperties}
          >
            {boost && (
              <MenuSection
                selectionMode="multiple"
                selectedKeys={boost.enabled ? [dialogueBoostId] : []}
                data-column="audio"
                className="md:col-start-1 md:row-start-1"
              >
                <MenuHeading>{audio?.label ?? t`Audio`}</MenuHeading>
                <SwitchItem
                  id={dialogueBoostId}
                  label={t`Dialogue boost`}
                  onAction={() => onDialogueBoost(!boost.enabled)}
                />
              </MenuSection>
            )}
            {audio && (
              <MenuSection
                selectionMode="single"
                selectedKeys={[audioTrackId ?? ""]}
                data-column="audio"
                className={`relative md:col-start-1 ${boost ? "md:row-start-2" : "md:row-span-2 md:row-start-1"} ${column}`}
              >
                {!boost && <MenuHeading>{audio.label}</MenuHeading>}
                {audio.options.map((o) => (
                  <OptionItem key={o.id} option={o} onAction={onAudio} track closeOnSelect={closeOnSelect(o.id)} />
                ))}
              </MenuSection>
            )}
            {subtitles && (
              // Every track can be chosen. A track the play can't show at once comes with a new play.
              <MenuSection
                selectionMode="single"
                selectedKeys={[subtitle]}
                data-column="subtitles"
                className={`relative ${twoColumns ? "md:col-start-2 md:row-span-2 md:row-start-1" : ""} ${column}`}
              >
                <MenuHeading>{subtitles.label}</MenuHeading>
                {subtitles.options.map((o) => (
                  <OptionItem
                    key={o.id}
                    option={o}
                    track
                    closeOnSelect={closeOnSelect(o.id)}
                    onAction={(id) => (id === searchSubtitles ? onSearchSubtitles() : onSubtitle(id))}
                  />
                ))}
              </MenuSection>
            )}
          </Menu>
        </div>
      </Popover>
    </MenuTrigger>
  );
}

/** A column's chosen track. */
const chosenIn = (rows: readonly HTMLElement[]) =>
  rows.find((row) => row.getAttribute("role") === "menuitemradio" && row.getAttribute("aria-checked") === "true");

/**
 * Scrolls each scrolling column to its chosen track as the menu opens. A
 * menu opened from the keyboard or a remote focuses its first row, the
 * boost, and focus goes on to the audio playing instead, or the subtitle
 * where there's no choice of audio.
 */
function openAtChosen(menu: HTMLElement | null) {
  if (!menu) return;
  requestAnimationFrame(() => {
    for (const column of menu.querySelectorAll<HTMLElement>("section")) {
      const chosen = column.querySelector<HTMLElement>('[aria-checked="true"]');
      if (!chosen || column.scrollHeight <= column.clientHeight) continue;
      column.scrollTop = chosen.offsetTop - (column.clientHeight - chosen.offsetHeight) / 2;
    }
    const focused = document.activeElement;
    if (focused === menu || !(focused instanceof HTMLElement) || !menu.contains(focused)) return;
    chosenIn([...menu.querySelectorAll<HTMLElement>('[role="menuitemradio"]')])?.focus();
  });
}

/**
 * The arrows in the columns while they're side by side: Up and Down move
 * within the focused row's column and stop at its ends, and Left and Right
 * go to the other column's chosen track, or its first row.
 */
function moveInColumns(e: KeyboardEvent<HTMLElement>) {
  const vertical = e.key === "ArrowUp" || e.key === "ArrowDown";
  if (!vertical && e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
  const row = e.target instanceof Element ? e.target.closest<HTMLElement>('[role^="menuitem"]') : null;
  const name = row?.closest<HTMLElement>("[data-column]")?.dataset.column;
  if (!row || !name) return;
  const rowsOf = (column: string) => [
    ...e.currentTarget.querySelectorAll<HTMLElement>(
      `[data-column="${column}"] [role^="menuitem"]:not([aria-disabled="true"])`,
    ),
  ];
  const own = rowsOf(name);
  const other = rowsOf(name === "audio" ? "subtitles" : "audio");
  const across = other[0]?.getBoundingClientRect().left;
  const here = row.getBoundingClientRect().left;
  // Stacked, the menu's own arrows go through them as one list.
  if (across === undefined || Math.abs(across - here) < 1) return;
  e.preventDefault();
  e.stopPropagation();
  let target: HTMLElement | undefined;
  if (vertical) target = own[own.indexOf(row) + (e.key === "ArrowUp" ? -1 : 1)];
  else if ((e.key === "ArrowRight") === across > here) target = chosenIn(other) ?? other[0];
  target?.focus();
}
