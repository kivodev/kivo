import { Popover as AriaPopover, type PopoverProps as AriaPopoverProps } from "react-aria-components";

type Props = Omit<AriaPopoverProps, "className"> & {
  /** The panel's width, such as `min-w-56`. */
  className?: string;
};

/**
 * The panel of a dropdown or a menu, raised over the page with a border and
 * a shadow. It scrolls when the window leaves it less room than it needs.
 */
export function Popover({ className = "", ...props }: Props) {
  return (
    <AriaPopover
      {...props}
      className={`overflow-auto rounded-md border border-border bg-surface-raised p-1 shadow-overlay outline-none ${className}`}
    />
  );
}
