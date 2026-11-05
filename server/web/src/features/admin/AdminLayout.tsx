import { Trans, useLingui } from "@lingui/react/macro";
import { Link, Outlet, useMatchRoute } from "@tanstack/react-router";
import { type ReactNode, useState } from "react";
import { Button } from "@/components/Button";
import { Icon, type IconName } from "@/components/Icon";
import { ModalDialog } from "@/components/ModalDialog";

type AdminPath =
  | "/admin"
  | "/admin/libraries"
  | "/admin/users"
  | "/admin/history"
  | "/admin/activity"
  | "/admin/tasks"
  | "/admin/logs"
  | "/admin/settings/general"
  | "/admin/settings/library"
  | "/admin/settings/transcoding"
  | "/admin/settings/network"
  | "/admin/settings/api-keys";

type Entry = { to: AdminPath; icon: IconName; label: ReactNode };
type Group = { label?: ReactNode; entries: Entry[] };

/** The admin pages: Overview, then the groups, each holding a list of pages. */
const groups: Group[] = [
  { entries: [{ to: "/admin", icon: "gauge", label: <Trans>Overview</Trans> }] },
  {
    label: <Trans>Manage</Trans>,
    entries: [
      { to: "/admin/libraries", icon: "folder", label: <Trans>Libraries</Trans> },
      { to: "/admin/users", icon: "user", label: <Trans>Users</Trans> },
      { to: "/admin/history", icon: "clock", label: <Trans>Watch history</Trans> },
    ],
  },
  {
    label: <Trans>Maintenance</Trans>,
    entries: [
      { to: "/admin/activity", icon: "list", label: <Trans>Activity</Trans> },
      { to: "/admin/tasks", icon: "activity", label: <Trans>Tasks</Trans> },
      { to: "/admin/logs", icon: "fileLines", label: <Trans>Logs</Trans> },
    ],
  },
  {
    label: <Trans>Settings</Trans>,
    entries: [
      { to: "/admin/settings/general", icon: "settings", label: <Trans>General</Trans> },
      { to: "/admin/settings/library", icon: "film", label: <Trans>Library</Trans> },
      { to: "/admin/settings/transcoding", icon: "chip", label: <Trans>Transcoding</Trans> },
      { to: "/admin/settings/network", icon: "network", label: <Trans>Network</Trans> },
      { to: "/admin/settings/api-keys", icon: "key", label: <Trans>API keys</Trans> },
    ],
  },
];

const linkClass =
  "flex h-9 shrink-0 items-center gap-3 rounded-md px-3 text-sm font-medium text-text-muted hover:bg-surface-hover hover:text-text";
const activeLink = { className: "bg-surface-raised text-text", "aria-current": "page" as const };

/**
 * A page's entry. It is current on its page and the pages under it, such
 * as a user's page under Users, whatever the filters. Overview is current
 * only on itself.
 */
function AdminLink({ entry, onNavigate }: { entry: Entry; onNavigate?: () => void }) {
  return (
    <Link
      to={entry.to}
      className={linkClass}
      activeProps={activeLink}
      activeOptions={{ exact: entry.to === "/admin", includeSearch: false }}
      onClick={onNavigate}
    >
      <Icon name={entry.icon} className="size-4 shrink-0" />
      <span className="flex-1">{entry.label}</span>
    </Link>
  );
}

/** The grouped list of pages, in the sidebar and in the phone's sheet. */
function AdminNav({ onNavigate }: { onNavigate?: () => void }) {
  return groups.map((group) => (
    <div key={group.entries[0]?.to} className="flex flex-col gap-1">
      {group.label && (
        <span className="px-3 pt-3 pb-1 text-xs font-semibold tracking-wide text-text-subtle uppercase">
          {group.label}
        </span>
      )}
      {group.entries.map((entry) => (
        <AdminLink key={entry.to} entry={entry} onNavigate={onNavigate} />
      ))}
    </div>
  ));
}

/**
 * The admin area, with its sidebar and then the page. The sidebar lists
 * Overview, then Manage, Maintenance and Settings. Below 768 px the
 * sidebar becomes a button naming the page, such as "Admin › Tasks",
 * which opens the list as a sheet.
 */
export function AdminLayout() {
  const { t } = useLingui();
  const matchRoute = useMatchRoute();
  const [open, setOpen] = useState(false);
  const entries = groups.flatMap((g) => g.entries);
  const current =
    entries.find((e) => e.to !== "/admin" && matchRoute({ to: e.to, fuzzy: true }) !== false) ?? entries[0];
  return (
    <div className="flex gap-8 px-4 py-6 max-md:flex-col max-md:gap-4 sm:px-8">
      <nav
        data-part="admin-sidebar"
        aria-label={t`Admin`}
        className="flex w-(--layout-sidebar-width) shrink-0 flex-col gap-1 max-md:hidden"
      >
        <AdminNav />
      </nav>
      <Button className="self-start md:hidden" aria-label={t`Admin pages`} onPress={() => setOpen(true)}>
        <span className="text-text-muted">
          <Trans>Admin</Trans>
        </span>
        <Icon name="chevronRight" className="size-4 text-text-muted" />
        {current?.label}
        <Icon name="chevronDown" className="size-4" />
      </Button>
      <ModalDialog isOpen={open} onClose={() => setOpen(false)} title={<Trans>Admin</Trans>} placement="side">
        <nav aria-label={t`Admin`} className="flex flex-col gap-1">
          <AdminNav onNavigate={() => setOpen(false)} />
        </nav>
      </ModalDialog>
      <div className="min-w-0 flex-1">
        <Outlet />
      </div>
    </div>
  );
}
