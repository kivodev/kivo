import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { type ReactNode, Suspense, useState } from "react";
import { homeQueries, homeRowKinds, useResetHomeRows, useSaveHomeRows } from "@/api/home";
import { libraryQueries, useSaveLibraries } from "@/api/libraries";
import { errorCode } from "@/api/problem";
import type { HomeRowInfo, PlaybackPreferencesUpdate, SubtitleMode } from "@/api/schemas";
import { serverQueries, useQualityLadder } from "@/api/server";
import { useChangeServer } from "@/api/session";
import { currentUserQueries, useUpdateCurrentUser } from "@/api/users";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { useDocumentTitle } from "@/components/documentTitle";
import { Select } from "@/components/Select";
import { ServerAddress, serverAddress } from "@/components/ServerAddress";
import { SettingsCard, SettingsLayout, SettingsRow, SettingsSection } from "@/components/SettingsLayout";
import { Switch } from "@/components/Switch";
import { channelLayout } from "@/components/trackLabels";
import { formatBitrate, formatRung } from "@/i18n/format";
import { describeProblem } from "@/i18n/problems";
import { trackLanguages } from "@/i18n/trackLanguages";
import {
  type AudioChannels,
  type DeviceSettings,
  deviceSettings,
  type SubtitleBackground,
  type SubtitleSize,
  saveDeviceSettings,
  type ThemeName,
} from "@/lib/deviceSettings";
import { qualityKey, qualityOf } from "@/lib/quality";
import { useTvBack } from "@/tv/back";
import { isPackaged, isTv } from "@/tv/flag";
import { TvFocusGroup, useTvFocusable } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { currentTvFocus, focusTv, isTvFocusIn } from "@/tv/spatial";
import { ChangePasswordDialog } from "./ChangePasswordDialog";
import { CreditsDialog } from "./CreditsDialog";
import { HomeRowOrder, HomeRowsTv } from "./HomeRowsEditor";
import { LibraryOrder, LibraryPins } from "./LibraryOrder";
import { Sessions, SignOutHereButton } from "./Sessions";
import { TvVersions } from "./TvVersions";

// Select IDs for the languages' defaults.
const original = "original";
const serverLanguage = "server";

/**
 * Settings, from the user menu. It shows your account and where you're
 * signed in, then a section per topic, then the server's version. In each
 * section, "All your devices" holds your own settings, which every browser
 * and app you sign in on shares. "This browser" holds settings kept in this
 * browser alone. Each change saves at once and says nothing unless it
 * fails.
 */
export function SettingsScreen() {
  const { t } = useLingui();
  useDocumentTitle(t`Settings`);
  const device = useDeviceSettings();
  if (isTv) return <TvSettings {...device} />;
  return (
    <div className="px-4 py-6 sm:px-8">
      <section data-part="settings" aria-labelledby="settings-title" className="flex flex-col gap-8">
        <h1 id="settings-title" className="text-2xl font-semibold">
          <Trans>Settings</Trans>
        </h1>
        <SettingsLayout>
          <AccountSection />
          {/* Devices are managed from a browser: a list of them is no use to a remote. */}
          {!isTv && (
            <SettingsSection id="devices" title={<Trans>Signed-in devices</Trans>}>
              <Sessions aria-labelledby="devices-title" />
            </SettingsSection>
          )}
          <PlaybackSection />
          <AudioSection {...device} />
          <SubtitlesSection {...device} />
          <QualitySection {...device} />
          <AppearanceSection {...device} />
          <LibrariesSection />
          <AboutSection />
        </SettingsLayout>
      </section>
    </div>
  );
}

/**
 * Settings on TV, as TV platforms lay theirs out: the sections in a list on
 * the left, and the one focused there on the right. Moving down the list
 * shows each section; Right or OK goes into it, and Left comes back to the
 * section it left. Signed-in devices aren't here: a list of them is no use
 * to a remote, and they're managed from a browser.
 */
function TvSettings(device: DeviceProps) {
  const { t } = useLingui();
  const sections: { id: string; label: ReactNode; content: ReactNode }[] = [
    { id: "account", label: <Trans>Account</Trans>, content: <AccountSection /> },
    { id: "playback", label: <Trans>Playback</Trans>, content: <PlaybackSection /> },
    { id: "audio", label: <Trans>Audio</Trans>, content: <AudioSection {...device} /> },
    { id: "subtitles", label: <Trans>Subtitles</Trans>, content: <SubtitlesSection {...device} /> },
    { id: "quality", label: <Trans>Quality</Trans>, content: <QualitySection {...device} /> },
    { id: "appearance", label: <Trans>Display</Trans>, content: <AppearanceSection {...device} /> },
    { id: "libraries", label: <Trans>Home & libraries</Trans>, content: <LibrariesSection /> },
    { id: "about", label: <Trans>About</Trans>, content: <AboutSection /> },
  ];
  const [shown, setShown] = useState("account");
  const current = sections.find((s) => s.id === shown) ?? sections[0];
  // Back from a setting goes to its section in the list, and from the list
  // on to the shell's Back.
  useTvBack(() => {
    const focus = currentTvFocus();
    if (focus === undefined || !isTvFocusIn(focus, "settings:pane")) return false;
    focusTv(`settings:section:${shown}`);
  });
  return (
    // Up and Down stop at each pane's ends, rather than going on into the
    // other pane or the rail. The list stays put as a long section scrolls.
    <section data-part="settings" aria-labelledby="settings-title" className="flex gap-10 px-4 py-6 sm:px-8">
      <TvFocusGroup
        focusKey="settings:sections"
        as="nav"
        label={t`Settings sections`}
        trap={["up", "down"]}
        className="sticky top-(--layout-tv-overscan-y) flex w-60 shrink-0 flex-col gap-1 self-start"
      >
        <h1 id="settings-title" className="mb-4 text-2xl font-semibold">
          <Trans>Settings</Trans>
        </h1>
        {sections.map((s, i) => (
          <SectionEntry key={s.id} id={s.id} first={i === 0} shown={s.id === shown} onShow={() => setShown(s.id)}>
            {s.label}
          </SectionEntry>
        ))}
      </TvFocusGroup>
      <div aria-hidden className="w-px shrink-0 bg-border" />
      <TvFocusGroup
        focusKey="settings:pane"
        part="settings-pane"
        trap={["up", "down"]}
        className="max-w-(--layout-settings-width) min-w-0 flex-1"
      >
        {/* A section that waits for its data waits here, beside the list, not over the whole screen. */}
        <Suspense fallback={null}>{current?.content}</Suspense>
      </TvFocusGroup>
    </section>
  );
}

/** A section in the TV's list. Focus shows it; OK goes into it. */
function SectionEntry({
  id,
  first,
  shown,
  onShow,
  children,
}: {
  id: string;
  first: boolean;
  shown: boolean;
  onShow: () => void;
  children: ReactNode;
}) {
  const enter = () => focusTv("settings:pane");
  const { ref, attributes } = useTvFocusable<HTMLButtonElement>({
    focusKey: `settings:section:${id}`,
    onFocus: onShow,
    onPress: enter,
  });
  return (
    <button
      ref={ref}
      type="button"
      {...attributes}
      data-tv-focus="button"
      data-tv-fill=""
      data-tv-autofocus={first || undefined}
      data-part="settings-section-entry"
      aria-current={shown ? "true" : undefined}
      onClick={enter}
      className={`flex h-10 items-center rounded-md px-4 text-left font-medium focus:bg-text focus:text-bg ${
        shown ? "bg-surface-hover text-text" : "text-text-muted"
      }`}
    >
      {children}
    </button>
  );
}

/** This browser's settings, and a way to change some of them, kept in the browser at once. */
function useDeviceSettings() {
  const [device, setDevice] = useState(deviceSettings);
  const change = (patch: Partial<DeviceSettings>) => {
    const next = { ...device, ...patch };
    setDevice(next);
    saveDeviceSettings(next);
  };
  return { device, change };
}

type DeviceProps = ReturnType<typeof useDeviceSettings>;

/** A setting's value in a row that only shows it. */
function Value({ children }: { children: string }) {
  return <span className="truncate text-sm text-text-muted">{children}</span>;
}

function AccountSection() {
  const { t } = useLingui();
  const username = useSuspenseQuery(currentUserQueries.current()).data.username;
  const server = useSuspenseQuery(serverQueries.info()).data.name;
  const [changingPassword, setChangingPassword] = useState(false);
  return (
    <SettingsSection id="account" title={<Trans>Account</Trans>}>
      <SettingsCard>
        <SettingsRow label={username} description={t`Signed in to ${server}`}>
          <SignOutHereButton />
        </SettingsRow>
        {/* Typing a password with a remote is the wrong way to change it; the web does it. */}
        {!isTv && (
          <SettingsRow label={<Trans>Password</Trans>}>
            <Button size="sm" onPress={() => setChangingPassword(true)}>
              <Trans>Change password…</Trans>
            </Button>
          </SettingsRow>
        )}
      </SettingsCard>
      {!isTv && <ChangePasswordDialog isOpen={changingPassword} onClose={() => setChangingPassword(false)} />}
    </SettingsSection>
  );
}

/**
 * Saves one playback preference. It returns the value while it's on its
 * way to the server, and the row's spinner, or its error with Retry. A
 * refused value leaves the saved one showing.
 */
function usePlaybackSave<K extends keyof PlaybackPreferencesUpdate>(key: K) {
  const { t } = useLingui();
  const update = useUpdateCurrentUser();
  const problem = update.isError ? describeProblem(errorCode(update.error)) : undefined;
  return {
    pending: update.isPending ? update.variables.body.playback?.[key] : undefined,
    save: (value: NonNullable<PlaybackPreferencesUpdate[K]>) =>
      update.mutate({ body: { playback: { [key]: value } as PlaybackPreferencesUpdate } }),
    row: {
      isPending: update.isPending,
      error: problem && t`Couldn't save. ${problem}`,
      onRetry: () => update.variables && update.mutate(update.variables),
    },
  };
}

function PlaybackSection() {
  const playback = useSuspenseQuery(currentUserQueries.current()).data.playback;
  const remember = usePlaybackSave("rememberTracks");
  const autoplayNext = usePlaybackSave("autoplayNext");
  return (
    <SettingsSection id="playback" title={<Trans>Playback</Trans>}>
      <SettingsCard scope="account">
        <SettingsRow label={<Trans>Play next episode automatically</Trans>} {...autoplayNext.row}>
          <Switch isSelected={autoplayNext.pending ?? playback.autoplayNext} onChange={autoplayNext.save} />
        </SettingsRow>
        <SettingsRow
          label={<Trans>Remember choices per show</Trans>}
          description={<Trans>Episodes use the audio and subtitle tracks last chosen for the show.</Trans>}
          {...remember.row}
        >
          <Switch isSelected={remember.pending ?? playback.rememberTracks} onChange={remember.save} />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function AudioSection({ device, change }: DeviceProps) {
  const { t } = useLingui();
  const playback = useSuspenseQuery(currentUserQueries.current()).data.playback;
  const audio = usePlaybackSave("audioLanguage");
  const channels = [
    { id: "auto", label: t`Auto`, detail: t`Based on this device's audio output` },
    { id: "2", label: channelLayout(2) },
    { id: "6", label: channelLayout(6) },
    { id: "8", label: channelLayout(8) },
  ];
  return (
    <SettingsSection id="audio" title={<Trans>Audio</Trans>}>
      <SettingsCard scope="account">
        <SettingsRow label={<Trans>Audio language</Trans>} {...audio.row}>
          <Select
            options={[{ id: original, label: t`Original language` }, ...trackLanguages(playback.audioLanguage)]}
            selected={(audio.pending ?? playback.audioLanguage) || original}
            onChange={(id) => audio.save(id === original ? "" : id)}
          />
        </SettingsRow>
      </SettingsCard>
      <SettingsCard scope="device">
        <SettingsRow label={<Trans>Maximum audio channels</Trans>}>
          <Select
            options={channels}
            selected={String(device.maxAudioChannels ?? "auto")}
            onChange={(id) => change({ maxAudioChannels: id === "auto" ? undefined : (Number(id) as AudioChannels) })}
          />
        </SettingsRow>
        <SettingsRow
          label={<Trans>Dialogue boost</Trans>}
          description={<Trans>Makes speech clearer and evens out loud scenes, playing some audio as AAC.</Trans>}
        >
          <Switch isSelected={device.dialogueBoost} onChange={(on) => change({ dialogueBoost: on })} />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function SubtitlesSection({ device, change }: DeviceProps) {
  const { t } = useLingui();
  const playback = useSuspenseQuery(currentUserQueries.current()).data.playback;
  const mode = usePlaybackSave("subtitleMode");
  const subtitleLanguage = usePlaybackSave("subtitleLanguage");
  const subtitleMode = mode.pending ?? playback.subtitleMode;

  const modes: { id: SubtitleMode; label: string; detail?: string }[] = [
    { id: "none", label: t`Off` },
    { id: "forced", label: t`Only forced`, detail: t`For the parts in another language` },
    { id: "smart", label: t`Foreign audio only`, detail: t`When the audio isn't in your subtitle language` },
    { id: "always", label: t`Always` },
  ];
  const sizes = [
    { id: "small", label: t`Small` },
    { id: "medium", label: t`Medium` },
    { id: "large", label: t`Large` },
  ];
  const backgrounds = [
    { id: "none", label: t`None` },
    { id: "shadow", label: t`Shadow` },
    { id: "box", label: t`Box` },
  ];

  return (
    <SettingsSection id="subtitles" title={<Trans>Subtitles</Trans>}>
      <SettingsCard scope="account">
        <SettingsRow label={<Trans>Show subtitles</Trans>} {...mode.row}>
          <Select options={modes} selected={subtitleMode} onChange={mode.save} />
        </SettingsRow>
        <SettingsRow label={<Trans>Subtitle language</Trans>} {...subtitleLanguage.row}>
          <Select
            options={[{ id: serverLanguage, label: t`Server default` }, ...trackLanguages(playback.subtitleLanguage)]}
            selected={(subtitleLanguage.pending ?? playback.subtitleLanguage) || serverLanguage}
            isDisabled={subtitleMode === "none"}
            onChange={(id) => subtitleLanguage.save(id === serverLanguage ? "" : id)}
          />
        </SettingsRow>
      </SettingsCard>
      <SettingsCard scope="device" footer={<Trans>Styled subtitles keep their own look.</Trans>}>
        <SettingsRow label={<Trans>Subtitle size</Trans>}>
          <Select
            options={sizes}
            selected={device.subtitleSize}
            onChange={(id) => change({ subtitleSize: id as SubtitleSize })}
          />
        </SettingsRow>
        <SettingsRow label={<Trans>Subtitle background</Trans>}>
          <Select
            options={backgrounds}
            selected={device.subtitleBackground}
            onChange={(id) => change({ subtitleBackground: id as SubtitleBackground })}
          />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function QualitySection({ device, change }: DeviceProps) {
  const { t } = useLingui();
  const qualityLadder = useQualityLadder();
  // The most bandwidth the server lets this user's streams use away from
  // home, if it limits them.
  const cap = useSuspenseQuery(currentUserQueries.current()).data.limits.remoteQualityCapBps;
  const capped = cap ? formatBitrate(cap) : undefined;
  const qualities = [
    { id: "auto", label: t`Auto`, detail: t`Based on connection speed and limits` },
    { id: "original", label: t`Original` },
    ...qualityLadder.map((rung) => ({ id: String(rung.bitrateBps), label: formatRung(rung) })),
  ];
  return (
    <SettingsSection id="quality" title={<Trans>Quality</Trans>}>
      <SettingsCard scope="device">
        <SettingsRow label={<Trans>Local network quality</Trans>}>
          <Select
            options={qualities}
            selected={qualityKey(device.localQuality)}
            onChange={(id) => change({ localQuality: qualityOf(id) })}
          />
        </SettingsRow>
        <SettingsRow
          label={<Trans>Remote quality</Trans>}
          description={capped && <Trans>Limited to {capped} by the server.</Trans>}
        >
          <Select
            options={qualities}
            selected={qualityKey(device.remoteQuality)}
            onChange={(id) => change({ remoteQuality: qualityOf(id) })}
          />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function AppearanceSection({ device, change }: DeviceProps) {
  const { t } = useLingui();
  const themes = [
    { id: "dark", label: t`Dark` },
    { id: "light", label: t`Light` },
  ];
  return (
    <SettingsSection id="appearance" title={isTv ? <Trans>Display</Trans> : <Trans>Appearance</Trans>}>
      <SettingsCard scope="device">
        <SettingsRow label={<Trans>Theme</Trans>}>
          <Select options={themes} selected={device.theme} onChange={(id) => change({ theme: id as ThemeName })} />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function HomeRowsCard() {
  const { t } = useLingui();
  const rows = useSuspenseQuery(homeQueries.layout()).data.items.filter((r) => homeRowKinds.has(r.kind));
  const saving = useSaveHomeRows();
  const reset = useResetHomeRows();
  const error = saving.error ?? reset.error;
  const problem = error ? describeProblem(errorCode(error)) : undefined;
  const save = (next: HomeRowInfo[]) => saving.save(next, reset.reset);
  const resetHome = () => reset.mutate({}, { onSuccess: () => saving.reset() });
  const retry = () => (reset.isError ? resetHome() : saving.retry(reset.reset));
  return (
    <SettingsCard
      scope="account"
      title={<Trans>Home</Trans>}
      footer={
        problem ? (
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span role="alert" className="text-danger">
              {t`Couldn't save. ${problem}`}
            </span>
            <TvControl focusKey="home-rows:retry">
              <Button size="sm" variant="ghost" onPress={retry}>
                <Trans>Retry</Trans>
              </Button>
            </TvControl>
          </span>
        ) : undefined
      }
    >
      {isTv ? <HomeRowsTv rows={rows} onChange={save} /> : <HomeRowOrder rows={rows} onChange={save} />}
      <SettingsRow label={<Trans>Reset Home</Trans>}>
        <TvControl focusKey="home-rows:reset">
          <Button size="sm" variant="ghost" onPress={resetHome}>
            <Trans>Reset</Trans>
          </Button>
        </TvControl>
      </SettingsRow>
    </SettingsCard>
  );
}

function LibrariesSection() {
  const { t } = useLingui();
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const saving = useSaveLibraries();
  const problem = saving.isError ? describeProblem(errorCode(saving.error)) : undefined;
  return (
    <SettingsSection id="libraries" title={<Trans>Home & libraries</Trans>}>
      <HomeRowsCard />
      {libraries.length > 0 ? (
        <SettingsCard
          scope="account"
          title={<Trans>Libraries</Trans>}
          footer={
            problem ? (
              <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span role="alert" className="text-danger">
                  {t`Couldn't save. ${problem}`}
                </span>
                <TvControl focusKey="libraries:retry">
                  <Button size="sm" variant="ghost" onPress={() => saving.retry(libraries)}>
                    <Trans>Retry</Trans>
                  </Button>
                </TvControl>
              </span>
            ) : (
              <Trans>Unchecked libraries are hidden from the sidebar.</Trans>
            )
          }
        >
          {isTv ? (
            <LibraryPins libraries={libraries} onChange={saving.save} />
          ) : (
            <LibraryOrder libraries={libraries} onChange={saving.save} />
          )}
        </SettingsCard>
      ) : (
        <SettingsCard scope="account">
          <SettingsRow label={<Trans>No libraries yet</Trans>} />
        </SettingsCard>
      )}
    </SettingsSection>
  );
}

function AboutSection() {
  const info = useSuspenseQuery(serverQueries.info()).data;
  const address = serverAddress(info);
  return (
    <SettingsSection id="about" title={<Trans>About</Trans>}>
      {/* On TV the versions are rows focus stops on, so the remote reaches them. */}
      {isTv ? (
        <TvVersions serverVersion={info.version} />
      ) : (
        <SettingsCard>
          <SettingsRow label={<Trans>Version</Trans>} focusable={false}>
            <Value>{info.version}</Value>
          </SettingsRow>
        </SettingsCard>
      )}
      <CreditsRow />
      {/* There is no admin area on TV; this is how to reach it from a browser. */}
      {isTv && address && (
        <SettingsCard
          part="server-address"
          footer={<Trans>To manage this server, open this address in a browser.</Trans>}
        >
          <div className="flex justify-center px-4 py-6">
            {/* Focus stops here so the page scrolls to the code. */}
            <TvControl focusKey="about:address" onPress={() => {}}>
              <ServerAddress address={address} />
            </TvControl>
          </div>
        </SettingsCard>
      )}
      {isPackaged && <PackagedChangeServer server={info.name} />}
    </SettingsSection>
  );
}

/** About's Credits row, which opens the credits. */
function CreditsRow() {
  const [open, setOpen] = useState(false);
  return (
    <SettingsCard>
      <SettingsRow label={<Trans>Credits</Trans>}>
        <Button size="sm" onPress={() => setOpen(true)}>
          <Trans>View</Trans>
        </Button>
      </SettingsRow>
      <CreditsDialog isOpen={open} onClose={() => setOpen(false)} />
    </SettingsCard>
  );
}

/**
 * The packaged app's Change server. It asks first, as Sign out
 * does on TV, since it signs the TV out. Then it signs out of the server,
 * forgets it and shows the address screen.
 */
function PackagedChangeServer({ server }: { server: string }) {
  const { t } = useLingui();
  const change = useChangeServer();
  const [asking, setAsking] = useState(false);
  return (
    <SettingsCard>
      <SettingsRow label={<Trans>Change server</Trans>} description={<Trans>Connect this TV to another server.</Trans>}>
        <Button size="sm" isPending={change.isPending} onPress={() => setAsking(true)}>
          <Trans>Change…</Trans>
        </Button>
      </SettingsRow>
      <ConfirmDialog
        isOpen={asking}
        title={t`Change server?`}
        confirmLabel={t`Change server`}
        tone="danger"
        onCancel={() => setAsking(false)}
        onConfirm={() => {
          setAsking(false);
          change.mutate();
        }}
      >
        <Trans>This TV will be signed out of {server}.</Trans>
      </ConfirmDialog>
    </SettingsCard>
  );
}
