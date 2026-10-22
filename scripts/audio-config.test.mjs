import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";
import vm from "node:vm";

const require = createRequire(new URL("../web/package.json", import.meta.url));
const ts = require("typescript");
function loadFeature(name) {
  const compiled = ts.transpileModule(
    readFileSync(
      new URL(`../web/src/features/audiogen/${name}.ts`, import.meta.url),
      "utf8",
    ),
    {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText;
  const module = { exports: {} };
  vm.runInNewContext(compiled, {
    exports: module.exports,
    module,
    require: () => ({ api: {} }),
  });
  return module.exports;
}
const {
  adapterFor,
  adaptSlotConnection,
  newSlot,
  defaultBaseURL,
  effectiveBaseURL,
  changeConnectionProtocol,
} = loadFeature("api");
const { voiceOptions } = loadFeature("voices");
const connection = (key, protocol) => ({
  key,
  protocol,
  name: "",
  base_url: "https://example.com/v1",
});

test("channel protocols automatically route each purpose", () => {
  const expected = {
    openai: ["openai_speech", "openai_chat_audio", undefined],
    openrouter: [
      "openrouter_speech",
      "openrouter_chat_audio",
      "openrouter_chat_audio",
    ],
    gemini: ["gemini_generate_content", undefined, "gemini_generate_content"],
    minimax: [undefined, undefined, "minimax_music"],
  };
  for (const [channel, protocols] of Object.entries(expected))
    ["speech", "audio", "music"].forEach((kind, i) =>
      assert.equal(adapterFor(channel, kind), protocols[i]),
    );
});

test("switching a connection immediately updates the adapter and input capabilities", () => {
  const initial = { ...newSlot("audio"), model: "gpt-audio" };
  const next = adaptSlotConnection(initial, [connection("main", "openrouter")]);
  assert.equal(next.protocol, "openrouter_chat_audio");
  assert.equal(next.capabilities.input_modalities.includes("audio"), true);
  const gemini = adaptSlotConnection(newSlot("speech"), [
    connection("main", "gemini"),
  ]);
  assert.equal(gemini.protocol, "gemini_generate_content");
  assert.equal(gemini.capabilities.max_speakers, 2);
  assert.equal(
    adaptSlotConnection(gemini, [connection("main", "gemini")]),
    gemini,
  );
});

test("unsupported purposes choose a compatible channel or require a new binding", () => {
  const music = newSlot("music");
  const missing = adaptSlotConnection(music, [connection("main", "openai")]);
  assert.equal(missing.connection_key, "");
  const bound = adaptSlotConnection(missing, [
    connection("main", "openai"),
    connection("songs", "minimax"),
  ]);
  assert.equal(bound.connection_key, "songs");
  assert.equal(bound.protocol, "minimax_music");
  assert.equal(bound.capabilities.lyrics_optimizer, true);
  assert.equal(bound.capabilities.input_modalities.includes("image"), false);
});

test("channel defaults follow protocol changes while custom gateways are preserved", () => {
  let current = {
    ...connection("main", "openai"),
    base_url: defaultBaseURL("openai"),
  };
  for (const protocol of ["gemini", "openrouter", "minimax", "openai"]) {
    current = changeConnectionProtocol(current, protocol);
    assert.equal(current.base_url, defaultBaseURL(protocol));
    const empty = { ...current, base_url: " \t " };
    assert.equal(effectiveBaseURL(empty), defaultBaseURL(protocol));
    assert.equal(
      changeConnectionProtocol(empty, "gemini").base_url,
      defaultBaseURL("gemini"),
    );
  }
  const custom = { ...current, base_url: "https://gateway.example/prefix/v1" };
  assert.equal(
    changeConnectionProtocol(custom, "gemini").base_url,
    custom.base_url,
  );
  assert.equal(effectiveBaseURL(custom), custom.base_url);
  assert.equal(
    changeConnectionProtocol(
      { ...current, base_url: `${current.base_url}/` },
      "gemini",
    ).base_url,
    defaultBaseURL("gemini"),
  );
  assert.equal(
    adaptSlotConnection(newSlot("speech"), [connection("main")]).connection_key,
    "",
  );
});

test("speech voices follow the model family across native and gateway protocols", () => {
  const speech = (protocol, model) =>
    voiceOptions({ kind: "speech", protocol, model });
  const gpt = speech("openai_speech", "gpt-4o-mini-tts-2025-12-15");
  assert.ok(gpt.includes("marin"));
  assert.ok(gpt.includes("verse"));
  assert.ok(!gpt.includes("Kore"));
  assert.deepEqual(speech("openrouter_speech", "openai/gpt-4o-mini-tts"), gpt);
  assert.deepEqual(speech("openai_speech", "gateway-tts-alias"), gpt);
  const older = speech("openrouter_speech", "openai/tts-1-hd");
  assert.ok(older.includes("nova"));
  assert.ok(!older.includes("marin"));
  assert.ok(!older.includes("verse"));
  assert.deepEqual(speech("openai_speech", "tts-1"), older);

  const google = speech(
    "gemini_generate_content",
    "models/gemini-2.5-flash-preview-tts",
  );
  assert.ok(google.includes("Kore"));
  assert.ok(google.includes("Puck"));
  assert.ok(!google.includes("alloy"));
  assert.deepEqual(
    speech("openrouter_speech", "google/gemini-3.8-flash-tts"),
    google,
  );
  assert.deepEqual(speech("gemini_interactions", "custom-tts-alias"), google);
});

test("provider voice metadata takes precedence without leaking voices to other models", () => {
  const slot = {
    kind: "speech",
    protocol: "openrouter_speech",
    model: "microsoft/mai-voice-2.1",
  };
  const upstream = [
    " en-US-Harper:MAI-Voice-2.1 ",
    "",
    "en-US-Harper:MAI-Voice-2.1",
    "zh-CN-Xiaochen:MAI-Voice-2.1",
  ];
  assert.deepEqual(Array.from(voiceOptions(slot, upstream)), [
    "en-US-Harper:MAI-Voice-2.1",
    "zh-CN-Xiaochen:MAI-Voice-2.1",
  ]);
  assert.equal(voiceOptions(slot).length, 0);
  assert.equal(voiceOptions({ ...slot, kind: "music" }, upstream).length, 0);
  const known = { ...slot, model: "openai/gpt-4o-mini-tts" };
  assert.deepEqual(Array.from(voiceOptions(known, ["gateway-voice"])), [
    "gateway-voice",
  ]);
  assert.ok(voiceOptions(known, [" "]).includes("alloy"));
});

test("chat audio uses its own voice list rather than the speech model list", () => {
  const slot = {
    kind: "audio",
    protocol: "openai_chat_audio",
    model: "gpt-audio",
  };
  const voices = voiceOptions(slot);
  assert.ok(voices.includes("verse"));
  assert.ok(!voices.includes("fable"));
  assert.ok(!voices.includes("Kore"));
  assert.deepEqual(
    voiceOptions({
      ...slot,
      protocol: "openrouter_chat_audio",
      model: "openai/gpt-audio",
    }),
    voices,
  );
  assert.equal(
    voiceOptions({
      ...slot,
      protocol: "openrouter_chat_audio",
      model: "custom/audio-model",
    }).length,
    0,
  );
});
