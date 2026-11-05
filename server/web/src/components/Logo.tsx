/**
 * Kivo's logo in the theme's colours: the Facet K beside "Kivo", or the
 * mark alone where the lockup doesn't fit. The drawings are the server's
 * static files, made from shared/brand, each in its exact tints for a dark
 * and a light background. The lockup's drawing is cropped to its artwork,
 * so its height is the logo's; the mark's keeps a square around it, as an
 * icon does. The height comes from the class. Each image has its
 * drawing's proportions, so it takes its width before the drawing has
 * loaded, rather than growing into it.
 */
export function Logo({ mark = false, className = "" }: { mark?: boolean; className?: string }) {
  const file = mark ? "mark" : "lockup";
  const size = mark ? { width: 64, height: 64 } : { width: 165, height: 46 };
  return (
    <span data-part="logo" className={`inline-flex shrink-0 ${className}`}>
      <img src={`${file}.svg`} alt="Kivo" {...size} className="h-full w-auto light:hidden" />
      <img src={`${file}-light.svg`} alt="Kivo" {...size} className="hidden h-full w-auto light:block" />
    </span>
  );
}
