import { Trans, useLingui } from "@lingui/react/macro";
import { type ReactNode, useState } from "react";
import { Button } from "@/components/Button";
import { ModalDialog } from "@/components/ModalDialog";
import { isTv } from "@/tv/flag";
import { TvDialogButton } from "@/tv/forms/dialog";
import { TvControl } from "@/tv/forms/TvControl";
import { LicensesDialog } from "./LicensesDialog";

/**
 * Credits, from Settings › About: TMDB's logo and the notice its API terms
 * ask for, then Audnexus and jellyfin-ffmpeg. Each name opens its site,
 * except on TV, where there's nowhere to open it. Under them, Open source
 * licenses opens the build's.
 */
export function CreditsDialog({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const { t } = useLingui();
  const [licenses, setLicenses] = useState(false);
  const link = (href: string, children: ReactNode) =>
    isTv ? (
      children
    ) : (
      <a href={href} target="_blank" rel="noreferrer" className="text-text hover:underline">
        {children}
      </a>
    );
  const tmdb = <img src="tmdb.svg" alt="TMDB" width={273} height={36} className="h-3 w-auto tv:h-4" />;
  const audnexus = link("https://github.com/laxamentumtech/audnexus", "Audnexus");
  const jellyfinFfmpeg = link("https://github.com/jellyfin/jellyfin-ffmpeg", "jellyfin-ffmpeg");
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} title={t`Credits`} width="form" part="credits">
      <div className="flex flex-col items-start gap-1 text-sm text-text-muted">
        <span className="mb-1">{link("https://www.themoviedb.org", tmdb)}</span>
        <p>
          <Trans>This product uses the TMDB API but is not endorsed or certified by TMDB.</Trans>
        </p>
        <p>
          <Trans>Audiobook metadata from {audnexus}</Trans>
        </p>
        <p>
          <Trans>Transcoding by {jellyfinFfmpeg} (GPL)</Trans>
        </p>
        <TvControl focusKey="credits:licenses">
          <Button variant="ghost" size="sm" className="-mx-3 mt-3" onPress={() => setLicenses(true)}>
            <Trans>Open source licenses</Trans>
          </Button>
        </TvControl>
      </div>
      <LicensesDialog isOpen={licenses} onClose={() => setLicenses(false)} />
      <div className="mt-5 flex justify-end">
        {isTv ? (
          <TvDialogButton focusKey="tv:credits:close" autoFocus onPress={onClose}>
            <Trans>Close</Trans>
          </TvDialogButton>
        ) : (
          <Button autoFocus onPress={onClose}>
            <Trans>Close</Trans>
          </Button>
        )}
      </div>
    </ModalDialog>
  );
}
