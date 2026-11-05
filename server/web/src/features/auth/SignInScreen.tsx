import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { getRouteApi, useRouter } from "@tanstack/react-router";
import { type ReactNode, useEffect, useState } from "react";
import type { KeyboardEvent } from "react-aria-components";
import { errorCode, isProblem } from "@/api/problem";
import { packagedSession, useBrowserLink, useSignIn } from "@/api/session";
import { Button } from "@/components/Button";
import { groupedCode } from "@/components/deviceCode";
import { useDocumentTitle } from "@/components/documentTitle";
import { FormError } from "@/components/FormError";
import { Logo } from "@/components/Logo";
import { QrCode } from "@/components/QrCode";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { describeProblem } from "@/i18n/problems";
import { browserName, currentBrowser } from "@/lib/browser";
import { useTvBack } from "@/tv/back";
import { TvExitOnBack } from "@/tv/ExitDialog";
import { isPackaged, isTv } from "@/tv/flag";
import { TvFocusGroup } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { focusTv } from "@/tv/spatial";

const signInRoute = getRouteApi("/sign-in");

/** Allows only same-app paths, because an open redirect would hand the session to anyone's link. */
function safeRedirect(target: string | undefined): string {
  return target?.startsWith("/") && !target.startsWith("//") ? target : "/";
}

/** Where to go once signed in: the page the expired session was on, or else Home. */
function useAfterSignIn(): () => void {
  const { redirect } = signInRoute.useSearch();
  const router = useRouter();
  return () => void router.navigate({ href: safeRedirect(redirect), replace: true });
}

/**
 * The sign-in page. It takes a username and password, or offers Sign in
 * with another device, which a signed-in device approves. A TV, which has
 * only a remote, opens on the code, with the panel at the TV's scale: the
 * code beside its QR code, in a panel as wide as they are, and the password
 * form in the address screen's panel.
 */
export function SignInScreen() {
  useDocumentTitle(undefined);
  const [withCode, setWithCode] = useState(() => isTv || currentBrowser().tv);
  const header = (
    <>
      <Logo className="mb-6 h-7" />
      <h1 className="mb-6 text-2xl font-semibold">
        <Trans>Sign in</Trans>
      </h1>
    </>
  );

  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg p-4 tv:px-(--layout-tv-overscan-x) tv:py-(--layout-tv-overscan-y)">
      <TvExitOnBack />
      <div
        data-part="auth-panel"
        className={`w-full max-w-form rounded-lg border border-border bg-surface p-8 shadow-overlay tv:p-12 ${withCode ? "tv:w-auto tv:max-w-none" : ""}`}
      >
        {withCode ? (
          <CodePanel header={header} onPassword={() => setWithCode(false)} />
        ) : (
          <PasswordForm header={header} onCode={() => setWithCode(true)} />
        )}
      </div>
    </main>
  );
}

/**
 * What the approving phone shows for this device, and its session's name in
 * Now playing and Signed-in devices. A TV is named for its make, and a
 * Samsung TV for its Tizen version too, such as "Samsung TV (Tizen 9.0)",
 * not "Web browser on Linux x86_64".
 */
function deviceName(): string {
  const browser = currentBrowser();
  if (!isTv && !browser.tv) return browserName();
  if (browser.tizen) return `Samsung TV (Tizen ${browser.tizen.toFixed(1)})`;
  return browser.webos ? "LG TV" : "TV";
}

// Tizen's keyboard sends its Done key as this key code, where others send Enter.
const tizenDone = 65376;

/**
 * Opens the TV's keyboard on the form's field, as OK on it does: focus
 * goes to its stop, which the arrows then move on from, and from there
 * into its input. Norigin moves focus a moment later, so the input takes
 * it once the stop has it.
 */
function typeIn(form: HTMLFormElement, field: string) {
  const input = form.elements.namedItem(field);
  const stop = input instanceof HTMLInputElement ? input.closest<HTMLElement>("[data-tv-focusable]") : null;
  if (!(input instanceof HTMLInputElement) || !stop) return;
  // Only for this move: a later arrow onto the stop leaves the input be.
  stop.addEventListener("focus", () => input.focus(), { once: true, signal: AbortSignal.timeout(1000) });
  focusTv(`signin:${field}`);
}

/**
 * On TV, the keyboard's Enter or Done in a field goes on, as a phone's
 * Next does: from the username to the password, and from the password it
 * signs in, or returns to a field still empty. Elsewhere Enter sends the
 * form, as a browser's does.
 */
function goesOn(next: "password" | "submit") {
  return (e: KeyboardEvent) => {
    if ((e.key !== "Enter" && e.keyCode !== tizenDone) || e.nativeEvent.isComposing) {
      e.continuePropagation();
      return;
    }
    e.preventDefault();
    const form = (e.target as HTMLInputElement).form;
    if (!form) return;
    if (next === "password") return typeIn(form, "password");
    const empty = form.querySelector<HTMLInputElement>("input:invalid");
    // The empty field shows its error, but typeIn takes it there by its
    // stop, rather than React Aria focusing its input first and the keyboard
    // closing and opening again.
    if (empty) form.addEventListener("invalid", (ev) => ev.preventDefault(), { capture: true, once: true });
    form.requestSubmit();
    if (empty) typeIn(form, empty.name);
    else focusTv("signin:submit");
  };
}

function PasswordForm({ header, onCode }: { header: ReactNode; onCode: () => void }) {
  const { t } = useLingui();
  const signIn = useSignIn();
  const afterSignIn = useAfterSignIn();
  useEffect(() => {
    if (isTv) focusTv("signin:username");
  }, []);
  // On TV the form is opened from the code, so Back goes back to it.
  useTvBack(() => {
    onCode();
    return true;
  });
  // OK on a field opens the TV's own keyboard (TvControl).
  const form = (
    <>
      <SubmitForm
        layout="panel"
        onSubmit={(data) =>
          signIn.mutate(
            {
              body: {
                username: String(data.get("username")),
                password: String(data.get("password")),
                name: deviceName(),
                ...(isPackaged ? packagedSession() : { kind: "browser" }),
              },
            },
            { onSuccess: afterSignIn },
          )
        }
        submitLabel={<Trans>Sign in</Trans>}
        submitFocusKey="signin:submit"
        isPending={signIn.isPending}
        error={signIn.error}
        errorMessage={
          isProblem(signIn.error) && signIn.error.retryAfterSeconds !== undefined ? (
            <Plural
              value={signIn.error.retryAfterSeconds}
              one="Too many attempts. Try again in # second."
              other="Too many attempts. Try again in # seconds."
            />
          ) : undefined
        }
      >
        <TvControl focusKey="signin:username" kind="field">
          <TextField
            variant="form"
            label={t`Username`}
            name="username"
            autoComplete="username"
            isRequired
            autoFocus={!isTv}
            // The TV keyboard's action key says what Enter does here: it goes
            // on to Password, and from Password it signs in.
            enterKeyHint={isTv ? "next" : undefined}
            onKeyDown={isTv ? goesOn("password") : undefined}
          />
        </TvControl>
        <TvControl focusKey="signin:password" kind="field">
          <TextField
            variant="form"
            label={t`Password`}
            name="password"
            type="password"
            autoComplete="current-password"
            isRequired
            enterKeyHint={isTv ? "go" : undefined}
            onKeyDown={isTv ? goesOn("submit") : undefined}
          />
        </TvControl>
      </SubmitForm>
      {/* On TV the other way in is a button apart, which leaves room for a two-line error in the safe area. */}
      {!isTv && (
        <div aria-hidden="true" className="my-4 flex items-center gap-3 text-sm text-text-muted">
          <span className="h-px flex-1 bg-border" />
          <Trans>or</Trans>
          <span className="h-px flex-1 bg-border" />
        </div>
      )}
      <TvControl focusKey="signin:code" className="mt-8">
        <Button variant="secondary" className="w-full tv:h-12" onPress={onCode}>
          <Trans>Sign in with another device</Trans>
        </Button>
      </TvControl>
    </>
  );
  return (
    <>
      {header}
      {isTv ? (
        <TvFocusGroup focusKey="signin:form" className="flex flex-col">
          {form}
        </TvFocusGroup>
      ) : (
        form
      )}
    </>
  );
}

/**
 * Sign in with another device. It shows the code and a QR code of its
 * /link address, for a signed-in device to approve, with the time left. A
 * code that expires or is declined is replaced by a new one.
 */
function CodePanel({ header, onPassword }: { header: ReactNode; onPassword: () => void }) {
  const { t } = useLingui();
  const [name] = useState(deviceName);
  const afterSignIn = useAfterSignIn();
  const router = useRouter();
  const { link, denied, error, retry } = useBrowserLink(name, afterSignIn);
  useEffect(() => {
    if (isTv) focusTv("signin:password");
  }, []);
  const instructions = (
    <p className="text-text-muted">
      <Trans>On a signed-in device, scan the QR code, or open Link a device and enter this code.</Trans>
    </p>
  );
  const code = link && (
    <p data-part="code" className="font-mono text-3xl font-semibold tracking-widest text-text">
      {groupedCode(link.code)}
    </p>
  );
  // On TV the actions are buttons at rest too, as a remote has no hover to
  // find them by.
  const action = isTv ? "secondary" : "ghost";
  const password = (
    <TvControl focusKey="signin:password">
      <Button variant={action} className="tv:h-12 tv:px-8" onPress={onPassword}>
        <Trans>Sign in with password</Trans>
      </Button>
    </TvControl>
  );
  // The packaged app's server may not be one this person can sign in to,
  // so the address screen is a press away, with the kept address to edit.
  const actions = isPackaged ? (
    <div className="flex gap-4">
      {password}
      <TvControl focusKey="signin:change-server">
        <Button variant={action} className="tv:h-12 tv:px-8" onPress={() => void router.navigate({ to: "/server" })}>
          <Trans>Change server</Trans>
        </Button>
      </TvControl>
    </div>
  ) : (
    password
  );
  const declined = denied && (
    <FormError>
      <Trans>Sign-in was declined. Here's a new code.</Trans>
    </FormError>
  );
  const failure = error !== undefined && (
    <>
      <FormError>{describeProblem(errorCode(error))}</FormError>
      <TvControl focusKey="signin:retry">
        <Button className="tv:h-12 tv:px-8" onPress={retry}>
          <Trans>Retry</Trans>
        </Button>
      </TvControl>
    </>
  );
  const pending = <Trans>Getting a code…</Trans>;
  if (isTv) {
    // The code and its QR code are read from across the room, side by
    // side: the steps on the left under the heading, the QR code on the
    // right. While the code is on its way its place and the QR code's are
    // kept, so the panel doesn't move as it arrives. Without a code, the
    // panel has the steps alone.
    return (
      <TvFocusGroup focusKey="signin:panel" part="sign-in-code" className="flex items-center gap-16">
        <div className="flex w-120 flex-col items-start">
          {header}
          <div className="flex flex-col items-start gap-6">
            {declined}
            {failure || (
              <>
                {instructions}
                {code || <p className="flex h-(--text-3xl--line-height) items-center text-text-muted">{pending}</p>}
              </>
            )}
            <div className="mt-6">{actions}</div>
          </div>
        </div>
        {link ? (
          <QrCode value={link.linkUrl} label={t`QR code to link this browser`} className="size-64 shrink-0" />
        ) : (
          !failure && <div className="size-64 shrink-0 rounded-md bg-surface-raised" />
        )}
      </TvFocusGroup>
    );
  }
  return (
    <>
      {header}
      <div data-part="sign-in-code" className="flex flex-col items-center gap-4 text-center">
        {declined}
        {failure}
        {error === undefined && !link && <p className="text-text-muted">{pending}</p>}
        {link && (
          <>
            {instructions}
            <QrCode value={link.linkUrl} label={t`QR code to link this browser`} className="size-48" />
            {code}
          </>
        )}
        {actions}
      </div>
    </>
  );
}
