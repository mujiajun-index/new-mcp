import { useEffect, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  AudioLines,
  Loader2,
  Music2,
  Plus,
  Save,
  Trash2,
  Volume2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import * as audio from "../api";
import { AudioTest } from "./audio-test";
import { AudioTasks } from "./audio-tasks";

type DraftSlot = audio.Slot & { configured: boolean; params: string };
type ValidationScope = { kind?: audio.Kind; discovery?: boolean };
const initialSlots = () =>
  Object.fromEntries(
    audio.kinds.map((kind) => [
      kind,
      { ...audio.newSlot(kind), configured: kind === "speech", params: "{}" },
    ]),
  ) as Record<audio.Kind, DraftSlot>;
const selectClass = "h-9 w-full rounded-md border bg-background px-3 text-sm";
const icons = { speech: Volume2, audio: AudioLines, music: Music2 };
const requiredMark = (
  <span aria-hidden="true" className="text-destructive">
    *
  </span>
);

export function AudioGenConfigPage({ id }: { id?: number }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const loaded = useQuery({
    queryKey: ["audiogen", id],
    queryFn: () => audio.config(id!),
    enabled: !!id,
    refetchOnWindowFocus: false,
  });
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [connections, setConnections] = useState<audio.Connection[]>([
    {
      key: "main",
      name: "",
      protocol: "openai",
      base_url: audio.defaultBaseURL("openai"),
      api_key: "",
    },
  ]);
  const [slots, setSlots] = useState(initialSlots);
  const [busy, setBusy] = useState(false);
  const [hydrated, setHydrated] = useState<number>();
  const [testKind, setTestKind] = useState<audio.Kind>();
  const [discovering, setDiscovering] = useState<audio.Kind>();
  const [modelOptions, setModelOptions] = useState<
    Partial<Record<audio.Kind, Awaited<ReturnType<typeof audio.models>>>>
  >({});
  useEffect(() => {
    const c = loaded.data;
    if (!c || hydrated === c.id) return;
    setName(c.name);
    setDescription(c.description);
    setConnections(
      c.connections.map((v) => ({
        ...v,
        api_key: "",
      })),
    );
    const next = initialSlots();
    audio.kinds.forEach((kind) => {
      next[kind].configured = false;
    });
    c.models.forEach((s) => {
      next[s.kind] = {
        ...s,
        configured: true,
        params:
          typeof s.default_params === "string"
            ? s.default_params
            : JSON.stringify(s.default_params, null, 2),
      };
    });
    setSlots(next);
    setHydrated(c.id);
  }, [loaded.data, hydrated]);
  useEffect(() => {
    if (id && hydrated !== id) return;
    const fallback = connections[0];
    if (!fallback) return;
    setSlots((current) => {
      let changed = false;
      const next = { ...current };
      for (const kind of audio.kinds) {
        next[kind] = audio.adaptSlotConnection(current[kind], connections);
        changed ||= next[kind] !== current[kind];
      }
      return changed ? next : current;
    });
  }, [connections, id, hydrated]);
  function update(kind: audio.Kind, value: Partial<DraftSlot>) {
    setSlots((v) => ({
      ...v,
      [kind]: audio.adaptSlotConnection({ ...v[kind], ...value }, connections),
    }));
    if (value.connection_key !== undefined)
      setModelOptions((current) => ({ ...current, [kind]: [] }));
  }
  function updateConnection(index: number, value: Partial<audio.Connection>) {
    const connection = connections[index];
    if (!connection) return;
    if (value.protocol !== undefined && value.base_url === undefined)
      value = {
        ...value,
        base_url: audio.changeConnectionProtocol(connection, value.protocol)
          .base_url,
      };
    const previousKey = connection.key;
    setConnections((current) =>
      current.map((c, i) => (i === index ? { ...c, ...value } : c)),
    );
    if (value.key !== undefined && value.key !== previousKey) {
      const nextKey = value.key;
      setSlots((current) => {
        const next = { ...current };
        for (const kind of audio.kinds) {
          if (current[kind].connection_key.trim() === previousKey.trim())
            next[kind] = { ...current[kind], connection_key: nextKey };
        }
        return next;
      });
    }
    if (
      value.key !== undefined ||
      value.base_url !== undefined ||
      value.protocol !== undefined
    )
      setModelOptions({});
  }
  function removeConnection(index: number) {
    const connection = connections[index];
    if (!connection) return;
    const removedKey = connection.key;
    const remaining = connections.filter((_, i) => i !== index);
    const fallback = remaining[0];
    if (!fallback) return;
    setConnections(remaining);
    setSlots((current) => {
      const next = { ...current };
      for (const kind of audio.kinds) {
        if (current[kind].connection_key.trim() === removedKey.trim())
          next[kind] = { ...current[kind], connection_key: fallback.key };
      }
      return next;
    });
    setModelOptions({});
  }
  function validate(scope: ValidationScope): Record<string, string> {
    const errors: Record<string, string> = {};
    const bytes = (value: string) => new TextEncoder().encode(value).length;
    const selected = audio.kinds.filter(
      (kind) => slots[kind].configured && (!scope.kind || kind === scope.kind),
    );
    if (!scope.kind) {
      if (!name.trim()) errors.name = t("audiogen.validation.nameRequired");
      else if (bytes(name.trim()) > 128)
        errors.name = t("audiogen.validation.nameLength");
      if (!selected.length)
        errors.slots = t("audiogen.validation.slotRequired");
      else if (
        loaded.data?.auto_register &&
        !selected.some((k) => slots[k].enabled)
      )
        errors.slots = t("audiogen.validation.enabledSlotRequired");
    }
    const keys = new Set(
      selected.map((kind) => slots[kind].connection_key.trim()),
    );
    const seen = new Set<string>();
    connections.forEach((connection, index) => {
      const key = connection.key.trim();
      if (scope.kind && !keys.has(key)) return;
      const field = `connections.${index}`;
      if (!key || bytes(key) > 64)
        errors[`${field}.key`] = t("audiogen.validation.connectionKey");
      else if (seen.has(key))
        errors[`${field}.key`] = t("audiogen.validation.duplicateConnection");
      seen.add(key);
      if (!audio.channelProtocols.includes(connection.protocol))
        errors[`${field}.protocol`] = t(
          "audiogen.validation.connectionProtocol",
        );
      const baseURL = audio.effectiveBaseURL(connection);
      try {
        const url = new URL(baseURL);
        if (
          !/^https?:\/\//i.test(baseURL) ||
          !["http:", "https:"].includes(url.protocol) ||
          url.username ||
          url.password ||
          baseURL.includes("?") ||
          baseURL.includes("#") ||
          bytes(baseURL) > 512
        )
          throw new Error();
      } catch {
        errors[`${field}.base_url`] = t("audiogen.validation.baseURL");
      }
      const previous = loaded.data?.connections.find((c) => c.key === key);
      if (!connection.api_key?.trim()) {
        if (!previous?.has_api_key)
          errors[`${field}.api_key`] = t("audiogen.validation.apiKeyRequired");
        else if (audio.effectiveBaseURL(previous) !== baseURL)
          errors[`${field}.api_key`] = t(
            "audiogen.validation.apiKeyChangedURL",
          );
      } else if (bytes(connection.api_key) > 4096)
        errors[`${field}.api_key`] = t("audiogen.validation.apiKeyLength");
    });
    selected.forEach((kind) => {
      const slot = slots[kind];
      const field = `slots.${kind}`;
      if (!connections.some((c) => c.key.trim() === slot.connection_key.trim()))
        errors[`${field}.connection`] = t(
          "audiogen.validation.connectionRequired",
        );
      const connection = connections.find(
        (c) => c.key.trim() === slot.connection_key.trim(),
      );
      if (connection && !audio.adapterFor(connection.protocol, kind))
        errors[`${field}.connection`] = t(
          "audiogen.validation.connectionUnsupported",
        );
      if (scope.discovery) return;
      if (!slot.model.trim())
        errors[`${field}.model`] = t("audiogen.validation.modelRequired");
      else if (bytes(slot.model.trim()) > 128)
        errors[`${field}.model`] = t("audiogen.validation.modelLength");
      else if (!slot.capabilities.output_modalities.includes("audio"))
        errors[`${field}.model`] = t("audiogen.validation.audioOutputRequired");
      if (
        !Number.isInteger(slot.timeout_seconds) ||
        slot.timeout_seconds < 1 ||
        slot.timeout_seconds > 86400
      )
        errors[`${field}.timeout`] = t("audiogen.validation.timeout");
      const maxSpeakers = audio.preset(kind, slot.protocol).max_speakers;
      if (
        !Number.isInteger(slot.capabilities.max_speakers) ||
        slot.capabilities.max_speakers < 1 ||
        slot.capabilities.max_speakers > maxSpeakers
      )
        errors[`${field}.speakers`] = t("audiogen.validation.speakers", {
          max: maxSpeakers,
        });
      try {
        audio.object(slot.params);
        if (bytes(slot.params) > 65536)
          errors[`${field}.params`] = t("audiogen.validation.paramsLength");
      } catch {
        errors[`${field}.params`] = t("audiogen.validation.paramsObject");
      }
      if (bytes(slot.description) > 4096)
        errors[`${field}.description`] = t(
          "audiogen.validation.descriptionLength",
        );
    });
    return errors;
  }
  const canSave = Object.keys(validate({})).length === 0;
  function payload(only?: audio.Kind, discovery = false): audio.ConfigInput {
    const firstError = Object.values(validate({ kind: only, discovery }))[0];
    if (firstError) throw new Error(firstError);
    const selected = audio.kinds.filter(
      (k) => slots[k].configured && (!only || k === only),
    );
    const keys = new Set(selected.map((k) => slots[k].connection_key.trim()));
    return {
      name: only ? t("audiogen.test") : name.trim(),
      description,
      connections: connections
        .filter((c) => !only || keys.has(c.key.trim()))
        .map((c) => ({
          key: c.key.trim(),
          name: c.name,
          protocol: c.protocol,
          base_url: c.base_url.trim(),
          ...(c.api_key?.trim() ? { api_key: c.api_key } : {}),
        })),
      models: selected.map((k) => {
        const {
          configured: _configured,
          protocol: _protocol,
          params,
          ...slot
        } = slots[k];
        return {
          ...slot,
          model: slot.model.trim(),
          connection_key: slot.connection_key.trim(),
          default_params: discovery ? {} : audio.object(params),
        };
      }),
    };
  }
  async function save() {
    if (!canSave) return;
    setBusy(true);
    try {
      const saved = await audio.saveConfig(payload(), id);
      qc.invalidateQueries({ queryKey: ["audiogen"] });
      if (!id && saved)
        await navigate({
          to: "/audiogen/$id",
          params: { id: String(saved.id) },
        });
      else {
        setConnections((v) =>
          v.map((c) => ({ ...c, api_key: "", has_api_key: true })),
        );
        qc.invalidateQueries({ queryKey: ["audiogen", id] });
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t("audiogen.failed"));
    } finally {
      setBusy(false);
    }
  }
  async function discover(kind: audio.Kind) {
    setDiscovering(kind);
    try {
      const s = slots[kind];
      const data = payload(kind, true);
      setModelOptions((v) => ({ ...v, [kind]: [] }));
      const list = await audio.models({
        config_id: id,
        connections: data.connections,
        kind,
        connection_key: s.connection_key.trim(),
      });
      setModelOptions((v) => ({ ...v, [kind]: list }));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t("audiogen.discoveryHint"));
    } finally {
      setDiscovering(undefined);
    }
  }
  if (id && loaded.isPending)
    return (
      <div className="p-8">
        <Loader2 className="h-5 w-5 animate-spin" />
      </div>
    );
  if (loaded.isError)
    return (
      <div role="alert" className="p-8 text-destructive">
        {t("audiogen.loadFailed")}
      </div>
    );
  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link
            to="/audiogen"
            className="mb-3 inline-flex items-center gap-1 text-xs text-muted-foreground"
          >
            <ArrowLeft className="h-3 w-3" />
            {t("audiogen.back")}
          </Link>
          <h1 className="text-2xl font-semibold tracking-tight">
            {id ? name : t("audiogen.create")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("audiogen.configHint")}
          </p>
        </div>
        <Button onClick={save} disabled={busy || !canSave}>
          {busy ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Save className="mr-2 h-4 w-4" />
          )}
          {t("audiogen.save")}
        </Button>
      </div>
      <section className="grid gap-4 rounded-xl border bg-card p-4 sm:grid-cols-2 sm:p-6">
        <label className="flex flex-col gap-1.5 text-sm">
          <span>
            {t("audiogen.name")} {requiredMark}
          </span>
          <Input
            aria-required="true"
            value={name}
            maxLength={128}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label className="space-y-1.5 text-sm">
          {t("audiogen.description")}
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
      </section>
      <section className="space-y-4 rounded-xl border bg-card p-4 sm:p-6">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="font-semibold">{t("audiogen.connections")}</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              {t("audiogen.connectionHint")}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              setConnections((v) => [
                ...v,
                {
                  key: `connection-${crypto.randomUUID().slice(0, 8)}`,
                  name: "",
                  protocol: "openai",
                  base_url: audio.defaultBaseURL("openai"),
                  api_key: "",
                },
              ])
            }
          >
            <Plus className="mr-1 h-3.5 w-3.5" />
            {t("audiogen.add")}
          </Button>
        </div>
        {connections.map((c, i) => {
          const previous = loaded.data?.connections.find(
            (v) => v.key === c.key.trim(),
          );
          const requiresAPIKey =
            !previous?.has_api_key ||
            audio.effectiveBaseURL(previous) !== audio.effectiveBaseURL(c);
          return (
            <div
              key={i}
              className="grid items-start gap-3 rounded-lg bg-muted/35 p-3 md:grid-cols-2 xl:grid-cols-[1fr_1fr_2fr_1.5fr_auto]"
            >
              <label className="flex flex-col gap-1 text-xs">
                <span>
                  {t("audiogen.connectionKey")} {requiredMark}
                </span>
                <Input
                  aria-required="true"
                  value={c.key}
                  onChange={(e) => updateConnection(i, { key: e.target.value })}
                />
              </label>
              <label className="flex flex-col gap-1 text-xs">
                <span>
                  {t("audiogen.protocol")} {requiredMark}
                </span>
                <select
                  aria-required="true"
                  className={selectClass}
                  value={c.protocol}
                  onChange={(e) =>
                    updateConnection(i, {
                      protocol: e.target.value as audio.ChannelProtocol,
                    })
                  }
                >
                  {audio.channelProtocols.map((protocol) => (
                    <option key={protocol} value={protocol}>
                      {t(`audiogen.channelProtocol.${protocol}`)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="flex flex-col gap-1 text-xs">
                <span>Base URL</span>
                <Input
                  value={c.base_url}
                  type="url"
                  placeholder={audio.defaultBaseURL(c.protocol)}
                  onChange={(e) =>
                    updateConnection(i, { base_url: e.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1 text-xs">
                <span>API Key {requiresAPIKey && requiredMark}</span>
                <Input
                  aria-required={requiresAPIKey}
                  value={c.api_key || ""}
                  type="password"
                  autoComplete="new-password"
                  placeholder={
                    requiresAPIKey
                      ? t("audiogen.enterKey")
                      : t("audiogen.keepKey")
                  }
                  onChange={(e) =>
                    updateConnection(i, { api_key: e.target.value })
                  }
                />
              </label>
              <Button
                variant="ghost"
                size="icon"
                className="self-end xl:mt-5 xl:self-start"
                disabled={connections.length === 1}
                aria-label={t("audiogen.remove")}
                onClick={() => removeConnection(i)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          );
        })}
      </section>
      <div className="grid items-start gap-4 lg:grid-cols-3">
        {audio.kinds.map((kind) => {
          const s = slots[kind];
          const Icon = icons[kind];
          const cap = audio.preset(kind, s.protocol);
          return (
            <section
              key={kind}
              className={`space-y-4 rounded-xl border bg-card p-4 ${s.configured ? "border-primary/25" : ""}`}
            >
              <div className="flex items-center gap-3">
                <span className="rounded-lg bg-muted p-2">
                  <Icon className="h-5 w-5" />
                </span>
                <div className="flex-1">
                  <h2 className="font-semibold">{t(`audiogen.${kind}`)}</h2>
                  <p className="font-mono text-[10px] text-muted-foreground">
                    generate_{kind}
                  </p>
                </div>
                <input
                  type="checkbox"
                  aria-label={t("audiogen.configureSlot", {
                    kind: t(`audiogen.${kind}`),
                  })}
                  checked={s.configured}
                  onChange={(e) =>
                    update(kind, { configured: e.target.checked })
                  }
                />
              </div>
              {s.configured ? (
                <>
                  <label className="flex items-center gap-2 text-xs">
                    <input
                      type="checkbox"
                      checked={s.enabled}
                      onChange={(e) =>
                        update(kind, { enabled: e.target.checked })
                      }
                    />
                    {t("audiogen.publishTool")}
                  </label>
                  <label className="flex flex-col gap-1 text-xs">
                    <span>
                      {t("audiogen.connection")} {requiredMark}
                    </span>
                    <select
                      aria-required="true"
                      className={selectClass}
                      value={s.connection_key}
                      onChange={(e) =>
                        update(kind, { connection_key: e.target.value })
                      }
                    >
                      {!s.connection_key && (
                        <option value="" disabled>
                          {t("audiogen.selectCompatibleConnection")}
                        </option>
                      )}
                      {connections.map((c) => (
                        <option
                          key={c.key}
                          value={c.key}
                          disabled={!audio.adapterFor(c.protocol, kind)}
                        >
                          {c.name || c.key}
                          {` · ${t(`audiogen.channelProtocol.${c.protocol}`)}`}
                        </option>
                      ))}
                    </select>
                    {s.connection_key && (
                      <span className="text-[10px] text-muted-foreground">
                        {t("audiogen.automaticProtocol", {
                          protocol: s.protocol,
                        })}
                      </span>
                    )}
                  </label>
                  <label className="flex flex-col gap-1 text-xs">
                    <span>
                      {t("audiogen.model")} {requiredMark}
                    </span>
                    <Input
                      aria-required="true"
                      list={`audio-models-${kind}`}
                      value={s.model}
                      placeholder={t("audiogen.modelPlaceholder")}
                      onChange={(e) => {
                        const m = modelOptions[kind]?.find(
                          (x) => x.id === e.target.value,
                        );
                        update(kind, {
                          model: e.target.value,
                          ...(m?.input_modalities?.length
                            ? {
                                capabilities: {
                                  ...s.capabilities,
                                  input_modalities: m.input_modalities.filter(
                                    (v) => cap.input_modalities.includes(v),
                                  ),
                                  output_modalities:
                                    m.output_modalities ||
                                    s.capabilities.output_modalities,
                                },
                              }
                            : {
                                capabilities: audio.modelPreset(
                                  kind,
                                  s.protocol,
                                  e.target.value,
                                ),
                              }),
                        });
                      }}
                    />
                    <datalist id={`audio-models-${kind}`}>
                      {modelOptions[kind]?.map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.name}
                        </option>
                      ))}
                    </datalist>
                  </label>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={
                      !!discovering ||
                      Object.keys(validate({ kind, discovery: true })).length >
                        0
                    }
                    onClick={() => discover(kind)}
                  >
                    {discovering === kind && (
                      <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                    )}
                    {t("audiogen.discover")}
                  </Button>
                  <div className="flex flex-wrap gap-3 text-xs">
                    {cap.input_modalities
                      .filter((x) => x !== "text")
                      .map((mod) => (
                        <label key={mod} className="flex items-center gap-1.5">
                          <input
                            type="checkbox"
                            checked={s.capabilities.input_modalities.includes(
                              mod,
                            )}
                            onChange={(e) =>
                              update(kind, {
                                capabilities: {
                                  ...s.capabilities,
                                  input_modalities: e.target.checked
                                    ? [...s.capabilities.input_modalities, mod]
                                    : s.capabilities.input_modalities.filter(
                                        (x) => x !== mod,
                                      ),
                                },
                              })
                            }
                          />
                          {t(`audiogen.input.${mod}`)}
                        </label>
                      ))}
                  </div>
                  {kind === "speech" && cap.max_speakers > 1 && (
                    <label className="flex flex-col gap-1 text-xs">
                      <span>
                        {t("audiogen.maxSpeakers")} {requiredMark}
                      </span>
                      <Input
                        aria-required="true"
                        type="number"
                        min={1}
                        max={cap.max_speakers}
                        value={s.capabilities.max_speakers}
                        onChange={(e) =>
                          update(kind, {
                            capabilities: {
                              ...s.capabilities,
                              max_speakers: Number(e.target.value),
                            },
                          })
                        }
                      />
                    </label>
                  )}
                  <label className="flex flex-col gap-1 text-xs">
                    <span>
                      {t("audiogen.timeout")} {requiredMark}
                    </span>
                    <Input
                      aria-required="true"
                      type="number"
                      min={1}
                      max={86400}
                      value={s.timeout_seconds}
                      onChange={(e) =>
                        update(kind, {
                          timeout_seconds: Number(e.target.value),
                        })
                      }
                    />
                  </label>
                  <details className="space-y-3 text-xs">
                    <summary className="cursor-pointer text-muted-foreground">
                      {t("audiogen.advanced")}
                    </summary>
                    <label className="flex flex-col gap-1 pt-3">
                      {t("audiogen.defaults")}
                      <Textarea
                        className="min-h-28 font-mono text-xs"
                        value={s.params}
                        onChange={(e) =>
                          update(kind, { params: e.target.value })
                        }
                      />
                    </label>
                    <label className="flex flex-col gap-1">
                      {t("audiogen.toolDescription")}
                      <Textarea
                        value={s.description}
                        maxLength={4096}
                        onChange={(e) =>
                          update(kind, { description: e.target.value })
                        }
                        placeholder={t("audiogen.defaultDescription")}
                      />
                    </label>
                    {kind === "music" &&
                      (["lyrics", "instrumental", "lyrics_optimizer"] as const)
                        .filter((k) => cap[k])
                        .map((k) => (
                          <label key={k} className="flex items-center gap-2">
                            <input
                              type="checkbox"
                              checked={s.capabilities[k]}
                              onChange={(e) =>
                                update(kind, {
                                  capabilities: {
                                    ...s.capabilities,
                                    [k]: e.target.checked,
                                  },
                                })
                              }
                            />
                            {t(`audiogen.capability.${k}`)}
                          </label>
                        ))}
                  </details>
                  {s.protocol === "minimax_music" && (
                    <p className="text-xs leading-relaxed text-muted-foreground">
                      {t("audiogen.minimaxNotice")}
                    </p>
                  )}
                  <Button
                    className="w-full"
                    variant="secondary"
                    disabled={
                      !s.enabled || Object.keys(validate({ kind })).length > 0
                    }
                    onClick={() => setTestKind(kind)}
                  >
                    {t("audiogen.test")}
                  </Button>
                </>
              ) : (
                <p className="py-4 text-xs leading-relaxed text-muted-foreground">
                  {t(`audiogen.hint.${kind}`)}
                </p>
              )}
            </section>
          );
        })}
      </div>
      <p className="text-xs text-muted-foreground">
        {t("audiogen.publishHint")}
      </p>
      {testKind && slots[testKind].configured && (
        <AudioTest
          key={`${testKind}-${slots[testKind].protocol}`}
          slot={slots[testKind]}
          supportedVoices={
            modelOptions[testKind]?.find(
              (m) => m.id === slots[testKind].model.trim(),
            )?.supported_voices
          }
          payload={() => payload(testKind)}
          configID={id}
        />
      )}
      {id && <AudioTasks configID={id} />}
    </div>
  );
}
