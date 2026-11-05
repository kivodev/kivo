import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { type ReactNode, useState } from "react";
import type { Invite, User } from "@/api/schemas";
import { useRevokeInvite, userQueries } from "@/api/users";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Section } from "@/components/Section";
import { formatDay } from "@/i18n/format";
import { AddUserButton } from "./AddUserButton";
import { AdminPage, AdminRow } from "./AdminPage";
import { CreateInviteButton } from "./CreateInviteButton";
import { useLibrariesSummary, useLimitsSummary } from "./userFields";

/**
 * Admin › Users. It lists every user, then the invite links not yet used,
 * each on one line with its libraries, any limits and its expiry. A link's
 * address shows only when the link is made, so a pending invite has Revoke
 * but no Copy.
 */
export function UsersScreen() {
  const { t } = useLingui();
  const users = useSuspenseQuery(userQueries.list()).data.items;
  const invites = useSuspenseQuery(userQueries.invites()).data.items;
  const summary = useLibrariesSummary();
  return (
    <AdminPage
      page="users"
      title={t`Users`}
      actions={
        <div className="flex flex-wrap gap-2">
          <CreateInviteButton />
          <AddUserButton />
        </div>
      }
    >
      <ul className="flex flex-col gap-2">
        {users.map((u) => (
          <AdminRow key={u.id} part="user">
            <span className="flex flex-wrap items-center gap-2">
              <Link
                to="/admin/users/$userId"
                params={{ userId: u.id }}
                className="font-semibold text-text hover:underline"
              >
                {u.username}
              </Link>
              <UserBadges user={u} />
            </span>
            <span className="text-sm text-text-muted">{summary(u.libraries)}</span>
          </AdminRow>
        ))}
      </ul>
      {invites.length > 0 && (
        <Section title={<Trans>Pending invites</Trans>}>
          <ul className="flex flex-col gap-2">
            {invites.map((i) => (
              <InviteRow key={i.id} invite={i} />
            ))}
          </ul>
        </Section>
      )}
    </AdminPage>
  );
}

function Badge({ children }: { children: ReactNode }) {
  return (
    <span className="rounded-full border border-border px-2 text-xs leading-5 font-semibold text-text-muted">
      {children}
    </span>
  );
}

export function UserBadges({ user }: { user: User }) {
  return (
    <>
      {user.admin && (
        <Badge>
          <Trans>Admin</Trans>
        </Badge>
      )}
      {user.disabled && (
        <Badge>
          <Trans>Disabled</Trans>
        </Badge>
      )}
    </>
  );
}

function InviteRow({ invite }: { invite: Invite }) {
  const { t } = useLingui();
  const revoke = useRevokeInvite();
  const summary = useLibrariesSummary();
  const limits = useLimitsSummary();
  const [confirming, setConfirming] = useState(false);
  const expires = formatDay(invite.expiresAt);
  const line = [summary(invite.libraries), limits(invite.limits), t`Expires ${expires}`].filter(Boolean).join(" · ");
  return (
    <AdminRow
      part="invite"
      actions={
        <Button size="sm" variant="ghost" onPress={() => setConfirming(true)}>
          <Trans>Revoke</Trans>
        </Button>
      }
    >
      <span className="text-text">{line}</span>
      <ConfirmDialog
        isOpen={confirming}
        title={t`Revoke this invite link?`}
        confirmLabel={t`Revoke`}
        tone="danger"
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          setConfirming(false);
          revoke.mutate({ params: { path: { id: invite.id } } });
        }}
      >
        <Trans>The link stops working immediately.</Trans>
      </ConfirmDialog>
    </AdminRow>
  );
}
