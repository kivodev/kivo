import { useLingui } from "@lingui/react/macro";
import { DropIndicator, GridList, GridListItem, Button as RacButton, useDragAndDrop } from "react-aria-components";
import type { HomeRowInfo } from "@/api/schemas";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { SettingsRow, settingsCardClass } from "@/components/SettingsLayout";
import { Switch } from "@/components/Switch";
import { useHomeRowTitle } from "@/features/home/rowTitle";
import { TvControl } from "@/tv/forms/TvControl";

type Props = {
  rows: HomeRowInfo[];
  onChange: (rows: HomeRowInfo[]) => void;
};

/**
 * Home's rows in order, as a settings card, each with a switch to show it.
 * Drag a row by its handle to reorder them; dragging works from the keyboard
 * too. Press Enter on the handle, use the arrows, then press Enter to drop.
 */
export function HomeRowOrder({ rows, onChange }: Props) {
  const { t } = useLingui();
  const title = useHomeRowTitle();
  const { dragAndDropHooks } = useDragAndDrop({
    getItems: (keys) => [...keys].map((key) => ({ "text/plain": String(key) })),
    onReorder: ({ keys, target }) => {
      const moving = rows.filter((r) => keys.has(r.id));
      const rest = rows.filter((r) => !keys.has(r.id));
      const at = rest.findIndex((r) => r.id === target.key) + (target.dropPosition === "after" ? 1 : 0);
      rest.splice(at, 0, ...moving);
      onChange(rest);
    },
    renderDropIndicator: (target) => (
      <DropIndicator
        target={target}
        className={({ isDropTarget }) => (isDropTarget ? "h-0.5 rounded-full bg-accent" : "")}
      />
    ),
  });
  return (
    <GridList
      data-part="home-row-order"
      aria-label={t`Home rows`}
      items={rows}
      dependencies={[rows]}
      dragAndDropHooks={dragAndDropHooks}
      className={settingsCardClass}
    >
      {(row) => {
        const name = title(row);
        return (
          <GridListItem
            id={row.id}
            textValue={name}
            className="flex items-center gap-3 px-2 py-1.5 outline-none data-dragging:opacity-50 data-focus-visible:bg-surface-hover"
          >
            <RacButton
              slot="drag"
              className="flex size-8 shrink-0 cursor-grab items-center justify-center rounded-md text-text-muted hover:bg-surface-hover hover:text-text"
            >
              <Icon name="grip" className="size-4" />
            </RacButton>
            <span className={`flex-1 truncate font-medium ${row.shown ? "text-text" : "text-text-muted"}`}>{name}</span>
            <Switch
              aria-label={t`Shown: ${name}`}
              isSelected={row.shown}
              onChange={(shown) => onChange(rows.map((r) => (r.id === row.id ? { ...r, shown } : r)))}
            />
          </GridListItem>
        );
      }}
    </GridList>
  );
}

/** Home's rows for TV: Move up and Move down for each, and OK on its switch to show it. */
export function HomeRowsTv({ rows, onChange }: Props) {
  const { t } = useLingui();
  const title = useHomeRowTitle();
  const move = (index: number, by: number) => {
    const next = [...rows];
    next.splice(index + by, 0, ...next.splice(index, 1));
    onChange(next);
  };
  return (
    <div data-part="home-rows-tv" className={settingsCardClass}>
      {rows.map((row, index) => {
        const name = title(row);
        return (
          <SettingsRow key={row.id} label={name}>
            <span className="flex items-center gap-2">
              <TvControl focusKey={`home-row:${row.id}:up`}>
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={index === 0}
                  aria-label={t`Move up: ${name}`}
                  onPress={() => move(index, -1)}
                >
                  <Icon name="chevronUp" className="size-4" />
                </Button>
              </TvControl>
              <TvControl focusKey={`home-row:${row.id}:down`}>
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={index === rows.length - 1}
                  aria-label={t`Move down: ${name}`}
                  onPress={() => move(index, 1)}
                >
                  <Icon name="chevronDown" className="size-4" />
                </Button>
              </TvControl>
              <Switch
                aria-label={t`Shown: ${name}`}
                isSelected={row.shown}
                onChange={(shown) => onChange(rows.map((r) => (r.id === row.id ? { ...r, shown } : r)))}
              />
            </span>
          </SettingsRow>
        );
      })}
    </div>
  );
}
