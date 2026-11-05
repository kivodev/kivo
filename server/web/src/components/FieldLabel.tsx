import { useLingui } from "@lingui/react/macro";
import { FieldError, Label, Text } from "react-aria-components";
import { Icon } from "./Icon";

/** A form field's label, with a lock when an admin's edit keeps the field through refreshes. */
export function FieldLabel({ label, locked = false }: { label: string; locked?: boolean }) {
  const { t } = useLingui();
  return (
    <Label
      aria-label={locked ? t`${label} (locked)` : undefined}
      className="flex items-center gap-1.5 text-sm font-medium text-text"
    >
      {label}
      {locked && <Icon name="lock" className="size-3.5 text-text-muted" />}
    </Label>
  );
}

/**
 * A field's look. It is a settings row's look by default. `form` is taller
 * and sits on the page's surface, for sign-in, setup and dialogs.
 */
export type FieldVariant = "settings" | "form";

/** A text or number input's classes in a field of that look. A single-line input also takes `fieldHeight`. */
export function fieldInputClass(variant: FieldVariant): string {
  const look = variant === "form" ? "bg-surface" : "w-full bg-surface-raised text-sm pointer-coarse:text-base";
  return `rounded-md border border-border px-3 text-text outline-none placeholder:text-text-subtle data-disabled:opacity-50 data-focused:border-accent data-invalid:border-danger ${look}`;
}

export const fieldHeight: Record<FieldVariant, string> = { settings: "h-9", form: "h-10" };

/**
 * A field's help and its error, shown under the field. A settings row
 * shows its own error, so a field in a row shows only the field's own
 * error.
 */
export function FieldNotes({
  variant,
  description,
  errorMessage,
}: {
  variant: FieldVariant;
  description?: string;
  errorMessage?: string;
}) {
  return (
    <>
      {description && (
        <Text slot="description" className="text-xs text-text-muted">
          {description}
        </Text>
      )}
      {(variant === "form" || errorMessage) && <FieldError className="text-xs text-danger">{errorMessage}</FieldError>}
    </>
  );
}
