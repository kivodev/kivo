import { Trans, useLingui } from "@lingui/react/macro";
import { getRouteApi, useRouter } from "@tanstack/react-router";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { awaitsSetup, type ServerProbe, useProbeServer } from "@/api/server";
import { revokeKeptSession } from "@/api/session";
import { useDocumentTitle } from "@/components/documentTitle";
import { FormError } from "@/components/FormError";
import { Logo } from "@/components/Logo";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { useTvBack } from "@/tv/back";
import { TvExitOnBack } from "@/tv/ExitDialog";
import { TvFocusGroup } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { packagedServer, setPackagedServer } from "@/tv/packaged";
import { focusTv } from "@/tv/spatial";
import { parseAddress } from "./address";

const addressRoute = getRouteApi("/server");

// A NAS whose disk has spun down can take 20 s or more to answer, so an
// address that doesn't answer within a probe's 15 s (the API client's
// limit) is tried once more, with "Waking the server" on screen from 3 s
// into the wait. A server still starting refuses at once, so the retry
// waits a little first.
const wakingAfterMs = 3000;
const retryAfterMs = 3000;

/** What the screen says under the field. */
type Status =
  | { kind: "idle" }
  | { kind: "invalid" }
  | { kind: "opening"; text: string }
  | { kind: "waking" }
  | { kind: "other"; text: string }
  | { kind: "insecure"; text: string }
  | { kind: "unreachable"; text: string }
  | { kind: "unencrypted" }
  | { kind: "unready"; text: string };

type Search = ReturnType<typeof addressRoute.useSearch>;

/** What the screen says as it opens, for why the app sent it here. */
function statusOf({ unready, failed }: Search): Status {
  const kept = packagedServer()?.address ?? "";
  if (unready) return { kind: "unready", text: kept };
  if (failed === "other") return { kind: "other", text: kept };
  // As for a typed address, a refused certificate and nothing listening
  // look alike, so an https server's message covers both.
  if (failed === "none") return { kind: kept.startsWith("https:") ? "insecure" : "unreachable", text: kept };
  return { kind: "idle" };
}

/**
 * The packaged TV app's first screen: one field and Open. Open asks the address what
 * it is, and keeps a Kivo server's address and ID. The app then starts
 * again on that server, or goes on when it's the one the app started on.
 * The app also comes here when its kept server didn't answer as it
 * started, or isn't Kivo, with the address in the field to open again or
 * change, and from sign-in's, Settings' and a load error's Change server.
 */
export function AddressScreen() {
  const { t } = useLingui();
  useDocumentTitle(undefined);
  const router = useRouter();
  const probeServer = useProbeServer();
  const search = addressRoute.useSearch();
  const [text, setText] = useState(() => packagedServer()?.address ?? "");
  const [status, setStatus] = useState<Status>(() => statusOf(search));
  // Sent here again for another reason, the screen says the new one.
  const reason = `${search.unready}|${search.failed}`;
  const [shown, setShown] = useState(reason);
  if (reason !== shown) {
    setShown(reason);
    setStatus(statusOf(search));
    setText(packagedServer()?.address ?? "");
  }
  // Each Open is an attempt, and only the latest one's answer counts.
  const attempt = useRef(0);
  // The plain-HTTP server the screen said is unencrypted, which Open then opens.
  const warned = useRef("");

  // Leaving the screen ends the attempt under way. A server that didn't
  // answer as the app started is most likely asleep or restarting, so Open
  // has focus, to try it again.
  const [firstFocus] = useState(search.failed === "none" ? "address:open" : "address:field");
  useEffect(() => {
    focusTv(firstFocus);
    const attempts = attempt;
    return () => {
      attempts.current++;
    };
  }, [firstFocus]);

  // Back returns to where the screen was opened from inside the app, such
  // as sign-in's Change server, while the server there is still kept.
  // Otherwise this is the first screen, and Back asks to exit.
  useTvBack(() => {
    if (packagedServer() === undefined || !router.history.canGoBack()) return false;
    router.history.back();
    return true;
  });

  const open = async () => {
    const typed = text.trim();
    const address = parseAddress(typed);
    const mine = ++attempt.current;
    if (address === undefined) {
      setStatus({ kind: "invalid" });
      return;
    }
    const current = () => mine === attempt.current;
    setStatus({ kind: "opening", text: typed });
    const waking = setTimeout(() => {
      if (current()) setStatus({ kind: "waking" });
    }, wakingAfterMs);
    let probe: ServerProbe;
    try {
      probe = await probeServer(address.base);
      if (current() && probe.result === "none") {
        setStatus({ kind: "waking" });
        await new Promise((resolve) => setTimeout(resolve, retryAfterMs));
        if (current()) probe = await probeServer(address.base);
      }
    } finally {
      clearTimeout(waking);
    }
    if (!current()) return;
    if (probe.result === "other") {
      setStatus({ kind: "other", text: typed });
      focusTv("address:field");
      return;
    }
    if (probe.result === "none") {
      // The engine reports a refused certificate and an address with
      // nothing listening alike, so the message covers both.
      setStatus({ kind: address.secure ? "insecure" : "unreachable", text: typed });
      focusTv("address:field");
      return;
    }
    const unready = await awaitsSetup(address.base);
    if (!current()) return;
    if (unready === true) {
      setStatus({ kind: "unready", text: typed });
      focusTv("address:field");
      return;
    }
    const kept = packagedServer();
    if (!address.secure && kept?.address !== address.base && warned.current !== address.base) {
      warned.current = address.base;
      setStatus({ kind: "unencrypted" });
      focusTv("address:open");
      return;
    }
    // The kept server's session is signed out of when any other address
    // opens, whatever ID it answers with: a server's ID is public, and its
    // token is for its own address alone. Another server than the app
    // started with starts it again there.
    if (kept !== undefined && (kept.address !== address.base || kept.serverId !== probe.id)) revokeKeptSession();
    if (setPackagedServer({ address: address.base, serverId: probe.id })) return;
    await router.navigate({ to: "/", replace: true });
  };

  let message: ReactNode;
  switch (status.kind) {
    case "invalid":
      message = <FormError>{t`That isn't an address.`}</FormError>;
      break;
    case "opening":
      message = <Note>{t`Opening ${status.text}…`}</Note>;
      break;
    case "waking":
      message = <Note>{t`Waking the server…`}</Note>;
      break;
    case "other":
      message = (
        <FormError>
          {t`${status.text} answered, but it isn't a Kivo server. Check the address and port, and that the server is up to date.`}
        </FormError>
      );
      break;
    case "insecure":
      message = (
        <FormError>
          {t`Couldn't connect securely to ${status.text}. Check the address, that the server is running and that the TV trusts its certificate, or type http:// before the address to connect without encryption.`}
        </FormError>
      );
      break;
    case "unreachable":
      message = (
        <FormError>{t`Can't reach ${status.text}. Check the address and that the server is running.`}</FormError>
      );
      break;
    case "unencrypted":
      message = <FormError tone="warning">{t`Connecting without encryption. Press Open to continue.`}</FormError>;
      break;
    case "unready":
      message = (
        <FormError>
          {t`${status.text} is a Kivo server that isn't set up yet. Set it up in a browser, then press Open.`}
        </FormError>
      );
      break;
    case "idle":
      message = null;
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg p-4 tv:px-(--layout-tv-overscan-x) tv:py-(--layout-tv-overscan-y)">
      <TvExitOnBack />
      <div
        data-part="auth-panel"
        className="w-full max-w-form rounded-lg border border-border bg-surface p-8 shadow-overlay tv:p-12"
      >
        <Logo className="mb-6 h-7" />
        <h1 className="mb-6 text-2xl font-semibold">
          <Trans>Your Kivo server's address</Trans>
        </h1>
        {/* OK on the field opens the TV's own keyboard. */}
        <TvFocusGroup focusKey="address:form" part="server-address-form" className="flex flex-col gap-4">
          <SubmitForm
            layout="panel"
            submitLabel={<Trans>Open</Trans>}
            submitFocusKey="address:open"
            onSubmit={() => void open()}
          >
            <TvControl focusKey="address:field" kind="field">
              <TextField
                variant="form"
                label={t`Address`}
                name="address"
                inputMode="url"
                autoComplete="off"
                autoCorrect="off"
                spellCheck="false"
                placeholder="192.168.1.10:8484"
                value={text}
                onChange={setText}
              />
            </TvControl>
          </SubmitForm>
          {/* Room for a message is kept, so the field and Open stay put as one shows. */}
          <div className="flex min-h-15 flex-col gap-4">{message}</div>
        </TvFocusGroup>
      </div>
    </main>
  );
}

/** What the screen is doing, announced as it changes. */
function Note({ children }: { children: ReactNode }) {
  return (
    <p role="status" className="text-sm text-text-muted">
      {children}
    </p>
  );
}
