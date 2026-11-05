import { Trans } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { libraryQueries } from "@/api/libraries";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { AddLibraryForm } from "./AddLibraryForm";

/**
 * The setup wizard's libraries step. It lists the libraries added during
 * setup and shows the form. The step can be skipped.
 */
export function LibrarySetup() {
  const libraries = useQuery(libraryQueries.list());
  const added = libraries.data?.items ?? [];
  return (
    <div className="flex flex-col gap-6">
      <p className="text-text-muted">
        <Trans>Add the folders that hold your movies and shows. You can skip this and add libraries later.</Trans>
      </p>
      {added.length > 0 && (
        <ul className="flex flex-col gap-2">
          {added.map((library) => (
            <li key={library.id} className="flex items-center gap-3 rounded-md bg-surface-raised px-3 py-2">
              <LibraryKindIcon kind={library.kind} />
              <span className="flex-1 font-medium">{library.name}</span>
              <Icon name="check" className="size-4 text-success" />
            </li>
          ))}
        </ul>
      )}
      <AddLibraryForm onAdded={() => {}} />
    </div>
  );
}
