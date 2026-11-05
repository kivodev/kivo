import { useState } from "react";
import type { Key, PopoverProps } from "react-aria-components";
import {
  Button as AriaButton,
  Select as AriaSelect,
  Label,
  ListBox,
  ListBoxItem,
  SelectValue,
  Text,
} from "react-aria-components";
import { useTvBack } from "@/tv/back";
import { FieldLabel, type FieldVariant } from "./FieldLabel";
import { Icon } from "./Icon";
import { Popover } from "./Popover";
import { useRowAria } from "./SettingsLayout";

/** A choice in a select or a player menu. */
export type Option = {
  id: string;
  label: string;
  /** A second line in the open list, such as "HEVC · 58 GB". The closed select shows only the label. */
  detail?: string;
  /** Listed, but can't be chosen now. Its detail says why. */
  disabled?: boolean;
};

/**
 * A select's look:
 * - `settings`, the default: a settings row's select, as wide as its longest option;
 * - `form`: a form field under its label, for dialogs;
 * - `inline`: its label, then the choice in bold, like the detail page's audio picker;
 * - `compact`: a word or two in running text, like the browse page's sort.
 */
type Variant = FieldVariant | "inline" | "compact";

const looks: Record<Variant, { select: string; trigger: string; chevron: string; popover: string }> = {
  settings: {
    select: "max-w-full min-w-0",
    trigger:
      "inline-flex h-9 max-w-full items-center gap-2 rounded-md border border-border bg-surface-raised px-3 text-sm text-text hover:bg-surface-hover",
    chevron: "text-text-muted",
    popover: "min-w-(--trigger-width)",
  },
  form: {
    select: "flex flex-col gap-1.5",
    trigger:
      "inline-flex h-10 items-center justify-between gap-1 rounded-md border border-border bg-surface px-3 text-text outline-none data-focus-visible:border-accent",
    chevron: "",
    popover: "min-w-(--trigger-width)",
  },
  inline: {
    // On TV the value lights up and grows when focused, so it keeps clear of its label.
    select: "flex flex-wrap items-center gap-x-2 tv:gap-x-4",
    trigger:
      "inline-flex h-9 items-center gap-1 rounded-md px-2 text-sm font-semibold text-text hover:bg-surface-hover",
    chevron: "",
    popover: "min-w-(--trigger-width)",
  },
  compact: {
    select: "inline-flex",
    trigger:
      "-mx-1 inline-flex items-center gap-0.5 rounded-sm px-1 font-semibold touch-target text-text-muted hover:text-text",
    chevron: "",
    popover: "min-w-48",
  },
};

const placements: Partial<Record<Variant, PopoverProps["placement"]>> = {
  settings: "bottom end",
  compact: "bottom start",
};

type Props<T extends string> = {
  options: readonly (Option & { id: T })[];
  selected: T;
  onChange: (id: T) => void;
  variant?: Variant;
  /**
   * The select's name. It shows above a form select and beside an inline
   * one. On a compact or settings select it is only read aloud. A settings
   * select without a label is named by its row's label and described by
   * its help.
   */
  label?: string;
  /** A line under it, such as "Remembered for this show". */
  description?: string;
  /** Words a compact select shows instead of the chosen option's label, such as "A–Z" for Title. */
  value?: string;
  isDisabled?: boolean;
  /** An admin edited this field, so refreshes keep it. A form select shows a lock. */
  locked?: boolean;
  part?: string;
};

/** A choice among fixed options, shown as a button that opens the list of them. */
export function Select<T extends string>({
  options,
  selected,
  onChange,
  variant = "settings",
  label,
  description,
  value,
  isDisabled = false,
  locked = false,
  part,
}: Props<T>) {
  const look = looks[variant];
  const settings = variant === "settings";
  const row = useRowAria(label !== undefined);
  // A remote's Back key isn't Escape, so on TV Back closes the open list.
  const [open, setOpen] = useState(false);
  useTvBack(() => {
    setOpen(false);
    return true;
  }, open);
  // The TV's type is large, so its fields are taller. Settings' choices
  // are one width there, so a card's controls line up down its right edge.
  const tvHeight =
    variant === "settings" ? "tv:h-12 tv:w-52 tv:justify-between tv:px-5" : variant === "form" ? "tv:h-12 tv:px-5" : "";
  return (
    <AriaSelect
      isOpen={open}
      onOpenChange={setOpen}
      data-part={part}
      aria-label={settings ? label : undefined}
      aria-labelledby={settings ? row["aria-labelledby"] : undefined}
      aria-describedby={settings ? row["aria-describedby"] : undefined}
      selectedKey={selected}
      // The keys are the options' IDs.
      onSelectionChange={(key: Key | null) => key !== null && onChange(String(key) as T)}
      isDisabled={isDisabled}
      className={`${look.select} data-disabled:opacity-50`}
    >
      {variant === "form" && label !== undefined && <FieldLabel label={label} locked={locked} />}
      {variant === "inline" && <Label className="text-sm text-text-muted">{label}</Label>}
      {variant === "compact" && <Label className="sr-only">{label}</Label>}
      <AriaButton
        // A compact select sits in a line of text, and an inline one beside
        // its label, which take focus with it on TV; the button alone lights
        // up (TvFocusable's face). A select's first child is a hidden part of
        // its own, so a wrapper can't light that.
        data-tv-face={variant === "compact" || variant === "inline" ? "" : undefined}
        className={`${look.trigger} ${tvHeight} cursor-pointer data-disabled:cursor-default`}
      >
        {settings ? (
          // Every option's label goes in one grid cell, with all but the
          // chosen one invisible. This sizes the button to the longest label.
          <span className="grid min-w-0 text-left">
            <SelectValue className="col-start-1 row-start-1 truncate">{({ selectedText }) => selectedText}</SelectValue>
            {options.map((o) => (
              <span key={o.id} aria-hidden className="invisible col-start-1 row-start-1 truncate">
                {o.label}
              </span>
            ))}
          </span>
        ) : (
          // The trigger names the choice. Its second line shows only in the
          // open list.
          <SelectValue className="truncate">{({ selectedText }) => value ?? selectedText}</SelectValue>
        )}
        <Icon name="chevronDown" className={`size-4 shrink-0 ${look.chevron}`} />
      </AriaButton>
      {description && (
        <Text slot="description" className="w-full text-xs text-text-muted">
          {description}
        </Text>
      )}
      <Popover placement={placements[variant]} className={look.popover}>
        <ListBox className="max-h-80 overflow-auto outline-none">
          {options.map((o) => (
            <ListBoxItem
              key={o.id}
              id={o.id}
              textValue={o.label}
              isDisabled={o.disabled ?? false}
              className="flex cursor-pointer flex-col rounded-sm px-3 py-2 text-sm text-text outline-none data-disabled:cursor-default data-disabled:opacity-50 data-focused:bg-surface-hover data-selected:font-semibold"
            >
              <Text slot="label">{o.label}</Text>
              {o.detail && (
                <Text slot="description" className="line-clamp-2 text-xs font-normal text-text-muted">
                  {o.detail}
                </Text>
              )}
            </ListBoxItem>
          ))}
        </ListBox>
      </Popover>
    </AriaSelect>
  );
}
