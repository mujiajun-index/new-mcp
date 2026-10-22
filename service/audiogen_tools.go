package service

import (
	"github.com/mujkjk/newmcp/internal/mcp/audiogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
)

func audioString() map[string]any { return map[string]any{"type": "string"} }
func buildAudioGenTools(c *model.AudioGenConfig) []map[string]any {
	tools := []map[string]any{}
	images, audios := false, false
	for _, s := range c.Models {
		if !s.Enabled {
			continue
		}
		p := map[string]any{"request_id": map[string]any{"type": "string", "maxLength": 128}, "wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 30, "default": 10}, "return_mode": map[string]any{"type": "string", "enum": []string{"url", "inline"}, "default": "url"}, "extra_body": map[string]any{"type": "object", "description": "Native upstream parameters; cannot override input, model, credentials or local controls"}, "format": audioString()}
		schema := map[string]any{"type": "object", "properties": p, "additionalProperties": false}
		if s.Kind == "speech" {
			p["text"] = audioString()
			p["voice"] = audioString()
			p["instructions"] = audioString()
			if s.Capabilities.MaxSpeakers > 1 {
				p["turns"] = map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "object", "properties": map[string]any{"role": audioString(), "text": audioString(), "instructions": audioString()}, "required": []string{"role", "text"}, "additionalProperties": false}}
				p["speakers"] = map[string]any{"type": "array", "minItems": 1, "maxItems": s.Capabilities.MaxSpeakers, "items": map[string]any{"type": "object", "properties": map[string]any{"role": audioString(), "voice": audioString()}, "required": []string{"role", "voice"}, "additionalProperties": false}}
				schema["oneOf"] = []any{map[string]any{"required": []string{"text"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"turns"}}, map[string]any{"required": []string{"speakers"}}}}}, map[string]any{"required": []string{"turns", "speakers"}, "not": map[string]any{"required": []string{"text"}}}}
			} else {
				schema["required"] = []string{"text"}
			}
		} else {
			p["prompt"] = audioString()
			anyOf := []any{map[string]any{"required": []string{"prompt"}}}
			if s.Kind == "audio" {
				p["voice"] = audioString()
				p["instructions"] = audioString()
				if audiogen.Contains(s.Capabilities.InputModalities, "audio") {
					p["audios"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 16, "items": audioString(), "description": "Audio URLs, data URLs or small Base64. For local files call upload_audio and execute its upload command."}
					audios = true
					anyOf = append(anyOf, map[string]any{"required": []string{"audios"}})
				}
			} else {
				if audiogen.Contains(s.Capabilities.InputModalities, "image") {
					p["images"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 16, "items": audioString(), "description": "Reference image URLs, data URLs or small Base64. Upload local images with upload_image first."}
					images = true
					anyOf = append(anyOf, map[string]any{"required": []string{"images"}})
				}
				if s.Capabilities.Lyrics {
					p["lyrics"] = audioString()
					anyOf = append(anyOf, map[string]any{"required": []string{"lyrics"}})
				}
				if s.Capabilities.Instrumental {
					p["is_instrumental"] = map[string]any{"type": "boolean"}
				}
				if s.Capabilities.LyricsOptimizer {
					p["lyrics_optimizer"] = map[string]any{"type": "boolean"}
				}
			}
			schema["anyOf"] = anyOf
		}
		desc := s.Description
		if desc == "" {
			desc = audiogen.DefaultDescription(s.Kind)
		}
		tools = append(tools, map[string]any{"name": audiogen.Tool(s.Kind), "description": desc, "inputSchema": schema})
	}
	tools = append(tools, map[string]any{"name": "get_audio_status", "description": "Query this service's audio task. Never regenerate to poll. retry_download retries storing an already generated result only, when can_retry_save is true.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"task_id": audioString(), "wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 30, "default": 0}, "retry_download": map[string]any{"type": "boolean"}, "return_mode": map[string]any{"type": "string", "enum": []string{"url", "inline"}}}, "required": []string{"task_id"}, "additionalProperties": false}})
	if images {
		upload := virtual.UploadImageTool()
		upload["description"] = "Stage a LOCAL image: call with local_path, execute upload_command in your shell, then pass image_url in generate_music.images."
		tools = append(tools, upload)
	}
	if audios {
		tools = append(tools, map[string]any{"name": "upload_audio", "description": "Stage a LOCAL WAV or MP3: call with local_path, execute upload_command in your shell, then pass audio_url in generate_audio.audios. The server cannot read your local disk.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"local_path": audioString()}, "required": []string{"local_path"}, "additionalProperties": false}})
	}
	return tools
}
