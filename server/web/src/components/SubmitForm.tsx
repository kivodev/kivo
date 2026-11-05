import { Trans } from "@lingui/react/macro";
import type { ReactNode } from "react";
import { Form, FormContext } from "react-aria-components";
import { TvControl } from "@/tv/forms/TvControl";
import { Button } from "./Button";
import { FormError } from "./FormError";

type Props = {
  /** Sends the form with the values of its named fields. Enter in a field submits too. */
  onSubmit: (data: FormData) => void;
  submitLabel: ReactNode;
  /** `danger` for a submit that can't be undone, such as stopping someone's play. */
  tone?: "default" | "danger";
  isPending?: boolean;
  /** Something stops the submit, such as a required choice not made yet. */
  isSubmitDisabled?: boolean;
  /** Cancel, beside the submit button. */
  onCancel?: () => void;
  /** The problem that made the last submit fail. Nothing shows while it's null. */
  error?: unknown;
  /** Words to show instead of the error's own. */
  errorMessage?: ReactNode;
  /**
   * How the form ends. "dialog" ends with Cancel and the submit button on
   * the right. "panel" ends with a full-width submit button.
   */
  layout?: "dialog" | "panel";
  /** The submit button's focus key on TV, for a screen that moves focus to it. */
  submitFocusKey?: string;
  className?: string;
  children: ReactNode;
};

/**
 * A form sent once with its submit button. It shows its fields, why the
 * last submit failed, then Cancel and the submit button. Dialog forms and
 * the sign-in panels' forms take this shape.
 */
export function SubmitForm({
  onSubmit,
  submitLabel,
  tone = "default",
  isPending = false,
  isSubmitDisabled = false,
  onCancel,
  error,
  errorMessage,
  layout = "dialog",
  submitFocusKey,
  className = "flex flex-col gap-4",
  children,
}: Props) {
  const submit = (
    <TvControl focusKey={submitFocusKey}>
      <Button
        type="submit"
        variant={tone === "danger" ? "danger" : "primary"}
        isDisabled={isSubmitDisabled}
        isPending={isPending}
        // On TV the button is inside its focus stop, so it's stretched
        // itself, and it's as tall as the fields above it.
        className={layout === "panel" ? "mt-2 w-full tv:h-12" : undefined}
      >
        {submitLabel}
      </Button>
    </TvControl>
  );
  return (
    // A dialog's form opened from inside another form stands alone. It
    // takes none of the outer form's props, such as its submit, and its own
    // submit doesn't reach the outer form through React's tree.
    <FormContext.Provider value={null}>
      <Form
        className={className}
        onSubmit={(e) => {
          e.preventDefault();
          e.stopPropagation();
          // Enter in a field submits as well as the button, which is pending.
          if (isPending) return;
          onSubmit(new FormData(e.currentTarget));
        }}
      >
        {children}
        <FormError error={error}>{errorMessage}</FormError>
        {layout === "panel" ? (
          submit
        ) : (
          <div className="flex justify-end gap-2">
            {onCancel && (
              <TvControl>
                <Button onPress={onCancel}>
                  <Trans>Cancel</Trans>
                </Button>
              </TvControl>
            )}
            {submit}
          </div>
        )}
      </Form>
    </FormContext.Provider>
  );
}

/** A named field's text, trimmed. */
export function fieldText(data: FormData, name: string): string {
  return String(data.get(name) ?? "").trim();
}

/** Names one per line, in order, as written. */
export function linesOf(text: string): string[] {
  return text
    .split("\n")
    .map((s) => s.trim())
    .filter((s) => s !== "");
}

/** Items separated by commas, in order, as written. */
export function listOf(text: string): string[] {
  return text
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s !== "");
}
