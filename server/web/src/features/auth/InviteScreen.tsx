import { Trans, useLingui } from "@lingui/react/macro";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { errorCode } from "@/api/problem";
import { useAcceptInvite, usernameMaxLength, userQueries } from "@/api/users";
import { useDocumentTitle } from "@/components/documentTitle";
import { Logo } from "@/components/Logo";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { describeProblem } from "@/i18n/problems";
import { browserName } from "@/lib/browser";
import { TvExitOnBack } from "@/tv/ExitDialog";

const inviteRoute = getRouteApi("/invite/$token");

/**
 * The invite page, reached without signing in. It shows who sent the
 * link, then asks for a username and password for the new user, who then
 * goes Home.
 */
export function InviteScreen() {
  const { t } = useLingui();
  const { token } = inviteRoute.useParams();
  const navigate = inviteRoute.useNavigate();
  useDocumentTitle(undefined);
  const accept = useAcceptInvite();
  // Accepting uses the link up, and signing in drops every query. So the
  // query pauses while accepting, and the invite keeps showing as it was
  // first read, rather than refetching and coming back as dead.
  const invite = useQuery({
    ...userQueries.inviteLink(token),
    enabled: accept.isIdle || accept.isError,
    placeholderData: keepPreviousData,
  });
  const code = errorCode(accept.error);
  const dead = invite.isError || code === "notFound";

  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg p-4">
      <TvExitOnBack />
      <div
        data-part="auth-panel"
        className="w-full max-w-form rounded-lg border border-border bg-surface p-8 shadow-overlay"
      >
        <Logo className="mb-6 h-7" />
        {dead ? (
          <>
            <h1 className="mb-4 text-2xl font-semibold">
              <Trans>This invite link doesn't work</Trans>
            </h1>
            <p className="text-text-muted">
              <Trans>This invite link has expired, was already used or was revoked. Ask for a new link.</Trans>
            </p>
          </>
        ) : invite.data ? (
          <>
            <h1 className="mb-6 text-2xl font-semibold">
              <Trans>
                {invite.data.invitedBy} invited you to Kivo on {invite.data.serverName}
              </Trans>
            </h1>
            <SubmitForm
              layout="panel"
              onSubmit={(data) =>
                accept.mutate(
                  {
                    params: { path: { token } },
                    body: {
                      username: String(data.get("username")),
                      password: String(data.get("password")),
                      kind: "browser",
                      name: browserName(),
                    },
                  },
                  { onSuccess: () => void navigate({ to: "/", replace: true }) },
                )
              }
              submitLabel={<Trans>Create account</Trans>}
              isPending={accept.isPending}
              error={code !== "nameTaken" ? accept.error : null}
            >
              <TextField
                variant="form"
                label={t`Username`}
                name="username"
                autoComplete="username"
                isRequired
                maxLength={usernameMaxLength}
                autoFocus
                errorMessage={code === "nameTaken" ? describeProblem(code) : undefined}
              />
              <TextField
                variant="form"
                label={t`Password`}
                name="password"
                type="password"
                autoComplete="new-password"
                isRequired
                minLength={8}
                description={t`At least 8 characters.`}
              />
            </SubmitForm>
          </>
        ) : null}
      </div>
    </main>
  );
}
