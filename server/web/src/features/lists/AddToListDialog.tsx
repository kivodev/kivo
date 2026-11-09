import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Checkbox } from "react-aria-components";
import { listQueries, useCreateList, useSetInList } from "@/api/lists";
import { workQueries } from "@/api/works";
import { Button } from "@/components/Button";
import { CheckBox } from "@/components/CheckBox";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { ModalDialog } from "@/components/ModalDialog";
import { settingsCardClass } from "@/components/SettingsLayout";
import { ListNameForm } from "./ListNameForm";

type Props = {
  workId: string;
  /** The work's title, under the dialog's. */
  title: string;
  onClose: () => void;
};

/**
 * Add to List: the user's lists, each checked when it holds the work. A
 * press adds or removes at once, so Done only closes. New List makes a list
 * and adds the work to it.
 */
export function AddToListDialog({ workId, title, onClose }: Props) {
  const [creating, setCreating] = useState(false);
  return (
    <ModalDialog
      isOpen
      onClose={onClose}
      title={creating ? <Trans>New List</Trans> : <Trans>Add to List</Trans>}
      subtitle={creating ? undefined : title}
      width="form"
      part="add-to-list"
    >
      {creating ? (
        <NewListForm workId={workId} onDone={() => setCreating(false)} />
      ) : (
        <ListChoices workId={workId} onNewList={() => setCreating(true)} onDone={onClose} />
      )}
    </ModalDialog>
  );
}

function ListChoices({ workId, onNewList, onDone }: { workId: string; onNewList: () => void; onDone: () => void }) {
  const { t } = useLingui();
  const lists = useQuery(listQueries.all());
  const work = useQuery(workQueries.detail(workId));
  const setInList = useSetInList();
  const held = new Set(work.data?.listIds);
  return (
    <div className="flex flex-col gap-4">
      <div className={settingsCardClass}>
        <button
          type="button"
          onClick={onNewList}
          className="flex min-h-12 w-full cursor-pointer items-center gap-3 rounded-lg px-4 py-2 font-medium text-accent hover:bg-surface-hover"
        >
          <Icon name="plus" className="size-5" />
          <Trans>New List</Trans>
        </button>
      </div>
      <FormError error={lists.error ?? work.error ?? setInList.error} />
      {lists.data && work.data && (
        <fieldset aria-label={t`Lists`} className={settingsCardClass}>
          {lists.data.items
            .filter((list) => list.kind === "manual")
            .map((list) => (
              <Checkbox
                key={list.id}
                isSelected={held.has(list.id)}
                onChange={(inList) => setInList.mutate({ listId: list.id, workId, inList })}
                className="group flex min-h-12 cursor-pointer items-center gap-3 px-4 py-2 first:rounded-t-lg last:rounded-b-lg hover:bg-surface-hover"
              >
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate font-medium text-text">{list.name}</span>
                  <span className="text-sm text-text-muted">
                    <Plural value={list.workCount} one="# title" other="# titles" />
                  </span>
                </span>
                <CheckBox />
              </Checkbox>
            ))}
        </fieldset>
      )}
      <div className="flex justify-end">
        <Button variant="primary" onPress={onDone}>
          <Trans>Done</Trans>
        </Button>
      </div>
    </div>
  );
}

/** New List: makes a list, and adds [workId] to it when given. */
export function NewListForm({ workId, onDone }: { workId?: string; onDone: () => void }) {
  const create = useCreateList();
  const setInList = useSetInList();
  return (
    <ListNameForm
      submitLabel={<Trans>Create</Trans>}
      isPending={create.isPending}
      error={create.error}
      onCancel={onDone}
      onSubmit={(name) =>
        create.mutate(
          { body: { name } },
          {
            onSuccess: (list) => {
              if (workId) setInList.mutate({ listId: list.id, workId, inList: true });
              onDone();
            },
          },
        )
      }
    />
  );
}
