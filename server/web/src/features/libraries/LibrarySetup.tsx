import { Trans } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { libraryQueries } from "@/api/libraries";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { AddLibraryButton } from "./AddLibraryButton";

/**
 * The setup wizard's libraries step. It lists the libraries added during
 * setup, each with its folders, and has Add library for another. The step
 * can be skipped.
 */
export function LibrarySetup() {
  const libraries = useQuery(libraryQueries.list());
  const added = libraries.data?.items ?? [];
  return (
    <div className="flex flex-col gap-6">
      <p className="text-text-muted">
        <Trans>A library is a set of folders that hold one kind of media, such as your movies.</Trans>
      </p>
      {added.length === 0 ? (
        <div className="flex flex-col items-center gap-4 rounded-md border border-dashed border-border p-6">
          <p className="text-text-muted">
            <Trans>No libraries yet</Trans>
          </p>
          <AddLibraryButton />
        </div>
      ) : (
        <>
          <ul className="flex flex-col gap-2">
            {added.map((library) => (
              <li key={library.id} className="flex items-center gap-3 rounded-md bg-surface-raised px-3 py-2">
                <LibraryKindIcon kind={library.kind} />
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="font-medium">{library.name}</span>
                  {library.folders?.map((folder) => (
                    <span key={folder.id} className="font-mono text-xs break-all text-text-muted">
                      {folder.path}
                    </span>
                  ))}
                </div>
                <Icon name="check" className="size-4 text-success" />
              </li>
            ))}
          </ul>
          <div>
            <AddLibraryButton variant="secondary">
              <Trans>Add another library</Trans>
            </AddLibraryButton>
          </div>
        </>
      )}
    </div>
  );
}
