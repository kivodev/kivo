import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { useState } from "react";
import { apiKeyQueries, useCreateApiKey, useRevokeApiKey } from "@/api/apiKeys";
import { errorCode } from "@/api/problem";
import type { ApiKey } from "@/api/schemas";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyField } from "@/components/CopyField";
import { type Dismissal, ModalDialog, useDismissal } from "@/components/ModalDialog";
import { SettingsCard, SettingsLayout, SettingsRow, settingsCardClass } from "@/components/SettingsLayout";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { formatTimeAgo } from "@/i18n/format";
import { describeProblem } from "@/i18n/problems";
import { AdminPage } from "./AdminPage";

/** Admin › API keys. Each key shows its name, when it was created and last used, and Revoke. */
export function ApiKeysScreen() {
  const { t } = useLingui();
  const keys = useSuspenseQuery(apiKeyQueries.list()).data.items;
  const [now] = useState(Date.now);
  return (
    <AdminPage page="api-keys" title={t`API keys`} actions={<CreateApiKeyButton />}>
      <SettingsLayout>
        {keys.length === 0 ? (
          <SettingsCard>
            <SettingsRow
              label={<Trans>No API keys</Trans>}
              description={<Trans>Let scripts and integrations such as Home Assistant access the server.</Trans>}
            />
          </SettingsCard>
        ) : (
          <ul aria-label={t`API keys`} className={settingsCardClass}>
            {keys.map((k) => (
              <ApiKeyRow key={k.id} apiKey={k} now={now} />
            ))}
          </ul>
        )}
      </SettingsLayout>
    </AdminPage>
  );
}

function ApiKeyRow({ apiKey, now }: { apiKey: ApiKey; now: number }) {
  const { t, i18n } = useLingui();
  const revoke = useRevokeApiKey();
  const [confirming, setConfirming] = useState(false);
  const name = apiKey.name;
  const created = i18n.date(apiKey.createdAt, { dateStyle: "medium" });
  const ago = apiKey.lastUsedAt && formatTimeAgo(now - Date.parse(apiKey.lastUsedAt));
  return (
    <li data-part="api-key">
      <SettingsRow
        label={name}
        description={
          <>
            <Trans>Created {created}</Trans>
            {" · "}
            {ago ? <Trans>Last used {ago}</Trans> : <Trans>Never used</Trans>}
          </>
        }
        error={revoke.isError ? describeProblem(errorCode(revoke.error)) : undefined}
      >
        <Button
          size="sm"
          aria-label={t`Revoke ${name}`}
          isPending={revoke.isPending}
          onPress={() => setConfirming(true)}
        >
          <Trans>Revoke</Trans>
        </Button>
      </SettingsRow>
      <ConfirmDialog
        isOpen={confirming}
        title={t`Revoke ${name}?`}
        confirmLabel={t`Revoke`}
        tone="danger"
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          setConfirming(false);
          revoke.mutate({ params: { path: { id: apiKey.id } } });
        }}
      >
        <Trans>Apps and scripts using this key stop working immediately.</Trans>
      </ConfirmDialog>
    </li>
  );
}

/** Create key. It asks for a name, then shows the key once to copy. */
function CreateApiKeyButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onPress={() => setOpen(true)}>
        <Trans>Create key</Trans>
      </Button>
      <ModalDialog isOpen={open} onClose={() => setOpen(false)} title={<Trans>Create an API key</Trans>} width="form">
        <CreateApiKeyForm onClose={() => setOpen(false)} />
      </ModalDialog>
    </>
  );
}

function CreateApiKeyForm({ onClose }: { onClose: () => void }) {
  const { t } = useLingui();
  const create = useCreateApiKey();
  const [name, setName] = useState("");
  const token = create.data?.token;
  // The key shows once, so its dialog closes only with Done.
  let dismissal: Dismissal = name.trim() === "" ? "close" : "ask";
  if (token) dismissal = "never";
  useDismissal(dismissal);

  if (token) return <NewKeyView token={token} onDone={onClose} />;

  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => {
        create.mutate({ body: { name: name.trim() } });
      }}
      submitLabel={<Trans>Create key</Trans>}
      isPending={create.isPending}
      isSubmitDisabled={!name.trim()}
      onCancel={onClose}
      error={create.error}
    >
      <TextField
        variant="form"
        label={t`Name`}
        description={t`For example, Home Assistant.`}
        value={name}
        onChange={setName}
        maxLength={100}
        autoComplete="off"
        autoFocus
      />
    </SubmitForm>
  );
}

function NewKeyView({ token, onDone }: { token: string; onDone: () => void }) {
  const { t } = useLingui();
  return (
    <div className="flex flex-col gap-4">
      <CopyField label={t`API key`} value={token} />
      <p className="text-sm text-text-muted">
        <Trans>
          This key has admin access. Send it in an <code>Authorization: Bearer</code> header. It won't be shown again.
        </Trans>
      </p>
      <div className="flex justify-end gap-2">
        <Button variant="primary" onPress={onDone}>
          <Trans>Done</Trans>
        </Button>
      </div>
    </div>
  );
}
