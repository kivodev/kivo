import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Checkbox, CheckboxGroup, Text } from "react-aria-components";
import { libraryQueries } from "@/api/libraries";
import type { Library, LibraryAccess, UserLimits } from "@/api/schemas";
import { useQualityLadder } from "@/api/server";
import { CheckBox } from "@/components/CheckBox";
import { Select } from "@/components/Select";
import { SettingsRow } from "@/components/SettingsLayout";
import { Switch } from "@/components/Switch";
import { formatBitrate, formatRung } from "@/i18n/format";
import { limitKey, limitOf, useLimitOptions } from "./limitOptions";

/** Nothing chosen yet, neither every library nor any one of them, so Save waits. */
export const noAccess: LibraryAccess = { all: false, libraryIds: [] };

export const accessChosen = (a: LibraryAccess) => a.all || a.libraryIds.length > 0;

/** "All libraries", or the libraries' names in the admin's order. */
export function useLibrariesSummary(): (a: LibraryAccess) => string {
  const { t } = useLingui();
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  return (a) =>
    a.all
      ? t`All libraries`
      : libraries
          .filter((l) => a.libraryIds.includes(l.id))
          .map((l) => l.name)
          .join(", ") || t`No libraries`;
}

/**
 * Which libraries a user sees, as rows of a settings card. All libraries
 * is a switch that includes libraries added later. While it's off, the
 * libraries are listed to tick, in the admin's order.
 */
export function LibraryAccessRows({ value, onChange }: { value: LibraryAccess; onChange: (a: LibraryAccess) => void }) {
  const { t } = useLingui();
  const libraries: Library[] = useSuspenseQuery(libraryQueries.list()).data.items;
  return (
    <>
      <SettingsRow label={<Trans>All libraries</Trans>} description={<Trans>Includes libraries added later.</Trans>}>
        <Switch isSelected={value.all} onChange={(all) => onChange({ all, libraryIds: all ? [] : value.libraryIds })} />
      </SettingsRow>
      {!value.all && (
        <CheckboxGroup
          value={value.libraryIds}
          onChange={(libraryIds) => onChange({ all: false, libraryIds })}
          className="flex flex-col gap-2.5 px-4 py-3.5"
          aria-label={t`Libraries`}
        >
          {libraries.map((l) => (
            <Checkbox
              key={l.id}
              value={l.id}
              className="group flex cursor-pointer items-center gap-2 text-sm text-text"
            >
              <CheckBox />
              {l.name}
            </Checkbox>
          ))}
          {!accessChosen(value) && (
            <Text slot="description" className="text-sm text-text-muted">
              <Trans>Select at least one library.</Trans>
            </Text>
          )}
        </CheckboxGroup>
      )}
    </>
  );
}

/** "Up to 2 streams · Remote up to 8 Mb/s", or nothing when there are none. */
export function useLimitsSummary(): (l: UserLimits) => string {
  const { t } = useLingui();
  return ({ maxStreams, remoteQualityCapBps }) => {
    const parts = [
      maxStreams ? plural(maxStreams, { one: "Up to # stream", other: "Up to # streams" }) : undefined,
      remoteQualityCapBps ? t`Remote up to ${formatBitrate(remoteQualityCapBps)}` : undefined,
    ];
    return parts.filter(Boolean).join(" · ");
  };
}

// The select's ID for no remote streaming limit of the user's own, when
// the server's applies.
const noCap = "none";

/** Simultaneous streams and the remote streaming limit, as rows of a settings card. */
export function LimitsRows({ value, onChange }: { value: UserLimits; onChange: (l: UserLimits) => void }) {
  const { t } = useLingui();
  const qualityLadder = useQualityLadder();
  const streams = useLimitOptions(value.maxStreams);
  const qualities = [
    { id: noCap, label: t`Server's limit` },
    ...qualityLadder.map((rung) => ({ id: String(rung.bitrateBps), label: formatRung(rung) })),
  ];
  return (
    <>
      <SettingsRow label={<Trans>Simultaneous streams</Trans>}>
        <Select
          options={streams}
          selected={limitKey(value.maxStreams)}
          onChange={(id) => onChange(withLimit(value, "maxStreams", limitOf(id)))}
        />
      </SettingsRow>
      <SettingsRow label={<Trans>Remote streaming limit</Trans>}>
        <Select
          options={qualities}
          selected={value.remoteQualityCapBps ? String(value.remoteQualityCapBps) : noCap}
          onChange={(id) => onChange(withLimit(value, "remoteQualityCapBps", id === noCap ? undefined : Number(id)))}
        />
      </SettingsRow>
    </>
  );
}

/** Returns limits with the limit at key set to n, or removed when n is undefined. */
function withLimit(limits: UserLimits, key: keyof UserLimits, n: number | undefined): UserLimits {
  const { [key]: _, ...rest } = limits;
  return n === undefined ? rest : { ...rest, [key]: n };
}
