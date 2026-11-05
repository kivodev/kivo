import { Trans } from "@lingui/react/macro";
import type { ReactNode } from "react";
import { SettingsCard, SettingsRow } from "@/components/SettingsLayout";
import { isPackaged } from "@/tv/flag";
import { TvControl } from "@/tv/forms/TvControl";
import { appVersion } from "@/tv/versions";

/** A row that only shows a value. Focus stops on the whole row, so the remote and Voice Guide reach it, label and all. */
function Row({ id, label, value }: { id: string; label: ReactNode; value: string }) {
  return (
    <TvControl focusKey={`about:version:${id}`} kind="row" part="version" onPress={() => {}}>
      <SettingsRow label={label} focusable={false}>
        <span className="truncate text-sm text-text-muted">{value}</span>
      </SettingsRow>
    </TvControl>
  );
}

/**
 * Settings › About's versions on TV. The packaged app is a release of its
 * own, so it shows both its version and the server's. The TV flag's dev
 * server, unpackaged, shows the server's alone.
 */
export function TvVersions({ serverVersion }: { serverVersion: string }) {
  return (
    <SettingsCard part="tv-versions">
      {isPackaged ? (
        <>
          <Row id="app" label={<Trans>App version</Trans>} value={appVersion} />
          <Row id="server" label={<Trans>Server version</Trans>} value={serverVersion} />
        </>
      ) : (
        <Row id="server" label={<Trans>Version</Trans>} value={serverVersion} />
      )}
    </SettingsCard>
  );
}
