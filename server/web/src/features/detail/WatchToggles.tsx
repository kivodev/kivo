import { useLingui } from "@lingui/react/macro";
import { useState } from "react";
import type { WorkSummary } from "@/api/schemas";
import { useUpdateUserState } from "@/api/works";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Icon } from "@/components/Icon";
import { TvFocusable } from "@/tv/focus";

type Props = {
  work: WorkSummary;
  /** How many episodes marking played covers, so a series or season can ask first. */
  cascadeCount?: number;
};

/** A detail page's favorite and played toggles, for you. */
export function WatchToggles({ work, cascadeCount = 0 }: Props) {
  const { t } = useLingui();
  const update = useUpdateUserState();
  const [confirming, setConfirming] = useState(false);
  const played = work.userState?.played ?? false;
  const favorite = work.userState?.favorite ?? false;
  const id = work.id;
  const setPlayed = (to: boolean) => update.mutate({ params: { path: { id } }, body: { played: to } });

  return (
    <>
      <TvFocusable focusKey="favorite">
        <Button
          size="icon"
          aria-label={favorite ? t`Remove from favorites` : t`Add to favorites`}
          aria-pressed={favorite}
          onPress={() => update.mutate({ params: { path: { id } }, body: { favorite: !favorite } })}
        >
          <Icon name={favorite ? "heartFill" : "heart"} />
        </Button>
      </TvFocusable>
      <TvFocusable focusKey="played">
        <Button
          size="icon"
          aria-label={played ? t`Mark as unwatched` : t`Mark as watched`}
          aria-pressed={played}
          onPress={() => (!played && cascadeCount > 1 ? setConfirming(true) : setPlayed(!played))}
        >
          <Icon name={played ? "checkCircleFill" : "checkCircle"} />
        </Button>
      </TvFocusable>
      <ConfirmDialog
        isOpen={confirming}
        title={t`Mark ${cascadeCount} episodes as watched?`}
        confirmLabel={t`Mark as watched`}
        onConfirm={() => {
          setConfirming(false);
          setPlayed(true);
        }}
        onCancel={() => setConfirming(false)}
      />
    </>
  );
}
