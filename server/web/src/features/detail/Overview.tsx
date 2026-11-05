import { Trans } from "@lingui/react/macro";
import { useLayoutEffect, useRef, useState } from "react";
import { TvFocusable } from "@/tv/focus";

type Props = {
  text: string | undefined;
  /** Lines shown until More expands it. */
  lines?: 3 | 4;
  /**
   * Keeps room for the collapsed lines and the More line even when the
   * text is shorter or missing, so stepping between episodes keeps the
   * hero's height.
   */
  hold?: boolean;
};

/**
 * A description or biography, clamped until More expands it. More shows
 * only when there's more to show.
 */
export function Overview({ text, lines = 4, hold = false }: Props) {
  const ref = useRef<HTMLParagraphElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [clamped, setClamped] = useState(false);

  // biome-ignore lint/correctness/useExhaustiveDependencies: a new text is measured again.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => setClamped(el.scrollHeight > el.clientHeight + 1);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [text]);

  return (
    <div className="flex flex-col items-start gap-1">
      <p
        ref={ref}
        className={`whitespace-pre-line text-text-muted ${expanded ? "" : lines === 3 ? "line-clamp-3" : "line-clamp-4"} ${hold ? "min-h-3lh" : ""}`}
      >
        {text}
      </p>
      {expanded || clamped ? (
        <TvFocusable focusKey="overview-more">
          <button
            type="button"
            className="cursor-pointer text-sm font-semibold text-text hover:text-accent"
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? <Trans>Less</Trans> : <Trans>More</Trans>}
          </button>
        </TvFocusable>
      ) : hold ? (
        <span aria-hidden="true" className="invisible text-sm">
          <Trans>More</Trans>
        </span>
      ) : null}
    </div>
  );
}
