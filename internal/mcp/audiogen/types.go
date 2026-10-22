package audiogen

import (
	"encoding/json"
	"fmt"
	"github.com/mujkjk/newmcp/model"
	"net/url"
	"strings"
)

var Protocols = []string{"openai_speech", "openai_chat_audio", "openai_realtime", "openrouter_speech", "openrouter_chat_audio", "gemini_generate_content", "gemini_interactions", "minimax_music"}

func Contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
func Tool(kind string) string { return "generate_" + kind }
func DefaultDescription(kind string) string {
	base := map[string]string{"speech": "Read text aloud or perform scripted turns with configured speaker voices. Provide text OR turns and speakers.", "audio": "Generate a spoken response to prompt and/or audios. Supports text, recordings or both when enabled. Upload local recordings with upload_audio first.", "music": "Compose music from prompt, lyrics and/or reference images supported by the configured model. Upload local reference images with upload_image first."}[kind]
	return base + " Returns a local task_id; generation continues after the call. Use get_audio_status to retrieve saved audio, text and lyrics. Reuse request_id for the same submission to avoid duplicate paid generation."
}
func Preset(kind, protocol string) model.AudioGenCapabilities {
	c := model.AudioGenCapabilities{InputModalities: []string{"text"}, OutputModalities: []string{"audio"}, MaxSpeakers: 1}
	if kind == "audio" && (protocol == "openai_chat_audio" || protocol == "openrouter_chat_audio") {
		c.InputModalities = append(c.InputModalities, "audio")
		c.OutputModalities = append(c.OutputModalities, "text")
	}
	if kind == "speech" && (strings.HasPrefix(protocol, "gemini_") || protocol == "openrouter_speech") {
		c.MaxSpeakers = 2
	}
	if kind == "music" {
		c.Lyrics = true
		c.Instrumental = true
		c.OutputModalities = append(c.OutputModalities, "text")
		if strings.HasPrefix(protocol, "gemini_") || protocol == "openrouter_chat_audio" {
			c.InputModalities = append(c.InputModalities, "image")
		}
		c.LyricsOptimizer = protocol == "minimax_music"
	}
	return c
}

// ModelPreset is conservative when a gateway supplies no model metadata.
// Preset describes the adapter ceiling, not every model behind that adapter.
func ModelPreset(kind, protocol, name string) model.AudioGenCapabilities {
	c := Preset(kind, protocol)
	lower := strings.ToLower(name)
	if kind == "audio" && !strings.Contains(lower, "gpt-audio") && !strings.Contains(lower, "gpt-4o-audio") {
		c.InputModalities = []string{"text"}
	}
	if kind == "music" && !strings.Contains(lower, "lyria") {
		c.InputModalities = []string{"text"}
	}
	if protocol == "openrouter_speech" && !strings.Contains(lower, "gemini") {
		c.MaxSpeakers = 1
	}
	return c
}
func ValidateSlot(s *model.AudioGenModel) error {
	if !Contains([]string{"speech", "audio", "music"}, s.Kind) || !Contains(Protocols, s.Protocol) {
		return fmt.Errorf("invalid audio kind or protocol")
	}
	valid := false
	switch s.Protocol {
	case "openai_speech", "openrouter_speech":
		valid = s.Kind == "speech"
	case "openai_chat_audio", "openai_realtime":
		valid = s.Kind != "music"
	case "gemini_generate_content", "gemini_interactions":
		valid = s.Kind == "speech" || s.Kind == "music"
	case "openrouter_chat_audio":
		valid = true
	case "minimax_music":
		valid = s.Kind == "music"
	}
	if !valid {
		return fmt.Errorf("protocol %s cannot serve %s", s.Protocol, s.Kind)
	}
	if s.Protocol == "minimax_music" && strings.HasPrefix(strings.ToLower(s.Model), "music-cover") {
		return fmt.Errorf("music cover models are not supported")
	}
	if s.Model == "" || len(s.Model) > 128 || s.ConnectionKey == "" {
		return fmt.Errorf("model and connection_key are required")
	}
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds > 86400 {
		return fmt.Errorf("timeout_seconds must be 1-86400")
	}
	c := s.Capabilities
	p := Preset(s.Kind, s.Protocol)
	if !Contains(c.OutputModalities, "audio") {
		return fmt.Errorf("model must support audio output")
	}
	for _, v := range c.InputModalities {
		if !Contains(p.InputModalities, v) {
			return fmt.Errorf("%s input unsupported by adapter", v)
		}
	}
	if len(c.InputModalities) == 0 || c.MaxSpeakers < 1 || c.MaxSpeakers > p.MaxSpeakers || (c.Lyrics && !p.Lyrics) || (c.Instrumental && !p.Instrumental) || (c.LyricsOptimizer && !p.LyricsOptimizer) {
		return fmt.Errorf("capabilities exceed protocol support")
	}
	var params map[string]any
	if json.Unmarshal([]byte(s.DefaultParams), &params) != nil || params == nil {
		return fmt.Errorf("default_params must be a JSON object")
	}
	for _, key := range []string{"request_id", "wait_seconds", "text", "prompt", "images", "audios", "turns", "speakers"} {
		if _, ok := params[key]; ok {
			return fmt.Errorf("%s belongs in generation arguments, not default_params", key)
		}
	}
	_, e := Normalize(s, params, true)
	return e
}
func BaseURL(raw, protocol string) (string, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("base_url must be an HTTP(S) base URL without credentials or query")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasPrefix(protocol, "gemini_") {
		u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/v1beta"), "/v1")
		u.Path += "/v1beta"
	} else {
		if strings.HasPrefix(protocol, "openrouter_") && strings.EqualFold(u.Hostname(), "openrouter.ai") && u.Path == "" {
			u.Path = "/api"
		}
		if !strings.HasSuffix(u.Path, "/v1") {
			u.Path += "/v1"
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}

type Turn struct {
	Role         string `json:"role"`
	Text         string `json:"text"`
	Instructions string `json:"instructions,omitempty"`
}
type Speaker struct {
	Role  string `json:"role"`
	Voice string `json:"voice"`
}
type Media struct {
	Data   []byte `json:"data"`
	MIME   string `json:"mime"`
	Format string `json:"format,omitempty"`
}
type Input struct {
	Params map[string]any `json:"params"`
	Images []Media        `json:"images,omitempty"`
	Audios []Media        `json:"audios,omitempty"`
}
type Connection struct {
	BaseURL string              `json:"base_url"`
	APIKey  string              `json:"api_key"`
	Slot    model.AudioGenModel `json:"slot"`
}
type Audio struct {
	Data     []byte `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
	MIME     string `json:"mime,omitempty"`
	Rate     int    `json:"rate,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Bits     int    `json:"bits,omitempty"`
}
type Output struct {
	Connection      string          `json:"connection,omitempty"` // encrypted; staging only, never public JSON
	Audios          []Audio         `json:"audios"`
	Text            string          `json:"text,omitempty"`
	Transcript      string          `json:"transcript,omitempty"`
	InputTranscript string          `json:"input_transcript,omitempty"`
	Lyrics          string          `json:"lyrics,omitempty"`
	Usage           json.RawMessage `json:"usage,omitempty"`
}

func Text(p map[string]any, k string) string { s, _ := p[k].(string); return strings.TrimSpace(s) }
func openRouterGeminiTTS(s model.AudioGenModel) bool {
	return s.Protocol == "openrouter_speech" && strings.Contains(strings.ToLower(s.Model), "gemini")
}
func outputFormat(s model.AudioGenModel, p map[string]any) string {
	format := Text(p, "format")
	// Gemini TTS returns raw PCM through OpenRouter. WAV is the saved container,
	// not a supported upstream response_format.
	if openRouterGeminiTTS(s) && (format == "" || format == "wav") {
		return "pcm"
	}
	if format == "" {
		return "mp3"
	}
	return format
}
func Turns(p map[string]any) ([]Turn, []Speaker) {
	var t []Turn
	var s []Speaker
	b, _ := json.Marshal(p["turns"])
	_ = json.Unmarshal(b, &t)
	b, _ = json.Marshal(p["speakers"])
	_ = json.Unmarshal(b, &s)
	return t, s
}
func Wait(p map[string]any, fallback int) (int, error) {
	if v, ok := p["wait_seconds"]; ok {
		f, ok := v.(float64)
		if !ok || f != float64(int(f)) || f < 0 || f > 30 {
			return 0, fmt.Errorf("wait_seconds must be an integer from 0 to 30")
		}
		return int(f), nil
	}
	return fallback, nil
}
func Normalize(s *model.AudioGenModel, p map[string]any, defaults bool) (map[string]any, error) {
	allowed := []string{"request_id", "wait_seconds", "return_mode", "extra_body", "format", "sample_rate", "channels", "bit_depth", "speed"}
	if s.Kind == "speech" {
		allowed = append(allowed, "text", "turns", "speakers", "voice", "instructions")
	}
	if s.Kind == "audio" {
		allowed = append(allowed, "prompt", "audios", "voice", "instructions")
	}
	if s.Kind == "music" {
		allowed = append(allowed, "prompt", "images", "lyrics", "is_instrumental", "lyrics_optimizer")
	}
	for k := range p {
		if !Contains(allowed, k) {
			return nil, fmt.Errorf("unsupported parameter %s", k)
		}
	}
	if s.Protocol == "openai_realtime" {
		if f := Text(p, "format"); f != "" && f != "wav" && f != "pcm" {
			return nil, fmt.Errorf("Realtime output is PCM wrapped as WAV; format must be wav or pcm")
		}
		if extra, ok := p["extra_body"].(map[string]any); ok && len(extra) > 0 {
			return nil, fmt.Errorf("Realtime accepts only declared voice and instructions parameters")
		}
	}
	if openRouterGeminiTTS(*s) && outputFormat(*s, p) != "pcm" {
		return nil, fmt.Errorf("Gemini TTS via OpenRouter returns PCM saved as WAV; format must be pcm or wav")
	}
	if format := Text(p, "format"); format != "" {
		formats := []string{"mp3", "wav", "opus", "aac", "flac", "pcm"}
		if s.Protocol == "openai_chat_audio" || s.Protocol == "openrouter_chat_audio" {
			formats = []string{"mp3", "wav", "opus", "flac"}
		}
		if strings.HasPrefix(s.Protocol, "gemini_") {
			if s.Kind == "music" {
				formats = []string{"mp3"}
			} else {
				formats = []string{"wav"}
			}
		}
		if s.Protocol == "minimax_music" {
			formats = []string{"mp3", "wav"}
		}
		if !Contains(formats, format) {
			return nil, fmt.Errorf("unsupported output format for %s", s.Protocol)
		}
	}
	for _, k := range []string{"sample_rate", "channels", "bit_depth"} {
		if _, ok := p[k]; ok {
			if k != "sample_rate" || s.Protocol != "openrouter_speech" || outputFormat(*s, p) != "pcm" {
				return nil, fmt.Errorf("%s is unsupported here; use native output settings in extra_body", k)
			}
			f, valid := p[k].(float64)
			if !valid || f < 8000 || f > 192000 || f != float64(int(f)) {
				return nil, fmt.Errorf("sample_rate must be an integer from 8000 to 192000")
			}
		}
	}
	if v, ok := p["speed"]; ok {
		f, valid := v.(float64)
		if s.Protocol != "openai_speech" && s.Protocol != "openrouter_speech" {
			return nil, fmt.Errorf("speed is unsupported by this protocol")
		}
		if !valid || f < 0.25 || f > 4 {
			return nil, fmt.Errorf("speed must be from 0.25 to 4")
		}
	}
	for _, k := range []string{"prompt", "text", "voice", "instructions", "lyrics", "format", "request_id", "return_mode"} {
		if v, ok := p[k]; ok {
			if _, ok = v.(string); !ok {
				return nil, fmt.Errorf("%s must be a string", k)
			}
		}
	}
	for _, k := range []string{"is_instrumental", "lyrics_optimizer"} {
		if v, ok := p[k]; ok {
			if _, ok = v.(bool); !ok {
				return nil, fmt.Errorf("%s must be boolean", k)
			}
		}
	}
	if mode := Text(p, "return_mode"); mode != "" && mode != "url" && mode != "inline" {
		return nil, fmt.Errorf("return_mode must be url or inline")
	}
	if len(Text(p, "request_id")) > 128 {
		return nil, fmt.Errorf("request_id exceeds 128 bytes")
	}
	if _, e := Wait(p, 10); e != nil {
		return nil, e
	}
	for _, k := range []string{"images", "audios"} {
		if v, ok := p[k]; ok {
			a, ok := v.([]any)
			if !ok || len(a) == 0 || len(a) > 16 {
				return nil, fmt.Errorf("%s must contain 1-16 references", k)
			}
			for _, r := range a {
				if t, ok := r.(string); !ok || strings.TrimSpace(t) == "" {
					return nil, fmt.Errorf("invalid %s reference", k)
				}
			}
			mod := "image"
			if k == "audios" {
				mod = "audio"
			}
			if !Contains(s.Capabilities.InputModalities, mod) {
				return nil, fmt.Errorf("model does not support %s input", mod)
			}
		}
	}
	if _, ok := p["lyrics"]; ok && !s.Capabilities.Lyrics {
		return nil, fmt.Errorf("lyrics unsupported")
	}
	if _, ok := p["is_instrumental"]; ok && !s.Capabilities.Instrumental {
		return nil, fmt.Errorf("instrumental unsupported")
	}
	if _, ok := p["lyrics_optimizer"]; ok && !s.Capabilities.LyricsOptimizer {
		return nil, fmt.Errorf("lyrics_optimizer unsupported")
	}
	if v, ok := p["extra_body"]; ok {
		extra, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("extra_body must be an object")
		}
		blocked := []string{"model", "prompt", "text", "input", "messages", "contents", "generation_config", "generationConfig", "audio", "speech_config", "speechConfig", "voice", "instructions", "images", "audios", "lyrics", "is_instrumental", "lyrics_optimizer", "stream", "modalities", "response_format", "response_modalities", "responseModalities", "headers", "authorization", "api_key", "base_url", "endpoint_url", "request_id", "wait_seconds", "return_mode", "tools", "tool_choice", "n", "input_references", "input_audio", "audio_url", "audio_base64", "image_url", "inlineData", "inline_data", "fileData", "file_data", "cover_feature_id"}
		for k := range extra {
			if Contains(blocked, k) {
				return nil, fmt.Errorf("extra_body cannot override %s", k)
			}
		}
	}
	if defaults {
		return p, nil
	}
	if s.Kind == "speech" {
		for key, allowed := range map[string][]string{"turns": {"role", "text", "instructions"}, "speakers": {"role", "voice"}} {
			if raw, ok := p[key]; ok {
				rows, valid := raw.([]any)
				if !valid || len(rows) == 0 {
					return nil, fmt.Errorf("%s must be a nonempty array", key)
				}
				for _, row := range rows {
					obj, valid := row.(map[string]any)
					if !valid {
						return nil, fmt.Errorf("invalid %s item", key)
					}
					for k, v := range obj {
						if !Contains(allowed, k) {
							return nil, fmt.Errorf("unknown %s field %s", key, k)
						}
						if _, valid := v.(string); !valid {
							return nil, fmt.Errorf("%s.%s must be text", key, k)
						}
					}
				}
			}
		}
		t, sp := Turns(p)
		hasText := Text(p, "text") != ""
		_, hasTurns := p["turns"]
		_, hasSpeakers := p["speakers"]
		if hasText && (hasTurns || hasSpeakers) {
			return nil, fmt.Errorf("text and turns/speakers are mutually exclusive")
		}
		if !hasText {
			if len(t) == 0 || len(sp) == 0 || len(sp) > s.Capabilities.MaxSpeakers {
				return nil, fmt.Errorf("provide text or supported turns and speakers")
			}
			roles := map[string]bool{}
			for _, v := range sp {
				if v.Role == "" || v.Voice == "" || roles[v.Role] {
					return nil, fmt.Errorf("invalid or duplicate speaker")
				}
				roles[v.Role] = true
			}
			for _, v := range t {
				if !roles[v.Role] || strings.TrimSpace(v.Text) == "" {
					return nil, fmt.Errorf("turn requires a configured role and text")
				}
			}
		}
	}
	if s.Kind == "audio" && Text(p, "prompt") == "" && p["audios"] == nil {
		return nil, fmt.Errorf("prompt or audios is required")
	}
	if s.Kind == "music" {
		if Text(p, "prompt") == "" && Text(p, "lyrics") == "" && p["images"] == nil {
			return nil, fmt.Errorf("prompt, images or lyrics is required")
		}
		inst, _ := p["is_instrumental"].(bool)
		if inst && Text(p, "lyrics") != "" {
			return nil, fmt.Errorf("instrumental and lyrics are mutually exclusive")
		}
		if optimize, _ := p["lyrics_optimizer"].(bool); inst && optimize {
			return nil, fmt.Errorf("instrumental music does not use lyrics_optimizer")
		}
		if s.Protocol == "minimax_music" {
			if inst && Text(p, "prompt") == "" {
				return nil, fmt.Errorf("instrumental music requires prompt")
			}
			if Text(p, "lyrics") == "" && !inst {
				v, exists := p["lyrics_optimizer"]
				if exists && v == false {
					return nil, fmt.Errorf("lyrics required when lyrics_optimizer is false")
				}
				p["lyrics_optimizer"] = true
			}
			if len([]rune(Text(p, "prompt"))) > 2000 || len([]rune(Text(p, "lyrics"))) > 3500 {
				return nil, fmt.Errorf("MiniMax prompt or lyrics exceeds model limits")
			}
		}
	}
	return p, nil
}
