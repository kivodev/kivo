import { useLingui } from "@lingui/react/macro";
import { Link, type LinkProps } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { Icon } from "@/components/Icon";
import { TvFocusable } from "@/tv/focus";

/** A row of a settings card that opens a place to browse: its icon, its name and how many items it has. */
export function LibraryRow({
  link,
  icon,
  name,
  count,
  detail,
  focusKey,
  autoFocus = false,
}: {
  link: LinkProps;
  icon: ReactNode;
  name: string;
  /** How many items it has, once known. */
  count?: number;
  /** A line under the name, such as what a smart list shows. */
  detail?: string;
  /** The row's focus key on TV. */
  focusKey: string;
  /** Whether it is the screen's initial focus on TV. */
  autoFocus?: boolean;
}) {
  const { i18n } = useLingui();
  return (
    <li className="first:*:rounded-t-lg last:*:rounded-b-lg">
      <TvFocusable focusKey={focusKey} autoFocus={autoFocus} kind="row">
        <Link
          {...link}
          data-part="library-row"
          className="flex min-h-12 items-center gap-3 px-4 py-3 text-text hover:bg-surface-hover"
        >
          <span className="shrink-0 text-text-muted">{icon}</span>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate font-medium">{name}</span>
            {detail && <span className="truncate text-sm text-text-muted">{detail}</span>}
          </span>
          {count !== undefined && <span className="text-text-muted">{i18n.number(count)}</span>}
          <Icon name="chevronRight" className="size-4 shrink-0 text-text-subtle" />
        </Link>
      </TvFocusable>
    </li>
  );
}
