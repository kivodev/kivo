import { useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
// biome-ignore lint/style/noRestrictedImports: the settings pages' form, with its save bar.
import { Form } from "react-aria-components";
import { isProblem } from "@/api/problem";
import type { SettingsUpdate } from "@/api/schemas";
import { settingsQueries, useUpdateSettings } from "@/api/settings";
import { SaveBar } from "@/components/SaveBar";
import { changes } from "@/lib/draft";

/** A settings page's save, and the message for a field the server refused. */
export function useSettingsSave() {
  const { t } = useLingui();
  const update = useUpdateSettings();
  const errors = isProblem(update.error) ? (update.error.errors ?? []) : [];
  return {
    update,
    /**
     * The error for a field's row, by its JSON pointer, when the server
     * refused the field. It is `hint`, which says what the field accepts,
     * or else a general error.
     */
    invalid: (field: string, hint?: string): string | undefined =>
      errors.some((e) => e.field === field) ? (hint ?? t`This value isn't valid.`) : undefined,
  };
}

type Props = {
  save: ReturnType<typeof useSettingsSave>;
  /** The settings this page changes, as the form has them. */
  body: SettingsUpdate;
  /** Puts the form back to the saved settings. */
  onDiscard: () => void;
  /** Called after the server saved the settings. */
  onSaved?: () => void;
  children: ReactNode;
};

/** A server settings page's form, whose Save sends every setting the page has. */
export function SettingsForm({ save: { update }, body, onDiscard, onSaved, children }: Props) {
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  return (
    <SaveForm
      isDirty={Object.keys(changes(settings, body)).length > 0}
      isPending={update.isPending}
      error={update.error}
      onSave={() => update.mutate({ body }, { onSuccess: () => onSaved?.() })}
      onDiscard={() => {
        update.reset();
        onDiscard();
      }}
    >
      {children}
    </SaveForm>
  );
}

type SaveFormProps = {
  /** Whether the fields differ from what's saved. The save bar shows only then. */
  isDirty: boolean;
  isPending: boolean;
  /** Something stops Save, such as a required choice not made yet. */
  isSaveDisabled?: boolean;
  /** Why the last save failed. It is null when a field's row says why instead. */
  error: unknown;
  onSave: () => void;
  onDiscard: () => void;
  children: ReactNode;
};

/**
 * A settings page's form. It shows its fields, then the save bar while they
 * differ from what's saved. Enter in a field saves too.
 */
export function SaveForm({
  isDirty,
  isPending,
  isSaveDisabled = false,
  error,
  onSave,
  onDiscard,
  children,
}: SaveFormProps) {
  return (
    <Form
      onSubmit={(e) => {
        e.preventDefault();
        if (isDirty && !isSaveDisabled && !isPending) onSave();
      }}
    >
      {children}
      <SaveBar
        isDirty={isDirty}
        isPending={isPending}
        isSaveDisabled={isSaveDisabled}
        error={error}
        onDiscard={onDiscard}
      />
    </Form>
  );
}
