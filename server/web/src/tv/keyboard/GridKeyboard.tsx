// The on-screen grid keyboard for the TV build: letters and digits
// in six columns, then Space, Backspace and Clear. Arrows move over it
// and OK types, so it works with any remote, beside the platform's own
// keyboard.

import { Trans, useLingui } from "@lingui/react/macro";
import { memo, type ReactNode } from "react";
import { Button } from "@/components/Button";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";

const columns = 6;
const letters = "abcdefghijklmnopqrstuvwxyz0123456789";

type Props = {
  /** A prefix for the keys' focus keys, unique on the screen. */
  name: string;
  /** Types a character. */
  onType: (character: string) => void;
  onBackspace: () => void;
  onClear: () => void;
  /** The key that takes focus as the screen opens. */
  autoFocus?: boolean;
  /** Right from the last column. Return false to keep focus where it is. */
  onRight?: () => boolean;
  className?: string;
};

const Key = memo(function Key({
  focusKey,
  span = 1,
  autoFocus,
  label,
  onPress,
  value = "",
  onRight,
  edge,
  children,
}: {
  focusKey: string;
  span?: 1 | 2 | 3;
  autoFocus?: boolean;
  label?: string;
  onPress: (value: string) => void;
  value?: string;
  onRight?: () => boolean;
  edge: boolean;
  children: ReactNode;
}) {
  const press = () => onPress(value);
  return (
    <TvFocusable
      focusKey={focusKey}
      autoFocus={autoFocus}
      part="tv-key"
      className={span === 3 ? "col-span-3" : span === 2 ? "col-span-2" : ""}
      onPress={press}
      onArrow={(direction) => (direction === "right" && edge && onRight ? onRight() : true)}
    >
      <Button aria-label={label} className="h-11 w-full text-lg" onPress={press}>
        {children}
      </Button>
    </TvFocusable>
  );
});

/**
 * Typing is the keys' job alone: each one calls back with what it types,
 * and the screen owns the text. The keys are one group that remembers the
 * key focus left from.
 */
export const GridKeyboard = memo(function GridKeyboard({
  name,
  onType,
  onBackspace,
  onClear,
  autoFocus = false,
  onRight,
  className = "",
}: Props) {
  const { t } = useLingui();
  return (
    <TvFocusGroup focusKey={`${name}:keyboard`} part="tv-keyboard" label={t`Keyboard`} className={className}>
      <div className="grid grid-cols-6 gap-2">
        {[...letters].map((c, i) => (
          <Key
            key={c}
            focusKey={`${name}:key:letters:${c}`}
            autoFocus={autoFocus && i === 0}
            edge={i % columns === columns - 1}
            onRight={onRight}
            onPress={onType}
            value={c}
          >
            {c}
          </Key>
        ))}
        <Key focusKey={`${name}:space`} span={2} edge={false} label={t`Space`} onPress={() => onType(" ")}>
          <span aria-hidden>␣</span>
        </Key>
        <Key focusKey={`${name}:backspace`} span={2} edge={false} label={t`Backspace`} onPress={onBackspace}>
          <span aria-hidden>⌫</span>
        </Key>
        <Key focusKey={`${name}:clear`} span={2} edge onRight={onRight} onPress={onClear}>
          <Trans>Clear</Trans>
        </Key>
      </div>
    </TvFocusGroup>
  );
});
