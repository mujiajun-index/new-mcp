package virtual

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mujkjk/newmcp/internal/imagegentask"
	"github.com/mujkjk/newmcp/model"
)

var ImageManager *imagegentask.Manager

// RunImageGenAsync is shared by public MCP calls and authenticated draft tests.
func RunImageGenAsync(ctx context.Context, uid int64, cfg *model.ImageGenConfig, tool string, raw json.RawMessage, source string) (json.RawMessage, error) {
	if ImageManager == nil {
		return nil, fmt.Errorf("image task manager not initialized")
	}
	if uid == 0 || cfg.UserID != uid {
		return nil, fmt.Errorf("image configuration is not accessible to this user")
	}
	if len(raw) > 32<<20 {
		return nil, fmt.Errorf("image arguments exceed 32 MiB")
	}
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil || args == nil {
		return nil, fmt.Errorf("arguments must be a JSON object")
	}
	mode := "url"
	if tool != model.GetImageStatusToolName {
		var defaults map[string]any
		_ = json.Unmarshal([]byte(cfg.DefaultParams), &defaults)
		if value, ok := defaults["return_mode"].(string); ok {
			mode = value
		}
	}
	if value, ok := args["return_mode"]; ok {
		var valid bool
		mode, valid = value.(string)
		if !valid || (mode != "url" && mode != "inline") {
			return nil, fmt.Errorf("return_mode must be url or inline")
		}
	}
	if tool == model.GetImageStatusToolName {
		for key := range args {
			if key != "task_id" && key != "wait_seconds" && key != "retry_download" && key != "return_mode" {
				return nil, fmt.Errorf("unknown status parameter %s", key)
			}
		}
		id, ok := args["task_id"].(string)
		if !ok || id == "" {
			return nil, fmt.Errorf("task_id is required")
		}
		wait, err := imagegentask.WaitSeconds(args, 0)
		if err != nil {
			return nil, err
		}
		t, err := ImageManager.Get(uid, &cfg.ID, id)
		if err != nil {
			return nil, err
		}
		if value, ok := args["retry_download"]; ok {
			retry, valid := value.(bool)
			if !valid {
				return nil, fmt.Errorf("retry_download must be a boolean")
			}
			if retry {
				t, err = ImageManager.RetrySave(uid, &cfg.ID, id)
				if err != nil {
					return nil, err
				}
			}
		}
		t, err = ImageManager.Wait(ctx, uid, t.ID, wait)
		if err != nil {
			return nil, err
		}
		return ImageManager.MCPResult(ctx, t, mode)
	}
	wait, err := imagegentask.WaitSeconds(args, 10)
	if err != nil {
		return nil, err
	}
	t, err := ImageManager.Submit(ctx, uid, cfg, tool, source, args)
	if err != nil {
		return nil, err
	}
	t, err = ImageManager.Wait(ctx, uid, t.ID, wait)
	if err != nil {
		return nil, err
	}
	return ImageManager.MCPResult(ctx, t, mode)
}
