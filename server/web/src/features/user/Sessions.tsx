import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { errorCode } from "@/api/problem";
import type { User } from "@/api/schemas";
import {
  sessionQueries,
  useRenameSession,
  useRevokeSession,
  useSignOutEverywhere,
  useSignOutHere,
} from "@/api/session";
import { Button, buttonClass } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { usePlatformName } from "@/components/deviceCode";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { ModalDialog } from "@/components/ModalDialog";
import { SettingsCard, SettingsRow, settingsCardClass } from "@/components/SettingsLayout";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { formatTimeAgo } from "@/i18n/format";
import { describeProblem } from "@/i18n/problems";
import { isTv } from "@/tv/flag";

/** Sign out for this browser, which says why when it fails. */
export function SignOutHereButton() {
  const { t } = useLingui();
  const here = useSignOutHere();
  // A remote presses by accident, and signing in again there is slow, so
  // the TV asks first.
  const [asking, setAsking] = useState(false);
  return (
    <>
      <FormError error={here.error} />
      <Button size="sm" isPending={here.isPending} onPress={() => (isTv ? setAsking(true) : here.mutate({}))}>
        <Trans>Sign out</Trans>
      </Button>
      {isTv && (
        <ConfirmDialog
          isOpen={asking}
          title={t`Sign out this TV?`}
          confirmLabel={t`Sign out`}
          tone="danger"
          onCancel={() => setAsking(false)}
          onConfirm={() => {
            setAsking(false);
            here.mutate({});
          }}
        >
          <Trans>You will need to sign in again.</Trans>
        </ConfirmDialog>
      )}
    </>
  );
}

/** A session being renamed, or asked about before it's signed out. */
type Pending = { id: string; name: string };

type Props = {
  "aria-labelledby": string;
  /** Another account's devices, as an admin sees them. When absent, the list shows yours. */
  user?: User;
};

/**
 * Signed-in devices. Each browser and app shows its name, platform and
 * last use, with Rename. This one is marked, and the others have Sign out.
 * Then comes Sign out everywhere, or Sign out everywhere else for your own
 * devices. Your own list starts with Link a device. Each sign-out asks
 * first. One that fails says why, with Retry.
 */
export function Sessions({ "aria-labelledby": labelledBy, user }: Props) {
  const { t } = useLingui();
  const sessions = useSuspenseQuery(user ? sessionQueries.forUser(user.id) : sessionQueries.list()).data.items;
  const revoke = useRevokeSession();
  const everywhere = useSignOutEverywhere();
  const others = sessions.filter((s) => !s.current);
  const signOutEverywhere = () => everywhere.mutate(user ? { ids: others.map((s) => s.id) } : { except: "current" });
  const [now] = useState(Date.now);
  const [confirming, setConfirming] = useState(false);
  const [pending, setPending] = useState<Pending>();
  const [renaming, setRenaming] = useState<Pending>();
  const revoking = revoke.variables?.params.path.id;
  const platformName = usePlatformName();
  const name = user?.username;

  return (
    <div data-part="sessions" className="flex flex-col gap-3">
      {!user && (
        <div>
          <Link to="/link" className={buttonClass("secondary", "sm")}>
            <Trans>Link a device</Trans>
          </Link>
        </div>
      )}
      {sessions.length === 0 ? (
        <SettingsCard>
          <SettingsRow label={<Trans>No signed-in devices</Trans>} />
        </SettingsCard>
      ) : (
        <ul aria-labelledby={labelledBy} className={settingsCardClass}>
          {sessions.map((s) => {
            const browser = s.kind === "browser";
            const device = s.name || (browser ? t`Web browser` : t`App`);
            const ago = formatTimeAgo(now - Date.parse(s.lastUsedAt));
            const failed = revoke.isError && revoking === s.id;
            return (
              <li key={s.id}>
                <SettingsRow
                  label={device}
                  description={
                    <>
                      {platformName(s.device?.platform) ?? (browser ? t`Browser` : t`App`)}
                      {" · "}
                      {s.current ? <Trans>This browser</Trans> : <Trans>Last used {ago}</Trans>}
                    </>
                  }
                  error={failed ? describeProblem(errorCode(revoke.error)) : undefined}
                  onRetry={() => revoke.variables && revoke.mutate(revoke.variables)}
                >
                  {!isTv && (
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t`Rename`}
                      onPress={() => setRenaming({ id: s.id, name: s.name })}
                    >
                      <Icon name="pencil" />
                    </Button>
                  )}
                  {!s.current && (
                    <Button
                      size="sm"
                      aria-label={t`Sign out ${device}`}
                      isPending={revoke.isPending && revoking === s.id}
                      onPress={() => setPending({ id: s.id, name: device })}
                    >
                      <Trans>Sign out</Trans>
                    </Button>
                  )}
                </SettingsRow>
              </li>
            );
          })}
        </ul>
      )}
      {others.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <Button size="sm" isPending={everywhere.isPending} onPress={() => setConfirming(true)}>
            {user ? <Trans>Sign out everywhere</Trans> : <Trans>Sign out everywhere else</Trans>}
          </Button>
          {everywhere.isError && (
            <>
              <FormError error={everywhere.error} />
              <Button size="sm" variant="ghost" onPress={signOutEverywhere}>
                <Trans>Retry</Trans>
              </Button>
            </>
          )}
        </div>
      )}
      {renaming && <RenameSessionDialog session={renaming} onClose={() => setRenaming(undefined)} />}
      {pending && (
        <SignOutOtherDialog
          name={pending.name}
          onCancel={() => setPending(undefined)}
          onConfirm={() => {
            revoke.mutate({ params: { path: { id: pending.id } } });
            setPending(undefined);
          }}
        />
      )}
      <ConfirmDialog
        isOpen={confirming}
        title={name ? t`Sign ${name} out everywhere?` : t`Sign out everywhere else?`}
        confirmLabel={name ? t`Sign out everywhere` : t`Sign out everywhere else`}
        tone="danger"
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          setConfirming(false);
          signOutEverywhere();
        }}
      >
        {name ? (
          <Trans>Signs {name} out of all browsers and apps. API keys keep working.</Trans>
        ) : (
          <Trans>Signs out all other browsers and apps. This browser stays signed in.</Trans>
        )}
      </ConfirmDialog>
    </div>
  );
}

/** Renames a browser or app, starting from the name it has. */
function RenameSessionDialog({ session, onClose }: { session: Pending; onClose: () => void }) {
  const { t } = useLingui();
  const rename = useRenameSession();
  return (
    <ModalDialog isOpen onClose={onClose} title={<Trans>Rename</Trans>} width="form">
      <SubmitForm
        onSubmit={(data) => {
          const name = String(data.get("name") ?? "").trim();
          if (name === "" || name === session.name) onClose();
          else rename.mutate({ params: { path: { id: session.id } }, body: { name } }, { onSuccess: onClose });
        }}
        submitLabel={<Trans>Save</Trans>}
        isPending={rename.isPending}
        onCancel={onClose}
        error={rename.error}
      >
        <TextField variant="form" label={t`Name`} name="name" defaultValue={session.name} isRequired maxLength={100} />
      </SubmitForm>
    </ModalDialog>
  );
}

/** Asks before another browser or app, by its name, is signed out. */
function SignOutOtherDialog({
  name,
  onCancel,
  onConfirm,
}: {
  name: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useLingui();
  return (
    <ConfirmDialog
      isOpen
      title={t`Sign out ${name}?`}
      confirmLabel={t`Sign out`}
      tone="danger"
      onCancel={onCancel}
      onConfirm={onConfirm}
    >
      <Trans>{name} has to sign in again.</Trans>
    </ConfirmDialog>
  );
}
