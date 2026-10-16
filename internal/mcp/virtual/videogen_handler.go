package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/videogen"
	"github.com/mujkjk/newmcp/model"
)

var VideoManager *videogen.Manager

func VideoGenHandler(ctx context.Context, serviceID int64, config map[string]any, tool string, args json.RawMessage) (json.RawMessage, error) {
	cfg, err := model.GetVideoGenConfigByServiceID(serviceID)
	if err != nil {
		return nil, fmt.Errorf("video configuration not found")
	}
	uid := CallerUserID(ctx)
	if uid == 0 || uid != cfg.UserID {
		return nil, fmt.Errorf("video service is not accessible to this user")
	}
	if !cfg.AutoRegister || cfg.Status != common.StatusEnabled {
		return nil, fmt.Errorf("video service is disabled")
	}
	if tool == UploadImageToolName {
		if cfg.Operation == "generate" {
			return nil, fmt.Errorf("image-to-video is disabled")
		}
		return handleUploadImageFor(ctx, uid, args, model.GenerateVideoFromImageToolName)
	}
	return RunVideoGen(ctx, uid, cfg, tool, args, "mcp")
}

func RunVideoGen(ctx context.Context, uid int64, cfg *model.VideoGenConfig, tool string, raw json.RawMessage, source string) (json.RawMessage, error) {
	if VideoManager == nil {
		return nil, fmt.Errorf("video manager not initialized")
	}
	if uid == 0 || uid != cfg.UserID {
		return nil, fmt.Errorf("video configuration is not accessible to this user")
	}
	if len(raw) > 16<<20 {
		return nil, fmt.Errorf("video arguments exceed 16 MiB")
	}
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil || args == nil {
		return nil, fmt.Errorf("arguments must be a JSON object")
	}
	if tool == model.GetVideoStatusToolName {
		for k := range args {
			if k != "task_id" && k != "wait_seconds" && k != "retry_download" {
				return nil, fmt.Errorf("unknown status parameter %s", k)
			}
		}
		id, ok := args["task_id"].(string)
		if !ok || id == "" {
			return nil, fmt.Errorf("task_id is required")
		}
		wait, err := videogen.WaitSeconds(args, 0)
		if err != nil {
			return nil, err
		}
		retry := false
		if v, exists := args["retry_download"]; exists {
			retry, ok = v.(bool)
			if !ok {
				return nil, fmt.Errorf("retry_download must be a boolean")
			}
		}
		t, err := VideoManager.Get(uid, &cfg.ID, id, true)
		if err != nil {
			return nil, err
		}
		if retry {
			t, err = VideoManager.RetryDownload(uid, &cfg.ID, id)
			if err != nil {
				return nil, err
			}
		}
		t, err = VideoManager.Wait(ctx, uid, t.ID, wait)
		if err != nil {
			return nil, err
		}
		return videogen.MCPResult(t)
	}
	op := "generate"
	if tool == model.GenerateVideoFromImageToolName {
		op = "image"
	} else if tool != model.GenerateVideoToolName {
		return nil, fmt.Errorf("unknown video tool")
	}
	prompt, _ := args["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	wait, err := videogen.WaitSeconds(args, 10)
	if err != nil {
		return nil, err
	}
	t, err := VideoManager.Submit(ctx, uid, cfg, op, source, args)
	if err != nil {
		return nil, err
	}
	t, err = VideoManager.Wait(ctx, uid, t.ID, wait)
	if err != nil {
		return nil, err
	}
	return videogen.MCPResult(t)
}
