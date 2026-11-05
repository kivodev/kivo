import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link, type LinkProps } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { libraryQueries } from "@/api/libraries";
import type { Library } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { useDocumentTitle } from "@/components/documentTitle";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { settingsCardClass } from "@/components/SettingsLayout";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";

/**
 * The Libraries tab on phones. It lists Favorites, then the pinned
 * libraries in the account's order, then More libraries with the rest.
 * Each row has its icon, name and item count, and opens its page.
 */
export function LibrariesScreen() {
  const { t } = useLingui();
  useDocumentTitle(t`Libraries`);
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const favorites = useQuery(workQueries.favoritesCount()).data;
  const pinned = libraries.filter((l) => l.pinned);
  const others = libraries.filter((l) => !l.pinned);
  const row = (library: Library) => (
    <LibraryRow
      key={library.id}
      link={{ to: "/libraries/$libraryId", params: { libraryId: library.id } }}
      icon={<LibraryKindIcon kind={library.kind} className="size-5" />}
      name={library.name}
      count={library.workCount}
      focusKey={`library:${library.id}`}
    />
  );
  return (
    <TvFocusGroup focusKey="libraries">
      <div data-part="libraries" className="flex flex-col gap-6 px-4 py-6 sm:px-8">
        <h1 className="text-2xl font-semibold">
          <Trans>Libraries</Trans>
        </h1>
        <ul aria-label={t`Favorites`} className={settingsCardClass}>
          <LibraryRow
            link={{ to: "/favorites" }}
            icon={<Icon name="heart" className="size-5" />}
            name={t`Favorites`}
            count={favorites}
            focusKey="favorites"
            autoFocus
          />
        </ul>
        {pinned.length > 0 && (
          <ul aria-label={t`Pinned libraries`} className={settingsCardClass}>
            {pinned.map(row)}
          </ul>
        )}
        {others.length > 0 && (
          <section aria-labelledby="more-libraries" className="flex flex-col gap-2">
            <h2 id="more-libraries" className="text-sm font-medium text-text-muted">
              <Trans>More libraries</Trans>
            </h2>
            <ul className={settingsCardClass}>{others.map(row)}</ul>
          </section>
        )}
      </div>
    </TvFocusGroup>
  );
}

function LibraryRow({
  link,
  icon,
  name,
  count,
  focusKey,
  autoFocus = false,
}: {
  link: LinkProps;
  icon: ReactNode;
  name: string;
  /** How many items it has, once known. */
  count: number | undefined;
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
          <span className="min-w-0 flex-1 truncate font-medium">{name}</span>
          {count !== undefined && <span className="text-text-muted">{i18n.number(count)}</span>}
          <Icon name="chevronRight" className="size-4 shrink-0 text-text-subtle" />
        </Link>
      </TvFocusable>
    </li>
  );
}
