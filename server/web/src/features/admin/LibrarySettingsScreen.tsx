import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { settingsQueries } from "@/api/settings";
import { NumberField } from "@/components/NumberField";
import { Select } from "@/components/Select";
import { SettingsCard, SettingsLayout, SettingsRow, SettingsSection } from "@/components/SettingsLayout";
import { Switch } from "@/components/Switch";
import { contentRatingCountries, metadataLanguages } from "@/i18n/metadataLocales";
import { useDraft } from "@/lib/draft";
import { AdminPage } from "./AdminPage";
import { SettingsForm, useSettingsSave } from "./SettingsForm";

/**
 * Settings › Library. It sets whether a folder is scanned as soon as its
 * files change, the metadata locale that libraries default to, and the
 * played threshold.
 */
export function LibrarySettingsScreen() {
  const { t } = useLingui();
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  const save = useSettingsSave();
  const { draft, edit, discard } = useDraft({
    fileWatching: settings.fileWatching,
    metadataLanguage: settings.metadataLanguage,
    contentRatingCountry: settings.contentRatingCountry,
    playedThreshold: settings.playedThreshold,
  });
  return (
    <AdminPage page="settings-library" title={t`Library`}>
      <SettingsForm save={save} body={draft} onDiscard={discard}>
        <SettingsLayout>
          <SettingsSection id="scanning" title={<Trans>Scanning</Trans>}>
            <SettingsCard>
              <SettingsRow
                label={<Trans>Scan when files change</Trans>}
                description={
                  <Trans>Network shares may not report changes. All libraries are also scanned every 12 hours.</Trans>
                }
              >
                <Switch isSelected={draft.fileWatching} onChange={(fileWatching) => edit({ fileWatching })} />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
          <SettingsSection
            id="metadata"
            title={<Trans>Metadata</Trans>}
            description={<Trans>Libraries can override these.</Trans>}
          >
            <SettingsCard>
              <SettingsRow label={<Trans>Metadata language</Trans>}>
                <Select
                  options={metadataLanguages(draft.metadataLanguage)}
                  selected={draft.metadataLanguage}
                  onChange={(metadataLanguage) => edit({ metadataLanguage })}
                />
              </SettingsRow>
              <SettingsRow label={<Trans>Certification country</Trans>}>
                <Select
                  options={contentRatingCountries(draft.contentRatingCountry)}
                  selected={draft.contentRatingCountry}
                  onChange={(contentRatingCountry) => edit({ contentRatingCountry })}
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="watch-state" title={<Trans>Watch state</Trans>}>
            <SettingsCard>
              <SettingsRow
                label={<Trans>Played threshold</Trans>}
                description={<Trans>Progress at which a title is marked played.</Trans>}
                error={save.invalid("/playedThreshold", t`From 50% to 100%.`)}
              >
                <NumberField
                  value={draft.playedThreshold}
                  onChange={(playedThreshold) => edit({ playedThreshold })}
                  minValue={0.5}
                  maxValue={1}
                  step={0.01}
                  formatOptions={{ style: "percent" }}
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
        </SettingsLayout>
      </SettingsForm>
    </AdminPage>
  );
}
