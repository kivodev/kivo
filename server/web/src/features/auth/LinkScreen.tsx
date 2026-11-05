import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { type ReactNode, useState } from "react";
import { errorCode } from "@/api/problem";
import { serverQueries } from "@/api/server";
import { deviceCodeQueries, useApproveDeviceCode, useDenyDeviceCode } from "@/api/session";
import { Button, buttonClass } from "@/components/Button";
import { usePlatformName } from "@/components/deviceCode";
import { useDocumentTitle } from "@/components/documentTitle";
import { FormError } from "@/components/FormError";
import { Logo } from "@/components/Logo";
import { ServerAddress, serverAddress } from "@/components/ServerAddress";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { formatTimeAgo } from "@/i18n/format";
import { describeProblem } from "@/i18n/problems";
import { currentBrowser } from "@/lib/browser";
import { TvExitOnBack } from "@/tv/ExitDialog";

const linkRoute = getRouteApi("/link");

/**
 * The /link page, Link a device. The signed-in user answers another
 * device's code here, from its QR code or typed in. The page shows who is
 * asking, from where and when, then Allow or Don't allow. Without a code
 * it asks for one. A code for another server isn't looked up. On a phone
 * it offers the app first.
 */
export function LinkScreen() {
  useDocumentTitle(undefined);
  const search = linkRoute.useSearch();
  // An empty code or server in the URL counts as none.
  const code = search.code || undefined;
  const server = search.server || undefined;
  const { username } = linkRoute.useRouteContext().user;
  const info = useQuery(serverQueries.info()).data;
  const elsewhere = code !== undefined && server !== undefined && info !== undefined && server !== info.id;
  const [inBrowser, setInBrowser] = useState(() => {
    const browser = currentBrowser();
    return !browser.mobile || browser.tv;
  });
  const waiting = useQuery({
    ...deviceCodeQueries.waiting(code ?? ""),
    enabled: code !== undefined && info !== undefined && !elsewhere && inBrowser,
  });
  const approve = useApproveDeviceCode();
  const deny = useDenyDeviceCode();
  const failed = waiting.isError || approve.isError || deny.isError;
  const error = waiting.error ?? approve.error ?? deny.error;
  const name = waiting.data?.name ?? "";
  const serverName = info?.name ?? "";

  let content: ReactNode;
  if (code !== undefined && !inBrowser) {
    const app = `kivo://link?${new URLSearchParams({ code, ...(server && { server }) })}`;
    content = (
      <>
        <Heading>
          <Trans>Link a device</Trans>
        </Heading>
        <div className="flex flex-col gap-3">
          <a href={app} className={buttonClass("primary")}>
            <Trans>Open in the Kivo app</Trans>
          </a>
          <Button onPress={() => setInBrowser(true)}>
            <Trans>Continue in browser</Trans>
          </Button>
        </div>
      </>
    );
  } else if (elsewhere) {
    content = (
      <>
        <Heading>
          <Trans>Link a device</Trans>
        </Heading>
        <p className="mb-6 text-text-muted">
          <Trans>This device is signing in to another server. You're signed in to {serverName}.</Trans>
        </p>
        <HomeLink />
      </>
    );
  } else if (code === undefined || failed) {
    content = (
      <>
        <Heading>
          <Trans>Link a device</Trans>
        </Heading>
        {failed && (
          <FormError className="mb-4">
            {errorCode(error) === "notFound" ? (
              <Trans>That code didn't work. Check the other device for a new one.</Trans>
            ) : (
              describeProblem(errorCode(error))
            )}
          </FormError>
        )}
        <CodeForm />
        {info && <ShowAddress address={serverAddress(info)} />}
      </>
    );
  } else if (approve.isSuccess || deny.isSuccess) {
    content = (
      <>
        <Heading>
          {deny.isSuccess ? <Trans>{name} wasn't signed in</Trans> : <Trans>{name} is signed in</Trans>}
        </Heading>
        <p className="mb-6 text-text-muted">
          {deny.isSuccess ? (
            <Trans>You can close this page.</Trans>
          ) : (
            <Trans>
              {name} is signed in as {username}. You can close this page.
            </Trans>
          )}
        </p>
        <HomeLink />
      </>
    );
  } else if (waiting.data) {
    const busy = approve.isPending || deny.isPending;
    const path = { params: { path: { code } } };
    content = (
      <>
        <h1 className="mb-2 text-2xl font-semibold">
          <Trans>
            Allow {name} to sign in as {username}?
          </Trans>
        </h1>
        <Asking platform={waiting.data.platform} nearby={waiting.data.nearby} startedAt={waiting.data.startedAt} />
        <div className="mt-6 flex gap-3">
          <Button
            variant="primary"
            isPending={approve.isPending}
            isDisabled={busy}
            onPress={() => approve.mutate(path)}
          >
            <Trans>Allow</Trans>
          </Button>
          <Button isPending={deny.isPending} isDisabled={busy} onPress={() => deny.mutate(path)}>
            <Trans>Don't allow</Trans>
          </Button>
        </div>
      </>
    );
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg p-4">
      <TvExitOnBack />
      <div
        data-part="auth-panel"
        className="w-full max-w-form rounded-lg border border-border bg-surface p-8 shadow-overlay"
      >
        <Logo className="mb-6 h-7" />
        {content}
      </div>
    </main>
  );
}

function Heading({ children }: { children: ReactNode }) {
  return <h1 className="mb-4 text-2xl font-semibold">{children}</h1>;
}

function HomeLink() {
  return (
    <Link to="/" className={buttonClass("secondary")}>
      <Trans>Go to Home</Trans>
    </Link>
  );
}

/**
 * What the server saw of the asking device, such as "Android TV · On your
 * home network · Just now". It adds a warning when the device isn't on the
 * home network.
 */
function Asking({ platform, nearby, startedAt }: { platform: string | undefined; nearby: boolean; startedAt: string }) {
  const { t } = useLingui();
  const platformName = usePlatformName();
  const [now] = useState(Date.now);
  const ago = now - Date.parse(startedAt);
  const facts = [
    platformName(platform),
    nearby ? t`On your home network` : t`Not on your home network`,
    ago < 60_000 ? t`Just now` : formatTimeAgo(ago),
  ];
  return (
    <>
      <p data-part="asking" className="text-text-muted">
        {facts.filter((f) => f !== undefined).join(" · ")}
      </p>
      {!nearby && (
        <p role="note" className="mt-3 text-warning">
          <Trans>
            This device isn't on your home network. Only allow it if it's yours and you started signing in on it just
            now.
          </Trans>
        </p>
      )}
    </>
  );
}

/**
 * Show server address. It gives the server's address as text and a QR
 * code, for a device that can't find the server.
 */
function ShowAddress({ address }: { address: string | undefined }) {
  const [shown, setShown] = useState(false);
  if (address === undefined) return null;
  return (
    <div className="mt-6 flex flex-col items-center gap-4">
      {shown ? (
        <ServerAddress address={address} />
      ) : (
        <Button variant="ghost" size="sm" onPress={() => setShown(true)}>
          <Trans>Show server address</Trans>
        </Button>
      )}
    </div>
  );
}

function CodeForm() {
  const { t } = useLingui();
  const navigate = linkRoute.useNavigate();
  return (
    <SubmitForm
      layout="panel"
      onSubmit={(data) => {
        const code = String(data.get("code") ?? "")
          .replace(/[\s-]/g, "")
          .toUpperCase();
        void navigate({ to: "/link", search: { code } });
      }}
      submitLabel={<Trans>Continue</Trans>}
    >
      <TextField
        variant="form"
        label={t`Code`}
        name="code"
        description={t`Shown on the device you're signing in.`}
        autoComplete="off"
        isRequired
        autoFocus
      />
    </SubmitForm>
  );
}
