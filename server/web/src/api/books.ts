import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { invalidatePaths, workViewPaths } from "./paths";

export const bookQueries = {
  /** An author's or narrator's page: the books they wrote, grouped by series, and the books they narrated. */
  person: (id: string) => $api.queryOptions("get", "/api/v1/people/{id}", { params: { path: { id } } }),
  /** A book series' page: its books in series order. */
  series: (id: string) => $api.queryOptions("get", "/api/v1/book-series/{id}", { params: { path: { id } } }),
};

/** Refetches every page and list that shows books, after books change. */
function useBooksChanged() {
  const queryClient = useQueryClient();
  return () => invalidatePaths(queryClient, workViewPaths);
}

/**
 * Rename an author or narrator. If someone else already has the name, the
 * two are merged and the answer is that other person.
 */
export function useRenamePerson() {
  const onSuccess = useBooksChanged();
  return $api.useMutation("patch", "/api/v1/people/{id}", { onSuccess });
}

/**
 * Rename a book series. If another series already has the name, the two
 * are merged and the answer is that other series.
 */
export function useRenameSeries() {
  const onSuccess = useBooksChanged();
  return $api.useMutation("patch", "/api/v1/book-series/{id}", { onSuccess });
}

/** A batch edit of books. Each field sent is written to every book and locked there. */
export function useUpdateBooks() {
  const onSuccess = useBooksChanged();
  return $api.useMutation("patch", "/api/v1/works", { onSuccess });
}
