import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { acceleratorQueries } from "@/api/playback";
import type { Accelerator, AcceleratorList, Settings } from "@/api/schemas";
import { settingsQueries } from "@/api/settings";
import { Banner } from "@/components/Banner";
import { Select } from "@/components/Select";
import { SettingsCard, SettingsLayout, SettingsRow, SettingsSection } from "@/components/SettingsLayout";
import { codecName } from "@/components/trackLabels";
import { useDraft } from "@/lib/draft";
import { AdminPage } from "./AdminPage";
import { limitKey, limitOf, useLimitOptions } from "./limitOptions";
import { SettingsForm, useSettingsSave } from "./SettingsForm";

/**
 * Settings › Transcoding. Hardware acceleration has the accelerator
 * setting, with what each accelerator the startup test passed does, and
 * the ones it couldn't use with the fix for each. Then comes the transcode
 * limit.
 */
export function TranscodingScreen() {
  const { t } = useLingui();
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  const accelerators = useSuspenseQuery(acceleratorQueries.list()).data;
  const usable = accelerators.items.filter((a) => !a.problem);
  const unusable = accelerators.items.filter((a) => a.problem);
  const save = useSettingsSave();
  const { draft, edit, discard } = useDraft({
    accelerator: settings.accelerator,
    // 0 on the wire means no limit.
    videoTranscodeLimit: settings.videoTranscodeLimit,
  });
  const limits = useLimitOptions(draft.videoTranscodeLimit);
  // Automatic uses the first accelerator, best first, that passed the test.
  const best = usable[0]?.name;
  const options = [
    { id: "auto", label: t`Automatic`, detail: best ? t`Uses ${best}` : t`Best available accelerator` },
    { id: "none", label: t`Off`, detail: t`CPU only` },
    ...usable.map((a) => ({ id: a.id, label: a.name, detail: a.id })),
  ];
  if (!options.some((o) => o.id === draft.accelerator)) {
    options.push({ id: draft.accelerator, label: draft.accelerator, detail: t`Not found` });
  }

  return (
    <AdminPage page="settings-transcoding" title={t`Transcoding`}>
      <AcceleratorStatus settings={settings} accelerators={accelerators} />
      <SettingsForm save={save} body={draft} onDiscard={discard}>
        <SettingsLayout>
          <SettingsSection id="acceleration" title={<Trans>Hardware acceleration</Trans>}>
            <SettingsCard>
              <SettingsRow
                label={<Trans>Accelerator</Trans>}
                description={
                  !accelerators.tested && (
                    <span role="status">
                      <Trans>Testing accelerators…</Trans>
                    </span>
                  )
                }
              >
                <Select
                  options={options}
                  selected={draft.accelerator}
                  onChange={(accelerator) => edit({ accelerator })}
                />
              </SettingsRow>
              {usable.map((a) => (
                <AcceleratorRow key={a.id} accelerator={a} />
              ))}
              {unusable.map((a) => (
                <ProblemRow key={a.id} accelerator={a} />
              ))}
            </SettingsCard>
          </SettingsSection>
          <SettingsSection id="limits" title={<Trans>Limits</Trans>}>
            <SettingsCard>
              <SettingsRow label={<Trans>Simultaneous video transcodes</Trans>}>
                <Select
                  options={limits}
                  selected={limitKey(draft.videoTranscodeLimit)}
                  onChange={(id) => edit({ videoTranscodeLimit: limitOf(id) ?? 0 })}
                />
              </SettingsRow>
            </SettingsCard>
          </SettingsSection>
        </SettingsLayout>
      </SettingsForm>
    </AdminPage>
  );
}

/** Why videos are converted on the CPU, when they are, with what to do about it. */
function AcceleratorStatus({ settings, accelerators }: { settings: Settings; accelerators: AcceleratorList }) {
  const usable = accelerators.items.filter((a) => !a.problem);
  if (settings.accelerator === "none") {
    return (
      <Banner tone="warning">
        <Trans>Hardware acceleration is off. Video is transcoded on the CPU.</Trans>
      </Banner>
    );
  }
  if (accelerators.tested && usable.length === 0) {
    return (
      <Banner tone="warning">
        {accelerators.items.length > 0 ? (
          <Trans>No usable GPU. Video is transcoded on the CPU.</Trans>
        ) : (
          <Trans>No GPU found. Video is transcoded on the CPU. To use a GPU, pass it to the container.</Trans>
        )}
      </Banner>
    );
  }
  return null;
}

/**
 * An accelerator the test passed, with its name and what it does in one
 * line, such as "Decodes H.264, HEVC 10-bit · Encodes H.264 · HDR tone
 * mapping on CPU". After its decoders and encoders, only the steps that
 * run on the CPU are named.
 */
function AcceleratorRow({ accelerator: a }: { accelerator: Accelerator }) {
  const { t } = useLingui();
  const formats = a.decoders.map((d) => {
    const name = codecName(d.codec);
    return d.bitDepth > 8 ? t`${name} ${d.bitDepth}-bit` : name;
  });
  const codecs = a.encoders.map((e) => codecName(e.codec));
  const line = [
    formats.length > 0 ? t`Decodes ${formats.join(", ")}` : t`Decoding on CPU`,
    codecs.length > 0 && t`Encodes ${codecs.join(", ")}`,
    a.toneMap === "cpu" && t`HDR tone mapping on CPU`,
    a.burnIn === "cpu" && t`Subtitle burn-in on CPU`,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <div data-part="accelerator" className="flex flex-col gap-0.5 px-4 py-3.5">
      <span className="font-medium text-text">{a.name}</span>
      <span className="text-sm text-text-muted">{line}</span>
    </div>
  );
}

/** An accelerator the test couldn't use, with its name, what's wrong and the fix. */
function ProblemRow({ accelerator }: { accelerator: Accelerator }) {
  const { t } = useLingui();
  const reasons: Record<string, { what: string; fix: string }> = {
    noDeviceNode: {
      what: t`Device not found.`,
      fix: t`Pass /dev/dri, or the NVIDIA devices, to the container.`,
    },
    noPermission: {
      what: t`Permission denied for the device.`,
      fix: t`Add the server user to the device's group (usually render or video).`,
    },
    noDriver: {
      what: t`Driver not found.`,
      fix: t`Install the driver on the host. For NVIDIA, use the NVIDIA container runtime.`,
    },
    testFailed: {
      what: t`Test encode failed.`,
      fix: t`Update the driver, then restart the server.`,
    },
  };
  const problem = accelerator.problem;
  const reason = problem && reasons[problem.reason];
  return (
    <div data-part="accelerator-problem" className="flex flex-col gap-1 px-4 py-3.5">
      <span className="font-medium text-text">{accelerator.name}</span>
      <span className="text-sm text-text" data-reason={problem?.reason}>
        {reason?.what ?? problem?.reason}
      </span>
      {reason && <span className="text-sm font-semibold text-text">{reason.fix}</span>}
      {problem?.detail && <span className="font-mono text-xs break-all text-text-muted">{problem.detail}</span>}
    </div>
  );
}
