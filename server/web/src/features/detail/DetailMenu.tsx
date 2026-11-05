import { useLingui } from "@lingui/react/macro";
import { lazy, type ReactElement, Suspense } from "react";
import type { WorkDetail } from "@/api/schemas";
import { useIsAdmin } from "@/api/users";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { Menu, MenuItem } from "@/components/Menu";
import { TvFocusable } from "@/tv/focus";

// The item admin menu is in its own chunk, loaded for admins only, and not
// in the TV build at all. The flag is read directly rather than isTv: the
// bundler keeps an import that an imported constant guards.
const ItemAdminMenu =
  import.meta.env.VITE_KIVO_TV === "1"
    ? undefined
    : lazy(() => import("@/features/admin/ItemAdminMenu").then((m) => ({ default: m.ItemAdminMenu })));

/**
 * The detail page's ⋯ menu. It has Go to series for an episode, and Go to
 * library. For admins it then has the item admin menu, except on TV, where
 * Fix match and the rest are web-only.
 */
export function DetailMenu({ work }: { work: WorkDetail }) {
  const { t } = useLingui();
  const isAdmin = useIsAdmin();
  const libraryId = work.libraryId;
  const seriesId = work.episode?.seriesId;
  const items: ReactElement[] = [];
  if (seriesId) {
    items.push(
      <MenuItem
        key="series"
        href={{ to: "/works/$workId", params: { workId: seriesId }, search: { season: work.episode?.seasonId } }}
      >
        {t`Go to series`}
      </MenuItem>,
    );
  }
  if (libraryId) {
    items.push(
      <MenuItem key="library" href={{ to: "/libraries/$libraryId", params: { libraryId } }}>
        {t`Go to library`}
      </MenuItem>,
    );
  }
  if (isAdmin && ItemAdminMenu) {
    return (
      <Suspense fallback={null}>
        <ItemAdminMenu work={work}>{items}</ItemAdminMenu>
      </Suspense>
    );
  }
  if (items.length === 0) return null;
  return (
    <span data-part="item-menu">
      <Menu>
        <TvFocusable focusKey="more">
          <Button aria-label={t`More actions for ${work.title}`} variant="secondary" size="icon">
            <Icon name="more" />
          </Button>
        </TvFocusable>
        {items}
      </Menu>
    </span>
  );
}
