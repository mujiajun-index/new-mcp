import { api } from "@/lib/api";
import type { ToolCallResult } from "@/types";
import type { TaskReferenceImage } from "@/components/task-reference-images";

export type Kind = "speech" | "audio" | "music";
export const kinds: Kind[] = ["speech", "audio", "music"];
export const protocols = [
  "openai_speech",
  "openai_chat_audio",
  "openai_realtime",
  "openrouter_speech",
  "openrouter_chat_audio",
  "gemini_generate_content",
  "gemini_interactions",
  "minimax_music",
] as const;
export type Protocol = (typeof protocols)[number];
export const channelProtocols = [
  "openai",
  "gemini",
  "openrouter",
  "minimax",
] as const;
export type ChannelProtocol = (typeof channelProtocols)[number];
const channelAdapters: Record<
  ChannelProtocol,
  Partial<Record<Kind, Protocol>>
> = {
  openai: { speech: "openai_speech", audio: "openai_chat_audio" },
  openrouter: {
    speech: "openrouter_speech",
    audio: "openrouter_chat_audio",
    music: "openrouter_chat_audio",
  },
  gemini: {
    speech: "gemini_generate_content",
    music: "gemini_generate_content",
  },
  minimax: { music: "minimax_music" },
};
export function defaultBaseURL(protocol: ChannelProtocol): string {
  return {
    openai: "https://api.openai.com/v1",
    gemini: "https://generativelanguage.googleapis.com/v1beta",
    openrouter: "https://openrouter.ai/api/v1",
    minimax: "https://api.minimax.io/v1",
  }[protocol];
}
export function effectiveBaseURL(connection: Connection): string {
  return connection.base_url.trim() || defaultBaseURL(connection.protocol);
}
export function changeConnectionProtocol(
  connection: Connection,
  protocol: ChannelProtocol,
): Connection {
  const current = connection.base_url.trim().replace(/\/+$/, "");
  return {
    ...connection,
    protocol,
    base_url:
      !current || current === defaultBaseURL(connection.protocol)
        ? defaultBaseURL(protocol)
        : connection.base_url,
  };
}
export function adapterFor(
  protocol: ChannelProtocol,
  kind: Kind,
): Protocol | undefined {
  return channelAdapters[protocol]?.[kind];
}
export function adaptSlotConnection<T extends Slot>(
  slot: T,
  connections: Connection[],
): T {
  const supports = (connection: Connection) =>
    !!adapterFor(connection.protocol, slot.kind);
  const bound = connections.find(
    (c) => c.key.trim() === slot.connection_key.trim(),
  );
  const connection =
    bound && supports(bound) ? bound : connections.find(supports);
  const protocol = connection && adapterFor(connection.protocol, slot.kind);
  if (!connection || !protocol)
    return slot.connection_key ? { ...slot, connection_key: "" } : slot;
  const connectionKey = connection.key;
  if (connectionKey === slot.connection_key && protocol === slot.protocol)
    return slot;
  return {
    ...slot,
    connection_key: connectionKey,
    protocol,
    capabilities:
      protocol !== slot.protocol
        ? modelPreset(slot.kind, protocol, slot.model)
        : slot.capabilities,
  };
}
export interface Capabilities {
  input_modalities: string[];
  output_modalities: string[];
  max_speakers: number;
  lyrics: boolean;
  instrumental: boolean;
  lyrics_optimizer: boolean;
}
export interface Connection {
  key: string;
  name: string;
  protocol: ChannelProtocol;
  base_url: string;
  api_key?: string;
  has_api_key?: boolean;
}
export interface Slot {
  kind: Kind;
  enabled: boolean;
  connection_key: string;
  protocol: Protocol;
  model: string;
  default_params: string | Record<string, unknown>;
  timeout_seconds: number;
  description: string;
  capabilities: Capabilities;
}
export interface AudioConfig {
  id: number;
  name: string;
  description: string;
  auto_register: boolean;
  registered_service_id?: number;
  connections: Connection[];
  models: Slot[];
}
export type ConfigInput = Pick<
  AudioConfig,
  "name" | "description" | "connections"
> & { models: Omit<Slot, "protocol">[] };
export interface AudioFile {
  id: number;
  url?: string;
  mime: string;
  size: number;
  status: string;
  expires_at?: string;
}
export interface AudioTask {
  task_id: string;
  config_id: number;
  execution_config_id: number;
  config_name: string;
  kind: Kind;
  source: string;
  protocol: string;
  model: string;
  prompt: string;
  status: string;
  artifact_status: string;
  error?: string;
  last_error?: string;
  audios: AudioFile[];
  reference_audios: AudioFile[];
  reference_images: TaskReferenceImage[];
  text?: string;
  transcript?: string;
  input_transcript?: string;
  lyrics?: string;
  created_at: string;
  retry_count: number;
  can_retry_save: boolean;
}
export const active = (t: AudioTask) =>
  ["queued", "running"].includes(t.status) ||
  ["pending", "saving"].includes(t.artifact_status);
export async function configs(): Promise<AudioConfig[]> {
  return (await api.get("/audiogen")).data.data;
}
export async function config(id: number): Promise<AudioConfig> {
  return (await api.get(`/audiogen/${id}`)).data.data;
}
export async function saveConfig(
  data: ConfigInput,
  id?: number,
): Promise<AudioConfig | undefined> {
  return (
    id
      ? await api.put(`/audiogen/${id}`, data)
      : await api.post("/audiogen", data)
  ).data.data;
}
export async function toggle(id: number, enabled: boolean) {
  await api.post(`/audiogen/${id}/${enabled ? "enable" : "disable"}`);
}
export async function removeConfig(id: number) {
  await api.delete(`/audiogen/${id}`);
}
export async function models(
  data: Partial<ConfigInput> & {
    config_id?: number;
    kind: Kind;
    connection_key: string;
  },
): Promise<
  Array<{
    id: string;
    name: string;
    input_modalities?: string[];
    output_modalities?: string[];
    supported_voices?: string[];
  }>
> {
  return (await api.post("/audiogen/models", data, { timeout: 20000 })).data
    .data;
}
export async function test(
  data: ConfigInput & {
    config_id?: number;
    kind: Kind;
    arguments: Record<string, unknown>;
  },
): Promise<ToolCallResult> {
  return (await api.post("/audiogen/test", data, { timeout: 40000 })).data.data;
}
export async function task(id: string): Promise<AudioTask> {
  return (await api.get(`/audiogen/tasks/${encodeURIComponent(id)}`)).data.data;
}
export function audioDownloadURL(url: string) {
  return `${url}${url.includes("?") ? "&" : "?"}download=1`;
}
export async function tasks(params: {
  page: number;
  status?: string;
  kind?: string;
  config_id?: number;
}): Promise<{ items: AudioTask[]; total: number; page_size: number }> {
  return (await api.get("/audiogen/tasks", { params })).data.data;
}
export async function retrySave(id: string) {
  await api.post(`/audiogen/tasks/${encodeURIComponent(id)}/retry-download`);
}
export async function retryInput(id: string): Promise<{
  config_id: number;
  kind: Kind;
  arguments: Record<string, unknown>;
}> {
  return (
    await api.get(`/audiogen/tasks/${encodeURIComponent(id)}/retry-input`)
  ).data.data;
}
export async function retry(
  id: string,
  config_id: number,
  args: Record<string, unknown>,
) {
  await api.post(`/audiogen/tasks/${encodeURIComponent(id)}/retry`, {
    config_id,
    arguments: args,
  });
}
export async function removeTask(id: string) {
  await api.delete(`/audiogen/tasks/${encodeURIComponent(id)}`);
}
export async function batchRemove(
  ids: string[],
): Promise<{ failed: Record<string, string> }> {
  return (await api.post("/audiogen/tasks/batch-delete", { ids })).data.data;
}
export async function upload(file: File, image = false): Promise<string> {
  const form = new FormData();
  form.append(image ? "image" : "audio", file);
  const r = await api.post(
    image ? "/vision/upload" : "/audiogen/uploads",
    form,
  );
  return r.data.data.url;
}
export function preset(kind: Kind, protocol: Protocol): Capabilities {
  return {
    input_modalities: [
      "text",
      ...(kind === "audio" &&
      ["openai_chat_audio", "openrouter_chat_audio"].includes(protocol)
        ? ["audio"]
        : []),
      ...(kind === "music" &&
      (protocol.startsWith("gemini_") || protocol === "openrouter_chat_audio")
        ? ["image"]
        : []),
    ],
    output_modalities: kind === "speech" ? ["audio"] : ["audio", "text"],
    max_speakers:
      kind === "speech" &&
      (protocol.startsWith("gemini_") || protocol === "openrouter_speech")
        ? 2
        : 1,
    lyrics: kind === "music",
    instrumental: kind === "music",
    lyrics_optimizer: protocol === "minimax_music",
  };
}
export function modelPreset(
  kind: Kind,
  protocol: Protocol,
  name: string,
): Capabilities {
  const cap = preset(kind, protocol);
  const lower = name.toLowerCase();
  if (
    kind === "audio" &&
    !lower.includes("gpt-audio") &&
    !lower.includes("gpt-4o-audio")
  )
    cap.input_modalities = ["text"];
  if (kind === "music" && !lower.includes("lyria"))
    cap.input_modalities = ["text"];
  if (protocol === "openrouter_speech" && !lower.includes("gemini"))
    cap.max_speakers = 1;
  return cap;
}
export function newSlot(kind: Kind): Slot {
  const protocol: Protocol =
    kind === "speech"
      ? "openai_speech"
      : kind === "audio"
        ? "openai_chat_audio"
        : "openrouter_chat_audio";
  return {
    kind,
    enabled: true,
    connection_key: "main",
    protocol,
    model: "",
    default_params: {},
    timeout_seconds: 600,
    description: "",
    capabilities: modelPreset(kind, protocol, ""),
  };
}
export function object(text: string): Record<string, unknown> {
  const v: unknown = JSON.parse(text || "{}");
  if (!v || typeof v !== "object" || Array.isArray(v))
    throw new Error("Parameters must be a JSON object");
  return v as Record<string, unknown>;
}
