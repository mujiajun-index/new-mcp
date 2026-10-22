import type { Kind, Protocol } from "./api";

// https://developers.openai.com/api/docs/guides/text-to-speech
const openaiTTS = [
  "alloy",
  "ash",
  "ballad",
  "coral",
  "echo",
  "fable",
  "nova",
  "onyx",
  "sage",
  "shimmer",
  "verse",
  "marin",
  "cedar",
];
const openaiTTS1 = [
  "alloy",
  "ash",
  "coral",
  "echo",
  "fable",
  "nova",
  "onyx",
  "sage",
  "shimmer",
];
// Chat Audio has a separate voice enum from the Speech API.
const openaiChatAudio = [
  "alloy",
  "ash",
  "ballad",
  "coral",
  "echo",
  "sage",
  "shimmer",
  "verse",
  "marin",
  "cedar",
];

// https://ai.google.dev/gemini-api/docs/speech-generation#voice-options
const geminiTTS = [
  "Zephyr",
  "Puck",
  "Charon",
  "Kore",
  "Fenrir",
  "Leda",
  "Orus",
  "Aoede",
  "Callirrhoe",
  "Autonoe",
  "Enceladus",
  "Iapetus",
  "Umbriel",
  "Algieba",
  "Despina",
  "Erinome",
  "Algenib",
  "Rasalgethi",
  "Laomedeia",
  "Achernar",
  "Alnilam",
  "Schedar",
  "Gacrux",
  "Pulcherrima",
  "Achird",
  "Zubenelgenubi",
  "Vindemiatrix",
  "Sadachbia",
  "Sadaltager",
  "Sulafat",
];

export function voiceOptions(
  slot: { kind: Kind; protocol: Protocol; model: string },
  supportedVoices?: string[],
): string[] {
  if (slot.kind === "music") return [];
  const discovered = [
    ...new Set(supportedVoices?.map((v) => v.trim()).filter(Boolean)),
  ];
  if (discovered.length) return discovered;

  const model = slot.model
    .trim()
    .toLowerCase()
    .replace(/^models\//, "");
  const id = model.split("/").at(-1) || "";
  if (slot.kind === "speech") {
    if (slot.protocol.startsWith("gemini_") || /^gemini-.*tts/.test(id))
      return [...geminiTTS];
    if (/^tts-1(?:-hd)?(?:-|$)/.test(id)) return [...openaiTTS1];
    if (slot.protocol === "openai_speech" || /^gpt-.*tts/.test(id))
      return [...openaiTTS];
  }
  if (
    slot.protocol === "openai_chat_audio" ||
    (slot.protocol === "openrouter_chat_audio" &&
      /^(?:openai\/)?gpt-.*audio/.test(model))
  )
    return [...openaiChatAudio];

  // Other providers use their own voice IDs. Suggestions never restrict manual input.
  return [];
}
