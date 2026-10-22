import { useEffect, useId, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Loader2, Play, Upload, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  ContentBlock,
  TestResultPanel,
} from "@/features/services/components/test-result-panel";
import type { ToolCallResult } from "@/types";
import * as audio from "../api";
import { voiceOptions } from "../voices";

export function AudioTest({
  slot,
  payload,
  configID,
  supportedVoices,
}: {
  slot: audio.Slot;
  payload: () => audio.ConfigInput;
  configID?: number;
  supportedVoices?: string[];
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [prompt, setPrompt] = useState("");
  const [voice, setVoice] = useState("");
  const [instructions, setInstructions] = useState("");
  const [lyrics, setLyrics] = useState("");
  const [instrumental, setInstrumental] = useState(false);
  const [multi, setMulti] = useState(false);
  const [turns, setTurns] = useState(
    '[{"role":"A","text":"Hello"},{"role":"B","text":"Hi"}]',
  );
  const [speakers, setSpeakers] = useState(
    '[{"role":"A","voice":"Kore"},{"role":"B","voice":"Puck"}]',
  );
  const [refs, setRefs] = useState<string[]>([]);
  const [reference, setReference] = useState("");
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<ToolCallResult>();
  const [taskID, setTaskID] = useState("");
  const [requestID, setRequestID] = useState("");
  const voicesID = useId();
  const voices = voiceOptions(slot, supportedVoices);
  useEffect(() => {
    setVoice("");
    setRequestID("");
  }, [slot.model, slot.protocol, slot.connection_key]);
  const image = slot.kind === "music";
  const media = slot.capabilities.input_modalities.includes(
    image ? "image" : "audio",
  );
  const current = useQuery({
    queryKey: ["audio-task", taskID],
    queryFn: () => audio.task(taskID),
    enabled: !!taskID,
    refetchInterval: (q) =>
      q.state.data && audio.active(q.state.data) ? 2000 : false,
  });
  const initialTask = result?.result?.structuredContent as
    | audio.AudioTask
    | undefined;
  const task = current.data ?? (initialTask?.task_id ? initialTask : undefined);
  async function run() {
    setBusy(true);
    setError("");
    setResult(undefined);
    setTaskID("");
    try {
      const args: Record<string, unknown> = {
        request_id: requestID || crypto.randomUUID(),
        wait_seconds: 10,
      };
      setRequestID(args.request_id as string);
      if (slot.kind === "speech" && multi) {
        args.turns = JSON.parse(turns);
        args.speakers = JSON.parse(speakers);
      } else if (prompt.trim())
        args[slot.kind === "speech" ? "text" : "prompt"] = prompt;
      if (slot.kind !== "music") {
        if (voice.trim()) args.voice = voice;
        if (instructions.trim()) args.instructions = instructions;
      }
      if (media && refs.length) args[image ? "images" : "audios"] = refs;
      if (slot.kind === "music") {
        if (lyrics.trim()) args.lyrics = lyrics;
        if (instrumental) args.is_instrumental = true;
      }
      const response = await audio.test({
        ...payload(),
        config_id: configID,
        kind: slot.kind,
        arguments: args,
      });
      setResult(response);
      const structured = response.result?.structuredContent as
        | { task_id?: string }
        | undefined;
      if (structured?.task_id) {
        setTaskID(structured.task_id);
        setRequestID("");
      }
      qc.invalidateQueries({ queryKey: ["audio-tasks"] });
    } catch (e) {
      setError(e instanceof Error ? e.message : t("audiogen.failed"));
    } finally {
      setBusy(false);
    }
  }
  function changed() {
    setRequestID("");
  }
  async function upload(file?: File) {
    if (!file) return;
    setUploading(true);
    setError("");
    try {
      const url = await audio.upload(file, image);
      setRefs((v) => [...v, url]);
      changed();
    } catch {
      setError(t("audiogen.uploadFailed"));
    } finally {
      setUploading(false);
    }
  }
  return (
    <section className="space-y-4 rounded-xl border bg-card p-4 sm:p-6">
      <div className="flex items-center justify-between gap-3">
        <h2 className="font-semibold">
          {t("audiogen.test")} · {t(`audiogen.${slot.kind}`)}
        </h2>
        <span className="text-xs text-muted-foreground">{slot.model}</span>
      </div>
      <p className="text-xs text-muted-foreground">{t("audiogen.testHint")}</p>
      {slot.kind === "speech" && slot.capabilities.max_speakers > 1 && (
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={multi}
            onChange={(e) => {
              setMulti(e.target.checked);
              changed();
            }}
          />
          {t("audiogen.multi")}
        </label>
      )}
      {multi && slot.kind === "speech" ? (
        <div className="grid gap-3 md:grid-cols-2">
          <label className="space-y-1 text-sm">
            {t("audiogen.turns")}
            <Textarea
              value={turns}
              className="min-h-32 font-mono text-xs"
              onChange={(e) => {
                setTurns(e.target.value);
                changed();
              }}
            />
          </label>
          <label className="space-y-1 text-sm">
            {t("audiogen.speakers")}
            <Textarea
              value={speakers}
              className="min-h-32 font-mono text-xs"
              onChange={(e) => {
                setSpeakers(e.target.value);
                changed();
              }}
            />
          </label>
        </div>
      ) : (
        <label className="block space-y-1 text-sm">
          {t(slot.kind === "speech" ? "audiogen.script" : "audiogen.prompt")}
          <Textarea
            value={prompt}
            onChange={(e) => {
              setPrompt(e.target.value);
              changed();
            }}
            className="min-h-28"
          />
        </label>
      )}
      {slot.kind !== "music" && (
        <div className="grid gap-3 md:grid-cols-2">
          <label className="space-y-1 text-sm">
            {t("audiogen.voice")}
            <Input
              list={voices.length ? voicesID : undefined}
              autoComplete="off"
              value={voice}
              onChange={(e) => {
                setVoice(e.target.value);
                changed();
              }}
              placeholder={t("audiogen.voicePlaceholder")}
            />
            <datalist id={voicesID}>
              {voices.map((v) => (
                <option key={v} value={v} />
              ))}
            </datalist>
          </label>
          <label className="space-y-1 text-sm">
            {t("audiogen.instructions")}
            <Input
              value={instructions}
              onChange={(e) => {
                setInstructions(e.target.value);
                changed();
              }}
            />
          </label>
        </div>
      )}
      {slot.kind === "music" && slot.capabilities.lyrics && (
        <label className="block space-y-1 text-sm">
          {t("audiogen.lyrics")}
          <Textarea
            value={lyrics}
            disabled={instrumental}
            onChange={(e) => {
              setLyrics(e.target.value);
              changed();
            }}
          />
        </label>
      )}
      {slot.kind === "music" && slot.capabilities.instrumental && (
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={instrumental}
            onChange={(e) => {
              setInstrumental(e.target.checked);
              if (e.target.checked) setLyrics("");
              changed();
            }}
          />
          {t("audiogen.instrumental")}
        </label>
      )}
      {media && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-sm font-medium">
              {t(
                image ? "audiogen.referenceImages" : "audiogen.referenceAudios",
              )}
            </span>
            <label className="inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-xs">
              <Upload className="h-3.5 w-3.5" />
              {t("audiogen.upload")}
              <input
                type="file"
                className="sr-only"
                disabled={uploading}
                accept={
                  image
                    ? "image/png,image/jpeg,image/webp"
                    : "audio/wav,audio/mpeg,.wav,.mp3"
                }
                onChange={(e) => {
                  void upload(e.target.files?.[0]);
                  e.target.value = "";
                }}
              />
            </label>
            {uploading && <Loader2 className="h-4 w-4 animate-spin" />}
          </div>
          <div className="flex gap-2">
            <Input
              aria-label={t("audiogen.referenceURL")}
              value={reference}
              onChange={(e) => setReference(e.target.value)}
              placeholder={t("audiogen.referenceURL")}
            />
            <Button
              variant="outline"
              disabled={!reference.trim()}
              onClick={() => {
                setRefs((v) => [...v, reference.trim()]);
                setReference("");
                changed();
              }}
            >
              {t("audiogen.add")}
            </Button>
          </div>
          <div className="flex flex-wrap gap-3">
            {refs.map((url, i) => (
              <div
                key={`${i}-${url}`}
                className="relative max-w-full rounded-lg border p-2 pr-8"
              >
                {image ? (
                  <img
                    src={url}
                    alt={`${t("audiogen.referenceImages")} ${i + 1}`}
                    className="h-24 w-32 rounded object-cover"
                  />
                ) : (
                  <audio
                    src={url}
                    controls
                    preload="metadata"
                    className="max-w-full"
                  />
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-7 w-7"
                  aria-label={t("audiogen.remove")}
                  onClick={() => {
                    setRefs((v) => v.filter((_, index) => index !== i));
                    changed();
                  }}
                >
                  <X className="h-3 w-3" />
                </Button>
              </div>
            ))}
          </div>
        </div>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <Button
        disabled={
          busy ||
          uploading ||
          (!!taskID && current.isPending) ||
          (!!current.data && audio.active(current.data))
        }
        onClick={run}
      >
        {busy ? (
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <Play className="mr-2 h-4 w-4" />
        )}
        {t("audiogen.generate")}
      </Button>
      {result && (
        <TestResultPanel result={result}>
          {task ? (
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <span>{t(`audiogen.status.${task.status}`)}</span>
                <span>{t(`audiogen.artifact.${task.artifact_status}`)}</span>
                <code className="break-all">{task.task_id}</code>
              </div>
              {task.error && (
                <p className="text-sm text-destructive">{task.error}</p>
              )}
              {task.last_error && (
                <p className="text-sm text-destructive">{task.last_error}</p>
              )}
              {task.audios.map((a, i) =>
                a.url ? (
                  <ContentBlock
                    key={a.id}
                    index={i}
                    block={{
                      type: "resource_link",
                      uri: a.url,
                      mimeType: a.mime,
                      name: `${t("audiogen.title")} ${i + 1}`,
                    }}
                  />
                ) : (
                  <p key={a.id} className="text-xs text-muted-foreground">
                    {t("audiogen.expired")}
                  </p>
                ),
              )}
              {[task.text, task.transcript, task.lyrics]
                .filter((text, i, all) => text && all.indexOf(text) === i)
                .map((text, i) => (
                  <pre
                    key={i}
                    className="whitespace-pre-wrap break-words text-sm"
                  >
                    {text}
                  </pre>
                ))}
            </div>
          ) : (
            result.result?.content?.map((block, i) => (
              <ContentBlock key={i} block={block} index={i} />
            ))
          )}
        </TestResultPanel>
      )}
    </section>
  );
}
