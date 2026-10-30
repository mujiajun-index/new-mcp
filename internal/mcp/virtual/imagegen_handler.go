package virtual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/imagegentask"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
)

func ImageGenHandler(ctx context.Context, serviceID int64, config map[string]interface{}, tool string, args json.RawMessage) (json.RawMessage, error) {
	cfg, err := model.GetImageGenConfigByServiceID(serviceID)
	if err != nil {
		return nil, fmt.Errorf("image generation config not found")
	}
	uid := CallerUserID(ctx)
	if uid == 0 || !model.CanExecuteConfig(ctx, uid, cfg.UserID, cfg.ID, "imagegen") {
		return nil, fmt.Errorf("image service is not accessible to this user")
	}
	if !cfg.AutoRegister || cfg.Status != common.StatusEnabled {
		return nil, fmt.Errorf("image service is disabled")
	}
	if tool == UploadImageToolName {
		if cfg.Operation == "generate" {
			return nil, fmt.Errorf("image editing is disabled")
		}
		return handleUploadImageFor(ctx, uid, args, model.EditImageToolName)
	}
	return RunImageGenAsync(ctx, uid, cfg, tool, args, "mcp")
}

type generatedImageMeta struct {
	URL           string     `json:"url"`
	MIME          string     `json:"mime"`
	Size          int64      `json:"size"`
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	ExpiresAt     *time.Time `json:"expires_at"`
	RevisedPrompt string     `json:"revised_prompt,omitempty"`
}

// RunImageGen retains the synchronous internal compatibility helper. Public
// MCP handlers and authenticated draft tests use RunImageGenAsync instead.
func RunImageGen(ctx context.Context, store storage.Storage, userID int64, cfg *model.ImageGenConfig, tool string, raw json.RawMessage, source ...string) (json.RawMessage, error) {
	if userID == 0 || cfg.UserID != userID {
		return nil, fmt.Errorf("image service is not accessible to this user")
	}
	if tool != model.GenerateImageToolName && tool != model.EditImageToolName {
		return nil, fmt.Errorf("unknown image tool: %s", tool)
	}
	if cfg.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	origin := "mcp"
	if len(source) > 0 {
		origin = source[0]
	}
	task, err := imagegentask.Start(ctx, userID, cfg, tool, origin, raw)
	if err != nil {
		return nil, err
	}
	var images []model.ImageGenTaskImage
	result, runErr := runImageGen(ctx, store, userID, cfg, tool, raw, task.ID, &images)
	historyErr := runErr
	if historyErr == nil {
		historyErr = ctx.Err()
	}
	if err := imagegentask.Finish(task, images, result, historyErr, cfg.ApiKey); err != nil {
		// A history write failure must not discard already paid-for image links.
		log.Printf("[imagegen] could not finish task %s history: %v", task.ID, err)
		if runErr != nil {
			return nil, fmt.Errorf("%w (could not update task history)", runErr)
		}
	}
	return result, runErr
}

func runImageGen(ctx context.Context, store storage.Storage, userID int64, cfg *model.ImageGenConfig, tool string, raw json.RawMessage, taskID string, images *[]model.ImageGenTaskImage) (json.RawMessage, error) {
	operation := "generate"
	switch tool {
	case model.GenerateImageToolName:
		if cfg.Operation == "edit" {
			return nil, fmt.Errorf("text-to-image generation is disabled")
		}
	case model.EditImageToolName:
		operation = "edit"
		if cfg.Operation == "generate" {
			return nil, fmt.Errorf("image editing is disabled")
		}
	default:
		return nil, fmt.Errorf("unknown image tool: %s", tool)
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil || args == nil {
		return nil, fmt.Errorf("arguments must be a JSON object")
	}
	if err := imagegen.ValidateParams(cfg.Provider, args, false); err != nil {
		return nil, err
	}
	var defaults map[string]any
	if err := json.Unmarshal([]byte(cfg.DefaultParams), &defaults); err != nil {
		return nil, fmt.Errorf("invalid default parameters")
	}
	params, _ := imagegen.MergeParams(defaults, args)
	prompt, _ := params["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	mode := "url"
	if value, ok := params["return_mode"]; ok {
		var valid bool
		mode, valid = value.(string)
		if !valid || (mode != "url" && mode != "inline") {
			return nil, fmt.Errorf("return_mode must be url or inline")
		}
	}
	if _, ok := params["mask"]; ok && cfg.Provider == "gemini" {
		return nil, fmt.Errorf("Gemini does not support mask")
	}
	if _, ok := params["mask"]; ok && cfg.Provider == imagegen.ProviderOpenRouter {
		return nil, fmt.Errorf("OpenRouter Image API does not support mask; use reference images and a prompt")
	}
	if _, ok := params["mask"]; ok && !imagegen.SupportsMask(cfg.Provider, cfg.EditRequestFormat) {
		return nil, fmt.Errorf("mask is not supported by the configured JSON generations editing interface")
	}
	max := imagegen.MaxImageBytes()
	var references []imagegen.Image
	var mask *imagegen.Image
	if operation == "edit" {
		inputs, ok := params["images"].([]any)
		if !ok || len(inputs) == 0 || len(inputs) > 16 {
			return nil, fmt.Errorf("images must contain 1 to 16 reference images")
		}
		for _, input := range inputs {
			ref, ok := input.(string)
			if !ok || ref == "" {
				return nil, fmt.Errorf("each reference image must be a URL or base64 string")
			}
			img, err := imagegen.LoadReference(ctx, store, userID, ref, max)
			if err != nil {
				return nil, fmt.Errorf("reference image: %w", err)
			}
			references = append(references, img)
		}
		if value, ok := params["mask"]; ok {
			ref, valid := value.(string)
			if !valid || ref == "" {
				return nil, fmt.Errorf("mask must be a URL or base64 string")
			}
			img, err := imagegen.LoadReference(ctx, store, userID, ref, max)
			if err != nil {
				return nil, fmt.Errorf("mask: %w", err)
			}
			if img.MIME != "image/png" {
				return nil, fmt.Errorf("mask must be a PNG image")
			}
			_, w, h, _ := imagegen.Inspect(img.Bytes, max)
			_, rw, rh, _ := imagegen.Inspect(references[0].Bytes, max)
			if w != rw || h != rh {
				return nil, fmt.Errorf("mask dimensions must match the first reference image")
			}
			mask = &img
		}
	} else {
		if _, ok := params["images"]; ok {
			return nil, fmt.Errorf("use edit_image for reference images")
		}
		if _, ok := params["mask"]; ok {
			return nil, fmt.Errorf("mask requires edit_image")
		}
	}
	if store == nil {
		return nil, fmt.Errorf("image storage not initialized")
	}
	if operation == "edit" {
		recorded := make([]taskreference.Image, 0, len(references)+1)
		for _, img := range references {
			item, err := taskreference.Capture(ctx, store, userID, img, "reference")
			if err != nil {
				return nil, fmt.Errorf("record reference image: %w", err)
			}
			recorded = append(recorded, item)
		}
		if mask != nil {
			item, err := taskreference.Capture(ctx, store, userID, *mask, "mask")
			if err != nil {
				return nil, fmt.Errorf("record mask image: %w", err)
			}
			recorded = append(recorded, item)
		}
		if err := model.DB.WithContext(ctx).Model(&model.ImageGenTask{}).Where("id = ? AND user_id = ?", taskID, userID).Updates(map[string]any{"reference_images": taskreference.Encode(recorded), "reference_count": len(references)}).Error; err != nil {
			return nil, fmt.Errorf("record reference images: %w", err)
		}
	}
	if err := imageasset.CheckGeneratedQuota(userID); err != nil {
		return nil, err
	}
	client := &imagegen.Client{Provider: cfg.Provider, EndpointURL: cfg.EndpointURL, APIKey: cfg.ApiKey, Model: cfg.ModelName, EditRequestFormat: cfg.EditRequestFormat}
	result, err := client.Run(ctx, operation, params, references, mask)
	if err != nil {
		return nil, err
	}
	metadata := []generatedImageMeta{}
	content := []map[string]any{}
	inline := []map[string]any{}
	warnings := []string{}
	for i, img := range result.Images {
		data := img.Bytes
		if len(data) == 0 {
			data, err = imagegen.Download(ctx, img.URL, cfg.EndpointURL, max)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("image %d: download failed: %v", i+1, err))
				continue
			}
		}
		inspected, w, h, err := imagegen.Inspect(data, max)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("image %d: %v", i+1, err))
			continue
		}
		saved, err := imageasset.SaveGenerated(ctx, store, userID, bytes.NewReader(data), max)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("image %d: save failed: %v", i+1, err))
			continue
		}
		metadata = append(metadata, generatedImageMeta{saved.URL, inspected.MIME, saved.Size, w, h, saved.ExpiresAt, img.RevisedPrompt})
		*images = append(*images, model.ImageGenTaskImage{UploadID: saved.UploadID, MIME: inspected.MIME, Size: saved.Size, Width: w, Height: h, RevisedPrompt: img.RevisedPrompt})
		content = append(content, map[string]any{"type": "resource_link", "uri": saved.URL, "name": fmt.Sprintf("generated-image-%d", i+1), "mimeType": inspected.MIME, "size": saved.Size})
		if mode == "inline" {
			inline = append(inline, map[string]any{"type": "image", "data": base64.StdEncoding.EncodeToString(data), "mimeType": inspected.MIME})
		}
	}
	structured := map[string]any{"task_id": taskID, "provider": cfg.Provider, "model": cfg.ModelName, "operation": operation, "images": metadata}
	if result.Text != "" {
		structured["text"] = result.Text
	}
	if len(result.Usage) > 0 {
		structured["usage"] = result.Usage
	}
	if len(warnings) > 0 {
		structured["warnings"] = warnings
	}
	text, _ := json.Marshal(structured)
	content = append([]map[string]any{{"type": "text", "text": string(text)}}, content...)
	response := map[string]any{"content": content, "structuredContent": structured}
	if len(warnings) > 0 || len(metadata) == 0 {
		response["isError"] = true
	}
	if mode == "inline" && len(inline) > 0 {
		response["content"] = append(append([]map[string]any{}, content...), inline...)
		encoded, _ := json.Marshal(response)
		limit := model.GetOptionInt64("ImageGenInlineMaxBytes")
		if limit <= 0 {
			limit = 1 << 20
		}
		if int64(len(encoded)) > limit {
			response["content"] = content
			warnings = append(warnings, fmt.Sprintf("Inline response exceeds %d bytes; use the image links.", limit))
			structured["warnings"] = warnings
			text, _ = json.Marshal(structured)
			content[0]["text"] = string(text)
		}
	}
	return json.Marshal(response)
}
