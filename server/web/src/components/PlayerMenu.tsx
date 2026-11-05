import type { ReactNode } from "react";
import { Header, MenuItem } from "react-aria-components";
import { Icon } from "@/components/Icon";
import type { Option } from "@/components/Select";

/** Controls over video. They are light on dark in every theme. */
export const playerButton = {
  icon: "inline-flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-md text-on-player hover:bg-player-track data-disabled:cursor-default data-disabled:opacity-50",
  /** Play or pause, larger in the middle of the transport. */
  primary:
    "inline-flex size-14 shrink-0 cursor-pointer items-center justify-center rounded-full text-on-player hover:bg-player-track data-disabled:cursor-default data-disabled:opacity-50",
  text: "inline-flex h-10 shrink-0 cursor-pointer items-center gap-1 rounded-md px-2 text-sm sm:px-3 font-semibold text-on-player hover:bg-player-track data-disabled:cursor-default data-disabled:opacity-50",
};

/** The popover over the video for a player menu or the stream info. */
export const playerPanel =
  "max-h-96 w-max min-w-56 max-w-(--layout-player-panel-width) overflow-auto rounded-md bg-player-panel p-1 shadow-overlay outline-none";

const menuItem =
  "flex cursor-pointer items-center gap-3 rounded-sm px-3 py-2 text-sm text-on-player outline-none data-disabled:cursor-default data-disabled:opacity-50 data-focused:bg-player-track";

/** A section's heading in a player menu: "Audio", "Subtitles". It stays in view as its section scrolls. */
export function MenuHeading({ children }: { children: ReactNode }) {
  return (
    <Header className="sticky top-0 z-10 bg-player-panel px-3 pt-2 pb-1 font-semibold text-on-player-muted text-xs">
      {children}
    </Header>
  );
}

/**
 * One of a player menu's choices, ticked when it's the current one. In
 * Audio & Subtitles, a track is a compact row whose name keeps to one
 * line, and the tracks not chosen are dimmed. Choosing it closes the menu
 * unless `closeOnSelect` is false. On TV, focus lights the row, which the
 * tick tells apart from the chosen one.
 */
export function OptionItem({
  option,
  onAction,
  track = false,
  closeOnSelect,
}: {
  option: Option;
  onAction: (id: string) => void;
  track?: boolean;
  closeOnSelect?: boolean;
}) {
  return (
    <MenuItem
      id={option.id}
      textValue={option.label}
      isDisabled={option.disabled ?? false}
      onAction={() => onAction(option.id)}
      shouldCloseOnSelect={closeOnSelect}
      data-tv-face="row"
      className={track ? `${menuItem} py-1.5` : menuItem}
    >
      {({ isSelected }) => (
        <>
          <Icon name="check" className={`size-4 shrink-0 ${isSelected ? "" : "invisible"}`} />
          <ItemText
            label={option.label}
            detail={option.detail}
            strong={isSelected}
            dim={track && !isSelected}
            oneLine={track}
          />
        </>
      )}
    </MenuItem>
  );
}

/** A switch in a player menu, Off or On. */
export function SwitchItem({ id, label, onAction }: { id: string; label: string; onAction: () => void }) {
  return (
    <MenuItem
      id={id}
      textValue={label}
      onAction={onAction}
      shouldCloseOnSelect={false}
      data-tv-face="row"
      className={menuItem}
    >
      {({ isSelected }) => (
        <>
          <ItemText label={label} />
          <span
            aria-hidden="true"
            className={`ms-auto flex h-5 w-9 shrink-0 items-center rounded-full p-0.5 transition-colors ${isSelected ? "bg-accent" : "bg-player-track"}`}
          >
            <span
              className={`size-4 rounded-full bg-on-player transition-transform ${isSelected ? "translate-x-4" : ""}`}
            />
          </span>
        </>
      )}
    </MenuItem>
  );
}

/** A menu row that opens a submenu or does something, such as "Quality   Auto ›". */
export function RowItem({
  id,
  label,
  value,
  submenu = false,
  onAction,
}: {
  id: string;
  label: string;
  /** The row's current choice, at its end. */
  value?: string;
  /** Whether the row opens a submenu. A chevron then follows the value. */
  submenu?: boolean;
  onAction?: () => void;
}) {
  return (
    <MenuItem id={id} textValue={label} onAction={onAction} data-tv-face="row" className={menuItem}>
      <span className="flex-1">{label}</span>
      {value && <span className="text-on-player-muted">{value}</span>}
      {submenu && <Icon name="chevronRight" className="size-4 shrink-0 text-on-player-muted" />}
    </MenuItem>
  );
}

function ItemText({
  label,
  detail,
  strong = false,
  dim = false,
  oneLine = false,
}: {
  label: string;
  detail?: string;
  strong?: boolean;
  dim?: boolean;
  oneLine?: boolean;
}) {
  return (
    <span className={`flex min-w-0 flex-col ${dim ? "text-on-player-muted" : ""}`}>
      <span className={`${strong ? "font-semibold" : ""} ${oneLine ? "truncate" : ""}`}>{label}</span>
      {detail && <span className="text-on-player-muted text-xs">{detail}</span>}
    </span>
  );
}
