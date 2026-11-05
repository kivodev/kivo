import { Trans, useLingui } from "@lingui/react/macro";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { FolderPicker } from "./FolderPicker";

type Props = {
  folders: readonly string[];
  onChange: (folders: string[]) => void;
};

/** A library's folders. It lists the folders chosen, each removable, and the picker to add another. */
export function FoldersField({ folders, onChange }: Props) {
  const { t } = useLingui();
  return (
    <div className="flex flex-col gap-2">
      <span className="text-sm font-medium text-text">
        <Trans>Folders</Trans>
      </span>
      {folders.length > 0 && (
        <ul className="flex flex-col gap-1">
          {folders.map((f) => (
            <li key={f} className="flex items-center gap-2 rounded-md bg-surface-raised px-3 py-1.5">
              <Icon name="folder" className="size-4 text-text-muted" />
              <span className="min-w-0 flex-1 truncate font-mono text-sm">{f}</span>
              <Button
                size="sm"
                variant="ghost"
                aria-label={t`Remove ${f}`}
                onPress={() => onChange(folders.filter((x) => x !== f))}
              >
                <Icon name="close" className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <FolderPicker onPick={(p) => !folders.includes(p) && onChange([...folders, p])} />
    </div>
  );
}
