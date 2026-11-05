import { useLingui } from "@lingui/react/macro";
import { Button as AriaButton } from "react-aria-components";
import { Icon } from "./Icon";

/** An active filter, removable with one press. */
export function Chip({ label, onRemove }: { label: string; onRemove: () => void }) {
  const { t } = useLingui();
  return (
    <span
      data-part="filter-chip"
      className="inline-flex h-8 items-center gap-1 rounded-full border border-border bg-surface pr-1 pl-3 text-sm text-text"
    >
      {label}
      <AriaButton
        aria-label={t`Remove filter ${label}`}
        onPress={onRemove}
        className="flex size-6 cursor-pointer items-center justify-center rounded-full text-text-muted touch-target hover:bg-surface-hover hover:text-text"
      >
        <Icon name="close" className="size-3.5" />
      </AriaButton>
    </span>
  );
}
