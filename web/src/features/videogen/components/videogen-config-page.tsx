import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  CheckCircle2,
  Film,
  List,
  Lock,
  Loader2,
  Power,
  PowerOff,
  RotateCcw,
  Save,
  Upload,
  Wrench,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  ContentBlock,
  TestResultPanel,
} from "@/features/services/components/test-result-panel";
import type { ToolCallResult } from "@/types";
import {
  createVideoGenConfig,
  defaultVideoEndpoint,
  videoProtocolLabel,
  getVideoGenConfig,
  jsonObject,
  newVideoRequestID,
  listVideoGenModels,
  testVideoGenConfig,
  toggleVideoGenConfig,
  updateVideoGenConfig,
  videoEndpoint,
  videoDownloadURL,
} from "../api";
import type { ConfigInput, VideoGenConfig } from "../api";
import { VideoTaskResult, VideoTasks } from "./video-tasks";

const initial = {
  name: "",
  description: "",
  protocol: "openai_video" as VideoGenConfig["protocol"],
  model_name: "",
  endpoint_url: "",
  api_key: "",
  operation: "both" as VideoGenConfig["operation"],
  image_request_format: "multipart" as VideoGenConfig["image_request_format"],
  request_timeout_seconds: 30,
  task_timeout_seconds: 3600,
};

const requiredMark = (
  <span aria-hidden="true" className="text-destructive">
    *
  </span>
);

const descriptionFields = [
  ["generate_video_desc", "generate_video", "generate_video_desc_default"],
  [
    "generate_video_from_image_desc",
    "generate_video_from_image",
    "generate_video_from_image_desc_default",
  ],
  [
    "get_video_status_desc",
    "get_video_status",
    "get_video_status_desc_default",
  ],
] as const;

export function VideoGenConfigPage({ id }: { id?: number }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState(initial);
  const [tools, setTools] = useState({
    generate_video_desc: "",
    generate_video_from_image_desc: "",
    get_video_status_desc: "",
  });
  const [defaults, setDefaults] = useState("{}");
  const [testParams, setTestParams] = useState("{}");
  const [prompt, setPrompt] = useState("");
  const [image, setImage] = useState("");
  const [testOperation, setTestOperation] = useState<"generate" | "image">(
    "generate",
  );
  const [models, setModels] = useState<Array<{ id: string; name: string }>>([]);
  const [modelsOpen, setModelsOpen] = useState(false);
  const [modelSearch, setModelSearch] = useState("video");
  const [uploading, setUploading] = useState(false);
  const [taskID, setTaskID] = useState<string | null>(null);
  const [result, setResult] = useState<ToolCallResult | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const requestID = useRef<string | null>(null);
  const { data, isLoading, isError } = useQuery({
    queryKey: ["videogen-config", id],
    queryFn: () => getVideoGenConfig(id!),
    enabled: !!id,
  });
  const config = data?.data;
  useEffect(() => {
    if (!config) return;
    setForm({
      name: config.name,
      description: config.description,
      protocol: config.protocol,
      model_name: config.model_name,
      endpoint_url: config.endpoint_url,
      api_key: "",
      operation: config.operation,
      image_request_format:
        config.protocol !== "openai_video"
          ? "json"
          : config.image_request_format,
      request_timeout_seconds: config.request_timeout_seconds,
      task_timeout_seconds: config.task_timeout_seconds,
    });
    setTools({
      generate_video_desc: config.generate_video_desc,
      generate_video_from_image_desc: config.generate_video_from_image_desc,
      get_video_status_desc: config.get_video_status_desc,
    });
    setDefaults(JSON.stringify(config.default_params, null, 2));
    setTestOperation(config.operation === "image" ? "image" : "generate");
  }, [config]);
  useEffect(() => {
    requestID.current = null;
  }, [form, defaults, prompt, image, testParams, testOperation]);
  const selectedOperation =
    form.operation === "both" ? testOperation : form.operation;
  const isOpenRouter = form.protocol === "openrouter_video";
  const isGemini = form.protocol === "gemini_video";
  const isMiniMax = form.protocol === "minimax_video";
  const jsonOnly = isOpenRouter || isGemini || isMiniMax;
  const refresh = () => {
    for (const key of [
      ["videogen-config", id],
      ["videogen"],
      ["services"],
      ["groups"],
    ])
      queryClient.invalidateQueries({ queryKey: key });
  };
  function payload(): ConfigInput {
    return {
      ...form,
      name: form.name.trim() || undefined,
      api_key: form.api_key.trim() || undefined,
      default_params: jsonObject(defaults, t("videogen.defaultParams")),
    };
  }
  const save = useMutation({
    mutationFn: async () => {
      if (id) {
        await updateVideoGenConfig(id, payload());
        return id;
      }
      return (await createVideoGenConfig(payload())).data.id;
    },
    onSuccess: () => {
      toast.success(t("videogen.saved"));
      refresh();
      if (!id) navigate({ to: "/videogen" });
    },
    onError: (err) => toast.error(err.message),
  });
  const toggle = useMutation({
    mutationFn: () => toggleVideoGenConfig(id!, !config?.auto_register),
    onSuccess: () => {
      toast.success(
        t(
          config?.auto_register
            ? "videogen.disableSuccess"
            : "videogen.enableSuccess",
        ),
      );
      refresh();
    },
    onError: (err) => toast.error(err.message),
  });
  const saveTool = useMutation({
    mutationFn: (data: ConfigInput) => updateVideoGenConfig(id!, data),
    onSuccess: () => {
      toast.success(t("videogen.toolSaved"));
      refresh();
    },
    onError: (err) => toast.error(err.message),
  });
  const fetchModels = useMutation({
    mutationFn: () =>
      listVideoGenModels({
        protocol: form.protocol,
        endpoint_url: form.endpoint_url.trim(),
        api_key: form.api_key.trim() || undefined,
        config_id: id,
      }),
    onSuccess: (res) => {
      setModels(res.data);
      setModelSearch("video");
      setModelsOpen(true);
    },
    onError: (err) => toast.error(err.message),
  });
  const test = useMutation({
    mutationFn: () => {
      requestID.current ??= newVideoRequestID();
      return testVideoGenConfig({
        ...payload(),
        config_id: id,
        tool:
          selectedOperation === "image"
            ? "generate_video_from_image"
            : "generate_video",
        arguments: {
          ...jsonObject(testParams, t("videogen.testParams")),
          prompt,
          ...(selectedOperation === "image" ? { image } : {}),
          wait_seconds: 0,
          request_id: requestID.current,
        },
      });
    },
    onMutate: () => {
      setResult(null);
      setTaskID(null);
    },
    onSuccess: (res) => {
      setResult(res.data);
      const structured = res.data.result?.structuredContent as
        | { task_id?: unknown }
        | undefined;
      if (typeof structured?.task_id === "string") {
        setTaskID(structured.task_id);
        requestID.current = null;
      }
      queryClient.invalidateQueries({ queryKey: ["video-tasks"] });
    },
    onError: (err) => toast.error(err.message),
  });
  async function upload(file: File) {
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
  }
  const bytes = (value: string) => new TextEncoder().encode(value).length;
  const canConnect =
    bytes(form.api_key.trim()) <= 512 &&
    (!!form.api_key.trim() || !!config?.has_api_key);
  const validDefaults = (() => {
    try {
      return (
        bytes(
          JSON.stringify(jsonObject(defaults, t("videogen.defaultParams"))),
        ) <= 65536
      );
    } catch {
      return false;
    }
  })();
  const canSave =
    canConnect &&
    !!form.name.trim() &&
    bytes(form.name.trim()) <= 128 &&
    !!form.model_name.trim() &&
    bytes(form.model_name.trim()) <= 128 &&
    validDefaults &&
    Number.isInteger(form.request_timeout_seconds) &&
    form.request_timeout_seconds >= 1 &&
    form.request_timeout_seconds <= 300 &&
    Number.isInteger(form.task_timeout_seconds) &&
    form.task_timeout_seconds >= 60 &&
    form.task_timeout_seconds <= 86400;
  const filteredModels = models.filter((model) =>
    `${model.id} ${model.name}${isGemini ? " video" : ""}`
      .toLowerCase()
      .includes(modelSearch.trim().toLowerCase()),
  );
  if (id && isLoading)
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        <Loader2 className="mr-2 h-5 w-5 animate-spin" />
        {t("common.loading")}
      </div>
    );
  if (id && isError)
    return <p className="p-8 text-destructive">{t("videogen.loadFailed")}</p>;
  return (
    <div
      className={
        id
          ? "space-y-6 p-4 sm:p-6 lg:p-8"
          : "mx-auto max-w-4xl space-y-6 p-4 sm:p-6 lg:p-8"
      }
    >
      <div className="flex flex-col items-start gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <Link to="/videogen">
            <Button variant="ghost" size="icon" aria-label={t("videogen.back")}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div className="min-w-0">
            <h1 className="break-words text-xl font-semibold">
              {id ? config?.name : t("videogen.create")}
            </h1>
            <p className="mt-0.5 break-words text-sm text-muted-foreground">
              {id
                ? `${videoProtocolLabel(config?.protocol || "openai_video")} / ${config?.model_name}`
                : t("videogen.configSubtitle")}
            </p>
          </div>
        </div>
        {id && (
          <Button
            variant={config?.auto_register ? "outline" : "default"}
            size="sm"
            className="gap-1.5"
            disabled={toggle.isPending}
            onClick={() => toggle.mutate()}
          >
            {toggle.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : config?.auto_register ? (
              <PowerOff className="h-3.5 w-3.5" />
            ) : (
              <Power className="h-3.5 w-3.5" />
            )}
            {t(config?.auto_register ? "videogen.disable" : "videogen.enable")}
          </Button>
        )}
      </div>
      {config?.auto_register && config.registered_service_id && (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-4 py-3 text-sm">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
          <span className="text-emerald-700 dark:text-emerald-300">
            {t("videogen.registered", { id: config.registered_service_id })}
          </span>
          <Link
            className="ml-auto text-primary hover:underline"
            to="/services/$id"
            params={{ id: String(config.registered_service_id) }}
          >
            {t("videogen.openService")}
          </Link>
        </div>
      )}
      <section className="space-y-5 rounded-xl border bg-card p-5">
        <h2 className="text-sm font-semibold">{t("videogen.basicConfig")}</h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="video-name">
              {t("videogen.name")} {requiredMark}
            </Label>
            <Input
              id="video-name"
              aria-required="true"
              value={form.name}
              maxLength={128}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="video-description">
              {t("videogen.description")}
            </Label>
            <Input
              id="video-description"
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
            />
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="video-protocol">{t("videogen.protocol")}</Label>
            <Select
              value={form.protocol}
              onValueChange={(protocol: VideoGenConfig["protocol"]) => {
                setForm({
                  ...form,
                  protocol,
                  endpoint_url:
                    form.endpoint_url.trim() ===
                    defaultVideoEndpoint(form.protocol)
                      ? ""
                      : form.endpoint_url,
                  image_request_format:
                    protocol !== "openai_video"
                      ? "json"
                      : form.image_request_format,
                });
                setModels([]);
              }}
            >
              <SelectTrigger id="video-protocol">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="openai_video">
                  {t("videogen.protocolLabel")}
                </SelectItem>
                <SelectItem value="gemini_video">
                  {t("videogen.geminiProtocolLabel")}
                </SelectItem>
                <SelectItem value="openrouter_video">
                  {t("videogen.openRouterProtocolLabel")}
                </SelectItem>
                <SelectItem value="minimax_video">
                  {t("videogen.miniMaxProtocolLabel")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="video-model">
              {t("videogen.model")} {requiredMark}
            </Label>
            <div className="flex gap-2">
              <Input
                id="video-model"
                aria-required="true"
                className="min-w-0 flex-1"
                value={form.model_name}
                placeholder={
                  isMiniMax
                    ? "hailuo-3-max / MiniMax-H3-Max"
                    : t("videogen.modelPlaceholder")
                }
                onChange={(e) =>
                  setForm({ ...form, model_name: e.target.value })
                }
              />
              <Button
                variant="outline"
                size="sm"
                className="shrink-0 gap-1.5"
                aria-label={t("videogen.models")}
                disabled={!canConnect || fetchModels.isPending}
                onClick={() => fetchModels.mutate()}
              >
                {fetchModels.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <List className="h-4 w-4" />
                )}
                {t("videogen.fetchModels")}
              </Button>
            </div>
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="video-endpoint">{t("videogen.endpoint")}</Label>
          <Input
            id="video-endpoint"
            value={form.endpoint_url}
            placeholder={defaultVideoEndpoint(form.protocol)}
            onChange={(e) => {
              setForm({ ...form, endpoint_url: e.target.value });
              setModels([]);
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t("videogen.endpointDefaultHint", {
              endpoint: defaultVideoEndpoint(form.protocol),
            })}
          </p>
          <p className="break-all text-xs text-muted-foreground">
            {t("videogen.endpointPreview")}:{" "}
            <code className="text-primary">
              POST{" "}
              {videoEndpoint(form.endpoint_url, form.protocol, form.model_name)}
            </code>
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="video-key">
            API Key {!config?.has_api_key && requiredMark}
          </Label>
          <Input
            id="video-key"
            aria-required={!config?.has_api_key}
            type="password"
            autoComplete="new-password"
            value={form.api_key}
            placeholder={config?.has_api_key ? t("videogen.keepKey") : "sk-..."}
            onChange={(e) => setForm({ ...form, api_key: e.target.value })}
          />
        </div>
        <div className="max-w-xs space-y-2">
          <Label htmlFor="video-operation">{t("videogen.capabilities")}</Label>
          <Select
            value={form.operation}
            onValueChange={(operation: VideoGenConfig["operation"]) => {
              setForm({ ...form, operation });
              setTestOperation(operation === "image" ? "image" : "generate");
            }}
          >
            <SelectTrigger id="video-operation">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(["both", "generate", "image"] as const).map((v) => (
                <SelectItem key={v} value={v}>
                  {t(`videogen.operation.${v}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {form.operation !== "generate" && (
          <div className="space-y-2">
            <Label htmlFor="video-format">{t("videogen.imageFormat")}</Label>
            <Select
              value={form.image_request_format}
              disabled={jsonOnly}
              onValueChange={(
                image_request_format: VideoGenConfig["image_request_format"],
              ) => setForm({ ...form, image_request_format })}
            >
              <SelectTrigger id="video-format">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {!jsonOnly && (
                  <SelectItem value="multipart">Multipart</SelectItem>
                )}
                <SelectItem value="json">JSON</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t(
                isOpenRouter
                  ? "videogen.openRouterFormatHint"
                  : isGemini
                    ? "videogen.geminiFormatHint"
                    : isMiniMax
                      ? "videogen.miniMaxFormatHint"
                      : "videogen.formatHint",
              )}
            </p>
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="video-defaults">{t("videogen.defaultParams")}</Label>
          <Textarea
            id="video-defaults"
            className="min-h-28 font-mono text-xs"
            value={defaults}
            onChange={(e) => setDefaults(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {t("videogen.jsonParamsHint")}{" "}
            {t(
              isOpenRouter
                ? "videogen.openRouterDefaultsHint"
                : isGemini
                  ? "videogen.geminiDefaultsHint"
                  : isMiniMax
                    ? "videogen.miniMaxDefaultsHint"
                    : "videogen.defaultsHint",
            )}
          </p>
        </div>
        <div className="grid max-w-2xl gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="video-request-timeout">
              {t("videogen.requestTimeout")} {requiredMark}
            </Label>
            <Input
              id="video-request-timeout"
              aria-required="true"
              type="number"
              min={1}
              max={300}
              value={form.request_timeout_seconds}
              onChange={(e) =>
                setForm({
                  ...form,
                  request_timeout_seconds: Number(e.target.value),
                })
              }
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="video-task-timeout">
              {t("videogen.taskTimeout")} {requiredMark}
            </Label>
            <Input
              id="video-task-timeout"
              aria-required="true"
              type="number"
              min={60}
              max={86400}
              value={form.task_timeout_seconds}
              onChange={(e) =>
                setForm({
                  ...form,
                  task_timeout_seconds: Number(e.target.value),
                })
              }
            />
          </div>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("videogen.timeoutHint")}
        </p>
        <div className="flex justify-end gap-3 pt-2">
          {!id && (
            <Link to="/videogen">
              <Button variant="ghost">{t("common.cancel")}</Button>
            </Link>
          )}
          <Button
            className="gap-2"
            disabled={!canSave || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            {t("videogen.save")}
          </Button>
        </div>
      </section>
      {id && config && (
        <section className="space-y-4 rounded-xl border bg-card p-5">
          <div className="flex items-center gap-2">
            <Wrench className="h-4 w-4 text-muted-foreground" />
            <h2 className="text-sm font-semibold">{t("videogen.tools")}</h2>
          </div>
          <p className="text-xs text-muted-foreground">
            {t("videogen.toolHint")}
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            {descriptionFields
              .filter(
                ([field]) =>
                  !(
                    field === "generate_video_desc" &&
                    config.operation === "image"
                  ) &&
                  !(
                    field === "generate_video_from_image_desc" &&
                    config.operation === "generate"
                  ),
              )
              .map(([field, name, fallback]) => {
                const defaultDescription = config[fallback];
                const saving =
                  saveTool.isPending &&
                  saveTool.variables?.[field] !== undefined;
                return (
                  <div key={field} className="space-y-3 rounded-lg border p-4">
                    <div className="flex items-center gap-2">
                      <span className="inline-flex items-center justify-center rounded-md bg-primary/10 p-1.5">
                        <Wrench className="h-3.5 w-3.5 text-primary" />
                      </span>
                      <span className="break-all text-sm font-semibold">
                        {name}
                      </span>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor={"video-" + field} className="text-xs">
                        {t("videogen.toolDescription")}
                      </Label>
                      <Textarea
                        id={"video-" + field}
                        aria-label={name + " " + t("videogen.toolDescription")}
                        className="min-h-[80px] text-xs"
                        maxLength={4096}
                        value={tools[field]}
                        onChange={(e) =>
                          setTools({ ...tools, [field]: e.target.value })
                        }
                      />
                    </div>
                    <div className="flex items-center justify-end gap-2 pt-1">
                      {tools[field] !== defaultDescription && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 gap-1.5 text-xs"
                          disabled={saveTool.isPending}
                          title={t("vision.restoreDefaultHint")}
                          onClick={() => {
                            setTools({ ...tools, [field]: defaultDescription });
                            toast.info(t("vision.restoreDefaultHint"));
                          }}
                        >
                          <RotateCcw className="h-3 w-3" />
                          {t("videogen.restoreDefault")}
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-7 gap-1.5 text-xs"
                        disabled={saveTool.isPending}
                        onClick={() =>
                          saveTool.mutate({ [field]: tools[field] })
                        }
                      >
                        {saving ? (
                          <Loader2 className="h-3 w-3 animate-spin" />
                        ) : (
                          <Save className="h-3 w-3" />
                        )}
                        {t("common.save")}
                      </Button>
                    </div>
                  </div>
                );
              })}
            {config.operation !== "generate" && (
              <div className="space-y-3 rounded-lg border border-dashed bg-muted/30 p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <span className="inline-flex items-center justify-center rounded-md bg-primary/10 p-1.5">
                      <Wrench className="h-3.5 w-3.5 text-primary" />
                    </span>
                    <span className="text-sm font-semibold">upload_image</span>
                  </div>
                  <Badge variant="secondary" className="gap-1 font-normal">
                    <Lock className="h-3 w-3" />
                    {t("vision.uploadImageBuiltinBadge")}
                  </Badge>
                </div>
                <div className="space-y-2">
                  <Label className="text-xs">
                    {t("videogen.toolDescription")}
                  </Label>
                  <p className="rounded-md border bg-background px-2.5 py-2 text-xs leading-relaxed text-muted-foreground">
                    {config.upload_image_desc}
                  </p>
                </div>
              </div>
            )}
          </div>
        </section>
      )}
      <section className="space-y-4 rounded-xl border bg-card p-5">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            <Film className="h-4 w-4 text-muted-foreground" />
            {t("videogen.testTitle")}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("videogen.testHint")}
          </p>
        </div>
        {form.operation === "both" && (
          <Select
            value={testOperation}
            onValueChange={(v: "generate" | "image") => setTestOperation(v)}
          >
            <SelectTrigger
              className="w-52"
              aria-label={t("videogen.testOperation")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="generate">
                {t("videogen.operation.generate")}
              </SelectItem>
              <SelectItem value="image">
                {t("videogen.operation.image")}
              </SelectItem>
            </SelectContent>
          </Select>
        )}
        <div className="space-y-2">
          <Label htmlFor="video-prompt">
            {t("videogen.prompt")} {requiredMark}
          </Label>
          <Textarea
            id="video-prompt"
            aria-required="true"
            rows={4}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={t("videogen.promptPlaceholder")}
          />
        </div>
        {selectedOperation === "image" && (
          <div className="space-y-2">
            <Label htmlFor="video-image">
              {t("videogen.reference")} {requiredMark}
            </Label>
            <div className="flex flex-wrap gap-2">
              <Input
                className="min-w-0 flex-1"
                id="video-image"
                aria-required="true"
                value={image}
                onChange={(e) => setImage(e.target.value)}
                placeholder="https://..."
              />
              <Button
                variant="outline"
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
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="video-test-params">{t("videogen.testParams")}</Label>
          <Textarea
            className="font-mono text-xs"
            id="video-test-params"
            rows={3}
            value={testParams}
            onChange={(e) => setTestParams(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {t("videogen.jsonParamsHint")}
          </p>
        </div>
        <Button
          disabled={
            !canConnect ||
            !form.model_name.trim() ||
            !prompt.trim() ||
            (selectedOperation === "image" && !image.trim()) ||
            test.isPending
          }
          onClick={() => test.mutate()}
        >
          {test.isPending ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Film className="mr-2 h-4 w-4" />
          )}
          {t("videogen.generate")}
        </Button>
        {taskID ? (
          <VideoTaskResult key={taskID} id={taskID} />
        ) : (
          result && (
            <TestResultPanel result={result}>
              {result.result?.content?.map((block, index) => (
                <ContentBlock
                  key={index}
                  block={block}
                  index={index}
                  downloadUri={
                    block.mimeType?.startsWith("video/") && block.uri
                      ? videoDownloadURL(block.uri)
                      : undefined
                  }
                />
              ))}
            </TestResultPanel>
          )
        )}
      </section>
      {id && <VideoTasks key={id} configID={id} />}
      <Dialog open={modelsOpen} onOpenChange={setModelsOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("videogen.models")}</DialogTitle>
            <DialogDescription>{t("videogen.modelsHint")}</DialogDescription>
          </DialogHeader>
          <Input
            aria-label={t("videogen.searchModel")}
            placeholder={t("videogen.searchModel")}
            value={modelSearch}
            onChange={(e) => setModelSearch(e.target.value)}
          />
          <div className="max-h-80 space-y-1 overflow-auto">
            {filteredModels.map((m) => (
              <button
                type="button"
                key={m.id}
                className="block w-full rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:outline focus-visible:outline-primary"
                onClick={() => {
                  setForm({ ...form, model_name: m.id });
                  setModelsOpen(false);
                }}
              >
                <span className="break-all font-mono text-xs">{m.id}</span>
              </button>
            ))}
            {!filteredModels.length && (
              <p className="p-4 text-sm text-muted-foreground">
                {t("videogen.noModels")}
              </p>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
