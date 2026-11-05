import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Button as AriaButton, FileTrigger, Tab, TabList, TabPanel, Tabs } from "react-aria-components";
import { errorCode, isProblem } from "@/api/problem";
import type { Artwork as ArtworkItem, ArtworkType, WorkDetail, WorkKind } from "@/api/schemas";
import { useSelectArtwork, useUploadArtwork, workQueries } from "@/api/works";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { ModalDialog } from "@/components/ModalDialog";
import { languageName } from "@/components/trackLabels";
import { describeProblem } from "@/i18n/problems";

type Props = { work: WorkDetail; isOpen: boolean; onClose: () => void };

/** The artwork each kind of work has, in the order the tabs show them. */
const typesOf: Partial<Record<WorkKind, ArtworkType[]>> = {
  movie: ["poster", "backdrop", "logo"],
  series: ["poster", "backdrop", "logo"],
  video: ["poster", "backdrop", "logo"],
  season: ["poster"],
  episode: ["still"],
  book: ["poster"],
};

/** Kinds of work Choose artwork applies to. */
export const artworkKinds = new Set(Object.keys(typesOf));

/** An artwork type in words. */
function useTypeName() {
  const { t } = useLingui();
  const names: Record<ArtworkType, string> = {
    poster: t`Poster`,
    backdrop: t`Background`,
    logo: t`Logo`,
    still: t`Still`,
  };
  return (type: ArtworkType) => names[type] ?? type;
}

const uploadTypes = ["image/jpeg", "image/png", "image/webp"];
const maxUploadBytes = 25 << 20;

/**
 * Choose artwork. For each type, it shows the current image, the files
 * next to the media and TMDb's images in a grid. Pressing an image makes
 * it the work's artwork. Upload image adds the admin's own. Refreshes keep
 * either choice.
 */
export function ChooseArtworkDialog({ work, isOpen, onClose }: Props) {
  const { t } = useLingui();
  const typeName = useTypeName();
  const title = work.title;
  const types = typesOf[work.kind] ?? [];
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} width="page" title={<Trans>Choose artwork for {title}</Trans>}>
      <Tabs>
        <TabList aria-label={t`Artwork type`} className="mb-4 flex gap-2">
          {types.map((type) => (
            <Tab
              key={type}
              id={type}
              className="cursor-pointer rounded-full border border-border px-3 py-1 text-sm text-text data-selected:border-accent data-selected:bg-surface-hover data-selected:font-semibold"
            >
              {typeName(type)}
            </Tab>
          ))}
        </TabList>
        {types.map((type) => (
          <TabPanel key={type} id={type} className="outline-none">
            <ArtworkGrid work={work} type={type} />
          </TabPanel>
        ))}
      </Tabs>
      <div className="mt-5 flex justify-end">
        <Button onPress={onClose}>
          <Trans>Close</Trans>
        </Button>
      </div>
    </ModalDialog>
  );
}

function ArtworkGrid({ work, type }: { work: WorkDetail; type: ArtworkType }) {
  const { t } = useLingui();
  const list = useQuery(workQueries.artwork(work.id, type));
  const select = useSelectArtwork();
  const upload = useUploadArtwork();
  const words = useArtworkWords();
  const [refused, setRefused] = useState<string>();
  const poster = type === "poster";
  const label = useTypeName()(type);

  const send = (files: FileList | null) => {
    const file = files?.[0];
    if (!file) return;
    if (!uploadTypes.includes(file.type)) {
      setRefused(words.notAnImage);
    } else if (file.size > maxUploadBytes) {
      setRefused(words.tooLarge);
    } else {
      setRefused(undefined);
      upload.upload(work.id, type, file);
    }
  };
  const failed = select.error ?? upload.error;

  return (
    <div className="flex flex-col gap-4">
      {list.isPending && (
        <p className="text-sm text-text-muted">
          <Trans>Loading artwork…</Trans>
        </p>
      )}
      <FormError error={list.error} />
      {list.data?.items.length === 0 && (
        <p className="text-sm text-text-muted">
          <Trans>No artwork available. Upload an image.</Trans>
        </p>
      )}
      {list.data && list.data.items.length > 0 && (
        <ul
          aria-label={t`${label} choices`}
          className={`grid gap-3 ${poster ? "grid-cols-3 sm:grid-cols-5" : "grid-cols-2 sm:grid-cols-3"}`}
        >
          {list.data.items.map((a) => (
            <li key={a.id}>
              <AriaButton
                aria-label={words.describe(a)}
                aria-pressed={a.selected}
                isDisabled={select.isPending || upload.isPending}
                onPress={() => select.mutate({ params: { path: { id: work.id, artworkId: a.id } } })}
                className={`flex w-full cursor-pointer flex-col gap-1 rounded-md border-2 p-1 text-left data-disabled:cursor-default ${a.selected ? "border-accent" : "border-transparent hover:border-border"}`}
              >
                <Artwork
                  image={a.image}
                  title={label}
                  shape={poster ? (work.kind === "book" ? "square" : "poster") : "landscape"}
                  fit={type === "logo" ? "contain" : "cover"}
                  width={poster ? 160 : 240}
                />
                <span className="truncate text-xs text-text-muted">
                  {a.selected ? <Trans>Current</Trans> : words.sourceLine(a)}
                </span>
              </AriaButton>
            </li>
          ))}
        </ul>
      )}
      {(refused || failed) && <FormError>{refused ?? words.uploadProblem(failed)}</FormError>}
      <div>
        <FileTrigger acceptedFileTypes={uploadTypes} onSelect={send}>
          <Button isPending={upload.isPending}>
            <Trans>Upload image</Trans>
          </Button>
        </FileTrigger>
      </div>
    </div>
  );
}

/** Words for an image: where it's from, and why an upload was refused. */
function useArtworkWords() {
  const { t } = useLingui();
  /** Where an image is from, with its language and size, such as "TMDb · English · 2000 × 3000". */
  const sourceLine = (a: ArtworkItem): string => {
    const names: Record<string, string> = { local: t`Local file`, upload: t`Uploaded` };
    const parts = [names[a.source] ?? "TMDb"];
    if (a.language) parts.push(languageName(a.language));
    if (a.width && a.height) parts.push(`${a.width} × ${a.height}`);
    return parts.join(" · ");
  };
  return {
    sourceLine,
    /** A tile's name. It is the source line, marked when the image is the current one. */
    describe: (a: ArtworkItem): string => {
      const line = sourceLine(a);
      return a.selected ? t`Current: ${line}` : line;
    },
    notAnImage: t`Choose a JPEG, PNG or WebP image.`,
    tooLarge: t`The image is over 25 MB.`,
    /** A refused upload's reason, from the field the server named. */
    uploadProblem: (error: unknown): string => {
      const field = isProblem(error) ? error.errors?.[0]?.code : undefined;
      if (field === "tooLarge") return t`The image is over 25 MB.`;
      if (field === "unsupported") return t`Choose a JPEG, PNG or WebP image.`;
      return describeProblem(errorCode(error));
    },
  };
}
