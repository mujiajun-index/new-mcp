package audiogen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mujkjk/newmcp/internal/audioasset"
)

// UncertainError means a request may already have generated/billed audio.
// It must never be automatically submitted a second time.
type UncertainError struct{ Err error }

func (e *UncertainError) Error() string {
	return "upstream submission outcome is unknown: " + e.Err.Error()
}
func (e *UncertainError) Unwrap() error { return e.Err }
func uncertain(e error) error           { return &UncertainError{e} }
func IsUncertain(e error) bool          { var u *UncertainError; return errors.As(e, &u) }

type Client struct {
	Connection Connection
	HTTPClient *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 2 || r.URL.Scheme != via[0].URL.Scheme || !strings.EqualFold(r.URL.Host, via[0].URL.Host) {
			return fmt.Errorf("upstream redirect changed origin")
		}
		return nil
	}}
}
func (c *Client) errorMessage(v map[string]any) string {
	message := ""
	if e, ok := v["error"].(map[string]any); ok {
		message, _ = e["message"].(string)
	}
	if br, ok := v["base_resp"].(map[string]any); ok {
		message, _ = br["status_msg"].(string)
	}
	if message == "" {
		message, _ = v["message"].(string)
	}
	if c.Connection.APIKey != "" {
		message = strings.ReplaceAll(message, c.Connection.APIKey, "[redacted]")
	}
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}
func voice(p map[string]any, gemini bool) string {
	if v := Text(p, "voice"); v != "" {
		return v
	}
	if gemini {
		return "Kore"
	}
	return "alloy"
}
func prompt(s string, p map[string]any) string {
	if s == "speech" {
		return Text(p, "text")
	}
	v := Text(p, "prompt")
	if s == "music" {
		if l := Text(p, "lyrics"); l != "" {
			v += "\nLyrics:\n" + l
		}
		if b, _ := p["is_instrumental"].(bool); b {
			v += "\nCreate instrumental music only, without vocals."
		}
	}
	return strings.TrimSpace(v)
}
func nativeExtra(p map[string]any) map[string]any {
	m := map[string]any{}
	if extra, ok := p["extra_body"].(map[string]any); ok {
		for k, v := range extra {
			m[k] = v
		}
	}
	return m
}
func (c *Client) Build(in Input) (string, map[string]any, error) {
	s := c.Connection.Slot
	p := in.Params
	body := nativeExtra(p)
	body["model"] = s.Model
	base, e := BaseURL(c.Connection.BaseURL, s.Protocol)
	if e != nil {
		return "", nil, e
	}
	turns, speakers := Turns(p)
	text := prompt(s.Kind, p)
	format := outputFormat(s, p)
	if openRouterGeminiTTS(s) && format != "pcm" {
		return "", nil, fmt.Errorf("Gemini TTS via OpenRouter returns PCM saved as WAV; format must be pcm or wav")
	}
	switch s.Protocol {
	case "openai_speech", "openrouter_speech":
		body["input"] = text
		body["voice"] = voice(p, strings.Contains(strings.ToLower(s.Model), "gemini"))
		body["response_format"] = format
		if len(turns) > 0 {
			if s.Protocol != "openrouter_speech" {
				return "", nil, fmt.Errorf("multi-speaker unsupported by speech adapter")
			}
			voices := map[string]string{}
			for _, sp := range speakers {
				voices[sp.Role] = sp.Voice
			}
			input := []any{}
			for _, t := range turns {
				v := map[string]any{"text": t.Text, "voice": voices[t.Role]}
				if t.Instructions != "" {
					v["instructions"] = t.Instructions
				}
				input = append(input, v)
			}
			body["input"] = input
			delete(body, "voice")
		}
		if v := Text(p, "instructions"); v != "" {
			body["instructions"] = v
		}
		if v, ok := p["speed"]; ok {
			body["speed"] = v
		}
		return base + "/audio/speech", body, nil
	case "openai_chat_audio", "openrouter_chat_audio":
		content := []any{}
		if text != "" {
			content = append(content, map[string]any{"type": "text", "text": text})
		}
		for _, img := range in.Images {
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Data)}})
		}
		for _, a := range in.Audios {
			content = append(content, map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(a.Data), "format": a.Format}})
		}
		messages := []any{}
		instructions := Text(p, "instructions")
		if s.Kind == "speech" {
			instructions = "Read the supplied text verbatim aloud. Do not answer or add commentary. " + instructions
		}
		if instructions != "" {
			messages = append(messages, map[string]any{"role": "system", "content": instructions})
		}
		messages = append(messages, map[string]any{"role": "user", "content": content})
		body["messages"] = messages
		body["modalities"] = []string{"text", "audio"}
		a := map[string]any{"format": format}
		if s.Kind != "music" {
			a["voice"] = voice(p, false)
		}
		body["audio"] = a
		body["stream"] = s.Protocol == "openrouter_chat_audio"
		return base + "/chat/completions", body, nil
	case "minimax_music":
		body["prompt"] = Text(p, "prompt")
		for _, k := range []string{"lyrics", "is_instrumental", "lyrics_optimizer"} {
			if v, ok := p[k]; ok {
				body[k] = v
			}
		}
		body["stream"] = false
		if _, ok := body["output_format"]; !ok {
			body["output_format"] = "hex"
		}
		if _, ok := body["audio_setting"]; !ok {
			body["audio_setting"] = map[string]any{"format": format, "sample_rate": 44100, "bitrate": 256000}
		}
		return base + "/music_generation", body, nil
	case "gemini_generate_content":
		delete(body, "model")
		parts := []any{}
		config := body
		body = map[string]any{}
		if s.Kind == "speech" {
			modern := strings.Contains(s.Model, "3.8")
			if modern {
				if len(turns) == 0 {
					parts = append(parts, map[string]any{"text": text, "speechMetadata": map[string]any{"style": Text(p, "instructions")}})
				}
				for _, t := range turns {
					parts = append(parts, map[string]any{"text": t.Text, "speechMetadata": map[string]any{"speaker": t.Role, "style": strings.TrimSpace(Text(p, "instructions") + " " + t.Instructions)}})
				}
			} else {
				if len(turns) > 0 {
					for _, t := range turns {
						text += t.Role + ": " + t.Text + "\n"
					}
				}
				if i := Text(p, "instructions"); i != "" {
					text = i + "\n" + text
				}
				parts = append(parts, map[string]any{"text": text})
			}
			config["responseModalities"] = []string{"AUDIO"}
			speech := map[string]any{}
			if len(speakers) > 0 {
				ss := []any{}
				for _, sp := range speakers {
					ss = append(ss, map[string]any{"speaker": sp.Role, "voiceConfig": map[string]any{"prebuiltVoiceConfig": map[string]any{"voiceName": sp.Voice}}})
				}
				speech["multiSpeakerVoiceConfig"] = map[string]any{"speakerVoiceConfigs": ss}
			} else {
				speech["voiceConfig"] = map[string]any{"prebuiltVoiceConfig": map[string]any{"voiceName": voice(p, true)}}
			}
			config["speechConfig"] = speech
		} else {
			if text != "" {
				parts = append(parts, map[string]any{"text": text})
			}
			for _, img := range in.Images {
				parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": img.MIME, "data": base64.StdEncoding.EncodeToString(img.Data)}})
			}
		}
		body["contents"] = []any{map[string]any{"role": "user", "parts": parts}}
		body["generationConfig"] = config
		return base + "/models/" + url.PathEscape(strings.TrimPrefix(s.Model, "models/")) + ":generateContent", body, nil
	case "gemini_interactions":
		config := body
		delete(config, "model")
		body = map[string]any{"model": s.Model}
		content := []any{}
		if s.Kind == "speech" {
			if len(turns) == 0 {
				turns = []Turn{{Text: text}}
			}
			for _, t := range turns {
				meta := map[string]any{"type": "speech_metadata", "style": strings.TrimSpace(Text(p, "instructions") + " " + t.Instructions)}
				if t.Role != "" {
					meta["speaker"] = t.Role
				}
				content = append(content, map[string]any{"type": "text", "text": t.Text, "annotations": []any{meta}})
			}
			speech := map[string]any{}
			if len(speakers) > 0 {
				ss := []any{}
				for _, sp := range speakers {
					ss = append(ss, map[string]any{"speaker": sp.Role, "voice": sp.Voice})
				}
				speech["speakers"] = ss
			} else {
				speech["voice"] = voice(p, true)
			}
			if len(speakers) > 0 {
				config["speech_config"] = speech
			} else {
				config["speech_config"] = []any{speech}
			}
			body["input"] = []any{map[string]any{"type": "user_input", "content": content}}
			body["response_format"] = map[string]any{"type": "audio"}
		} else {
			if text != "" {
				content = append(content, map[string]any{"type": "text", "text": text})
			}
			for _, img := range in.Images {
				content = append(content, map[string]any{"type": "image", "mime_type": img.MIME, "data": base64.StdEncoding.EncodeToString(img.Data)})
			}
			body["input"] = content
		}
		body["generation_config"] = config
		return base + "/interactions", body, nil
	}
	return "", nil, fmt.Errorf("unsupported HTTP audio protocol")
}
func (c *Client) Generate(ctx context.Context, in Input) (*Output, error) {
	if c.Connection.Slot.Protocol == "openai_realtime" {
		return c.realtime(ctx, in)
	}
	endpoint, body, e := c.Build(in)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(body)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(c.Connection.Slot.Protocol, "gemini_") {
		req.Header.Set("x-goog-api-key", c.Connection.APIKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.Connection.APIKey)
	}
	resp, e := c.httpClient().Do(req)
	if e != nil {
		return nil, uncertain(fmt.Errorf("audio request interrupted"))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := audioasset.Read(resp.Body, 64<<10)
		var detail map[string]any
		_ = json.Unmarshal(body, &detail)
		if resp.StatusCode >= 500 || resp.StatusCode == 408 {
			return nil, uncertain(fmt.Errorf("upstream HTTP %d", resp.StatusCode))
		}
		return nil, fmt.Errorf("upstream rejected audio request (HTTP %d): %s", resp.StatusCode, c.errorMessage(detail))
	}
	if c.Connection.Slot.Protocol == "openrouter_chat_audio" {
		return c.stream(resp.Body)
	}
	limit := audioasset.OutputLimit()
	if c.Connection.Slot.Protocol == "openai_speech" || c.Connection.Slot.Protocol == "openrouter_speech" {
		data, e := audioasset.Read(resp.Body, limit)
		if e != nil {
			return nil, uncertain(e)
		}
		if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
			var detail map[string]any
			if json.Unmarshal(data, &detail) == nil && detail["error"] != nil {
				return nil, fmt.Errorf("upstream rejected speech: %s", c.errorMessage(detail))
			}
		}
		a := Audio{Data: data, MIME: resp.Header.Get("Content-Type")}
		if Text(body, "response_format") == "pcm" || strings.Contains(a.MIME, "pcm") || strings.Contains(a.MIME, "L16") {
			_, metadata, _ := mime.ParseMediaType(a.MIME)
			a.MIME = "audio/pcm"
			a.Bits = 16
			a.Channels = 1
			if c.Connection.Slot.Protocol == "openai_speech" || openRouterGeminiTTS(c.Connection.Slot) {
				// Both providers document 24 kHz, mono, signed 16-bit PCM.
				// Explicit response metadata takes precedence for custom gateways.
				a.Rate = 24000
			}
			if f, ok := in.Params["sample_rate"].(float64); ok {
				a.Rate = int(f)
			}
			if rate, _ := strconv.Atoi(metadata["rate"]); rate > 0 {
				a.Rate = rate
			}
			if channels, _ := strconv.Atoi(metadata["channels"]); channels > 0 {
				a.Channels = channels
			}
			if a.Rate == 0 {
				return nil, fmt.Errorf("PCM output has no known sample rate")
			}
		}
		return &Output{Audios: []Audio{a}}, nil
	}
	b, e = audioasset.Read(resp.Body, limit*3+1<<20)
	if e != nil {
		return nil, uncertain(e)
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil {
		return nil, uncertain(fmt.Errorf("invalid upstream audio JSON"))
	}
	out := &Output{}
	out.InputTranscript, _ = v["input_transcript"].(string)
	if usage, ok := v["usage"]; ok {
		out.Usage, _ = json.Marshal(usage)
	}
	switch c.Connection.Slot.Protocol {
	case "minimax_music":
		br, _ := v["base_resp"].(map[string]any)
		if br == nil {
			return nil, uncertain(fmt.Errorf("missing MiniMax business status"))
		}
		if code, _ := br["status_code"].(float64); code != 0 {
			return nil, fmt.Errorf("MiniMax rejected music request (code %.0f): %s", code, c.errorMessage(v))
		}
		d, _ := v["data"].(map[string]any)
		if status, ok := d["status"].(float64); ok && status != 2 {
			return nil, uncertain(fmt.Errorf("MiniMax music response was not complete"))
		}
		a, _ := d["audio"].(string)
		if strings.HasPrefix(a, "https://") || strings.HasPrefix(a, "http://") {
			out.Audios = append(out.Audios, Audio{URL: a})
		} else {
			data, e := hex.DecodeString(a)
			if e != nil {
				return nil, uncertain(fmt.Errorf("invalid MiniMax Hex audio"))
			}
			out.Audios = append(out.Audios, Audio{Data: data})
		}
		if l, _ := d["lyrics"].(string); l != "" {
			out.Lyrics = l
		}
	case "openai_chat_audio":
		choices, _ := v["choices"].([]any)
		if len(choices) > 0 {
			choice, _ := choices[0].(map[string]any)
			if reason, ok := choice["finish_reason"].(string); ok && reason != "stop" {
				return nil, fmt.Errorf("audio generation stopped: %s", reason)
			}
			msg, _ := choice["message"].(map[string]any)
			out.Text, _ = msg["content"].(string)
			if a, ok := msg["audio"].(map[string]any); ok {
				encoded, _ := a["data"].(string)
				data, e := base64.StdEncoding.DecodeString(encoded)
				if e != nil {
					return nil, uncertain(fmt.Errorf("invalid audio Base64"))
				}
				out.Transcript, _ = a["transcript"].(string)
				out.Audios = append(out.Audios, Audio{Data: data})
			}
		}
	default:
		if v["error"] != nil {
			return nil, fmt.Errorf("Gemini rejected audio generation: %s", c.errorMessage(v))
		}
		if status, ok := v["status"].(string); ok && status != "completed" {
			if status == "failed" || status == "cancelled" {
				return nil, fmt.Errorf("Gemini audio generation %s", status)
			}
			return nil, uncertain(fmt.Errorf("Gemini audio response is not complete"))
		}
		if e = c.extract(v, out); e != nil {
			return nil, uncertain(e)
		}
		if usage, ok := v["usageMetadata"]; ok {
			out.Usage, _ = json.Marshal(usage)
		}
		if c.Connection.Slot.Kind == "music" {
			out.Lyrics = out.Text
			out.Text = ""
		}
	}
	if len(out.Audios) == 0 {
		return nil, fmt.Errorf("upstream returned no audio")
	}
	return out, nil
}

// extract visits all Gemini output parts; text/lyrics need not precede audio.
func (c *Client) extract(value any, out *Output) error {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if e := c.extract(item, out); e != nil {
				return e
			}
		}
	case map[string]any:
		if typ, _ := v["type"].(string); typ == "user_input" || typ == "tool_call" || typ == "tool_result" || typ == "thought" {
			return nil
		}
		if text, ok := v["text"].(string); ok {
			if out.Text != "" {
				out.Text += "\n"
			}
			out.Text += text
		}
		encoded, _ := v["data"].(string)
		mt, _ := v["mimeType"].(string)
		if mt == "" {
			mt, _ = v["mime_type"].(string)
		}
		typ, _ := v["type"].(string)
		if encoded != "" && (strings.HasPrefix(mt, "audio/") || typ == "audio") {
			data, e := base64.StdEncoding.DecodeString(encoded)
			if e != nil {
				return e
			}
			a := Audio{Data: data, MIME: mt}
			if strings.Contains(mt, "L16") || strings.Contains(mt, "pcm") {
				_, params, _ := mime.ParseMediaType(mt)
				a.MIME = "audio/pcm"
				a.Rate, _ = strconv.Atoi(params["rate"])
				a.Channels = 1
				if channels, _ := strconv.Atoi(params["channels"]); channels > 0 {
					a.Channels = channels
				}
				a.Bits = 16
				if a.Rate == 0 {
					return fmt.Errorf("Gemini PCM has no sample rate")
				}
			}
			out.Audios = append(out.Audios, a)
			return nil
		}
		for _, key := range []string{"candidates", "content", "parts", "inlineData", "inline_data", "outputs", "output", "output_audio", "steps"} {
			if child, ok := v[key]; ok {
				if e := c.extract(child, out); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func (c *Client) stream(r io.Reader) (*Output, error) {
	scanner := bufio.NewScanner(io.LimitReader(r, audioasset.OutputLimit()*3+8<<20))
	scanner.Buffer(make([]byte, 4096), 8<<20)
	out := &Output{}
	var audio bytes.Buffer
	completed := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var v map[string]any
		if json.Unmarshal([]byte(data), &v) != nil {
			return nil, uncertain(fmt.Errorf("invalid audio stream event"))
		}
		if v["error"] != nil {
			return nil, fmt.Errorf("upstream audio stream error")
		}
		if usage, ok := v["usage"]; ok {
			out.Usage, _ = json.Marshal(usage)
		}
		choices, _ := v["choices"].([]any)
		for _, raw := range choices {
			ch, _ := raw.(map[string]any)
			if reason, ok := ch["finish_reason"].(string); ok {
				if reason != "stop" {
					return nil, fmt.Errorf("audio generation stopped: %s", reason)
				}
				completed = true
			}
			d, _ := ch["delta"].(map[string]any)
			if t, _ := d["content"].(string); t != "" {
				out.Text += t
			}
			if a, ok := d["audio"].(map[string]any); ok {
				if s, _ := a["data"].(string); s != "" {
					b, e := base64.StdEncoding.DecodeString(s)
					if e != nil {
						return nil, uncertain(fmt.Errorf("invalid audio stream Base64"))
					}
					if int64(audio.Len()+len(b)) > audioasset.OutputLimit() {
						return nil, fmt.Errorf("audio exceeds output limit")
					}
					audio.Write(b)
				}
				if t, _ := a["transcript"].(string); t != "" {
					out.Transcript += t
				}
			}
		}
	}
	if e := scanner.Err(); e != nil {
		return nil, uncertain(e)
	}
	if !completed {
		return nil, uncertain(fmt.Errorf("audio stream ended without completion"))
	}
	if audio.Len() == 0 {
		return nil, fmt.Errorf("upstream returned no audio")
	}
	out.Audios = []Audio{{Data: audio.Bytes()}}
	if c.Connection.Slot.Kind == "music" {
		out.Lyrics = out.Text
		out.Text = ""
	}
	return out, nil
}
func (c *Client) realtime(ctx context.Context, in Input) (*Output, error) {
	base, e := BaseURL(c.Connection.BaseURL, c.Connection.Slot.Protocol)
	if e != nil {
		return nil, e
	}
	u, _ := url.Parse(base + "/realtime")
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	q := u.Query()
	q.Set("model", c.Connection.Slot.Model)
	u.RawQuery = q.Encode()
	headers := http.Header{"Authorization": []string{"Bearer " + c.Connection.APIKey}}
	d := websocket.Dialer{HandshakeTimeout: 30 * time.Second}
	ws, resp, e := d.DialContext(ctx, u.String(), headers)
	if e != nil {
		if resp != nil && resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, fmt.Errorf("Realtime connection rejected (HTTP %d)", resp.StatusCode)
		}
		return nil, fmt.Errorf("Realtime connection failed")
	}
	defer ws.Close()
	ws.SetReadLimit(8 << 20)
	if deadline, ok := ctx.Deadline(); ok {
		_ = ws.SetReadDeadline(deadline)
		_ = ws.SetWriteDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = ws.Close() })
	defer stop()
	instructions := Text(in.Params, "instructions")
	if c.Connection.Slot.Kind == "speech" {
		instructions = "Read the user's text verbatim aloud without commentary. " + instructions
	}
	if e = ws.WriteJSON(map[string]any{"type": "session.update", "session": map[string]any{"type": "realtime", "output_modalities": []string{"audio"}, "instructions": instructions, "audio": map[string]any{"output": map[string]any{"voice": voice(in.Params, false), "format": map[string]any{"type": "audio/pcm", "rate": 24000}}}}}); e != nil {
		return nil, uncertain(e)
	}
	out := &Output{}
	var b bytes.Buffer
	submitted := false
	for {
		var event map[string]any
		if e = ws.ReadJSON(&event); e != nil {
			return nil, uncertain(fmt.Errorf("Realtime interrupted"))
		}
		switch event["type"] {
		case "session.updated":
			if !submitted {
				if e = ws.WriteJSON(map[string]any{"type": "conversation.item.create", "item": map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": prompt(c.Connection.Slot.Kind, in.Params)}}}}); e != nil {
					return nil, uncertain(e)
				}
				if e = ws.WriteJSON(map[string]any{"type": "response.create"}); e != nil {
					return nil, uncertain(e)
				}
				submitted = true
			}
		case "response.output_audio.delta", "response.audio.delta":
			s, _ := event["delta"].(string)
			chunk, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return nil, uncertain(err)
			}
			if int64(b.Len()+len(chunk)) > audioasset.OutputLimit() {
				return nil, fmt.Errorf("audio exceeds output limit")
			}
			b.Write(chunk)
		case "response.output_audio_transcript.delta", "response.audio_transcript.delta":
			s, _ := event["delta"].(string)
			out.Transcript += s
		case "error":
			return nil, fmt.Errorf("Realtime rejected request")
		case "response.done":
			response, _ := event["response"].(map[string]any)
			if response["status"] != "completed" {
				return nil, fmt.Errorf("Realtime response did not complete")
			}
			if b.Len() == 0 {
				return nil, fmt.Errorf("Realtime returned no audio")
			}
			out.Audios = []Audio{{Data: b.Bytes(), MIME: "audio/pcm", Rate: 24000, Channels: 1, Bits: 16}}
			out.Usage, _ = json.Marshal(response["usage"])
			return out, nil
		}
	}
}
