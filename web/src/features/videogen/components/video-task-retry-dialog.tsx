import { useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Loader2, RefreshCw, Upload } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  getVideoGenConfigs,
  getVideoRetryInput,
  jsonObject,
  newVideoRequestID,
  retryVideoGeneration,
} from "../api";
import type { VideoTask } from "../api";

export function VideoTaskRetryDialog({
  task,
  variant = "ghost",
}: {
  task: VideoTask;
  variant?: "ghost" | "outline";
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [configID, setConfigID] = useState("");
  const [prompt, setPrompt] = useState(task.prompt);
  const [image, setImage] = useState("");
  const [params, setParams] = useState("{}");
  const [uploading, setUploading] = useState(false);
  const requestID = useRef<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const inputID = "video-retry-" + task.task_id;
  const input = useQuery({
    queryKey: ["video-retry-input", task.task_id],
    queryFn: () => getVideoRetryInput(task.task_id),
    enabled: open,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const configs = useQuery({
    queryKey: ["videogen"],
    queryFn: getVideoGenConfigs,
    enabled: open,
  });
  const operation = input.data?.data.operation ?? task.operation;
  const candidates = (configs.data?.data ?? []).filter(
    (config) => config.operation === "both" || config.operation === operation,
  );
  useEffect(() => {
    if (!open || !input.data) return;
    const {
      prompt: storedPrompt,
      image: storedImage,
      ...parameters
    } = input.data.data.arguments;
    setPrompt(typeof storedPrompt === "string" ? storedPrompt : task.prompt);
    setImage(typeof storedImage === "string" ? storedImage : "");
    setParams(JSON.stringify(parameters, null, 2));
    setConfigID(
      input.data.data.config_id > 0 ? String(input.data.data.config_id) : "",
    );
  }, [open, input.data, task.prompt]);
  useEffect(() => {
    requestID.current = null;
  }, [configID, prompt, image, params]);
  const retry = useMutation({
    mutationFn: () => {
      const parameters = jsonObject(params, t("videogen.retryParams"));
      for (const key of ["prompt", "image", "wait_seconds", "request_id"]) {
        if (key in parameters)
          throw new Error(t("videogen.retryReservedParams"));
      }
      requestID.current ??= newVideoRequestID();
      return retryVideoGeneration(task.task_id, {
        config_id: Number(configID),
        request_id: requestID.current,
        arguments: {
          ...parameters,
          prompt,
          ...(operation === "image" ? { image } : {}),
        },
      });
    },
    onSuccess: (reply) => {
      setOpen(false);
      requestID.current = null;
      queryClient.setQueryData(["video-task", task.task_id], reply);
      queryClient.invalidateQueries({ queryKey: ["video-tasks"] });
      queryClient.invalidateQueries({
        queryKey: ["video-retry-input", task.task_id],
        refetchType: "none",
      });
      toast.success(t("videogen.retrySubmitted"));
    },
    onError: (err) => toast.error(err.message),
  });
  const upload = async (file: File) => {
    setUploading(true);
    try {
      const body = new FormData();
      body.append("image", file);
      const reply = await api.post("/vision/upload", body);
      setImage(reply.data.data.url);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("videogen.uploadFailed"),
      );
    } finally {
      setUploading(false);
      if (fileInput.current) fileInput.current.value = "";
    }
  };
  const canSubmit =
    !!input.data &&
    !input.isError &&
    !configs.isError &&
    candidates.some((config) => String(config.id) === configID) &&
    !!prompt.trim() &&
    (operation !== "image" || !!image.trim());
  return (
    <>
      <Button
        variant={variant}
        size="sm"
        className="gap-1"
        onClick={() => setOpen(true)}
      >
        <RefreshCw className="h-3.5 w-3.5" />
        {t("videogen.retryGenerate")}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (retry.isPending) return;
          setOpen(value);
          if (!value) {
            requestID.current = null;
          }
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("videogen.retryGenerate")}</DialogTitle>
            <DialogDescription>
              {t("videogen.retryGenerateHint")}
            </DialogDescription>
          </DialogHeader>
          {input.isFetching || configs.isLoading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin" />
            </div>
          ) : input.isError || configs.isError ? (
            <p className="text-sm text-destructive">
              {t("videogen.retryLoadFailed")}
            </p>
          ) : (
            <div className="space-y-4">
              {task.error && (
                <p className="break-words rounded-md bg-destructive/10 p-3 text-xs text-destructive">
                  {task.error}
                </p>
              )}
              {!input.data?.data.has_input_snapshot && (
                <p className="text-xs text-muted-foreground">
                  {t("videogen.retryLegacyHint")}
                </p>
              )}
              <div className="space-y-2">
                <Label htmlFor={inputID + "-config"}>
                  {t("videogen.retryConfig")}
                </Label>
                <Select value={configID} onValueChange={setConfigID}>
                  <SelectTrigger id={inputID + "-config"}>
                    <SelectValue
                      placeholder={t("videogen.retrySelectConfig")}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {candidates.map((config) => (
                      <SelectItem key={config.id} value={String(config.id)}>
                        {config.name} / {config.model_name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  {t("videogen.retryConfigHint")}
                </p>
                {!candidates.length && (
                  <Link
                    to="/videogen/create"
                    className="text-sm text-primary hover:underline"
                  >
                    {t("videogen.create")}
                  </Link>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor={inputID + "-prompt"}>
                  {t("videogen.prompt")}
                </Label>
                <Textarea
                  id={inputID + "-prompt"}
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                />
              </div>
              {operation === "image" && (
                <div className="space-y-2">
                  <Label htmlFor={inputID + "-image"}>
                    {t("videogen.reference")}
                  </Label>
                  <Input
                    id={inputID + "-image"}
                    value={image}
                    onChange={(e) => setImage(e.target.value)}
                    placeholder="https://..."
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={uploading}
                    onClick={() => fileInput.current?.click()}
                  >
                    {uploading ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Upload className="mr-2 h-4 w-4" />
                    )}
                    {t("videogen.upload")}
                  </Button>
                  <input
                    ref={fileInput}
                    type="file"
                    accept="image/png,image/jpeg,image/webp"
                    className="hidden"
                    onChange={(e) => {
                      if (e.target.files?.[0]) void upload(e.target.files[0]);
                    }}
                  />
                </div>
              )}
              <div className="space-y-2">
                <Label htmlFor={inputID + "-params"}>
                  {t("videogen.retryParams")}
                </Label>
                <Textarea
                  id={inputID + "-params"}
                  className="min-h-28 font-mono text-xs"
                  value={params}
                  onChange={(e) => setParams(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  {t("videogen.jsonParamsHint")}
                </p>
              </div>
              <div className="flex justify-end gap-2">
                <Button
                  variant="ghost"
                  onClick={() => {
                    setOpen(false);
                    requestID.current = null;
                  }}
                  disabled={retry.isPending}
                >
                  {t("common.cancel")}
                </Button>
                <Button
                  disabled={!canSubmit || retry.isPending || uploading}
                  onClick={() => retry.mutate()}
                  className="gap-2"
                >
                  {retry.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <RefreshCw className="h-4 w-4" />
                  )}
                  {t("videogen.retryConfirm")}
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
