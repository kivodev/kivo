import { Icon } from "./Icon";

/**
 * The box a checkbox draws, filled when it's selected. It goes inside any
 * Checkbox, whose class has `group`.
 */
export function CheckBox() {
  return (
    <span className="flex size-5 shrink-0 items-center justify-center rounded-sm border border-border text-on-accent group-data-selected:border-accent group-data-selected:bg-accent">
      <Icon name="check" className="size-3.5 opacity-0 group-data-selected:opacity-100" />
    </span>
  );
}
