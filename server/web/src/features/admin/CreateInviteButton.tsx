import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import type { UserLimits } from "@/api/schemas";
import { settingsQueries } from "@/api/settings";
import { useCreateInvite } from "@/api/users";
import { Button } from "@/components/Button";
import { CopyField } from "@/components/CopyField";
import { type Dismissal, ModalDialog, useDismissal } from "@/components/ModalDialog";
import { Select } from "@/components/Select";
import { SettingsCard, SettingsRow } from "@/components/SettingsLayout";
import { SubmitForm } from "@/components/SubmitForm";
import { accessChosen, LibraryAccessRows, LimitsRows, noAccess } from "./userFields";

const day = 24 * 60 * 60 * 1000;

/** Create invite link. It asks for libraries, limits and an expiry, then gives a link to copy and send. */
export function CreateInviteButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onPress={() => setOpen(true)}>
        <Trans>Create invite link</Trans>
      </Button>
      <ModalDialog isOpen={open} onClose={() => setOpen(false)} title={<Trans>Invite someone</Trans>} width="form">
        <InviteForm onClose={() => setOpen(false)} />
      </ModalDialog>
    </>
  );
}

function InviteForm({ onClose }: { onClose: () => void }) {
  const { t } = useLingui();
  const create = useCreateInvite();
  const publicUrl = useQuery(settingsQueries.current()).data?.publicUrl ?? "";
  const [access, setAccess] = useState(noAccess);
  const [limits, setLimits] = useState<UserLimits>({});
  const [days, setDays] = useState("7");
  const token = create.data?.token;
  // The link shows once, so its dialog closes only with Done.
  let dismissal: Dismissal = accessChosen(access) || Object.keys(limits).length > 0 ? "ask" : "close";
  if (token) dismissal = "never";
  useDismissal(dismissal);

  if (token) {
    const base = publicUrl ? publicUrl.replace(/\/?$/, "/") : document.baseURI;
    return (
      <InviteLinkView link={new URL(`invite/${token}`, base).href} publicUrl={publicUrl !== ""} onDone={onClose} />
    );
  }

  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => {
        const expiresAt = new Date(Date.now() + Number(days) * day).toISOString();
        create.mutate({ body: { libraries: access, limits, expiresAt } });
      }}
      submitLabel={<Trans>Create link</Trans>}
      isPending={create.isPending}
      isSubmitDisabled={!accessChosen(access)}
      onCancel={onClose}
      error={create.error}
    >
      <SettingsCard title={<Trans>Libraries</Trans>}>
        <LibraryAccessRows value={access} onChange={setAccess} />
      </SettingsCard>
      <SettingsCard title={<Trans>Limits</Trans>}>
        <LimitsRows value={limits} onChange={setLimits} />
      </SettingsCard>
      <SettingsCard>
        <SettingsRow label={<Trans>Expires after</Trans>}>
          <Select
            options={[
              { id: "1", label: t`1 day` },
              { id: "7", label: t`7 days` },
              { id: "30", label: t`30 days` },
            ]}
            selected={days}
            onChange={setDays}
          />
        </SettingsRow>
      </SettingsCard>
    </SubmitForm>
  );
}

/** The new link, to copy now, with a warning when it's on this browser's address. */
function InviteLinkView({ link, publicUrl, onDone }: { link: string; publicUrl: boolean; onDone: () => void }) {
  const { t } = useLingui();
  const network = (
    <Link to="/admin/settings/network" className="font-semibold text-text hover:underline" onClick={onDone}>
      <Trans>Network settings</Trans>
    </Link>
  );
  return (
    <div className="flex flex-col gap-4">
      <CopyField label={t`Invite link`} value={link} />
      <p className="text-sm text-text-muted">
        <Trans>Anyone with this link can create one account. It won't be shown again.</Trans>
      </p>
      {!publicUrl && (
        <p className="text-sm text-text-muted">
          <Trans>
            Uses this browser's address. For links that work outside the local network, set a public URL in {network}.
          </Trans>
        </p>
      )}
      <div className="flex justify-end">
        <Button variant="primary" onPress={onDone}>
          <Trans>Done</Trans>
        </Button>
      </div>
    </div>
  );
}
