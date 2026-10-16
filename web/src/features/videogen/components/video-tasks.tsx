import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Copy, Download, Film, Loader2, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  ContentBlock,
  copyText,
} from "@/features/services/components/test-result-panel";
import {
  deleteVideoTask,
  getVideoTask,
  listVideoTasks,
  retryVideoDownload,
  taskIsActive,
  videoDownloadURL,
} from "../api";
import type { VideoTask } from "../api";
import { VideoTaskRetryDialog } from "./video-task-retry-dialog";

function useVideoTaskActions(task: VideoTask, onDeleted?: () => void) {
  const queryClient = useQueryClient();
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["video-task", task.task_id] });
    queryClient.invalidateQueries({ queryKey: ["video-tasks"] });
  };
  const retry = useMutation({
    mutationFn: () => retryVideoDownload(task.task_id),
    onSuccess: refresh,
    onError: (err) => toast.error(err.message),
  });
  const resume = useMutation({
    mutationFn: () => getVideoTask(task.task_id),
    onSuccess: refresh,
    onError: (err) => toast.error(err.message),
  });
  const remove = useMutation({
    mutationFn: () => deleteVideoTask(task.task_id),
    onSuccess: () => {
      refresh();
      onDeleted?.();
    },
    onError: (err) => toast.error(err.message),
  });
  return { retry, resume, remove };
}

export function VideoTaskCard({
  task,
  onDeleted,
}: {
  task: VideoTask;
  onDeleted?: () => void;
}) {
  const { t } = useTranslation();
  const { retry, resume, remove } = useVideoTaskActions(task, onDeleted);
  const active = taskIsActive(task);
  return (
    <article className="space-y-3 rounded-xl border bg-card p-4 sm:p-5">
      <div className="flex flex-wrap items-center gap-2">
        {active ? (
          <Loader2 className="h-4 w-4 animate-spin text-primary" />
        ) : (
          <Film className="h-4 w-4 text-muted-foreground" />
        )}
        <span className="min-w-0 break-all text-sm font-semibold">
          {task.model}
        </span>
        <Badge
          variant={
            task.status === "failed" || task.status === "submission_unknown"
              ? "destructive"
              : "secondary"
          }
        >
          {t(`videogen.status.${task.status}`)}
        </Badge>
        {task.status === "completed" && (
          <Badge variant="outline">
            {t(`videogen.artifact.${task.artifact_status}`)}
          </Badge>
        )}
        <span className="ml-auto text-xs text-muted-foreground">
          {new Date(task.created_at).toLocaleString()}
        </span>
      </div>
      <p className="whitespace-pre-wrap text-sm [overflow-wrap:anywhere]">
        {task.prompt}
      </p>
      {active && (
        <div className="space-y-1">
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>{t("videogen.continues")}</span>
            <span>{task.progress}%</span>
          </div>
          <div
            role="progressbar"
            aria-label={t("videogen.progress")}
            aria-valuenow={task.progress}
            aria-valuemin={0}
            aria-valuemax={100}
            className="h-1.5 overflow-hidden rounded-full bg-muted"
          >
            <div
              className="h-full rounded-full bg-primary transition-all"
              style={{ width: `${task.progress}%` }}
            />
          </div>
        </div>
      )}
      {task.error && (
        <p className="break-words rounded-md bg-destructive/10 p-3 text-xs text-destructive">
          {task.error}
        </p>
      )}
      {task.last_error && (
        <p className="break-words text-xs text-muted-foreground">
          {task.last_error}
        </p>
      )}
      {task.videos.map((video, index) => (
        <div key={video.url} className="space-y-1">
          <ContentBlock
            block={{
              type: "resource_link",
              uri: video.url,
              name: task.task_id,
              mimeType: video.mime,
            }}
            index={index}
            downloadUri={videoDownloadURL(video.url)}
          />
          <p className="text-xs text-muted-foreground">
            {t(video.expires_at ? "videogen.expires" : "videogen.permanent", {
              date: video.expires_at
                ? new Date(video.expires_at).toLocaleString()
                : undefined,
              size: (video.size / 1024 / 1024).toFixed(1),
            })}
          </p>
        </div>
      ))}
      <div className="flex flex-wrap items-center gap-2 border-t pt-3">
        <code className="min-w-0 break-all text-[11px] text-muted-foreground">
          {task.task_id}
        </code>
        <Button
          variant="ghost"
          size="sm"
          aria-label={t("videogen.copyTask")}
          onClick={() =>
            copyText(task.task_id)
              .then(() => toast.success(t("videogen.copied")))
              .catch(() => toast.error(t("common.copyFailed")))
          }
        >
          <Copy className="h-3.5 w-3.5" />
        </Button>
        {task.tracking_paused && (
          <Button
            size="sm"
            variant="outline"
            disabled={resume.isPending}
            onClick={() => resume.mutate()}
          >
            {t("videogen.resume")}
          </Button>
        )}
        {task.status === "completed" && task.artifact_status === "failed" && (
          <Button
            size="sm"
            variant="outline"
            disabled={retry.isPending}
            onClick={() => retry.mutate()}
          >
            <RefreshCw className="mr-2 h-3.5 w-3.5" />
            {t("videogen.retrySave")}
          </Button>
        )}
        {task.status === "failed" && (
          <VideoTaskRetryDialog task={task} variant="outline" />
        )}
        {!active && (
          <Button
            className="ml-auto text-destructive"
            size="sm"
            variant="ghost"
            disabled={remove.isPending}
            onClick={() => {
              if (confirm(t("videogen.deleteTaskConfirm"))) remove.mutate();
            }}
          >
            <Trash2 className="mr-2 h-3.5 w-3.5" />
            {t("common.delete")}
          </Button>
        )}
      </div>
    </article>
  );
}

export function VideoTaskResult({
  id,
  onDeleted,
}: {
  id: string;
  onDeleted?: () => void;
}) {
  const { t } = useTranslation();
  const { data, isError } = useQuery({
    queryKey: ["video-task", id],
    queryFn: () => getVideoTask(id),
    refetchInterval: (query) =>
      query.state.data && taskIsActive(query.state.data.data) ? 5000 : false,
  });
  if (isError)
    return (
      <p className="text-sm text-destructive">{t("videogen.taskLoadFailed")}</p>
    );
  if (!data) return <Loader2 className="h-4 w-4 animate-spin" />;
  return <VideoTaskCard task={data.data} onDeleted={onDeleted} />;
}

function VideoTaskRow({
  task,
  configID,
  onDetails,
  onDeleted,
}: {
  task: VideoTask;
  configID?: number;
  onDetails: () => void;
  onDeleted: () => void;
}) {
  const { t } = useTranslation();
  const { retry, resume, remove } = useVideoTaskActions(task, onDeleted);
  const active = taskIsActive(task);
  const busy = retry.isPending || resume.isPending || remove.isPending;
  const video = task.videos[0];
  return (
    <tr className="border-b transition-colors last:border-0 hover:bg-muted/30">
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
              {task.config_name || t("videogen.draftConfig")}
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
              aria-label={t("videogen.copyTask")}
              onClick={() =>
                copyText(task.task_id)
                  .then(() => toast.success(t("videogen.copied")))
                  .catch(() => toast.error(t("common.copyFailed")))
              }
            >
              <Copy className="h-3 w-3" />
            </Button>
          </div>
        </div>
      </td>
      <td className="px-4 py-3">
        <p
          className="line-clamp-2 min-w-32 max-w-60 whitespace-pre-wrap break-words text-xs"
          title={task.prompt}
        >
          {task.prompt}
        </p>
      </td>
      <td className="px-4 py-3">
        <div className="space-y-1.5">
          <Badge
            className="whitespace-nowrap"
            variant={
              task.status === "failed" || task.status === "submission_unknown"
                ? "destructive"
                : "secondary"
            }
          >
            {t(`videogen.status.${task.status}`)}
          </Badge>
          {active && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Loader2 className="h-3 w-3 animate-spin" />
              {task.progress}%
            </div>
          )}
          {task.tracking_paused && (
            <p className="whitespace-nowrap text-xs text-muted-foreground">
              {t("videogen.trackingPaused")}
            </p>
          )}
        </div>
      </td>
      <td className="px-4 py-3">
        {task.status === "completed" ? (
          <Badge
            className="whitespace-nowrap"
            variant={
              task.artifact_status === "ready"
                ? "success"
                : task.artifact_status === "failed"
                  ? "destructive"
                  : "outline"
            }
          >
            {t(`videogen.artifact.${task.artifact_status}`)}
          </Badge>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">
        {new Date(task.created_at).toLocaleString()}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">
        {task.expires_at
          ? new Date(task.expires_at).toLocaleString()
          : task.artifact_status === "ready"
            ? t("videogen.permanentLabel")
            : "—"}
      </td>
      <td className="px-4 py-3 text-right">
        <div className="flex items-center justify-end gap-1 whitespace-nowrap">
          <Button variant="ghost" size="sm" onClick={onDetails}>
            {t("videogen.detail")}
          </Button>
          {video && (
            <Button variant="ghost" size="icon" className="h-8 w-8" asChild>
              <a
                href={videoDownloadURL(video.url)}
                download
                aria-label={t("videogen.download")}
                title={t("videogen.download")}
              >
                <Download className="h-4 w-4" />
              </a>
            </Button>
          )}
          {task.tracking_paused && (
            <Button
              variant="ghost"
              size="sm"
              disabled={busy}
              onClick={() => resume.mutate()}
            >
              {t("videogen.resume")}
            </Button>
          )}
          {task.status === "completed" && task.artifact_status === "failed" && (
            <Button
              variant="ghost"
              size="sm"
              className="gap-1"
              disabled={busy}
              onClick={() => retry.mutate()}
            >
              <RefreshCw className="h-3.5 w-3.5" />
              {t("videogen.retrySave")}
            </Button>
          )}
          {task.status === "failed" && <VideoTaskRetryDialog task={task} />}
          {!active && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-destructive hover:text-destructive"
              aria-label={t("common.delete")}
              title={t("common.delete")}
              disabled={busy}
              onClick={() => {
                if (confirm(t("videogen.deleteTaskConfirm"))) remove.mutate();
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          )}
        </div>
      </td>
    </tr>
  );
}

export function VideoTasks({ configID }: { configID?: number }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [selectedTaskID, setSelectedTaskID] = useState<string | null>(null);
  const { data, isLoading, isError } = useQuery({
    queryKey: ["video-tasks", configID, page],
    queryFn: () => listVideoTasks(configID, page),
    refetchInterval: (query) =>
      query.state.data?.data.items.some(taskIsActive) ? 5000 : false,
  });
  const pageSize = data?.data.page_size ?? 20;
  const totalPages = Math.max(1, Math.ceil((data?.data.total ?? 0) / pageSize));
  const handleDeleted = () => {
    setSelectedTaskID(null);
    if (page > 1 && data?.data.items.length === 1) setPage(page - 1);
  };
  return (
    <section className="space-y-4">
      <div>
        <h2 className="text-base font-semibold">{t("videogen.history")}</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("videogen.historyHint")}
        </p>
      </div>
      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t("common.loading")}
          </div>
        ) : isError ? (
          <p className="p-8 text-center text-sm text-destructive">
            {t("videogen.taskLoadFailed")}
          </p>
        ) : !data?.data.items.length ? (
          <p className="p-12 text-center text-sm text-muted-foreground">
            {t("videogen.noTasks")}
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table
              className="w-full text-sm"
              aria-label={t("videogen.history")}
            >
              <thead>
                <tr className="border-b bg-muted/50">
                  {[
                    "videogen.task",
                    "videogen.prompt",
                    "videogen.generationStatus",
                    "videogen.savingStatus",
                    "common.createdAt",
                    "videogen.expiresAt",
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
                {data.data.items.map((task) => (
                  <VideoTaskRow
                    key={task.task_id}
                    task={task}
                    configID={configID}
                    onDetails={() => setSelectedTaskID(task.task_id)}
                    onDeleted={handleDeleted}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {totalPages > 1 && (
        <div className="flex items-center justify-end gap-3">
          <Button
            variant="outline"
            size="sm"
            disabled={page === 1 || isLoading}
            onClick={() => setPage(page - 1)}
          >
            {t("videogen.previous")}
          </Button>
          <span className="text-sm">
            {page} / {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages || isLoading}
            onClick={() => setPage(page + 1)}
          >
            {t("videogen.next")}
          </Button>
        </div>
      )}
      <Dialog
        open={!!selectedTaskID}
        onOpenChange={(open) => {
          if (!open) setSelectedTaskID(null);
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t("videogen.taskDetail")}</DialogTitle>
            <DialogDescription className="break-all">
              {selectedTaskID}
            </DialogDescription>
          </DialogHeader>
          {selectedTaskID && (
            <VideoTaskResult
              key={selectedTaskID}
              id={selectedTaskID}
              onDeleted={handleDeleted}
            />
          )}
        </DialogContent>
      </Dialog>
    </section>
  );
}
