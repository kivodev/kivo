import { useSyncExternalStore } from "react";

// The phone and wide variants in index.css use the same media queries.
const phoneQuery = "(width < 40rem) or ((height < 32rem) and (pointer: coarse))";
const railQuery = "(width >= 40rem) and (width < 64rem) and ((height >= 32rem) or (not (pointer: coarse)))";

function useMediaQuery(query: string): boolean {
  const subscribe = (onChange: () => void) => {
    const list = globalThis.matchMedia?.(query);
    list?.addEventListener("change", onChange);
    return () => list?.removeEventListener("change", onChange);
  };
  return useSyncExternalStore(subscribe, () => globalThis.matchMedia?.(query).matches ?? false);
}

/**
 * Whether the page has the phone layout, with the bottom bar. A narrow
 * screen or a short touch screen gets it.
 */
export function usePhoneLayout(): boolean {
  return useMediaQuery(phoneQuery);
}

/** Whether the sidebar is collapsed to its icons, below the lg breakpoint of the wide layout. */
export function useSidebarRail(): boolean {
  return useMediaQuery(railQuery);
}
