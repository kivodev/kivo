import { Trans } from "@lingui/react/macro";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { client } from "@/api/client";
import type { LiveState, LiveStatus } from "@/api/live";
import { setupQueries } from "@/api/setup";
import { Banner } from "@/components/Banner";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { isPackaged, isTv } from "@/tv/flag";
import { TvControl } from "@/tv/forms/TvControl";

/**
 * "Can't reach the server. Reconnecting…" while the server doesn't answer,
 * with Retry now on the web, and the update notice. On TV, with the mini
 * player docked, the notice sits above it.
 */
export function ConnectionBanner({ docked = false, ...live }: LiveState & { docked?: boolean }) {
  return <LiveBanner {...live} docked={docked} />;
}

/**
 * Whether the server doesn't answer, asked once an outage starts and again
 * at each failed attempt at the socket. The socket can fail while the
 * server answers, as behind a proxy that doesn't pass sockets on, and
 * saying it can't be reached then would be wrong. A question still out
 * when the next attempt fails is waited for, not asked again, as a server
 * that doesn't answer may take the client's whole timeout to say so. It
 * asks even when the browser says it's offline, which may be wrong both
 * ways. Each outage starts with no answer, so one the socket alone has
 * doesn't inherit an earlier one's.
 */
function useServerSilent(status: LiveStatus): boolean {
  const outage = status.state === "retrying" ? status.since : undefined;
  const attempt = status.state === "retrying" ? status.retryAt : undefined;
  const answer = useQuery({
    queryKey: ["live", "serverAnswers", outage],
    queryFn: () =>
      client.GET("/api/v1/server").then(
        ({ response }) => response.ok,
        () => false,
      ),
    enabled: outage !== undefined,
    networkMode: "always",
    retry: false,
    gcTime: 0,
  });
  const { refetch } = answer;
  useEffect(() => {
    if (attempt !== undefined) void refetch({ cancelRefetch: false });
  }, [attempt, refetch]);
  return answer.data === false;
}

function LiveBanner({ status, updateAvailable, retryNow, docked }: LiveState & { docked: boolean }) {
  const server = useSuspenseQuery(setupQueries.state()).data.serverName;
  const unreachable = useServerSilent(status) && status.state === "retrying";
  const reconnecting = server ? (
    <Trans>Can't reach {server}. Reconnecting…</Trans>
  ) : (
    <Trans>Can't reach the server. Reconnecting…</Trans>
  );

  if (unreachable && isTv) {
    // On TV it floats over the screen with nothing to press: the app tries
    // again by itself, and a strip in the page would move what's on it, or
    // sit under a screen laid out to the full height, as Search is.
    return (
      <div
        data-part="connection-notice"
        className={`pointer-events-none fixed right-(--layout-tv-overscan-x) left-(--layout-tv-content-left) z-40 flex justify-center ${
          docked ? "bottom-(--layout-tv-notice-docked-bottom)" : "bottom-(--layout-tv-overscan-y)"
        }`}
      >
        <div
          role="status"
          className="flex items-center gap-4 rounded-full border border-border bg-surface px-8 py-4 text-text shadow-overlay"
        >
          <Icon name="alert" className="size-7 shrink-0 text-warning" />
          {reconnecting}
        </div>
      </div>
    );
  }
  if (unreachable) {
    return (
      <div className="px-4 pt-4 sm:px-8">
        <Banner
          tone="warning"
          action={
            <Button size="sm" onPress={retryNow}>
              <Trans>Retry now</Trans>
            </Button>
          }
        >
          {reconnecting}
        </Banner>
      </div>
    );
  }
  // The packaged app would only run the same code again, so it offers no
  // reload. On TV the banner sits above the screen's
  // first row inside the content group, so Up from the first row reaches
  // Reload. It doesn't take focus itself: an update can land while someone
  // is mid-scroll or typing, and a focus jump would eat their next key press.
  if (updateAvailable && !isPackaged) {
    return (
      <div className="px-4 pt-4 sm:px-8">
        <Banner
          action={
            <TvControl focusKey="banner:reload">
              <Button size="sm" onPress={() => window.location.reload()}>
                <Trans>Reload</Trans>
              </Button>
            </TvControl>
          }
        >
          <Trans>The server was updated. Reload to use the new version.</Trans>
        </Banner>
      </div>
    );
  }
  return null;
}
