import { Children, type ReactNode, type RefObject, useState } from "react";
import {
  Menu as AriaMenu,
  MenuItem as AriaMenuItem,
  type MenuItemProps,
  MenuTrigger,
  Separator,
} from "react-aria-components";
import { useTvDialog } from "@/tv/forms/dialog";
import { Popover } from "./Popover";

const menuWidth = "min-w-56";
const menuList = "max-h-80 overflow-auto outline-none";

type Props = {
  /** The trigger, a button that names the menu, then the menu's items. */
  children: ReactNode;
  onAction?: (key: string | number) => void;
};

/**
 * A button that opens a menu, named by the button. Menus add no history
 * entry. Escape closes them, and so does a TV remote's Back.
 */
export function Menu({ children, onAction }: Props) {
  const [trigger, ...items] = Children.toArray(children);
  const [open, setOpen] = useState(false);
  // Back closes it on TV, and focus returns to the trigger's place.
  useTvDialog(open, () => setOpen(false));
  return (
    <MenuTrigger isOpen={open} onOpenChange={setOpen}>
      {trigger}
      <Popover className={menuWidth}>
        <AriaMenu className={menuList} onAction={onAction}>
          {items}
        </AriaMenu>
      </Popover>
    </MenuTrigger>
  );
}

/**
 * A menu opened from elsewhere than one trigger button, such as a card's ⋯
 * or a right-click on the card, shown beside `triggerRef`.
 */
export function AnchoredMenu({
  label,
  triggerRef,
  isOpen,
  onOpenChange,
  children,
}: {
  label: string;
  triggerRef: RefObject<Element | null>;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
}) {
  return (
    <Popover
      triggerRef={triggerRef}
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      placement="bottom end"
      className={menuWidth}
    >
      <AriaMenu aria-label={label} className={menuList} autoFocus="first" onClose={() => onOpenChange(false)}>
        {children}
      </AriaMenu>
    </Popover>
  );
}

/** A line between groups of a menu's items. */
export function MenuSeparator() {
  return <Separator className="my-1 border-t border-border" />;
}

/** A menu's item. Use `danger` for one that deletes, such as Delete library. */
export function MenuItem({
  tone = "default",
  ...props
}: Omit<MenuItemProps, "className"> & { tone?: "default" | "danger" }) {
  return (
    <AriaMenuItem
      {...props}
      className={`flex cursor-pointer items-center gap-2 rounded-sm px-3 py-2 text-sm outline-none data-focused:bg-surface-hover data-disabled:opacity-50 ${tone === "danger" ? "text-danger" : "text-text"}`}
    />
  );
}
