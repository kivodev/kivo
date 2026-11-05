import { Trans } from "@lingui/react/macro";
import { createContext, type ReactNode, useContext, useId } from "react";
import { isTv } from "@/tv/flag";
import { TvControl } from "@/tv/forms/TvControl";
import { Button } from "./Button";
import { FormError } from "./FormError";
import { Icon } from "./Icon";

/**
 * A settings page's sections, one under another. The layout follows the
 * page's own width, not the window's, because the sidebars beside it take
 * a varying share.
 */
export function SettingsLayout({ children }: { children: ReactNode }) {
  return (
    <div className="@container/settings flex max-w-(--layout-settings-width) min-w-0 flex-col gap-12">{children}</div>
  );
}

type SectionProps = {
  /** The section's anchor. */
  id: string;
  /** Absent for a card that speaks for itself, such as a red card of account actions. */
  title?: ReactNode;
  /** One short line under the title, only where it adds to it, such as where the settings apply. */
  description?: ReactNode;
  children: ReactNode;
};

/**
 * A titled group of settings: the title and its description, then the
 * cards. When the page is wide enough they sit side by side, with the
 * title in a column on the left.
 */
export function SettingsSection({ id, title, description, children }: SectionProps) {
  const titleId = `${id}-title`;
  return (
    <section
      id={id}
      aria-labelledby={title ? titleId : undefined}
      className="flex scroll-mt-6 flex-col gap-4 @settings-split/settings:grid @settings-split/settings:grid-cols-(--layout-settings-section-grid) @settings-split/settings:gap-10"
    >
      <header className="flex flex-col gap-1">
        {title && (
          <h2 id={titleId} className="text-lg font-semibold text-text tv:text-xl">
            {title}
          </h2>
        )}
        {description && <p className="text-sm text-text-muted">{description}</p>}
      </header>
      <div className="flex min-w-0 flex-col gap-8">{children}</div>
    </section>
  );
}

type CardProps = {
  /** A heading over the card, for a group within a section. */
  title?: ReactNode;
  /**
   * Where the card's settings apply: to the account on every device, or
   * to this browser alone. A browser says so over the card. A TV marks
   * only the rows kept on it, This TV, beside their labels.
   */
  scope?: "account" | "device";
  /** A note under the card about the whole group. */
  footer?: ReactNode;
  /** Which card, for tests. */
  part?: string;
  /** `danger` outlines the card in red, for actions that can't be undone. */
  tone?: "default" | "danger";
  children: ReactNode;
};

/** The look of a settings card: rounded, with a line between its rows. Also for a list that looks like a card. */
export const settingsCardClass = "divide-y divide-border rounded-lg border border-border bg-surface";

/** Whether the rows inside are kept on this device, for a TV to mark them. */
const DeviceRows = createContext(false);

/** A rounded card of settings rows, with a line between each. */
export function SettingsCard({ title, scope, footer, part, tone = "default", children }: CardProps) {
  const heading =
    title ??
    (isTv ? undefined : scope === "account" ? (
      <Trans>All your devices</Trans>
    ) : scope === "device" ? (
      <Trans>This browser</Trans>
    ) : undefined);
  return (
    <div data-part={part} className="flex flex-col gap-2">
      {heading && <h3 className="text-sm font-medium text-text-muted">{heading}</h3>}
      <div className={tone === "danger" ? `${settingsCardClass} border-danger` : settingsCardClass}>
        <DeviceRows value={scope === "device"}>{children}</DeviceRows>
      </div>
      {footer && <p className="text-sm text-text-muted">{footer}</p>}
    </div>
  );
}

type RowLabels = {
  labelId: string;
  descriptionId: string | undefined;
  /** The row's error, while it has one. */
  errorId: string | undefined;
};

const RowContext = createContext<RowLabels | undefined>(undefined);

/**
 * The ARIA props a control takes from the settings row around it. The
 * control is named by the row's label unless it has a name of its own. It
 * is described by the row's help and error, and is invalid while the row
 * has an error. Outside a row there are no props.
 */
export function useRowAria(named: boolean) {
  const row = useContext(RowContext);
  return {
    "aria-labelledby": named ? undefined : row?.labelId,
    "aria-describedby": [row?.descriptionId, row?.errorId].filter(Boolean).join(" ") || undefined,
    // Undefined rather than false when there's no error, so the field
    // still runs its own validation.
    isInvalid: row?.errorId ? true : undefined,
  };
}

type RowProps = {
  label: ReactNode;
  /** A muted line under the label. */
  description?: ReactNode;
  /** Shows a spinner beside the control while a change is being saved. */
  isPending?: boolean;
  /**
   * What's wrong, shown under the label. It is either why the last change
   * wasn't saved, with Retry when there's onRetry, or what the field
   * accepts.
   */
  error?: string;
  onRetry?: () => void;
  /** The control, on the right, or under the label when it doesn't fit beside it. */
  children?: ReactNode;
  /** Whether the remote can focus the control on TV. A row that only shows a value says no. */
  focusable?: boolean;
};

/**
 * One setting, with its label and any help on the left and its control on
 * the right. A control inside takes its name from the label (see
 * useRowAria).
 */
export function SettingsRow({
  label,
  description,
  isPending = false,
  error,
  onRetry,
  children,
  focusable = true,
}: RowProps) {
  const id = useId();
  const onDevice = useContext(DeviceRows);
  const labels = {
    labelId: `${id}-label`,
    descriptionId: description ? `${id}-description` : undefined,
    errorId: error ? `${id}-error` : undefined,
  };
  const line = (
    <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
      <div className="flex min-w-0 grow basis-32 flex-col gap-0.5">
        <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span id={labels.labelId} className="font-medium text-text">
            {label}
          </span>
          {isTv && onDevice && (
            <span
              data-part="this-tv"
              className="rounded-full border border-border bg-surface-raised px-3 text-xs text-text-muted"
            >
              <Trans>This TV</Trans>
            </span>
          )}
        </span>
        {description && (
          <span id={labels.descriptionId} className="text-sm text-text-muted">
            {description}
          </span>
        )}
      </div>
      {children && (
        <div className="flex max-w-full min-w-0 items-center gap-3">
          {isPending && <Icon name="spinner" className="size-4 shrink-0 animate-spin text-text-muted" />}
          <RowContext value={labels}>{children}</RowContext>
        </div>
      )}
    </div>
  );
  return (
    <div aria-busy={isPending} className="flex flex-col gap-2 px-4 py-3.5">
      {children && focusable ? (
        // On TV the whole line takes focus and lights up to the row's
        // edges, and OK presses its control.
        <TvControl focusKey={`setting:${id}`} kind="row" className="-mx-4 -my-3.5 px-4 py-3.5">
          {line}
        </TvControl>
      ) : (
        line
      )}
      {error && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <FormError id={labels.errorId}>{error}</FormError>
          {onRetry && (
            <TvControl focusKey={`setting:${id}:retry`}>
              <Button size="sm" variant="ghost" onPress={onRetry}>
                <Trans>Retry</Trans>
              </Button>
            </TvControl>
          )}
        </div>
      )}
    </div>
  );
}
