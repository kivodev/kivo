import { type ReactNode, useId } from "react";
import { useDocumentTitle } from "@/components/documentTitle";

type Props = {
  /** Which page, for tests: `overview`, `libraries` and so on. */
  page: string;
  /** The heading, which names the browser tab too. */
  title: string;
  /** What shows beside the title in the heading, such as a user's Admin and Disabled badges. */
  badges?: ReactNode;
  /** A muted line under the title, such as when Tasks' nightly tasks run. */
  description?: ReactNode;
  /** The page's own actions, beside the title, such as Add library. */
  actions?: ReactNode;
  /** The parent page, as a link over the title, such as "Users ›" on a user's page. */
  parent?: ReactNode;
  /** Narrower than a page, for a page of tables such as Tasks. */
  narrow?: boolean;
  children: ReactNode;
};

/** An admin page: its title, its actions, then its content. */
export function AdminPage({ page, title, badges, description, actions, parent, narrow = false, children }: Props) {
  const titleId = useId();
  useDocumentTitle(title);
  return (
    <section
      data-part="admin-page"
      data-page={page}
      aria-labelledby={titleId}
      className={`flex flex-col gap-6 ${narrow ? "max-w-256" : "max-w-page"}`}
    >
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-1">
          {parent && <div className="text-sm text-text-muted">{parent}</div>}
          <h1 id={titleId} className="text-2xl font-semibold">
            {badges ? (
              <span className="flex flex-wrap items-center gap-3">
                {title}
                {badges}
              </span>
            ) : (
              title
            )}
          </h1>
          {description && <p className="text-sm text-text-muted">{description}</p>}
        </div>
        {actions}
      </header>
      {children}
    </section>
  );
}

/** One entry of an admin list, as a bordered row with its text and then its actions. */
export function AdminRow({ part, children, actions }: { part?: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <li
      data-part={part}
      className="flex flex-wrap items-start gap-x-4 gap-y-2 rounded-lg border border-border bg-surface px-4 py-3"
    >
      <div className="flex min-w-0 flex-1 flex-col gap-1">{children}</div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </li>
  );
}

/**
 * Compact entries in one bordered box, divided by lines. The Overview's
 * sections and the Libraries page use it.
 */
export function AdminList({ children }: { children: ReactNode }) {
  return (
    <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-surface">{children}</ul>
  );
}

/** One entry of an AdminList. A ProgressLine along its edge sits on its bottom line. */
export function AdminListItem({ part, children }: { part?: string; children: ReactNode }) {
  return (
    <li data-part={part} className="relative px-4 py-2.5 text-sm">
      {children}
    </li>
  );
}

/**
 * A table in a bordered box, with its column headers, if any, and then its
 * rows. `columns` are the columns' widths. The first is left empty so it
 * fills the rest, which lets tables with the same columns line up one
 * under another. The cells keep to one line. On a narrow screen the table
 * scrolls sideways in its box.
 */
export function AdminTable({ head, columns, children }: { head?: ReactNode; columns: string[]; children: ReactNode }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-border bg-surface">
      <table className="w-full min-w-216 table-fixed text-sm">
        <colgroup>
          {columns.map((width, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: the columns are fixed.
            <col key={i} className={width} />
          ))}
        </colgroup>
        {head && (
          <thead>
            <tr className="border-b border-border">{head}</tr>
          </thead>
        )}
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}

/** A column's header in an AdminTable. */
export function AdminColumn({ children }: { children?: ReactNode }) {
  return (
    <th
      scope="col"
      className="px-3 py-2.5 text-left text-xs font-semibold whitespace-nowrap text-text-muted first:pl-4"
    >
      {children}
    </th>
  );
}

/** A row of an AdminTable. Every row but the first has a line above it. */
export function AdminTableRow({ part, children }: { part?: string; children: ReactNode }) {
  return (
    <tr data-part={part} className="border-t border-border first:border-t-0">
      {children}
    </tr>
  );
}

/** A cell of an AdminTable. `action` marks the last cell, which holds the row's action at its end. */
export function AdminCell({ action = false, children }: { action?: boolean; children?: ReactNode }) {
  return (
    <td className={`px-3 whitespace-nowrap first:pl-4 last:pr-4 ${action ? "py-2 text-right" : "py-3"}`}>{children}</td>
  );
}
