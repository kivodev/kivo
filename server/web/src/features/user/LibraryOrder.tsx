import { useLingui } from "@lingui/react/macro";
import { Button, Checkbox, DropIndicator, GridList, GridListItem, useDragAndDrop } from "react-aria-components";
import type { Library } from "@/api/schemas";
import { CheckBox } from "@/components/CheckBox";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { SettingsRow, settingsCardClass } from "@/components/SettingsLayout";
import { Switch } from "@/components/Switch";

type Props = {
  libraries: Library[];
  onChange: (libraries: Library[]) => void;
};

/**
 * Your libraries in your order, as a settings card. Tick the ones to pin,
 * which the sidebar and Home show, and drag a library by its handle to
 * reorder them. Dragging works from the keyboard and a TV remote too.
 * Press Enter on the handle, use the arrows, then press Enter to drop.
 */
export function LibraryOrder({ libraries, onChange }: Props) {
  const { t } = useLingui();
  const { dragAndDropHooks } = useDragAndDrop({
    getItems: (keys) => [...keys].map((key) => ({ "text/plain": libraries.find((l) => l.id === key)?.name ?? "" })),
    onReorder: ({ keys, target }) => {
      const moving = libraries.filter((l) => keys.has(l.id));
      const rest = libraries.filter((l) => !keys.has(l.id));
      const at = rest.findIndex((l) => l.id === target.key) + (target.dropPosition === "after" ? 1 : 0);
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
      data-part="library-order"
      aria-label={t`Libraries`}
      items={libraries}
      dependencies={[libraries]}
      dragAndDropHooks={dragAndDropHooks}
      className={settingsCardClass}
    >
      {(library) => {
        const name = library.name;
        return (
          <GridListItem
            id={library.id}
            textValue={name}
            className="flex items-center gap-1 px-2 py-1.5 outline-none data-dragging:opacity-50 data-focus-visible:bg-surface-hover"
          >
            <Button
              slot="drag"
              className="flex size-8 shrink-0 cursor-grab items-center justify-center rounded-md text-text-muted hover:bg-surface-hover hover:text-text"
            >
              <Icon name="grip" className="size-4" />
            </Button>
            <Checkbox
              isSelected={library.pinned}
              onChange={(pinned) => onChange(libraries.map((l) => (l.id === library.id ? { ...l, pinned } : l)))}
              aria-label={t`Pinned: ${name}`}
              className="group flex min-w-0 flex-1 cursor-pointer items-center gap-3 px-1 py-1 text-text"
            >
              <CheckBox />
              <LibraryKindIcon kind={library.kind} className="size-5 shrink-0 text-text-muted" />
              <span className="truncate font-medium">{name}</span>
            </Checkbox>
          </GridListItem>
        );
      }}
    </GridList>
  );
}

/**
 * Your libraries as a switch each, for TV: whether each is pinned, which
 * the rail and Home show. Putting them in order is for the web, since
 * dragging needs a pointer.
 */
export function LibraryPins({ libraries, onChange }: Props) {
  const { t } = useLingui();
  return (
    <div data-part="library-pins" className={settingsCardClass}>
      {libraries.map((library) => (
        <SettingsRow key={library.id} label={library.name}>
          <Switch
            aria-label={t`Pinned: ${library.name}`}
            isSelected={library.pinned}
            onChange={(pinned) => onChange(libraries.map((l) => (l.id === library.id ? { ...l, pinned } : l)))}
          />
        </SettingsRow>
      ))}
    </div>
  );
}
