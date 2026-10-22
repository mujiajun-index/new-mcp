import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { AudioLines, Copy, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { TaskReferenceImages } from "@/components/task-reference-images";
import {
  ContentBlock,
  copyText,
} from "@/features/services/components/test-result-panel";
import * as audio from "../api";

export function AudioTaskStatus({
  task,
  saving = false,
}: {
  task: audio.AudioTask;
  saving?: boolean;
}) {
  const { t } = useTranslation();
  const status = saving ? task.artifact_status : task.status;
  const pending = saving
    ? ["pending", "saving"].includes(status)
    : ["queued", "running"].includes(status);
  return (
    <Badge
      className="whitespace-nowrap"
      variant={
        ["failed", "submission_unknown"].includes(status)
          ? "destructive"
          : status === (saving ? "ready" : "completed")
            ? "success"
            : saving
              ? "outline"
              : "secondary"
      }
    >
      {pending && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
      {t(`audiogen.${saving ? "artifact" : "status"}.${status}`)}
    </Badge>
  );
}

function AudioAsset({
  asset,
  index,
  label,
}: {
  asset: audio.AudioFile;
  index: number;
  label: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-2">
      {asset.url ? (
        <ContentBlock
          index={index}
          block={{
            type: "resource_link",
            uri: asset.url,
            name: label,
            mimeType: asset.mime,
          }}
          downloadUri={audio.audioDownloadURL(asset.url)}
        />
      ) : (
        <div className="flex items-center justify-center gap-2 rounded-lg border border-dashed bg-muted/30 px-3 py-6 text-sm text-muted-foreground">
          <AudioLines className="h-5 w-5" />
          {t("audiogen.expired")}
        </div>
      )}
      <p className="text-xs text-muted-foreground">
        {asset.mime} · {(asset.size / 1048576).toFixed(2)} MiB ·{" "}
        {!asset.url
          ? t("audiogen.expired")
          : asset.expires_at
            ? t("audiogen.expires", {
                date: new Date(asset.expires_at).toLocaleString(),
              })
            : t("audiogen.permanent")}
      </p>
    </div>
  );
}

export function AudioTaskDetail({ id }: { id: string }) {
  const { t } = useTranslation();
  const query = useQuery({
    queryKey: ["audio-task", id],
    queryFn: () => audio.task(id),
    refetchInterval: (q) =>
      q.state.data && audio.active(q.state.data) ? 2000 : false,
  });
  if (query.isError)
    return (
      <p role="alert" className="text-sm text-destructive">
        {t("audiogen.loadFailed")}
      </p>
    );
  if (!query.data)
    return (
      <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
        <Loader2 className="mr-2 h-5 w-5 animate-spin" />
        {t("common.loading")}
      </div>
    );
  const task = query.data;
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="min-w-0 break-all font-mono text-sm">
          {task.model}
        </span>
        <AudioTaskStatus task={task} />
        {task.status === "completed" && <AudioTaskStatus task={task} saving />}
        <Badge variant="outline">{t(`audiogen.${task.kind}`)}</Badge>
      </div>
      <dl className="grid grid-cols-1 gap-3 text-xs text-muted-foreground sm:grid-cols-2">
        <div>
          <dt>{t("audiogen.configuration")}</dt>
          <dd className="mt-1 break-words text-foreground">
            {task.config_name ||
              (task.config_id
                ? `#${task.config_id}`
                : t("audiogen.draftTests"))}
          </dd>
        </div>
        <div>
          <dt>{t("audiogen.taskSource")}</dt>
          <dd className="mt-1 text-foreground">
            {t(`audiogen.taskSources.${task.source}`, {
              defaultValue: task.source || "—",
            })}
          </dd>
        </div>
        <div>
          <dt>{t("common.createdAt")}</dt>
          <dd className="mt-1 text-foreground">
            {new Date(task.created_at).toLocaleString()}
          </dd>
        </div>
        <div>
          <dt>{t("audiogen.protocol")}</dt>
          <dd className="mt-1 break-all font-mono text-foreground">
            {task.protocol}
          </dd>
        </div>
        {!!task.retry_count && (
          <div>
            <dt>{t("audiogen.retryCount")}</dt>
            <dd className="mt-1 text-foreground">{task.retry_count}</dd>
          </div>
        )}
      </dl>
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-medium">
            {t(
              task.kind === "speech"
                ? "audiogen.script"
                : "audiogen.inputContent",
            )}
          </h3>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 gap-1.5 px-2 text-xs"
            disabled={!task.prompt}
            onClick={() =>
              copyText(task.prompt)
                .then(() => toast.success(t("common.copied")))
                .catch(() => toast.error(t("common.copyFailed")))
            }
          >
            <Copy className="h-3.5 w-3.5" />
            {t("common.copyPrompt")}
          </Button>
        </div>
        <p className="max-h-60 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-muted/50 p-3 text-sm [overflow-wrap:anywhere]">
          {task.prompt || "—"}
        </p>
      </div>
      {audio.active(task) && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          {t("audiogen.continues")}
        </p>
      )}
      {task.error && (
        <p className="whitespace-pre-wrap break-words rounded-lg bg-destructive/10 p-3 text-xs text-destructive">
          {task.error}
        </p>
      )}
      {task.last_error && (
        <p className="whitespace-pre-wrap break-words rounded-lg bg-destructive/10 p-3 text-xs text-destructive">
          {task.last_error}
        </p>
      )}
      {!!task.reference_images?.length && (
        <TaskReferenceImages images={task.reference_images} />
      )}
      {!!task.reference_audios?.length && (
        <section className="space-y-3">
          <h3 className="text-sm font-medium">
            {t("audiogen.referenceAudios")}
          </h3>
          {task.reference_audios.map((asset, i) => (
            <AudioAsset
              key={asset.id}
              asset={asset}
              index={i}
              label={`${t("audiogen.referenceAudios")} ${i + 1}`}
            />
          ))}
        </section>
      )}
      {!!task.audios.length && (
        <section className="space-y-3">
          <h3 className="text-sm font-medium">{t("audiogen.taskAudios")}</h3>
          {task.audios.map((asset, i) => (
            <AudioAsset
              key={asset.id}
              asset={asset}
              index={i}
              label={`${t("audiogen.taskAudios")} ${i + 1}`}
            />
          ))}
        </section>
      )}
      {(["text", "transcript", "input_transcript", "lyrics"] as const)
        .filter((k) => !!task[k])
        .map((k) => (
          <details key={k} className="rounded-lg border p-3 text-sm">
            <summary className="cursor-pointer font-medium">
              {t(`audiogen.${k}`)}
            </summary>
            <pre className="mt-3 whitespace-pre-wrap break-words font-sans leading-relaxed">
              {task[k]}
            </pre>
          </details>
        ))}
    </div>
  );
}
