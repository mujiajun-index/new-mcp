package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/audioasset"
	"github.com/mujkjk/newmcp/internal/mcp/audiogen"
	"github.com/mujkjk/newmcp/model"
	"strings"
)

var AudioManager *audiogen.Manager

func AudioGenHandler(ctx context.Context, serviceID int64, config map[string]any, tool string, args json.RawMessage) (json.RawMessage, error) {
	var row model.AudioGenConfig
	if e := model.DB.Where("registered_service_id = ?", serviceID).First(&row).Error; e != nil {
		return nil, fmt.Errorf("audio configuration not found")
	}
	uid := CallerUserID(ctx)
	if uid == 0 || uid != row.UserID || !row.AutoRegister || row.Status != common.StatusEnabled {
		return nil, fmt.Errorf("audio service is not accessible")
	}
	c, e := model.GetAudioGenConfig(uid, row.ID)
	if e != nil {
		return nil, e
	}
	return RunAudioGen(ctx, uid, c, tool, args, "mcp")
}
func RunAudioGen(ctx context.Context, uid int64, c *model.AudioGenConfig, tool string, raw json.RawMessage, source string) (json.RawMessage, error) {
	if AudioManager == nil {
		return nil, fmt.Errorf("audio manager not initialized")
	}
	if uid == 0 || uid != c.UserID {
		return nil, fmt.Errorf("audio configuration is not accessible")
	}
	if len(raw) > 32<<20 {
		return nil, fmt.Errorf("audio arguments exceed 32 MiB")
	}
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil || args == nil {
		return nil, fmt.Errorf("arguments must be a JSON object")
	}
	mode := audiogen.Text(args, "return_mode")
	if mode == "" {
		mode = "url"
	}
	if mode != "url" && mode != "inline" {
		return nil, fmt.Errorf("return_mode must be url or inline")
	}
	if tool == "get_audio_status" {
		for k := range args {
			if !audiogen.Contains([]string{"task_id", "wait_seconds", "retry_download", "return_mode"}, k) {
				return nil, fmt.Errorf("unknown status parameter %s", k)
			}
		}
		id := audiogen.Text(args, "task_id")
		if id == "" {
			return nil, fmt.Errorf("task_id is required")
		}
		wait, e := audiogen.Wait(args, 0)
		if e != nil {
			return nil, e
		}
		t, e := AudioManager.Get(uid, &c.ID, id)
		if e != nil {
			return nil, e
		}
		if v, ok := args["retry_download"]; ok {
			retry, valid := v.(bool)
			if !valid {
				return nil, fmt.Errorf("retry_download must be boolean")
			}
			if retry {
				t, e = AudioManager.RetrySave(uid, &c.ID, id)
				if e != nil {
					return nil, e
				}
			}
		}
		t, e = AudioManager.Wait(ctx, uid, t.ID, wait)
		if e != nil {
			return nil, e
		}
		return AudioManager.Result(ctx, t, mode)
	}
	if tool == UploadImageToolName {
		conn, e := audiogen.Resolve(c, "music")
		if e != nil || !audiogen.Contains(conn.Slot.Capabilities.InputModalities, "image") {
			return nil, fmt.Errorf("image input is disabled")
		}
		return handleUploadImageFor(ctx, uid, raw, "generate_music (put image_url in images)")
	}
	if tool == "upload_audio" {
		conn, e := audiogen.Resolve(c, "audio")
		if e != nil || !audiogen.Contains(conn.Slot.Capabilities.InputModalities, "audio") {
			return nil, fmt.Errorf("audio input is disabled")
		}
		path := audiogen.Text(args, "local_path")
		absolute := strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) || (len(path) > 2 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && (path[2] == '\\' || path[2] == '/'))
		if !absolute {
			return nil, fmt.Errorf("local_path must be absolute")
		}
		asset, e := audioasset.NewPending(AudioManager.DB, uid)
		if e != nil {
			return nil, e
		}
		return json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": fmt.Sprintf("Run upload_command, then pass audio_url in generate_audio.audios. Upload expires in 15 minutes.\nupload_command: %s\naudio_url: %s", uploadCommand(path, audioasset.URL("PUT", asset.ShortID)), audioasset.URL("GET", asset.ShortID))}}})
	}
	kind := strings.TrimPrefix(tool, "generate_")
	if tool != audiogen.Tool(kind) || !audiogen.Contains([]string{"speech", "audio", "music"}, kind) {
		return nil, fmt.Errorf("unknown audio tool")
	}
	if _, specified := args["return_mode"]; !specified {
		if conn, err := audiogen.Resolve(c, kind); err == nil {
			var defaults map[string]any
			_ = json.Unmarshal([]byte(conn.Slot.DefaultParams), &defaults)
			if configured := audiogen.Text(defaults, "return_mode"); configured != "" {
				mode = configured
			}
		}
	}
	wait, e := audiogen.Wait(args, 10)
	if e != nil {
		return nil, e
	}
	t, e := AudioManager.Submit(ctx, uid, c, kind, args, source)
	if e != nil {
		return nil, e
	}
	t, e = AudioManager.Wait(ctx, uid, t.ID, wait)
	if e != nil {
		return nil, e
	}
	return AudioManager.Result(ctx, t, mode)
}
