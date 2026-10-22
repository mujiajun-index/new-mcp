import {
  TaskHistoryMobileCard,
  TaskHistoryMobileSelectAll,
} from "@/components/task-history-mobile-card";
import { useIsMobile } from "@/hooks/use-mobile";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Copy, Download, Loader2, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LocalPager } from "@/components/local-pager";
import {
  TaskHistoryBatchActions,
  TaskHistoryCheckbox,
  TaskHistoryStatusFilter,
} from "@/components/task-history-actions";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { copyText } from "@/features/services/components/test-result-panel";
import { useTaskHistorySelection } from "@/hooks/use-task-history-selection";
import { historyPageCount } from "@/lib/task-history";
import * as audio from "../api";
import { AudioTaskDetail, AudioTaskStatus } from "./audio-task-detail";

function RetryDialog({
  task,
  close,
}: {
  task: audio.AudioTask;
  close: () => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const available = useQuery({
    queryKey: ["audiogen"],
    queryFn: audio.configs,
  });
  const input = useQuery({
    queryKey: ["audio-retry-input", task.task_id],
    queryFn: () => audio.retryInput(task.task_id),
    retry: false,
  });
  const [configID, setConfigID] = useState(task.execution_config_id);
  const [params, setParams] = useState(
    JSON.stringify(
      { [task.kind === "speech" ? "text" : "prompt"]: task.prompt },
      null,
      2,
    ),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (input.data) {
      setConfigID(input.data.config_id);
      setParams(JSON.stringify(input.data.arguments, null, 2));
    }
  }, [input.data]);
  const choices =
    available.data?.filter((c) =>
      c.models.some((s) => s.kind === task.kind && s.enabled),
    ) || [];
  async function run() {
    setBusy(true);
    setError("");
    try {
      await audio.retry(task.task_id, configID, audio.object(params));
      qc.invalidateQueries({ queryKey: ["audio-tasks"] });
      qc.invalidateQueries({ queryKey: ["audio-task", task.task_id] });
      qc.invalidateQueries({ queryKey: ["audio-retry-input", task.task_id] });
      close();
    } catch {
      setError(t("audiogen.failed"));
    } finally {
      setBusy(false);
    }
  }
  async function upload(file?: File, image = false) {
    if (!file) return;
    setBusy(true);
    try {
      const url = await audio.upload(file, image);
      const p = audio.object(params);
      const field = image ? "images" : "audios";
      p[field] = [
        ...(Array.isArray(p[field]) ? (p[field] as unknown[]) : []),
        url,
      ];
      setParams(JSON.stringify(p, null, 2));
    } catch {
      setError(t("audiogen.uploadFailed"));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v) close();
      }}
    >
      <DialogContent className="max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("audiogen.retry")}</DialogTitle>
          <DialogDescription>{t("audiogen.retryHint")}</DialogDescription>
        </DialogHeader>
        {input.isError && (
          <p className="text-sm text-destructive">
            {t("audiogen.expiredRetry")}
          </p>
        )}
        <label className="space-y-1 text-sm">
          {t("audiogen.configuration")}
          <select
            className="h-9 w-full rounded-md border bg-background px-3"
            value={configID}
            onChange={(e) => setConfigID(Number(e.target.value))}
          >
            <option value={0}>{t("audiogen.selectConfig")}</option>
            {choices.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-1 text-sm">
          {t("audiogen.parameters")}
          <Textarea
            value={params}
            onChange={(e) => setParams(e.target.value)}
            className="min-h-64 font-mono text-xs"
          />
        </label>
        {["audio", "music"].includes(task.kind) && (
          <label className="cursor-pointer rounded-md border p-2 text-center text-sm">
            {t("audiogen.uploadReplacement")}
            <input
              className="sr-only"
              type="file"
              accept={
                task.kind === "music"
                  ? "image/png,image/jpeg,image/webp"
                  : "audio/wav,audio/mpeg"
              }
              onChange={(e) => {
                void upload(e.target.files?.[0], task.kind === "music");
                e.target.value = "";
              }}
            />
          </label>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Button
          disabled={busy || !choices.some((c) => c.id === configID)}
          onClick={run}
        >
          {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {t("audiogen.retry")}
        </Button>
      </DialogContent>
    </Dialog>
  );
}

export function AudioTasks({ configID }: { configID?: number }) {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const qc = useQueryClient();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("all");
  const [kind, setKind] = useState("all");
  const [filterConfig, setFilterConfig] = useState("all");
  const [selectedID, setSelectedID] = useState<string>();
  const [retryTask, setRetryTask] = useState<audio.AudioTask>();
  const configList = useQuery({
    queryKey: ["audiogen"],
    queryFn: audio.configs,
    enabled: configID === undefined,
  });
  const query = useQuery({
    queryKey: ["audio-tasks", configID, filterConfig, page, status, kind],
    queryFn: () =>
      audio.tasks({
        page,
        config_id:
          configID ??
          (filterConfig === "all" ? undefined : Number(filterConfig)),
        status,
        kind,
      }),
    refetchInterval: (q) =>
      q.state.data?.items.some(audio.active) ? 2000 : false,
  });
  const items = query.data?.items || [];
  const total = query.data?.total ?? 0;
  const pages = historyPageCount(total, query.data?.page_size ?? 10);
  const handleDeleted = (ids: string[]) => {
    setSelectedID((id) => (id && ids.includes(id) ? undefined : id));
    setRetryTask((task) =>
      task && ids.includes(task.task_id) ? undefined : task,
    );
  };
  const remove = useMutation({
    mutationFn: audio.removeTask,
    onSuccess: (_, id) => {
      toast.success(t("audiogen.taskDeleted"));
      handleDeleted([id]);
      qc.removeQueries({ queryKey: ["audio-task", id] });
      qc.removeQueries({ queryKey: ["audio-retry-input", id] });
      qc.invalidateQueries({ queryKey: ["audio-tasks"] });
    },
    onError: (err) => toast.error(err.message),
  });
  const retrySave = useMutation({
    mutationFn: audio.retrySave,
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ["audio-task", id] });
      qc.invalidateQueries({ queryKey: ["audio-tasks"] });
    },
    onError: (err) => toast.error(err.message),
  });
  const selection = useTaskHistorySelection({
    items,
    scope: `${configID ?? "all"}:${filterConfig}:${page}:${status}:${kind}`,
    canDelete: (task) => !audio.active(task),
    remove: audio.removeTask,
    kind: "audio",
    onDeleted: handleDeleted,
  });
  const busy =
    remove.isPending || retrySave.isPending || selection.batchDelete.isPending;
  useEffect(() => {
    if (query.data && page > pages) setPage(pages);
  }, [query.data, page, pages]);

  const taskActions = (task: audio.AudioTask) => {
    const ready = task.audios.filter((asset) => !!asset.url);
    return (
      <>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setSelectedID(task.task_id)}
        >
          {t("common.details")}
        </Button>
        {ready[0]?.url && (
          <Button variant="ghost" size="icon" className="h-8 w-8" asChild>
            <a
              href={audio.audioDownloadURL(ready[0].url)}
              download
              aria-label={t("audiogen.download")}
              title={t("audiogen.download")}
            >
              <Download className="h-4 w-4" />
            </a>
          </Button>
        )}
        {task.can_retry_save && (
          <Button
            variant="ghost"
            size="sm"
            className="gap-1"
            disabled={busy}
            onClick={() => retrySave.mutate(task.task_id)}
          >
            <RefreshCw className="h-3.5 w-3.5" />
            {t("audiogen.retrySave")}
          </Button>
        )}
        {task.status === "failed" && (
          <Button
            variant="ghost"
            size="sm"
            className="gap-1"
            disabled={busy}
            onClick={() => setRetryTask(task)}
          >
            <RefreshCw className="h-3.5 w-3.5" />
            {t("audiogen.retry")}
          </Button>
        )}
        {!audio.active(task) && (
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 text-destructive hover:text-destructive"
            aria-label={t("common.delete")}
            title={t("common.delete")}
            disabled={busy}
            onClick={() => {
              if (confirm(t("audiogen.deleteTaskConfirm")))
                remove.mutate(task.task_id);
            }}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
      </>
    );
  };

  return (
    <section className="min-w-0 space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{t("audiogen.history")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("audiogen.historyHint")}
          </p>
        </div>
        <div className="grid w-full min-w-0 grid-cols-2 gap-2 sm:flex sm:w-auto sm:flex-wrap sm:items-center">
          <Select
            value={kind}
            disabled={busy}
            onValueChange={(value) => {
              setKind(value);
              setPage(1);
            }}
          >
            <SelectTrigger
              className="h-8 w-full min-w-0 text-xs sm:w-32 [&>span]:truncate"
              aria-label={t("audiogen.filterKind")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("audiogen.allKinds")}</SelectItem>
              {audio.kinds.map((k) => (
                <SelectItem key={k} value={k}>
                  {t(`audiogen.${k}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <TaskHistoryStatusFilter
            kind="audio"
            className="w-full min-w-0 sm:w-36"
            value={status}
            disabled={busy}
            onChange={(value) => {
              setStatus(value);
              setPage(1);
            }}
          />
          {configID === undefined && (
            <Select
              value={filterConfig}
              disabled={busy}
              onValueChange={(value) => {
                setFilterConfig(value);
                setPage(1);
              }}
            >
              <SelectTrigger
                className="h-8 w-full min-w-0 text-xs sm:w-44 [&>span]:truncate"
                aria-label={t("audiogen.configuration")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("audiogen.allConfigs")}</SelectItem>
                <SelectItem value="0">{t("audiogen.draftTests")}</SelectItem>
                {configList.data?.map((c) => (
                  <SelectItem key={c.id} value={String(c.id)}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <Button
            size="sm"
            variant="outline"
            disabled={query.isFetching || busy}
            onClick={() => query.refetch()}
            className="shrink-0 gap-2"
          >
            <RefreshCw
              className={`h-3.5 w-3.5 ${query.isFetching ? "animate-spin" : ""}`}
            />
            {t("common.refresh")}
          </Button>
        </div>
      </div>
      <TaskHistoryBatchActions
        count={selection.selected.size}
        busy={busy}
        kind="audio"
        onDelete={() => selection.batchDelete.mutate([...selection.selected])}
        onClear={selection.clear}
      />
      <div className="overflow-hidden rounded-xl border bg-card">
        {query.isPending ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t("common.loading")}
          </div>
        ) : query.isError ? (
          <p role="alert" className="p-8 text-center text-sm text-destructive">
            {t("audiogen.loadFailed")}
          </p>
        ) : !items.length ? (
          <p className="p-12 text-center text-sm text-muted-foreground">
            {t(
              status === "all" && kind === "all" && filterConfig === "all"
                ? "audiogen.emptyHistory"
                : "audiogen.noMatchingTasks",
            )}
          </p>
        ) : isMobile ? (
          <div>
            <TaskHistoryMobileSelectAll
              checked={
                selection.allSelected
                  ? true
                  : selection.selected.size > 0
                    ? "mixed"
                    : false
              }
              disabled={busy || !selection.hasEligible}
              onChange={selection.toggleAll}
            />
            <div className="divide-y">
              {items.map((task) => {
                const ready = task.audios.filter((asset) => !!asset.url);
                const times = (ready.length ? ready : task.audios)
                  .flatMap((asset) =>
                    asset.expires_at
                      ? [new Date(asset.expires_at).getTime()]
                      : [],
                  )
                  .filter(Number.isFinite);
                const expires = times.length
                  ? new Date(Math.min(...times)).toLocaleString()
                  : ready.length
                    ? t("audiogen.permanent")
                    : task.audios.length
                      ? t("audiogen.expired")
                      : "—";
                return (
                  <TaskHistoryMobileCard
                    key={task.task_id}
                    taskID={task.task_id}
                    model={task.model}
                    configName={
                      configID === undefined
                        ? task.config_name ||
                          (task.config_id
                            ? `#${task.config_id}`
                            : t("audiogen.draftTests"))
                        : undefined
                    }
                    prompt={task.prompt}
                    badge={<AudioTaskStatus task={task} />}
                    meta={[
                      {
                        label: t("audiogen.filterKind"),
                        value: t(`audiogen.${task.kind}`),
                      },
                      {
                        label: t("audiogen.taskSource"),
                        value: t(`audiogen.taskSources.${task.source}`, {
                          defaultValue: task.source || "—",
                        }),
                      },
                      {
                        label: t("audiogen.taskAudios"),
                        value: task.audios.length
                          ? t("audiogen.audioCount", {
                              ready: ready.length,
                              total: task.audios.length,
                            })
                          : "—",
                      },
                      {
                        label: t("audiogen.savingStatus"),
                        value:
                          task.status === "completed" ? (
                            <AudioTaskStatus task={task} saving />
                          ) : (
                            "—"
                          ),
                      },
                      {
                        label: t("common.createdAt"),
                        value: new Date(task.created_at).toLocaleString(),
                      },
                      { label: t("audiogen.expiresAt"), value: expires },
                    ]}
                    selected={selection.selected.has(task.task_id)}
                    disabled={busy || audio.active(task)}
                    onToggle={() => selection.toggle(task.task_id)}
                    onDetails={() => setSelectedID(task.task_id)}
                    actions={taskActions(task)}
                  />
                );
              })}
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table
              className="w-full text-sm"
              aria-label={t("audiogen.history")}
            >
              <thead>
                <tr className="border-b bg-muted/50">
                  <th scope="col" className="w-12 px-3 py-3">
                    <TaskHistoryCheckbox
                      checked={
                        selection.allSelected
                          ? true
                          : selection.selected.size > 0
                            ? "mixed"
                            : false
                      }
                      disabled={busy || !selection.hasEligible}
                      label={t("taskHistory.selectAll")}
                      onChange={selection.toggleAll}
                    />
                  </th>
                  {[
                    "audiogen.task",
                    "audiogen.kindSource",
                    "audiogen.inputContent",
                    "audiogen.generationStatus",
                    "audiogen.savingStatus",
                    "audiogen.taskAudios",
                    "common.createdAt",
                    "audiogen.expiresAt",
                  ].map((label) => (
                    <th
                      key={label}
                      scope="col"
                      className="whitespace-nowrap px-4 py-3 text-left font-medium text-muted-foreground"
                    >
                      {t(label)}
                    </th>
                  ))}
                  <th
                    scope="col"
                    className="px-4 py-3 text-right font-medium text-muted-foreground"
                  >
                    {t("common.actions")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((task) => {
                  const ready = task.audios.filter((asset) => !!asset.url);
                  const expiryTimes = (ready.length ? ready : task.audios)
                    .flatMap((asset) =>
                      asset.expires_at
                        ? [new Date(asset.expires_at).getTime()]
                        : [],
                    )
                    .filter(Number.isFinite);
                  const expiresAt = expiryTimes.length
                    ? Math.min(...expiryTimes)
                    : undefined;
                  return (
                    <tr
                      key={task.task_id}
                      className={`border-b transition-colors last:border-0 hover:bg-muted/30 ${selection.selected.has(task.task_id) ? "bg-muted/40" : ""}`}
                    >
                      <td className="px-3 py-3">
                        <TaskHistoryCheckbox
                          checked={selection.selected.has(task.task_id)}
                          disabled={busy || audio.active(task)}
                          label={t("taskHistory.selectTask", {
                            id: task.task_id,
                          })}
                          onChange={() => selection.toggle(task.task_id)}
                        />
                      </td>
                      <td className="px-4 py-3">
                        <div className="min-w-40 max-w-52 space-y-1">
                          <span className="block break-all font-mono text-xs">
                            {task.model}
                          </span>
                          {configID === undefined && (
                            <p
                              className="truncate text-xs text-muted-foreground"
                              title={task.config_name}
                            >
                              {task.config_name ||
                                (task.config_id
                                  ? `#${task.config_id}`
                                  : t("audiogen.draftTests"))}
                            </p>
                          )}
                          <div className="flex items-center gap-1">
                            <code
                              className="truncate text-[11px] text-muted-foreground"
                              title={task.task_id}
                            >
                              {task.task_id}
                            </code>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-6 w-6 shrink-0"
                              aria-label={t("audiogen.copyTask")}
                              onClick={() =>
                                copyText(task.task_id)
                                  .then(() => toast.success(t("common.copied")))
                                  .catch(() =>
                                    toast.error(t("common.copyFailed")),
                                  )
                              }
                            >
                              <Copy className="h-3 w-3" />
                            </Button>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs">
                        <span className="whitespace-nowrap">
                          {t(`audiogen.${task.kind}`)}
                        </span>
                        <p className="mt-1 whitespace-nowrap text-muted-foreground">
                          {t(`audiogen.taskSources.${task.source}`, {
                            defaultValue: task.source || "—",
                          })}
                        </p>
                      </td>
                      <td className="px-4 py-3">
                        <p
                          className="line-clamp-2 min-w-32 max-w-60 whitespace-pre-wrap break-words text-xs"
                          title={task.prompt}
                        >
                          {task.prompt || "—"}
                        </p>
                      </td>
                      <td className="px-4 py-3">
                        <AudioTaskStatus task={task} />
                      </td>
                      <td className="px-4 py-3">
                        {task.status === "completed" ? (
                          <AudioTaskStatus task={task} saving />
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">
                        {task.audios.length
                          ? t("audiogen.audioCount", {
                              ready: ready.length,
                              total: task.audios.length,
                            })
                          : "—"}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">
                        {new Date(task.created_at).toLocaleString()}
                      </td>
                      <td
                        className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground"
                        title={
                          task.audios.length > 1
                            ? t("audiogen.earliestExpires")
                            : undefined
                        }
                      >
                        {expiresAt !== undefined
                          ? new Date(expiresAt).toLocaleString()
                          : ready.length
                            ? t("audiogen.permanent")
                            : task.audios.length
                              ? t("audiogen.expired")
                              : "—"}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1 whitespace-nowrap">
                          {taskActions(task)}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {query.data && !query.isError && (
        <LocalPager
          total={total}
          page={page}
          totalPages={pages}
          onPage={setPage}
          disabled={busy || query.isFetching}
        />
      )}
      <Dialog
        open={!!selectedID}
        onOpenChange={(open) => {
          if (!open) setSelectedID(undefined);
        }}
      >
        <DialogContent className="max-h-[90dvh] overflow-y-auto p-4 sm:max-w-3xl sm:p-6">
          <DialogHeader>
            <DialogTitle>{t("audiogen.taskDetail")}</DialogTitle>
            <DialogDescription className="break-all">
              {selectedID}
            </DialogDescription>
          </DialogHeader>
          {selectedID && <AudioTaskDetail key={selectedID} id={selectedID} />}
        </DialogContent>
      </Dialog>
      {retryTask && (
        <RetryDialog task={retryTask} close={() => setRetryTask(undefined)} />
      )}
    </section>
  );
}
