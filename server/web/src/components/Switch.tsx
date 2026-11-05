import { Switch as AriaSwitch } from "react-aria-components";
import { useRowAria } from "./SettingsLayout";

type Props = {
  isSelected: boolean;
  onChange: (selected: boolean) => void;
  isDisabled?: boolean;
  /** The switch's name, for when no settings row or visible label names it. */
  "aria-label"?: string;
  "aria-labelledby"?: string;
};

/**
 * An on/off setting that applies as soon as it's flipped. In a settings
 * row it's named by the row's label and described by its help.
 */
export function Switch({ isSelected, onChange, isDisabled = false, ...aria }: Props) {
  const row = useRowAria(aria["aria-label"] !== undefined || aria["aria-labelledby"] !== undefined);
  // On TV, seen from across the room, the switch is half as big again.
  return (
    <AriaSwitch
      aria-labelledby={row["aria-labelledby"]}
      aria-describedby={row["aria-describedby"]}
      {...aria}
      isSelected={isSelected}
      onChange={onChange}
      isDisabled={isDisabled}
      className="group inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border border-border bg-surface-hover p-0.5 transition-colors data-disabled:cursor-default data-disabled:opacity-50 data-focus-visible:outline-2 data-focus-visible:outline-offset-2 data-focus-visible:outline-focus data-focus-visible:outline-solid data-selected:border-accent data-selected:bg-accent tv:h-10 tv:w-20"
    >
      <span className="size-4.5 rounded-full bg-text-muted shadow-card transition-transform group-data-selected:translate-x-5 group-data-selected:bg-on-accent tv:size-8 tv:group-data-selected:translate-x-10" />
    </AriaSwitch>
  );
}
