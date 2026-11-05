import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Button as AriaButton, Header, MenuSection } from "react-aria-components";
import { libraryQueries } from "@/api/libraries";
import type { Library } from "@/api/schemas";
import { buttonClass } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { Menu, MenuItem } from "@/components/Menu";
import { usePhoneLayout } from "@/components/phoneLayout";
import { isTv } from "@/tv/flag";

/** Favorites' key in the menu, which no library ID can be. */
export const favoritesKey = "favorites";

type Props = {
  /** The page's title on the button. */
  title: string;
  /** The open library's ID, or favoritesKey. */
  current: string;
};

/**
 * The page's title, as a menu of where you can browse. It lists
 * Favorites, then the user's pinned libraries in their order, then More
 * libraries with the rest. In the phone layout, where the Libraries tab
 * lists them, it is the title alone, after Back to that list. It is the
 * title alone on TV too.
 */
export function LibrarySwitcher({ title, current }: Props) {
  const { t } = useLingui();
  const phone = usePhoneLayout();
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const pinned = libraries.filter((l) => l.pinned);
  const others = libraries.filter((l) => !l.pinned);
  const check = (id: string) => id === current && <Icon name="check" className="size-4 text-accent" />;
  const item = (l: Library) => (
    <MenuItem
      key={l.id}
      id={l.id}
      textValue={l.name}
      href={{ to: "/libraries/$libraryId", params: { libraryId: l.id } }}
    >
      <LibraryKindIcon kind={l.kind} className="size-4" />
      <span className="flex-1">{l.name}</span>
      {check(l.id)}
    </MenuItem>
  );
  const section = "mt-1 border-t border-border pt-1";
  // The TV's rail lists Favorites and the libraries, so the title is the title alone.
  if (isTv) return <h1 className="min-w-0 flex-1 truncate text-2xl font-semibold">{title}</h1>;
  if (phone) {
    return (
      <div className="flex min-w-0 flex-1 items-center gap-1">
        <Link to="/libraries" aria-label={t`Libraries`} className={`-ml-3 ${buttonClass("ghost", "icon")}`}>
          <Icon name="chevronLeft" />
        </Link>
        <h1 className="min-w-0 truncate text-2xl font-semibold">{title}</h1>
      </div>
    );
  }
  return (
    <h1 className="min-w-0 flex-1 text-2xl font-semibold">
      <Menu>
        <AriaButton
          data-part="library-switcher"
          className="-mx-2 inline-flex max-w-full cursor-pointer items-center gap-1 rounded-md px-2 hover:bg-surface-hover"
        >
          <span className="truncate">{title}</span>
          <Icon name="chevronDown" className="size-5 shrink-0 text-text-muted" />
        </AriaButton>
        <MenuSection aria-label={t`Favorites`}>
          <MenuItem id={favoritesKey} textValue={t`Favorites`} href={{ to: "/favorites" }}>
            <Icon name="heart" className="size-4" />
            <span className="flex-1">
              <Trans>Favorites</Trans>
            </span>
            {check(favoritesKey)}
          </MenuItem>
        </MenuSection>
        {pinned.length > 0 && (
          <MenuSection aria-label={t`Pinned libraries`} className={section}>
            {pinned.map(item)}
          </MenuSection>
        )}
        {others.length > 0 && (
          <MenuSection className={section}>
            <Header className="px-3 py-1.5 text-xs font-semibold text-text-muted">
              <Trans>More libraries</Trans>
            </Header>
            {others.map(item)}
          </MenuSection>
        )}
      </Menu>
    </h1>
  );
}
