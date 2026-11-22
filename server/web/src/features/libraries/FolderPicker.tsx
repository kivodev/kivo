import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Button as AriaButton, Input, TextField } from "react-aria-components";
import { directoryQueries } from "@/api/directories";
import { errorCode } from "@/api/problem";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { describeProblem } from "@/i18n/problems";

// parentOf is the folder above path, or undefined above a root, where the
// picker lists where it starts. A Windows server's paths have a drive and
// backslashes, and the parent of C:\Movies is C:\.
function parentOf(path: string): string | undefined {
  const trimmed = path.replace(/[\\/]+$/, "");
  const i = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  if (i < 0) return undefined;
  return trimmed.slice(0, i === 0 || trimmed[i - 1] === ":" ? i + 1 : i);
}

type Props = {
  /** Shown only while open. It keeps its place while closed. */
  isOpen: boolean;
  /** The folders chosen so far, which can't be chosen again. */
  chosen: readonly string[];
  onPick: (path: string) => void;
  onClose: () => void;
};

/**
 * Picks one of the server's folders. It browses from the server's mounts,
 * or goes to a typed path on Enter. After a pick it shows the folder
 * above, where the next one is likely to be.
 */
export function FolderPicker({ isOpen, chosen, onPick, onClose }: Props) {
  const { t } = useLingui();
  const [path, setPath] = useState<string | undefined>(undefined);
  const [typed, setTyped] = useState("");
  const folders = useQuery(directoryQueries.list(path));
  const go = (to: string | undefined) => {
    setPath(to);
    setTyped(to ?? "");
  };

  if (!isOpen) return null;
  const taken = path !== undefined && chosen.includes(path);
  return (
    <div data-part="folder-picker" className="flex flex-col gap-2 rounded-md border border-accent bg-bg p-2">
      <div className="flex items-center gap-2 px-1">
        <Button
          size="sm"
          variant="ghost"
          aria-label={t`Up one folder`}
          isDisabled={path === undefined}
          onPress={() => go(path === undefined ? undefined : parentOf(path))}
        >
          <Icon name="chevronLeft" className="size-4" />
        </Button>
        <TextField aria-label={t`Folder path`} value={typed} onChange={setTyped} className="min-w-0 flex-1">
          <Input
            placeholder={t`Browse server folders or enter a path`}
            onKeyDown={(e) => {
              if (e.key !== "Enter") return;
              e.preventDefault();
              const to = typed.trim();
              go(to === "" ? undefined : to);
            }}
            className="h-8 w-full rounded-sm border border-transparent bg-transparent px-1 font-mono text-sm text-text outline-none placeholder:text-text-muted data-focused:border-accent pointer-coarse:text-base"
          />
        </TextField>
      </div>
      <ul className="max-h-60 overflow-auto">
        {folders.isPending && (
          <li className="px-3 py-2 text-sm text-text-muted">
            <Trans>Loading folders…</Trans>
          </li>
        )}
        {folders.isError && (
          <li role="alert" className="px-3 py-2 text-sm text-danger">
            {describeProblem(errorCode(folders.error))}
          </li>
        )}
        {folders.data?.items.length === 0 && (
          <li className="px-3 py-2 text-sm text-text-muted">
            <Trans>No folders inside.</Trans>
          </li>
        )}
        {folders.data?.items.map((dir) => (
          <li key={dir.path}>
            <AriaButton
              onPress={() => go(dir.path)}
              className="flex w-full cursor-pointer items-center gap-2 rounded-sm px-3 py-2 text-left text-sm text-text hover:bg-surface-hover"
            >
              <Icon name="folder" className="size-4 text-text-muted" />
              {dir.name}
            </AriaButton>
          </li>
        ))}
      </ul>
      <div className="flex justify-end gap-2 border-t border-border px-1 pt-2">
        <Button size="sm" variant="ghost" onPress={onClose}>
          <Trans>Close</Trans>
        </Button>
        <Button
          size="sm"
          variant="primary"
          isDisabled={path === undefined || taken}
          onPress={() => {
            if (path === undefined) return;
            go(parentOf(path));
            onPick(path);
          }}
        >
          {path === undefined ? (
            <Trans>Open a folder first</Trans>
          ) : taken ? (
            <Trans>Already added</Trans>
          ) : (
            <Trans>Use this folder</Trans>
          )}
        </Button>
      </div>
    </div>
  );
}
