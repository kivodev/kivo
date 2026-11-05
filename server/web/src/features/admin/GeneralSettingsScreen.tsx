import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { settingsQueries } from "@/api/settings";
import { SettingsCard, SettingsLayout, SettingsRow, SettingsSection } from "@/components/SettingsLayout";
import { TextField } from "@/components/TextField";
import { useDraft } from "@/lib/draft";
import { AdminPage } from "./AdminPage";
import { SettingsForm, useSettingsSave } from "./SettingsForm";

/**
 * Settings › General, with the server's name and then OpenSubtitles. The
 * OpenSubtitles password is never shown. The field sets a new password,
 * and clearing the username clears the password.
 */
export function GeneralSettingsScreen() {
  const { t } = useLingui();
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  const save = useSettingsSave();
  const { draft, edit, discard } = useDraft({
    serverName: settings.serverName,
    openSubtitlesKey: settings.openSubtitlesKey,
    username: settings.openSubtitlesUsername,
    password: "",
  });
  const openSubtitlesUsername = draft.username.trim();
  // The password is sent only when there's a new one, or empty to clear
  // it along with the username.
  const newPassword = openSubtitlesUsername === "" ? "" : draft.password;
  const clearPassword = openSubtitlesUsername === "" && settings.openSubtitlesPasswordSet;
  const openSubtitlesPassword = newPassword !== "" || clearPassword ? newPassword : undefined;
  const serverNameError = save.invalid("/serverName", t`Enter a name.`);
  const openSubtitlesKeyError = save.invalid("/openSubtitlesKey");
  return (
    <AdminPage page="settings-general" title={t`General`}>
      <SettingsForm
        save={save}
        body={{
          serverName: draft.serverName.trim(),
          openSubtitlesKey: draft.openSubtitlesKey.trim(),
          openSubtitlesUsername,
          openSubtitlesPassword,
        }}
        onDiscard={discard}
        onSaved={() => edit({ password: "" })}
      >
        <SettingsLayout>
          <SettingsSection id="server" title={<Trans>Server</Trans>}>
            <SettingsCard>
              <SettingsRow label={<Trans>Server name</Trans>} error={serverNameError}>
                <TextField value={draft.serverName} onChange={(serverName) => edit({ serverName })} isRequired />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="subtitles" title={<Trans>Subtitles</Trans>}>
            <SettingsCard>
              <SettingsRow
                label={<Trans>OpenSubtitles API key</Trans>}
                description={<Trans>From opensubtitles.com. Required for subtitle downloads.</Trans>}
                error={openSubtitlesKeyError}
              >
                <TextField
                  value={draft.openSubtitlesKey}
                  onChange={(openSubtitlesKey) => edit({ openSubtitlesKey })}
                  autoComplete="off"
                  spellCheck="false"
                />
              </SettingsRow>
              <SettingsRow
                label={<Trans>OpenSubtitles username</Trans>}
                description={<Trans>Raises the daily download limit.</Trans>}
              >
                <TextField value={draft.username} onChange={(username) => edit({ username })} autoComplete="off" />
              </SettingsRow>
              <SettingsRow
                label={<Trans>OpenSubtitles password</Trans>}
                description={
                  settings.openSubtitlesPasswordSet && openSubtitlesUsername !== "" ? (
                    <Trans>Saved. Enter a new password to replace it.</Trans>
                  ) : (
                    <Trans>Required if a username is set.</Trans>
                  )
                }
              >
                <TextField
                  type="password"
                  autoComplete="new-password"
                  value={draft.password}
                  onChange={(password) => edit({ password })}
                  isDisabled={openSubtitlesUsername === ""}
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
        </SettingsLayout>
      </SettingsForm>
    </AdminPage>
  );
}
