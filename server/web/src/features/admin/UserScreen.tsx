import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { errorCode } from "@/api/problem";
import type { User, UserUpdate } from "@/api/schemas";
import {
  currentUserQueries,
  useDeleteUser,
  usernameMaxLength,
  userQueries,
  useSetUserPassword,
  useUpdateUser,
} from "@/api/users";
import { workQueries } from "@/api/works";
import { Button, buttonClass } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ModalDialog, useDismissal } from "@/components/ModalDialog";
import { Select } from "@/components/Select";
import { SettingsCard, SettingsLayout, SettingsRow, SettingsSection } from "@/components/SettingsLayout";
import { SubmitForm } from "@/components/SubmitForm";
import { Switch } from "@/components/Switch";
import { TextField } from "@/components/TextField";
import { Sessions } from "@/features/user/Sessions";
import { describeProblem } from "@/i18n/problems";
import { changes, useDraft } from "@/lib/draft";
import { AdminPage } from "./AdminPage";
import { SaveForm } from "./SettingsForm";
import { UserBadges } from "./UsersScreen";
import { accessChosen, LibraryAccessRows, LimitsRows } from "./userFields";

const userRoute = getRouteApi("/authed/admin/users/$userId");

/**
 * A user's page, under Users. Account, Access, Limits and Parental
 * controls are saved together from the save bar. Set password in Account
 * acts on its own. Then come Devices and the red card of Disable and
 * Delete, each acting on its own, and a link to the user's watch history.
 */
export function UserScreen() {
  const { t } = useLingui();
  const { userId } = userRoute.useParams();
  const user = useSuspenseQuery(userQueries.detail(userId)).data;
  const me = useSuspenseQuery(currentUserQueries.current()).data.id;
  // Why the admin switch, Disable and Delete are disabled, if they are.
  const locked =
    user.id === me
      ? t`Not available for your own account.`
      : user.onlyAdmin
        ? t`Only admin. Make another user an admin first.`
        : undefined;

  return (
    <AdminPage
      page="user"
      parent={
        <Link to="/admin/users" className="hover:text-text hover:underline">
          <Trans>Users ›</Trans>
        </Link>
      }
      title={user.username}
      badges={<UserBadges user={user} />}
      actions={
        <Link to="/admin/history" search={{ userId: user.id }} className={buttonClass()}>
          <Trans>Watch history</Trans>
        </Link>
      }
    >
      <UserSettings key={user.id} user={user} locked={locked} />
    </AdminPage>
  );
}

// The select's ID for no ceiling.
const noCeiling = "none";

/**
 * The user's sections, with the save bar for Account, Access, Limits and
 * Parental controls. Save sends the settings that changed. It asks first
 * when the change makes the user an admin.
 */
function UserSettings({ user, locked }: { user: User; locked: string | undefined }) {
  const { t } = useLingui();
  const update = useUpdateUser();
  const ratings = useSuspenseQuery(workQueries.contentRatings()).data.items;
  const { draft, edit, discard } = useDraft({
    username: user.username,
    libraries: user.libraries,
    admin: user.admin,
    limits: user.limits,
    parental: user.parental,
  });
  const [confirming, setConfirming] = useState(false);
  const name = user.username;
  const username = draft.username.trim();
  const body: UserUpdate = changes(user, { ...draft, username });
  const nameTaken = update.isError && errorCode(update.error) === "nameTaken";
  const save = () => update.mutate({ params: { path: { id: user.id } }, body });
  const setCeiling = (maxAge: number | undefined) => {
    const { maxAge: _, ...parental } = draft.parental;
    edit({ parental: maxAge === undefined ? parental : { ...parental, maxAge } });
  };
  // One option per age, named by the ratings for that age.
  const ages = [...new Set(ratings.map((r) => r.age))].sort((a, b) => a - b);
  const ceilings = [
    { id: noCeiling, label: t`No limit` },
    ...ages.map((age) => ({
      id: String(age),
      label: ratings
        .filter((r) => r.age === age)
        .map((r) => r.name)
        .join(", "),
    })),
  ];

  return (
    <>
      <SaveForm
        isDirty={Object.keys(body).length > 0}
        isPending={update.isPending}
        isSaveDisabled={!accessChosen(draft.libraries) || username === ""}
        error={nameTaken ? null : update.error}
        onSave={() => (body.admin ? setConfirming(true) : save())}
        onDiscard={() => {
          update.reset();
          discard();
        }}
      >
        <SettingsLayout>
          <SettingsSection id="account" title={<Trans>Account</Trans>}>
            <SettingsCard>
              <SettingsRow label={<Trans>Username</Trans>} error={nameTaken ? describeProblem("nameTaken") : undefined}>
                <TextField
                  value={draft.username}
                  onChange={(username) => edit({ username })}
                  isRequired
                  maxLength={usernameMaxLength}
                  autoComplete="off"
                />
              </SettingsRow>
              <PasswordRow user={user} />
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="access" title={<Trans>Access</Trans>}>
            <SettingsCard>
              <LibraryAccessRows value={draft.libraries} onChange={(libraries) => edit({ libraries })} />
            </SettingsCard>
            <SettingsCard>
              <SettingsRow label={<Trans>Admin</Trans>} description={locked && user.admin ? locked : undefined}>
                <Switch
                  isSelected={draft.admin}
                  onChange={(admin) => edit({ admin })}
                  isDisabled={locked !== undefined && user.admin}
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="limits" title={<Trans>Limits</Trans>}>
            <SettingsCard>
              <LimitsRows value={draft.limits} onChange={(limits) => edit({ limits })} />
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="parental" title={<Trans>Parental controls</Trans>}>
            <SettingsCard>
              <SettingsRow label={<Trans>Highest content rating</Trans>}>
                <Select
                  options={ceilings}
                  selected={draft.parental.maxAge === undefined ? noCeiling : String(draft.parental.maxAge)}
                  onChange={(id) => setCeiling(id === noCeiling ? undefined : Number(id))}
                />
              </SettingsRow>
              <SettingsRow
                label={<Trans>Block unrated items</Trans>}
                description={<Trans>Audiobooks aren't affected.</Trans>}
              >
                <Switch
                  isSelected={draft.parental.blockUnrated}
                  onChange={(blockUnrated) => edit({ parental: { ...draft.parental, blockUnrated } })}
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="devices" title={<Trans>Devices</Trans>}>
            <Sessions aria-labelledby="devices-title" user={user} />
          </SettingsSection>
          <DangerSection user={user} locked={locked} />
        </SettingsLayout>
      </SaveForm>
      <ConfirmDialog
        isOpen={confirming}
        title={t`Make ${name} an admin?`}
        confirmLabel={t`Make admin and save`}
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          setConfirming(false);
          save();
        }}
      >
        <Trans>Admins can manage all libraries, users and settings.</Trans>
      </ConfirmDialog>
    </>
  );
}

/** Set password…, in Account, which opens its own dialog and acts at once. */
function PasswordRow({ user }: { user: User }) {
  const [open, setOpen] = useState(false);
  // Setting your own password keeps this browser signed in.
  const self = useSuspenseQuery(currentUserQueries.current()).data.id === user.id;
  const name = user.username;
  return (
    <>
      <SettingsRow
        label={<Trans>Password</Trans>}
        description={
          self ? (
            <Trans>Setting a password signs you out everywhere else.</Trans>
          ) : (
            <Trans>Setting a password signs {name} out everywhere.</Trans>
          )
        }
      >
        <Button size="sm" onPress={() => setOpen(true)}>
          <Trans>Set password…</Trans>
        </Button>
      </SettingsRow>
      <ModalDialog
        isOpen={open}
        onClose={() => setOpen(false)}
        title={<Trans>Set {user.username}'s password</Trans>}
        width="form"
        part="set-password"
      >
        <SetPasswordForm user={user} onDone={() => setOpen(false)} />
      </ModalDialog>
    </>
  );
}

function SetPasswordForm({ user, onDone }: { user: User; onDone: () => void }) {
  const { t } = useLingui();
  const setPassword = useSetUserPassword();
  const [password, setValue] = useState("");
  useDismissal(password ? "ask" : "close");
  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => {
        setPassword.mutate({ params: { path: { id: user.id } }, body: { password } }, { onSuccess: onDone });
      }}
      submitLabel={<Trans>Set password</Trans>}
      isPending={setPassword.isPending}
      isSubmitDisabled={password.length < 8}
      onCancel={onDone}
      error={setPassword.error}
    >
      <TextField
        variant="form"
        label={t`New password`}
        type="password"
        autoComplete="new-password"
        isRequired
        minLength={8}
        autoFocus
        value={password}
        onChange={setValue}
        description={t`At least 8 characters.`}
      />
    </SubmitForm>
  );
}

type Confirming = "disable" | "delete" | undefined;

/**
 * Disable or Enable, and Delete, in a red card with no heading. Each acts
 * at once, but Disable and Delete ask first.
 */
function DangerSection({ user, locked }: { user: User; locked: string | undefined }) {
  const { t } = useLingui();
  const update = useUpdateUser();
  const remove = useDeleteUser();
  const navigate = useNavigate();
  const [confirming, setConfirming] = useState<Confirming>(undefined);
  const name = user.username;
  const setDisabled = (disabled: boolean) => update.mutate({ params: { path: { id: user.id } }, body: { disabled } });
  return (
    <SettingsSection id="danger">
      <SettingsCard tone="danger" part="danger-zone" footer={locked}>
        <SettingsRow
          label={user.disabled ? <Trans>Enable account</Trans> : <Trans>Disable account</Trans>}
          error={update.isError ? describeProblem(errorCode(update.error)) : undefined}
        >
          <Button
            size="sm"
            isDisabled={locked !== undefined && !user.disabled}
            isPending={update.isPending}
            onPress={() => (user.disabled ? setDisabled(false) : setConfirming("disable"))}
          >
            {user.disabled ? <Trans>Enable</Trans> : <Trans>Disable</Trans>}
          </Button>
        </SettingsRow>
        <SettingsRow
          label={<Trans>Delete account</Trans>}
          error={remove.isError ? describeProblem(errorCode(remove.error)) : undefined}
        >
          <Button
            size="sm"
            variant="danger"
            isDisabled={locked !== undefined}
            isPending={remove.isPending}
            onPress={() => setConfirming("delete")}
          >
            <Trans>Delete</Trans>
          </Button>
        </SettingsRow>
      </SettingsCard>
      <ConfirmDialog
        isOpen={confirming === "disable"}
        title={t`Disable ${name}?`}
        confirmLabel={t`Disable`}
        onCancel={() => setConfirming(undefined)}
        onConfirm={() => {
          setConfirming(undefined);
          setDisabled(true);
        }}
      >
        <Trans>{name} is signed out everywhere and can't sign in until enabled again.</Trans>
      </ConfirmDialog>
      <ConfirmDialog
        isOpen={confirming === "delete"}
        title={t`Delete ${name}?`}
        confirmLabel={t`Delete`}
        tone="danger"
        onCancel={() => setConfirming(undefined)}
        onConfirm={() => {
          setConfirming(undefined);
          remove.mutate(
            { params: { path: { id: user.id } } },
            { onSuccess: () => void navigate({ to: "/admin/users" }) },
          );
        }}
      >
        <Trans>The account, watch history and progress are permanently deleted.</Trans>
      </ConfirmDialog>
    </SettingsSection>
  );
}
