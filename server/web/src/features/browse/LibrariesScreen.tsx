import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { libraryQueries } from "@/api/libraries";
import type { Library } from "@/api/schemas";
import { useDocumentTitle } from "@/components/documentTitle";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { settingsCardClass } from "@/components/SettingsLayout";
import { TvFocusGroup } from "@/tv/focus";
import { LibraryRow } from "./LibraryRow";

/**
 * The Libraries tab on phones: Lists, then the pinned libraries in the
 * account's order, then More libraries with the rest.
 * A library's row has its icon, name and item count. Each row opens its page.
 */
export function LibrariesScreen() {
  const { t } = useLingui();
  useDocumentTitle(t`Libraries`);
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
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
        <ul aria-label={t`Lists`} className={settingsCardClass}>
          <LibraryRow
            link={{ to: "/lists" }}
            icon={<Icon name="list" className="size-5" />}
            name={t`Lists`}
            focusKey="lists"
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
