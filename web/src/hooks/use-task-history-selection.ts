import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { deleteTaskHistory } from "@/lib/task-history";

export function useTaskHistorySelection<T extends { task_id: string }>({
  items,
  scope,
  canDelete,
  remove,
  kind,
  onDeleted,
}: {
  items: T[];
  scope: string;
  canDelete: (task: T) => boolean;
  remove: (id: string) => Promise<unknown>;
  kind: "image" | "video" | "audio";
  onDeleted: (ids: string[]) => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [selection, setSelection] = useState({ scope, ids: new Set<string>() });
  const eligible = items.filter(canDelete);
  // Changing page/configuration immediately removes old selections. Polling or
  // retrying a selected task cannot leave a now-running task in the delete set.
  const selected = new Set(
    eligible
      .filter(
        (task) => selection.scope === scope && selection.ids.has(task.task_id),
      )
      .map((task) => task.task_id),
  );
  const allSelected = eligible.length > 0 && selected.size === eligible.length;
  useEffect(() => {
    setSelection((previous) => {
      if (previous.scope !== scope) return { scope, ids: new Set<string>() };
      const ids = new Set(
        items
          .filter((task) => canDelete(task) && previous.ids.has(task.task_id))
          .map((task) => task.task_id),
      );
      return ids.size === previous.ids.size ? previous : { scope, ids };
    });
  }, [items, scope, canDelete]);
  const toggle = (id: string) => {
    if (!eligible.some((task) => task.task_id === id)) return;
    const ids = new Set(selected);
    if (ids.has(id)) ids.delete(id);
    else ids.add(id);
    setSelection({ scope, ids });
  };
  const toggleAll = () =>
    setSelection({
      scope,
      ids: allSelected
        ? new Set()
        : new Set(eligible.map((task) => task.task_id)),
    });
  const clear = () => setSelection({ scope, ids: new Set() });
  const batchDelete = useMutation({
    mutationFn: (ids: string[]) => deleteTaskHistory(ids, remove),
    onSuccess: async ({ deleted, failed }) => {
      setSelection({ scope, ids: new Set(failed.map((task) => task.id)) });
      deleted.forEach((id) => {
        queryClient.removeQueries({ queryKey: [`${kind}-task`, id] });
        if (kind !== "image")
          queryClient.removeQueries({ queryKey: [`${kind}-retry-input`, id] });
      });
      if (failed.length) {
        toast.warning(
          t("taskHistory.deletePartial", {
            deleted: deleted.length,
            failed: failed.length,
          }),
        );
      } else {
        toast.success(
          t("taskHistory.deleteSuccess", { count: deleted.length }),
        );
      }
      onDeleted(deleted);
      await queryClient.invalidateQueries({ queryKey: [`${kind}-tasks`] });
    },
    onError: (err) => toast.error(err.message),
  });
  return {
    selected,
    allSelected,
    hasEligible: eligible.length > 0,
    toggle,
    toggleAll,
    clear,
    batchDelete,
  };
}
