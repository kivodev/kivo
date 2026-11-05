import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useState } from "react";
import { useUpdateLibrary } from "@/api/libraries";
import type { AdminLibrary, LibraryUpdate, ProcessingSchedule, RefreshDays } from "@/api/schemas";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { useDismissal } from "@/components/ModalDialog";
import { Select } from "@/components/Select";
import { SettingsCard, SettingsRow } from "@/components/SettingsLayout";
import { SubmitForm } from "@/components/SubmitForm";
import { Switch } from "@/components/Switch";
import { TextField } from "@/components/TextField";
import { contentRatingCountries, metadataLanguages } from "@/i18n/metadataLocales";
import { changes } from "@/lib/draft";
import { FoldersField } from "./FoldersField";

type Props = {
  library: AdminLibrary;
  onSaved: () => void;
  onCancel: () => void;
};

// The select's ID for the server's metadata language or content rating country.
const serverDefault = "server";

/**
 * A library's settings. They are its name and folders, the metadata
 * language and content rating country (the server's unless the library
 * has its own), local metadata, preview thumbnails, a shows library's
 * intro and credits markers, subtitles prepared ahead of a play, and how
 * often its metadata is refreshed.
 *
 * A removed folder's files are forgotten, so Save asks first. An added
 * folder is scanned. An audiobooks library makes no preview thumbnails and
 * has no subtitles, so it has neither choice. Its content rating country
 * picks the Audible store that its books are described from.
 */
export function EditLibraryForm({ library, onSaved, onCancel }: Props) {
  const { t } = useLingui();
  const update = useUpdateLibrary();
  const saved = library.folders.map((f) => f.path);
  const [name, setName] = useState(library.name);
  const [folders, setFolders] = useState(saved);
  const [confirming, setConfirming] = useState(false);
  const [useLocalMetadata, setUseLocalMetadata] = useState(library.useLocalMetadata);
  const [previews, setPreviews] = useState(library.previews);
  const [markers, setMarkers] = useState(library.markers);
  const [subtitles, setSubtitles] = useState(library.subtitles);
  const [refreshDays, setRefreshDays] = useState(library.refreshDays);
  const [language, setLanguage] = useState(library.metadataLanguage ?? serverDefault);
  const [country, setCountry] = useState(library.contentRatingCountry ?? serverDefault);
  const books = library.kind === "audiobooks";
  const shows = library.kind === "shows";
  const removedFolders = saved.filter((f) => !folders.includes(f));
  const removed = removedFolders.join(", ");
  const body: LibraryUpdate = changes(
    {
      name: library.name,
      folders: saved,
      metadataLanguage: library.metadataLanguage ?? "",
      contentRatingCountry: library.contentRatingCountry ?? "",
      useLocalMetadata: library.useLocalMetadata,
      previews: library.previews,
      markers: library.markers,
      subtitles: library.subtitles,
      refreshDays: library.refreshDays,
    },
    {
      name: name.trim(),
      folders,
      metadataLanguage: language === serverDefault ? "" : language,
      contentRatingCountry: country === serverDefault ? "" : country,
      useLocalMetadata,
      ...(!books && { previews }),
      ...(shows && { markers }),
      ...(!books && { subtitles }),
      refreshDays,
    },
  );
  const changed = Object.keys(body).length > 0;
  useDismissal(changed ? "ask" : "close");
  const save = () => update.mutate({ params: { path: { id: library.id } }, body }, { onSuccess: onSaved });
  const previewOptions: { id: ProcessingSchedule; label: string }[] = [
    { id: "never", label: t`Never` },
    { id: "nightly", label: t`Nightly` },
    { id: "nightlyAndAdded", label: t`Nightly and when media is added` },
  ];
  const refreshOptions: { id: `${RefreshDays}`; label: string }[] = [
    { id: "0", label: t`Never` },
    { id: "30", label: t`Every 30 days` },
    { id: "60", label: t`Every 60 days` },
    { id: "90", label: t`Every 90 days` },
  ];

  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => (removedFolders.length > 0 ? setConfirming(true) : save())}
      submitLabel={<Trans>Save</Trans>}
      isPending={update.isPending}
      isSubmitDisabled={!changed || folders.length === 0}
      onCancel={onCancel}
      error={update.error}
    >
      <TextField variant="form" label={t`Name`} value={name} onChange={setName} isRequired maxLength={100} />
      <FoldersField folders={folders} onChange={setFolders} />
      <SettingsCard>
        <SettingsRow label={<Trans>Metadata language</Trans>}>
          <Select
            options={[
              { id: serverDefault, label: t`Server default` },
              ...metadataLanguages(language === serverDefault ? "" : language),
            ]}
            selected={language}
            onChange={setLanguage}
          />
        </SettingsRow>
        <SettingsRow
          label={<Trans>Certification country</Trans>}
          description={books && <Trans>Also sets the Audible store used for book metadata.</Trans>}
        >
          <Select
            options={[
              { id: serverDefault, label: t`Server default` },
              ...contentRatingCountries(country === serverDefault ? "" : country),
            ]}
            selected={country}
            onChange={setCountry}
          />
        </SettingsRow>
        <SettingsRow
          label={books ? <Trans>Use local artwork</Trans> : <Trans>Use local metadata</Trans>}
          description={
            books ? (
              <Trans>Cover images next to the books.</Trans>
            ) : (
              <Trans>.nfo files and artwork next to the media.</Trans>
            )
          }
        >
          <Switch isSelected={useLocalMetadata} onChange={setUseLocalMetadata} />
        </SettingsRow>
        {!books && (
          <SettingsRow label={<Trans>Preview thumbnails</Trans>}>
            <Select options={previewOptions} selected={previews} onChange={setPreviews} />
          </SettingsRow>
        )}
        {shows && (
          <SettingsRow label={<Trans>Intro and credits markers</Trans>}>
            <Select options={previewOptions} selected={markers} onChange={setMarkers} />
          </SettingsRow>
        )}
        {!books && (
          <SettingsRow label={<Trans>Subtitles</Trans>} description={<Trans>Prepared ahead of the first play.</Trans>}>
            <Select options={previewOptions} selected={subtitles} onChange={setSubtitles} />
          </SettingsRow>
        )}
        <SettingsRow label={<Trans>Refresh metadata</Trans>}>
          <Select
            options={refreshOptions}
            selected={`${refreshDays}`}
            onChange={(days) => setRefreshDays(Number(days) as RefreshDays)}
          />
        </SettingsRow>
      </SettingsCard>
      <ConfirmDialog
        isOpen={confirming}
        title={plural(removedFolders.length, { one: "Remove # folder?", other: "Remove # folders?" })}
        confirmLabel={t`Remove and save`}
        tone="danger"
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          setConfirming(false);
          save();
        }}
      >
        <Trans>
          Items from {removed} are removed from the library. Files on disk aren't touched. Adding the folder again
          restores the items and their watch state.
        </Trans>
      </ConfirmDialog>
    </SubmitForm>
  );
}
