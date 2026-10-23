package imagegentask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
)

type snapshot struct {
	Provider   string
	Endpoint   string
	APIKey     string
	Model      string
	Format     string
	Timeout    int
	Params     map[string]any
	References []taskreference.Image
}

func prepare(ctx context.Context, store storage.Storage, uid int64, cfg *model.ImageGenConfig, operation string, args map[string]any) (*snapshot, error) {
	if err := imagegen.ValidateParams(cfg.Provider, args, false); err != nil {
		return nil, err
	}
	var defaults map[string]any
	if json.Unmarshal([]byte(cfg.DefaultParams), &defaults) != nil {
		return nil, fmt.Errorf("invalid default parameters")
	}
	p, _ := imagegen.MergeParams(defaults, args)
	if err := imagegen.ValidateParams(cfg.Provider, p, false); err != nil {
		return nil, err
	}
	prompt, _ := p["prompt"].(string)
	if strings.TrimSpace(prompt) == "" || len(prompt) > 65536 {
		return nil, fmt.Errorf("prompt must contain 1 to 65536 bytes")
	}
	if mode, ok := p["return_mode"]; ok && mode != "url" && mode != "inline" {
		return nil, fmt.Errorf("return_mode must be url or inline")
	}
	if operation != "generate" && operation != "edit" {
		return nil, fmt.Errorf("unknown image operation")
	}
	if cfg.Operation != "both" && cfg.Operation != operation {
		return nil, fmt.Errorf("image operation is disabled")
	}
	if _, ok := p["mask"]; ok && !imagegen.SupportsMask(cfg.Provider, cfg.EditRequestFormat) {
		return nil, fmt.Errorf("%s does not support mask with this interface", cfg.Provider)
	}
	s := &snapshot{Provider: cfg.Provider, Endpoint: cfg.EndpointURL, APIKey: cfg.ApiKey, Model: cfg.ModelName, Format: cfg.EditRequestFormat, Timeout: cfg.TimeoutSeconds, Params: p}
	if _, err := (&imagegen.Client{Provider: s.Provider, EndpointURL: s.Endpoint, Model: s.Model, EditRequestFormat: s.Format}).Endpoint(operation); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("image storage not initialized")
	}
	if operation == "generate" {
		if _, ok := p["images"]; ok {
			return nil, fmt.Errorf("use edit_image for reference images")
		}
		if _, ok := p["mask"]; ok {
			return nil, fmt.Errorf("mask requires edit_image")
		}
	} else {
		inputs, ok := p["images"].([]any)
		if !ok || len(inputs) == 0 || len(inputs) > 16 {
			return nil, fmt.Errorf("images must contain 1 to 16 reference images")
		}
		for _, value := range inputs {
			ref, ok := value.(string)
			if !ok || ref == "" {
				return nil, fmt.Errorf("each reference image must be a URL or base64 string")
			}
			img, err := imagegen.LoadReference(ctx, store, uid, ref, imagegen.MaxImageBytes())
			if err != nil {
				return nil, fmt.Errorf("reference image: %w", err)
			}
			item, err := taskreference.Capture(ctx, store, uid, img, "reference")
			if err != nil {
				return nil, fmt.Errorf("record reference image: %w", err)
			}
			s.References = append(s.References, item)
		}
		if value, ok := p["mask"]; ok {
			ref, ok := value.(string)
			if !ok || ref == "" {
				return nil, fmt.Errorf("mask must be a URL or base64 string")
			}
			img, err := imagegen.LoadReference(ctx, store, uid, ref, imagegen.MaxImageBytes())
			if err != nil {
				return nil, fmt.Errorf("mask: %w", err)
			}
			_, width, height, _ := imagegen.Inspect(img.Bytes, imagegen.MaxImageBytes())
			if img.MIME != "image/png" {
				return nil, fmt.Errorf("mask must be a PNG image")
			}
			if width != s.References[0].Width || height != s.References[0].Height {
				return nil, fmt.Errorf("mask dimensions must match the first reference image")
			}
			item, err := taskreference.Capture(ctx, store, uid, img, "mask")
			if err != nil {
				return nil, err
			}
			s.References = append(s.References, item)
		}
	}
	delete(p, "images")
	delete(p, "mask")
	if err := imageasset.CheckGeneratedQuota(uid); err != nil {
		return nil, err
	}
	return s, nil
}
