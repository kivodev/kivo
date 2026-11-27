import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { useState } from "react";
import { libraryQueries } from "@/api/libraries";
import { useUpdateSettings } from "@/api/settings";
import { useCompleteSetup } from "@/api/setup";
import { currentUserQueries, usernameMaxLength } from "@/api/users";
import { Button, type ButtonVariant } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { Logo } from "@/components/Logo";
import { Select } from "@/components/Select";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { useDocumentTitle } from "@/components/useDocumentTitle";
import { LibrarySetup } from "@/features/libraries/LibrarySetup";
import { contentRatingCountries, metadataLanguages, preferredMetadataDefaults } from "@/i18n/metadataLocales";
import { browserName } from "@/lib/browser";
import { TvExitOnBack } from "@/tv/ExitDialog";

const setupRoute = getRouteApi("/setup");

const steps = ["admin", "libraries", "metadata", "remote", "done"] as const;
type Step = (typeof steps)[number];

/**
 * First-run setup, which ends at Home. The step lives in component state,
 * so the wizard doesn't resume. After the user exists, a reload lands on
 * Home like any signed-in visit.
 */
export function SetupWizard() {
  const { t } = useLingui();
  const navigate = setupRoute.useNavigate();
  useDocumentTitle(undefined);
  const [step, setStep] = useState<Step>("admin");
  const index = steps.indexOf(step);
  const next = () => setStep(steps[Math.min(index + 1, steps.length - 1)] ?? "done");
  const back = () => setStep(steps[Math.max(index - 1, 0)] ?? "admin");
  const titles: Record<Step, string> = {
    admin: t`Create the admin account`,
    libraries: t`Add libraries`,
    metadata: t`Metadata`,
    remote: t`Remote access`,
    done: t`Kivo is ready`,
  };

  return (
    <main className="flex min-h-dvh items-start justify-center bg-bg p-4 sm:items-center">
      <TvExitOnBack />
      <div
        data-part="setup-wizard"
        data-step={step}
        className="w-full max-w-prose rounded-lg border border-border bg-surface p-8 shadow-overlay"
      >
        <Logo className="mb-2 h-7" />
        <p className="text-sm text-text-muted">
          <Trans>
            Step {index + 1} of {steps.length}
          </Trans>
        </p>
        <h1 className="mt-1 mb-6 text-2xl font-semibold">{titles[step]}</h1>
        {step === "admin" && <AdminStep onDone={next} />}
        {step === "libraries" && <LibrariesStep onBack={back} onNext={next} />}
        {step === "metadata" && <MetadataStep onBack={back} onDone={next} />}
        {step === "remote" && (
          <div className="flex flex-col gap-6">
            <p className="text-text-muted">
              <Trans>Streaming works on your local network. Set up remote access later in Network settings.</Trans>
            </p>
            <StepNav onBack={back} onNext={next} nextLabel={t`Set up later`} />
          </div>
        )}
        {step === "done" && <DoneStep onBack={back} onNext={() => void navigate({ to: "/" })} />}
      </div>
    </main>
  );
}

type NavProps = {
  onBack: () => void;
  onNext: () => void;
  nextLabel: string;
  /** Secondary where going on isn't the step's main action. */
  nextVariant?: ButtonVariant;
};

function StepNav({ onBack, onNext, nextLabel, nextVariant = "primary" }: NavProps) {
  return (
    <div className="flex justify-between gap-2">
      <Button onPress={onBack}>
        <Trans>Back</Trans>
      </Button>
      <Button variant={nextVariant} onPress={onNext}>
        {nextLabel}
      </Button>
    </div>
  );
}

type StepProps = { onBack: () => void; onNext: () => void };

/** Whether setup has added no library yet. */
function useNoLibraries(): boolean {
  return (useQuery(libraryQueries.list()).data?.items.length ?? 0) === 0;
}

/** The libraries step. Going on without a library is skipping it, and says so. */
function LibrariesStep(nav: StepProps) {
  const { t } = useLingui();
  const none = useNoLibraries();
  return (
    <div className="flex flex-col gap-6">
      <LibrarySetup />
      <StepNav {...nav} nextLabel={none ? t`Skip for now` : t`Next`} nextVariant={none ? "secondary" : "primary"} />
    </div>
  );
}

/** The last step, which says what happens next: the first scans, or that there's no library yet. */
function DoneStep(nav: StepProps) {
  const { t } = useLingui();
  return (
    <div className="flex flex-col gap-6">
      <p className="text-text-muted">
        {useNoLibraries() ? (
          <Trans>No libraries yet. Add one from Home when you're ready.</Trans>
        ) : (
          <Trans>Scanning libraries. Items appear on Home as they're found.</Trans>
        )}
      </p>
      <StepNav {...nav} nextLabel={t`Go to Home`} />
    </div>
  );
}

function AdminStep({ onDone }: { onDone: () => void }) {
  const { t } = useLingui();
  const complete = useCompleteSetup();
  // Creating the user caches who is signed in. It's never fetched here,
  // because before setup nobody is. Coming back to this step then shows
  // who it is.
  const user = useQuery({ ...currentUserQueries.current(), enabled: false });
  if (user.data) {
    const username = user.data.username;
    return (
      <div className="flex flex-col gap-6">
        <p className="text-text-muted">
          <Trans>Signed in as {username}.</Trans>
        </p>
        <div className="flex justify-end">
          <Button variant="primary" onPress={onDone}>
            <Trans>Next</Trans>
          </Button>
        </div>
      </div>
    );
  }
  return (
    <SubmitForm
      onSubmit={(data) => {
        complete.mutate(
          {
            body: {
              username: String(data.get("username")),
              password: String(data.get("password")),
              kind: "browser",
              name: browserName(),
            },
          },
          { onSuccess: onDone },
        );
      }}
      submitLabel={<Trans>Create account</Trans>}
      isPending={complete.isPending}
      error={complete.error}
    >
      <TextField
        variant="form"
        label={t`Username`}
        name="username"
        autoComplete="username"
        isRequired
        maxLength={usernameMaxLength}
        autoFocus
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
  );
}

function MetadataStep({ onBack, onDone }: { onBack: () => void; onDone: () => void }) {
  const { t } = useLingui();
  const update = useUpdateSettings();
  const defaults = preferredMetadataDefaults(navigator.language);
  const [language, setLanguage] = useState(defaults.language);
  const [country, setCountry] = useState(defaults.country);

  return (
    <div className="flex flex-col gap-6">
      <p className="text-text-muted">
        <Trans>Language for titles and descriptions. Country for content ratings.</Trans>
      </p>
      <Select
        variant="inline"
        label={t`Language`}
        options={metadataLanguages(language)}
        selected={language}
        onChange={setLanguage}
      />
      <Select
        variant="inline"
        label={t`Certification country`}
        options={contentRatingCountries(country)}
        selected={country}
        onChange={setCountry}
      />
      <FormError error={update.error} />
      <div className="flex justify-between gap-2">
        <Button onPress={onBack}>
          <Trans>Back</Trans>
        </Button>
        <Button
          variant="primary"
          isPending={update.isPending}
          onPress={() =>
            update.mutate(
              { body: { metadataLanguage: language, contentRatingCountry: country } },
              { onSuccess: onDone },
            )
          }
        >
          <Trans>Next</Trans>
        </Button>
      </div>
    </div>
  );
}
