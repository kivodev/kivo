import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { useDeferredValue, useLayoutEffect, useRef, useState } from "react";
import { Input, SearchField } from "react-aria-components";
import { type LogLevel, logFileUrl, logQueries } from "@/api/logs";
import type { LogLine } from "@/api/schemas";
import { buttonClass } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { Select } from "@/components/Select";
import { AdminPage } from "./AdminPage";

/** Each level's colour, from the text tokens. */
const levelClass: Record<string, string> = {
  error: "text-danger",
  warning: "text-warning",
  info: "text-text-muted",
  debug: "text-text-subtle",
};

/** How close to the bottom counts as at the bottom, in pixels. */
const bottomSlack = 24;

/**
 * The Logs page. It shows the server's recent log lines, newest last,
 * filtered by level and text. Live events keep it current while the page
 * is open. A download of the whole log is offered for a bug report.
 */
export function LogsScreen() {
  const { t } = useLingui();
  const [level, setLevel] = useState<LogLevel>("info");
  const [text, setText] = useState("");
  // The lines shown stay until the lines for the new level or search
  // arrive.
  const shownLevel = useDeferredValue(level);
  const search = useDeferredValue(text.trim());
  const lines = useSuspenseQuery(logQueries.list(shownLevel, search)).data.items;
  const levels: { id: LogLevel; label: string }[] = [
    { id: "error", label: t`Errors` },
    { id: "warning", label: t`Warnings` },
    { id: "info", label: t`Info` },
    { id: "debug", label: t`Debug` },
  ];

  return (
    <AdminPage
      page="logs"
      title={t`Logs`}
      actions={
        <a href={logFileUrl()} download className={`${buttonClass("secondary", "sm")} gap-2`}>
          <Icon name="download" className="size-4" />
          <Trans>Download</Trans>
        </a>
      }
    >
      <div className="flex flex-wrap items-center gap-4">
        <Select
          variant="inline"
          label={t`Level`}
          options={levels}
          selected={level}
          onChange={(id) => setLevel(id as LogLevel)}
        />
        <SearchField
          value={text}
          onChange={setText}
          aria-label={t`Search the log`}
          className="flex min-w-60 flex-1 items-center gap-2 rounded-md border border-border bg-surface px-3"
        >
          <Icon name="search" className="size-4 text-text-muted" />
          <Input
            placeholder={t`Search the log`}
            className="h-9 min-w-0 flex-1 bg-transparent text-sm text-text outline-none placeholder:text-text-subtle pointer-coarse:text-base"
          />
        </SearchField>
      </div>
      {level === "debug" && (
        <p className="text-sm text-text-muted">
          <Trans>Debug lines appear when the server's log level is debug.</Trans>
        </p>
      )}
      <LogList lines={lines} />
    </AdminPage>
  );
}

/**
 * The lines in a scrolling box that follows the newest, unless the admin
 * has scrolled up to read.
 */
function LogList({ lines }: { lines: LogLine[] }) {
  const { i18n } = useLingui();
  const box = useRef<HTMLDivElement>(null);
  const following = useRef(true);

  useLayoutEffect(() => {
    const el = box.current;
    if (el && following.current && lines.length > 0) el.scrollTop = el.scrollHeight;
  }, [lines]);

  if (lines.length === 0) {
    return (
      <p className="text-sm text-text-muted">
        <Trans>No log lines match.</Trans>
      </p>
    );
  }
  return (
    <div
      ref={box}
      data-part="log-lines"
      onScroll={(e) => {
        const el = e.currentTarget;
        following.current = el.scrollHeight - el.scrollTop - el.clientHeight < bottomSlack;
      }}
      className="h-160 overflow-auto rounded-md border border-border bg-surface px-3 py-2 font-mono text-xs"
    >
      <ol className="flex flex-col gap-0.5">
        {lines.map((line) => (
          <li key={line.id} data-part="log-line" data-level={line.level} className="break-all whitespace-pre-wrap">
            <span className="text-text-subtle">{i18n.date(line.loggedAt, { timeStyle: "medium" })}</span>{" "}
            <span className={`font-semibold uppercase ${levelClass[line.level] ?? "text-text-muted"}`}>
              {line.level}
            </span>{" "}
            <span className="text-text">{line.message}</span>
            <span className="text-text-muted">{line.attributes.map((a) => ` ${a.key}=${a.value}`).join("")}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}
