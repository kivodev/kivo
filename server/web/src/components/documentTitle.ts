import { useEffect } from "react";

/**
 * The page's name in the browser tab and history, such as "Dune: Part Two
 * – Kivo", or "Kivo" alone for Home and the sign-in pages. Every screen
 * sets its own title, so no screen is left with an old one.
 */
export function useDocumentTitle(title: string | undefined): void {
  useEffect(() => {
    document.title = title ? `${title} – Kivo` : "Kivo";
  }, [title]);
}
