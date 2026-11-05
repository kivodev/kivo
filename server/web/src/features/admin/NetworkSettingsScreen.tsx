import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { useQualityLadder } from "@/api/server";
import { settingsQueries } from "@/api/settings";
import { Select } from "@/components/Select";
import { SettingsCard, SettingsLayout, SettingsRow, SettingsSection } from "@/components/SettingsLayout";
import { TextField } from "@/components/TextField";
import { formatRung } from "@/i18n/format";
import { useDraft } from "@/lib/draft";
import { AdminPage } from "./AdminPage";
import { SettingsForm, useSettingsSave } from "./SettingsForm";

/**
 * Network: which addresses count as local, the address apps use away from
 * home, and the remote streaming limit for users without one of their own.
 */
export function NetworkSettingsScreen() {
  const { t } = useLingui();
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  const save = useSettingsSave();
  const { draft, edit, discard } = useDraft({
    networks: settings.localNetworks.join(", "),
    publicUrl: settings.publicUrl,
    remoteCap: settings.remoteQualityCapBps,
  });
  const qualityLadder = useQualityLadder();
  const caps = [
    { id: "0", label: t`No limit` },
    ...qualityLadder.map((rung) => ({ id: String(rung.bitrateBps), label: formatRung(rung) })),
  ];
  return (
    <AdminPage page="settings-network" title={t`Network`}>
      <SettingsForm
        save={save}
        body={{
          localNetworks: parseNetworks(draft.networks),
          publicUrl: draft.publicUrl.trim(),
          remoteQualityCapBps: draft.remoteCap,
        }}
        onDiscard={discard}
      >
        <SettingsLayout>
          <SettingsSection id="local" title={<Trans>Local network</Trans>}>
            <SettingsCard>
              <SettingsRow
                label={<Trans>Local networks</Trans>}
                description={<Trans>Leave empty to use private address ranges. Other addresses are remote.</Trans>}
                error={save.invalid(
                  "/localNetworks",
                  t`Use address ranges such as 192.168.1.0/24, separated by commas.`,
                )}
              >
                <TextField
                  value={draft.networks}
                  onChange={(networks) => edit({ networks })}
                  placeholder="192.168.1.0/24, 10.0.0.0/8"
                  autoComplete="off"
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="remote" title={<Trans>Remote access</Trans>}>
            <SettingsCard>
              <SettingsRow
                label={<Trans>Public URL</Trans>}
                description={<Trans>Address used by apps and invite links outside the local network.</Trans>}
                error={save.invalid("/publicUrl", t`Use a full address starting with http:// or https://.`)}
              >
                <TextField
                  type="url"
                  value={draft.publicUrl}
                  onChange={(publicUrl) => edit({ publicUrl })}
                  placeholder="https://kivo.example.com"
                  autoComplete="off"
                />
              </SettingsRow>
              <SettingsRow
                label={<Trans>Remote streaming limit</Trans>}
                description={<Trans>For users without a limit of their own.</Trans>}
              >
                <Select
                  options={caps}
                  selected={String(draft.remoteCap)}
                  onChange={(id) => edit({ remoteCap: Number(id) })}
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
        </SettingsLayout>
      </SettingsForm>
    </AdminPage>
  );
}

/** Parses a list such as "10.0.0.0/8, 192.168.1.0/24" into its ranges. */
function parseNetworks(text: string): string[] {
  return text
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter((s) => s !== "");
}
