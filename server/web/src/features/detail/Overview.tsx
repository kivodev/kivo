import { Trans, useLingui } from "@lingui/react/macro";
import { type ReactNode, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ModalDialog } from "@/components/ModalDialog";
import { isTv } from "@/tv/flag";
import { TvFocusable } from "@/tv/focus";
import { TvDialogButton } from "@/tv/forms/dialog";
import { focusTv } from "@/tv/spatial";

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
  /** On TV, the lines shown beside the buttons before the full text panel, as an episode's page gives it more room. */
  tvLines?: 3 | 7;
  /** On TV, heads the full text panel, such as the work's title. */
  title?: ReactNode;
};

/**
 * A description or biography, clamped until More expands it. More shows
 * only when there's more to show.
 */
export function Overview(props: Props) {
  return isTv ? <TvOverview {...props} /> : <PageOverview {...props} />;
}

function PageOverview({ text, lines = 4, hold = false }: Props) {
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

/**
 * A description on TV, which shows as many lines as its page has room for.
 * Only when there's more does it take focus, the whole block, and OK opens
 * the full text in a panel beside the page. A remote can't aim at a small
 * More link, and expanding in place would move everything under it.
 */
function TvOverview({ text, tvLines = 3, hold = false, title }: Props) {
  const { t } = useLingui();
  const ref = useRef<HTMLParagraphElement>(null);
  const [clamped, setClamped] = useState(false);
  const [open, setOpen] = useState(false);

  // biome-ignore lint/correctness/useExhaustiveDependencies: a new text is measured again.
  useLayoutEffect(() => {
    const el = ref.current;
    if (el) setClamped(el.scrollHeight > el.clientHeight + 1);
  }, [text]);

  const held = hold ? (tvLines === 7 ? "min-h-7lh" : "min-h-3lh") : "";
  return (
    <div data-part="overview" className="relative pr-24">
      {/* Its paragraphs run together here, for the most text in its lines; the panel keeps them apart. */}
      <p ref={ref} className={`text-text-muted ${tvLines === 7 ? "line-clamp-7" : "line-clamp-3"} ${held}`}>
        {text}
      </p>
      {clamped && (
        <>
          {/* At the end of its last line, so the hint takes no line of its own. */}
          <span aria-hidden="true" className="absolute right-0 bottom-0 text-sm font-semibold text-text">
            <Trans>More</Trans>
          </span>
          <TvFocusable
            focusKey="overview"
            face="none"
            part="overview-open"
            className="absolute -inset-x-3 -inset-y-1 rounded-md"
          >
            <button
              type="button"
              aria-label={t`Read the full description`}
              className="size-full cursor-pointer"
              onClick={() => setOpen(true)}
            />
          </TvFocusable>
          <ModalDialog isOpen={open} onClose={() => setOpen(false)} title={title} placement="side" part="overview-full">
            <OverviewFull text={text} onClose={() => setOpen(false)} />
          </ModalDialog>
        </>
      )}
    </div>
  );
}

/**
 * The full text in its panel. Up and Down scroll it a few lines at a time,
 * at once, as a TV would paint a smooth scroll over and over, then go on to
 * Close at its end.
 */
function OverviewFull({ text, onClose }: { text: string | undefined; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => focusTv("overview-text"), []);
  const scroll = (direction: string) => {
    const el = ref.current;
    if (!el || (direction !== "up" && direction !== "down")) return false;
    const room = direction === "down" ? el.scrollTop + el.clientHeight < el.scrollHeight - 1 : el.scrollTop > 0;
    if (!room) return true;
    el.scrollTop += (direction === "down" ? 1 : -1) * Math.round(el.clientHeight * 0.6);
    return false;
  };
  return (
    <>
      <TvFocusable focusKey="overview-text" face="none" onArrow={scroll}>
        <div ref={ref} data-part="overview-full-text" className="overflow-hidden whitespace-pre-line text-text">
          {text}
        </div>
      </TvFocusable>
      <div className="mt-8 flex justify-end">
        <TvDialogButton focusKey="overview-close" onPress={onClose}>
          <Trans>Close</Trans>
        </TvDialogButton>
      </div>
    </>
  );
}
