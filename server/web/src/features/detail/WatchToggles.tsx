import { useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { listQueries, useSetInList } from "@/api/lists";
import type { WorkDetail } from "@/api/schemas";
import { useUpdateUserState } from "@/api/works";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Icon } from "@/components/Icon";
import { TvFocusable } from "@/tv/focus";

type Props = {
  work: WorkDetail;
  /** How many episodes marking played covers, so a series or season can ask first. */
  cascadeCount?: number;
};

/** A detail page's My List and played toggles, for you. */
export function WatchToggles({ work, cascadeCount = 0 }: Props) {
  const { t } = useLingui();
  const update = useUpdateUserState();
  const [confirming, setConfirming] = useState(false);
  const played = work.userState?.played ?? false;
  const myList = useQuery(listQueries.all()).data?.items.find((list) => list.builtIn);
  const setInList = useSetInList();
  const onMyList = myList !== undefined && (work.listIds ?? []).includes(myList.id);
  const id = work.id;
  const setPlayed = (to: boolean) => update.mutate({ params: { path: { id } }, body: { played: to } });

  return (
    <>
      <TvFocusable focusKey="my-list">
        <Button
          size="icon"
          aria-label={onMyList ? t`Remove from Watchlist` : t`Add to Watchlist`}
          aria-pressed={onMyList}
          isDisabled={myList === undefined}
          onPress={() => myList && setInList.mutate({ listId: myList.id, workId: id, inList: !onMyList })}
        >
          <Icon name={onMyList ? "listCheck" : "listPlus"} />
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
