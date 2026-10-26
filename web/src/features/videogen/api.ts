import { api } from "@/lib/api";
import i18n from "@/i18n/config";
import type { ToolCallResult } from "@/types";
import type { TaskReferenceImage } from "@/components/task-reference-images";

export interface VideoGenConfig {
  id: number;
  name: string;
  description: string;
  protocol:
    | "openai_video"
    | "gemini_video"
    | "openrouter_video"
    | "minimax_video";
  model_name: string;
  endpoint_url: string;
  operation: "generate" | "image" | "both";
  image_request_format: "multipart" | "json";
  default_params: Record<string, unknown>;
  request_timeout_seconds: number;
  task_timeout_seconds: number;
  generate_video_desc: string;
  generate_video_from_image_desc: string;
  get_video_status_desc: string;
  generate_video_desc_default: string;
  generate_video_from_image_desc_default: string;
  get_video_status_desc_default: string;
  upload_image_desc: string;
  has_api_key: boolean;
  auto_register: boolean;
  registered_service_id: number | null;
}

export type ConfigInput = Partial<
  Pick<
    VideoGenConfig,
    | "name"
    | "description"
    | "protocol"
    | "model_name"
    | "endpoint_url"
    | "operation"
    | "image_request_format"
    | "default_params"
    | "request_timeout_seconds"
    | "task_timeout_seconds"
    | "generate_video_desc"
    | "generate_video_from_image_desc"
    | "get_video_status_desc"
  >
> & { api_key?: string };

export type ModelListInput = Pick<
  ConfigInput,
  "protocol" | "endpoint_url" | "api_key"
> & { config_id?: number };

export interface VideoTask {
  task_id: string;
  config_id: number;
  config_name: string;
  source: string;
  operation: "generate" | "image";
  protocol?: VideoGenConfig["protocol"];
  retry_of?: string;
  retry_count?: number;
  model: string;
  prompt: string;
  reference_images?: TaskReferenceImage[];
  status: string;
  progress: number;
  artifact_status: string;
  tracking_paused: boolean;
  error?: string;
  last_error?: string;
  created_at: string;
  stopped_at: string | null;
  duration_ms: number | null;
  expires_at?: string | null;
  videos: Array<{
    url: string;
    mime: string;
    size: number;
    expires_at: string | null;
  }>;
}

export const taskIsActive = (task: VideoTask) =>
  !task.tracking_paused &&
  (["submitting", "queued", "in_progress", "unknown"].includes(task.status) ||
    (task.status === "completed" &&
      ["pending", "saving"].includes(task.artifact_status)));

export async function getVideoGenConfigs(): Promise<{
  data: VideoGenConfig[];
}> {
  return (await api.get("/videogen")).data;
}
export async function getVideoGenConfig(
  id: number,
): Promise<{ data: VideoGenConfig }> {
  return (await api.get(`/videogen/${id}`)).data;
}
export async function createVideoGenConfig(
  data: ConfigInput,
): Promise<{ data: VideoGenConfig }> {
  return (await api.post("/videogen", data)).data;
}
export async function updateVideoGenConfig(id: number, data: ConfigInput) {
  return (await api.put(`/videogen/${id}`, data)).data;
}
export async function deleteVideoGenConfig(id: number) {
  return (await api.delete(`/videogen/${id}`)).data;
}
export async function toggleVideoGenConfig(id: number, enabled: boolean) {
  return (await api.post(`/videogen/${id}/${enabled ? "enable" : "disable"}`))
    .data;
}
export async function listVideoGenModels(
  data: ModelListInput,
): Promise<{ data: Array<{ id: string; name: string }> }> {
  return (await api.post("/videogen/models", data, { timeout: 20_000 })).data;
}
export async function testVideoGenConfig(
  data: ConfigInput & {
    config_id?: number;
    tool: string;
    arguments: Record<string, unknown>;
  },
): Promise<{ data: ToolCallResult }> {
  return (await api.post("/videogen/test", data, { timeout: 40_000 })).data;
}
export async function getVideoTask(id: string): Promise<{ data: VideoTask }> {
  return (await api.get(`/videogen/tasks/${encodeURIComponent(id)}`)).data;
}
export async function retryVideoDownload(id: string) {
  return (
    await api.post(`/videogen/tasks/${encodeURIComponent(id)}/retry-download`)
  ).data;
}
export async function getVideoRetryInput(id: string): Promise<{
  data: {
    config_id: number;
    operation: "generate" | "image";
    arguments: Record<string, unknown>;
    has_input_snapshot: boolean;
  };
}> {
  return (
    await api.get(`/videogen/tasks/${encodeURIComponent(id)}/retry-input`)
  ).data;
}
export async function retryVideoGeneration(
  id: string,
  data: {
    config_id: number;
    request_id: string;
    arguments: Record<string, unknown>;
  },
): Promise<{ data: VideoTask }> {
  return (
    await api.post(`/videogen/tasks/${encodeURIComponent(id)}/retry`, data, {
      timeout: 40_000,
    })
  ).data;
}
export async function deleteVideoTask(id: string) {
  return (await api.delete(`/videogen/tasks/${encodeURIComponent(id)}`)).data;
}
export async function listVideoTasks(
  configID: number | undefined,
  page: number,
  status = "all",
): Promise<{ data: { items: VideoTask[]; total: number; page_size: number } }> {
  return (
    await api.get("/videogen/tasks", {
      params: {
        config_id: configID,
        page,
        page_size: 10,
        status: status === "all" ? undefined : status,
      },
    })
  ).data;
}

export function videoDownloadURL(url: string) {
  return `${url}${url.includes("?") ? "&" : "?"}download=1`;
}

export function defaultVideoEndpoint(
  protocol: VideoGenConfig["protocol"] = "openai_video",
) {
  return protocol === "openrouter_video"
    ? "https://openrouter.ai/api/v1"
    : protocol === "minimax_video"
      ? "https://api.minimax.io/v2"
      : protocol === "gemini_video"
        ? "https://generativelanguage.googleapis.com/v1beta"
        : "https://api.openai.com/v1";
}

export function videoProtocolLabel(protocol: VideoGenConfig["protocol"]) {
  const keys = {
    openai_video: "videogen.protocolLabel",
    gemini_video: "videogen.geminiProtocolLabel",
    openrouter_video: "videogen.openRouterProtocolLabel",
    minimax_video: "videogen.miniMaxProtocolLabel",
  };
  return i18n.t(keys[protocol] || protocol);
}

export function videoEndpoint(
  base: string,
  protocol: VideoGenConfig["protocol"] = "openai_video",
  model = "{model}",
) {
  let endpoint = base.trim() || defaultVideoEndpoint(protocol);
  if (protocol === "minimax_video") {
    const root = endpoint.replace(/\/+$/, "").replace(/\/v[12]$/, "");
    return `${root}/v2/video_generation`;
  }
  if (protocol === "gemini_video") {
    const root = endpoint.replace(/\/+$/, "").replace(/\/v1(?:beta)?$/, "");
    const modelID = model.trim().replace(/^models\//, "") || "{model}";
    return `${root}/v1beta/models/${encodeURIComponent(modelID)}:predictLongRunning`;
  }
  if (protocol === "openrouter_video") {
    try {
      const url = new URL(endpoint);
      if (
        url.hostname.toLowerCase() === "openrouter.ai" &&
        !url.pathname.replace(/\//g, "")
      ) {
        url.pathname = "/api";
        endpoint = url.toString();
      }
    } catch {
      // Preserve the preview while the base URL is still being edited.
    }
  }
  return `${endpoint.replace(/\/+$/, "").replace(/\/v1$/, "")}/v1/videos`;
}

export function jsonObject(
  text: string,
  field: string,
): Record<string, unknown> {
  if (!text.trim()) return {};
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    throw new Error(i18n.t("videogen.invalidParamsJson", { field }));
  }
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error(i18n.t("videogen.invalidParamsObject", { field }));
  return value as Record<string, unknown>;
}

export function newVideoRequestID(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (value) =>
    value.toString(16).padStart(2, "0"),
  ).join("");
}
