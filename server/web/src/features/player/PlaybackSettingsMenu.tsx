import { useLingui } from "@lingui/react/macro";
import type { Ref } from "react";
import { Button, Menu, MenuTrigger, Popover, SubmenuTrigger } from "react-aria-components";
import type { PlayStarted, QualityRequest, Version } from "@/api/schemas";
import { useQualityLadder } from "@/api/server";
import { Icon } from "@/components/Icon";
import { OptionItem, playerButton, playerPanel, RowItem } from "@/components/PlayerMenu";
import type { Option } from "@/components/Select";
import { versionOption } from "@/components/trackLabels";
import { formatBitrate, formatQuality, formatRung } from "@/i18n/format";
import { qualityKey, qualityOf } from "@/lib/quality";

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

/**
 * PlaybackSettingsMenu is the gear's menu. Audio & Subtitles opens that
 * menu. Quality, Version (when there are several) and Speed (when the
 * player has speeds) follow, each a submenu with the current choice beside
 * it. Stream info comes last.
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
  return (
    <MenuTrigger isOpen={isOpen} onOpenChange={onOpenChange}>
      <Button ref={triggerRef} aria-label={t`Playback Settings`} data-tv-face="" className={playerButton.icon}>
        <Icon name="settings" />
      </Button>
      <Popover placement="top end" className={playerPanel}>
        <Menu aria-label={t`Playback Settings`} className="outline-none">
          {hasTracks && <RowItem id="tracks" label={t`Audio & Subtitles`} onAction={onTracks} />}
          <SubmenuTrigger>
            <RowItem id="quality" label={t`Quality`} value={qualityLabel} submenu />
            <Popover className={playerPanel}>
              <Menu selectionMode="single" selectedKeys={[qualityKey(quality)]} className="outline-none">
                {qualities.map((o) => (
                  <OptionItem key={o.id} option={o} onAction={(id) => onQuality(qualityOf(id) ?? { mode: "auto" })} />
                ))}
              </Menu>
            </Popover>
          </SubmenuTrigger>
          {versions.length > 1 && (
            <SubmenuTrigger>
              <RowItem id="version" label={t`Version`} value={version && versionOption(version).label} submenu />
              <Popover className={playerPanel}>
                <Menu selectionMode="single" selectedKeys={[play.versionId]} className="outline-none">
                  {versionOptions.map((o) => (
                    <OptionItem key={o.id} option={o} onAction={onVersion} />
                  ))}
                </Menu>
              </Popover>
            </SubmenuTrigger>
          )}
          {onSpeed && (
            <SubmenuTrigger>
              <RowItem id="speed" label={t`Speed`} value={speedLabel(speed)} submenu />
              <Popover className={playerPanel}>
                <Menu selectionMode="single" selectedKeys={[String(speed)]} className="outline-none">
                  {speeds.map((s) => (
                    <OptionItem key={s} option={{ id: String(s), label: speedLabel(s) }} onAction={() => onSpeed(s)} />
                  ))}
                </Menu>
              </Popover>
            </SubmenuTrigger>
          )}
          {onStreamInfo && <RowItem id="streamInfo" label={t`Stream info`} onAction={onStreamInfo} />}
        </Menu>
      </Popover>
    </MenuTrigger>
  );
}

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
