package virtual

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/model"
)

func VisionHandler(ctx context.Context, serviceID int64, config map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error) {
	refID, _ := config["ref_id"].(float64)
	if refID == 0 {
		return nil, fmt.Errorf("invalid ref_id in virtual service config")
	}

	vc, err := model.GetVisionConfigByServiceID(serviceID)
	if err != nil {
		return nil, fmt.Errorf("vision config not found: %w", err)
	}

	var params struct {
		Image string `json:"image"`
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Image == "" {
		return nil, fmt.Errorf("image parameter is required")
	}

	client := &vision.VisionClient{
		Provider:    vc.Provider,
		EndpointURL: vc.EndpointURL,
		ApiKey:      vc.ApiKey,
		ModelName:   vc.ModelName,
		MaxTokens:   vc.MaxTokens,
	}

	var systemPrompt, userPrompt string

	switch {
	case strings.HasSuffix(toolName, "analyze_image") || toolName == "vision.analyze_image":
		systemPrompt = vc.SystemPrompt
		if systemPrompt == "" {
			systemPrompt = "你是一个图像分析助手，请分析用户提供的图片。"
		}
		if params.Prompt != "" {
			userPrompt = params.Prompt
		} else {
			userPrompt = "请详细分析这张图片的内容。"
		}
	case strings.HasSuffix(toolName, "describe_scene") || toolName == "vision.describe_scene":
		systemPrompt = vc.SystemPrompt
		if systemPrompt == "" {
			systemPrompt = "你是一个场景描述助手，请描述图片中的场景。"
		}
		userPrompt = "请描述这张图片中的场景和整体内容。"
	default:
		return nil, fmt.Errorf("unknown vision tool: %s", toolName)
	}

	result, err := client.Analyze(ctx, systemPrompt, userPrompt, params.Image)
	if err != nil {
		return nil, err
	}

	resp := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": result},
		},
	}
	return json.Marshal(resp)
}

// ExtractBase64Image removes data URL prefix if present.
func ExtractBase64Image(data string) string {
	if idx := strings.Index(data, ","); idx >= 0 {
		return data[idx+1:]
	}
	return data
}

// EncodeFrameToBase64 encodes raw JPEG bytes to base64.
func EncodeFrameToBase64(frame []byte) string {
	return base64.StdEncoding.EncodeToString(frame)
}
