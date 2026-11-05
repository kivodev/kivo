import { infiniteQueryOptions } from "@tanstack/react-query";

/**
 * A list read one keyset page at a time. `page` fetches the page after a
 * cursor, or the first page without one. Each page's `next` is the cursor
 * of the page after it.
 */
export function keysetPages<Key extends readonly unknown[], Page extends { next?: string }>(
  queryKey: Key,
  page: (after: string | undefined, signal: AbortSignal) => Promise<Page>,
) {
  return infiniteQueryOptions({
    queryKey,
    queryFn: ({ pageParam, signal }) => page(pageParam === "" ? undefined : pageParam, signal),
    initialPageParam: "",
    getNextPageParam: (last: Page) => last.next,
  });
}
