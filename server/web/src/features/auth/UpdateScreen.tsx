import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { probeCompatibility, serverQueries, useProbeServer } from "@/api/server";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { TvExitOnBack } from "@/tv/ExitDialog";
import { TvFocusGroup } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { packagedServer } from "@/tv/packaged";
import { focusTv } from "@/tv/spatial";

/**
 * The packaged TV app's screen for a server it doesn't work with, in place
 * of sign-in and of the server's screens: which of the two needs an
 * update. Try again asks the server what it is again and goes on once they
 * work together, and Change server opens the address screen.
 */
export function UpdateScreen() {
  const { t } = useLingui();
  useDocumentTitle(undefined);
  const router = useRouter();
  const probeServer = useProbeServer();
  const address = packagedServer()?.address ?? "";
  const probe = useQuery(serverQueries.probe(address)).data;
  const [asking, setAsking] = useState(false);
  const found = probe?.result === "kivo" ? probe : undefined;
  const problem = found && probeCompatibility(found);

  const retry = async () => {
    if (asking) return;
    setAsking(true);
    try {
      await probeServer(address);
      // The router's check as the page opens decides where to go.
      await router.navigate({ to: "/", replace: true });
    } finally {
      setAsking(false);
    }
  };

  let title = "";
  let sentence = "";
  if (found && problem?.state === "serverTooOld") {
    title = t`This server needs an update`;
    sentence = t`Kivo ${problem.appVersion} works with server ${problem.minServerVersion} or newer. ${found.name} runs ${problem.serverVersion}. Ask whoever runs it to update.`;
  } else if (found && problem?.state === "appTooOld") {
    title = t`Update Kivo`;
    sentence = t`${found.name} needs Kivo ${problem.minAppVersion} or newer. You have ${problem.appVersion}.`;
  }

  // Try again has focus as the screen opens, to ask again once the update is done.
  const shown = title !== "";
  useEffect(() => {
    if (shown) focusTv("update:retry");
  }, [shown]);

  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg p-4 tv:px-(--layout-tv-overscan-x) tv:py-(--layout-tv-overscan-y)">
      <TvExitOnBack />
      {shown && (
        <TvFocusGroup focusKey="update" part="update">
          <EmptyState
            title={title}
            action={
              <div className="flex justify-center gap-4">
                <TvControl focusKey="update:retry">
                  <Button variant="primary" onPress={() => void retry()}>
                    <Trans>Try again</Trans>
                  </Button>
                </TvControl>
                <TvControl focusKey="update:change-server">
                  <Button onPress={() => void router.navigate({ to: "/server" })}>
                    <Trans>Change server</Trans>
                  </Button>
                </TvControl>
              </div>
            }
          >
            {sentence}
          </EmptyState>
        </TvFocusGroup>
      )}
    </main>
  );
}
