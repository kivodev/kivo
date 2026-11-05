import { Trans } from "@lingui/react/macro";
import { useQueryErrorResetBoundary } from "@tanstack/react-query";
import { type ErrorComponentProps, useRouter } from "@tanstack/react-router";
import { useEffect } from "react";
import { reportClientError } from "@/api/clientErrors";
import { Button } from "@/components/Button";
import { LoadError } from "@/components/LoadError";
import { isPackaged } from "@/tv/flag";
import { TvControl } from "@/tv/forms/TvControl";
import { packagedServer } from "@/tv/packaged";

/**
 * A route that failed to load. It says what failed and offers Try again, so
 * it's never a dead end. The packaged app offers Change server too, for a
 * server whose address has changed or that won't come back.
 */
export function RouteError({ error }: ErrorComponentProps) {
  const router = useRouter();
  const queryReset = useQueryErrorResetBoundary();
  // The route's template, such as /works/$id, never its URL.
  const route = router.state.matches.at(-1)?.fullPath;
  useEffect(() => {
    // A chunk that failed to load is reported as one; a problem or no answer from the server isn't reported.
    reportClientError("render", error, route);
  }, [error, route]);
  return (
    <LoadError
      error={error}
      onRetry={() => {
        queryReset.reset();
        void router.invalidate();
      }}
      alternative={
        isPackaged &&
        packagedServer() !== undefined && (
          <TvControl focusKey="load-error:change-server">
            <Button onPress={() => void router.navigate({ to: "/server" })}>
              <Trans>Change server</Trans>
            </Button>
          </TvControl>
        )
      }
    />
  );
}
