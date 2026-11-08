import { useLingui } from "@lingui/react/macro";
import { type KeyboardEvent, type Ref, useCallback, useId, useState } from "react";
import { Button, Menu, MenuSection, MenuTrigger, Popover, SubmenuTrigger } from "react-aria-components";
import type { PlayStarted, QualityRequest, Version } from "@/api/schemas";
import { useQualityLadder } from "@/api/server";
import { Icon } from "@/components/Icon";
import { BackItem, OptionItem, playerButton, playerMenuPadding, playerPanel, RowItem } from "@/components/PlayerMenu";
import type { Option } from "@/components/Select";
import { versionOption } from "@/components/trackLabels";
import { formatBitrate, formatQuality, formatRung } from "@/i18n/format";
import { qualityKey, qualityOf } from "@/lib/quality";
import { isTv } from "@/tv/flag";

/** The speeds a video plays at, normal among them. */
const speeds = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] as const;

type Props = {
  /** The play's version and the server's cap on it. */
  play: Pick<PlayStarted, "versionId" | "limitBps">;
  version: Version | undefined;
  /** The work's versions. Version shows when there is more than one. */
  versions: readonly Version[];
  /** The quality the play applies, or the one picked here. */
  quality: QualityRequest | undefined;
  speed: number;
  /** Offers Audio & Subtitles, which opens the speech bubble's menu. */
  hasTracks: boolean;
  onTracks: () => void;
  onQuality: (quality: QualityRequest) => void;
  onVersion: (versionId: string) => void;
  /** Absent when the play's player has only normal speed, as AVPlay does. Speed shows only with it. */
  onSpeed?: (speed: number) => void;
  /** Absent when there is no stream info to show, such as while casting. */
  onStreamInfo?: () => void;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  triggerRef?: Ref<HTMLButtonElement>;
};

/** A list a row of Playback Settings opens, and what choosing from it does. */
type SettingsList = {
  id: "quality" | "version" | "speed";
  label: string;
  /** The current choice, shown on the row. */
  value: string | undefined;
  options: Option[];
  selected: string;
  onChoose: (id: string) => void;
};

/**
 * PlaybackSettingsMenu is the gear's menu. Audio & Subtitles opens that
 * menu. Quality, Version (when there are several) and Speed (when the
 * player has speeds) follow, each opening a list with the current choice
 * beside it. Stream info comes last. A list opens as a submenu beside the
 * menu, and on a TV in the menu's place, under a row of its name that goes
 * back to the menu as Left does, so it stays above the controls and on
 * screen.
 */
export function PlaybackSettingsMenu({
  play,
  version,
  versions,
  quality,
  speed,
  hasTracks,
  onTracks,
  onQuality,
  onVersion,
  onSpeed,
  onStreamInfo,
  isOpen,
  onOpenChange,
  triggerRef,
}: Props) {
  const { t } = useLingui();
  const qualities = useQualityOptions(play, version);
  const qualityLabel =
    qualities.find((o) => o.id === qualityKey(quality))?.label ??
    formatQuality(quality?.mode === "limit" && quality.maxBitrateBps ? quality.maxBitrateBps : "auto");
  const speedLabel = (s: number) => (s === 1 ? t`Normal` : t`${s}×`);
  const versionOptions: Option[] = versions.map((v) => ({ ...versionOption(v), disabled: !v.available }));
  const lists: SettingsList[] = [
    {
      id: "quality",
      label: t`Quality`,
      value: qualityLabel,
      options: qualities,
      selected: qualityKey(quality),
      onChoose: (id) => onQuality(qualityOf(id) ?? { mode: "auto" }),
    },
  ];
  if (versions.length > 1) {
    lists.push({
      id: "version",
      label: t`Version`,
      value: version && versionOption(version).label,
      options: versionOptions,
      selected: play.versionId,
      onChoose: onVersion,
    });
  }
  if (onSpeed) {
    lists.push({
      id: "speed",
      label: t`Speed`,
      value: speedLabel(speed),
      options: speeds.map((s) => ({ id: String(s), label: speedLabel(s) })),
      selected: String(speed),
      onChoose: (id) => onSpeed(Number(id)),
    });
  }
  return (
    <MenuTrigger isOpen={isOpen} onOpenChange={onOpenChange}>
      <Button ref={triggerRef} aria-label={t`Playback Settings`} data-tv-face="" className={playerButton.icon}>
        <Icon name="settings" />
      </Button>
      <Popover placement="top end" containerPadding={playerMenuPadding()} className={playerPanel}>
        <SettingsRows lists={lists} onTracks={hasTracks ? onTracks : undefined} onStreamInfo={onStreamInfo} />
      </Popover>
    </MenuTrigger>
  );
}

/**
 * The menu's rows, or on a TV the list a row opened in their place. It
 * lives in the popover, so the menu opens on its rows each time.
 */
function SettingsRows({
  lists,
  onTracks,
  onStreamInfo,
}: {
  lists: SettingsList[];
  onTracks: (() => void) | undefined;
  onStreamInfo: (() => void) | undefined;
}) {
  const { t } = useLingui();
  // The list open in the menu's place, and the row it went back to, which then has focus.
  const [shown, setShown] = useState<SettingsList["id"]>();
  const [left, setLeft] = useState<SettingsList["id"]>();
  const list = isTv ? lists.find((l) => l.id === shown) : undefined;
  const show = (id: SettingsList["id"] | undefined) => {
    setLeft(shown);
    setShown(id);
  };
  const focusLeft = useCallback((menu: HTMLElement | null) => focusOn(menu, left && `[data-key="${left}"]`), [left]);
  if (list) return <ListInPlace list={list} onBack={() => show(undefined)} />;
  return (
    // Right opens a row's list, as OK does.
    <div className="contents" onKeyDownCapture={isTv ? (e) => opensOn(e, lists, show) : undefined}>
      <Menu ref={focusLeft} aria-label={t`Playback Settings`} className="outline-none">
        {onTracks && <RowItem id="tracks" label={t`Audio & Subtitles`} onAction={onTracks} />}
        {lists.map((l) =>
          isTv ? (
            <RowItem key={l.id} id={l.id} label={l.label} value={l.value} submenu onAction={() => show(l.id)} />
          ) : (
            <SubmenuTrigger key={l.id}>
              <RowItem id={l.id} label={l.label} value={l.value} submenu />
              <Popover className={playerPanel}>
                <Menu selectionMode="single" selectedKeys={[l.selected]} className="outline-none">
                  {l.options.map((o) => (
                    <OptionItem key={o.id} option={o} onAction={l.onChoose} />
                  ))}
                </Menu>
              </Popover>
            </SubmenuTrigger>
          ),
        )}
        {onStreamInfo && <RowItem id="streamInfo" label={t`Stream info`} onAction={onStreamInfo} />}
      </Menu>
    </div>
  );
}

/**
 * A list in the menu's place, under a row of its name. That row, Left
 * (Right in a right-to-left language) and Back go back to the menu, and
 * choosing applies the choice and closes it. It opens on the current
 * choice.
 */
function ListInPlace({ list, onBack }: { list: SettingsList; onBack: () => void }) {
  // Named by its first row, rather than by the gear that opened the menu.
  const label = useId();
  return (
    <div
      className="contents"
      data-submenu=""
      onKeyDownCapture={(e) => {
        if (e.key !== backKey(e.currentTarget)) return;
        e.preventDefault();
        e.stopPropagation();
        onBack();
      }}
    >
      <Menu ref={focusChosen} aria-labelledby={label} className="outline-none">
        <MenuSection>
          <BackItem label={list.label} labelId={label} onAction={onBack} />
        </MenuSection>
        <MenuSection selectionMode="single" selectedKeys={[list.selected]}>
          {list.options.map((o) => (
            <OptionItem key={o.id} option={o} onAction={list.onChoose} />
          ))}
        </MenuSection>
      </Menu>
    </div>
  );
}

/** The arrow back towards a list's menu. */
const backKey = (el: Element) => (getComputedStyle(el).direction === "rtl" ? "ArrowRight" : "ArrowLeft");

/** Opens the focused row's list on the arrow away from the menu's start, as a submenu's arrow does. */
function opensOn(e: KeyboardEvent<HTMLElement>, lists: SettingsList[], show: (id: SettingsList["id"]) => void) {
  if (e.key !== (backKey(e.currentTarget) === "ArrowLeft" ? "ArrowRight" : "ArrowLeft")) return;
  const key = e.target instanceof Element ? e.target.closest<HTMLElement>("[data-key]")?.dataset.key : undefined;
  const opened = lists.find((l) => l.id === key);
  if (!opened) return;
  e.preventDefault();
  e.stopPropagation();
  show(opened.id);
}

/**
 * Focuses the menu's row that `selector` finds as the menu shows, after the
 * menu's own focus on its first row, as a list in its place opens on its
 * choice and the menu comes back to the row that opened it.
 */
function focusOn(menu: HTMLElement | null, selector: string | undefined) {
  if (!menu || !selector) return;
  requestAnimationFrame(() => menu.querySelector<HTMLElement>(selector)?.focus());
}

const focusChosen = (menu: HTMLElement | null) => focusOn(menu, '[aria-checked="true"]');

/**
 * The quality menu's options: Auto, Original with the file's bitrate, and
 * the ladder's rungs below that bitrate. Rungs at or above it are left out,
 * because such a limit would change nothing. Options over the server's cap
 * on the play are disabled and say why.
 */
function useQualityOptions(play: Pick<PlayStarted, "limitBps">, version: Version | undefined): Option[] {
  const { t } = useLingui();
  const ladder = useQualityLadder();
  // Use the server's cap, not the quality picked, because picking Original lifts the pick.
  const cap = play.limitBps;
  const capped = cap ? { disabled: true, detail: t`Limited to ${formatBitrate(cap)} for this play` } : {};
  const source = version?.bitrateBps;
  return [
    { id: "auto", label: formatQuality("auto"), detail: t`Based on connection speed and limits` },
    { id: "original", label: formatQuality("original", undefined, source || undefined), ...capped },
    ...ladder
      .filter(({ bitrateBps }) => !source || bitrateBps < source)
      .map((rung) => ({
        id: String(rung.bitrateBps),
        label: formatRung(rung),
        ...(cap && rung.bitrateBps > cap ? capped : {}),
      })),
  ];
}
