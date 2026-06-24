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
	// Normalize to a raw base64 string + detected media type. Accepts raw
	// base64, data URLs, and url()-wrapped data URLs.
	imageBase64, mediaType := NormalizeImage(params.Image)
	if imageBase64 == "" {
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
		if params.Prompt != "" {
			userPrompt = params.Prompt
		} else {
			userPrompt = "请描述这张图片中的场景和整体内容。"
		}
	default:
		return nil, fmt.Errorf("unknown vision tool: %s", toolName)
	}

	result, err := client.Analyze(ctx, systemPrompt, userPrompt, imageBase64, mediaType)
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

// NormalizeImage normalizes an image input into a raw base64 string and its
// detected media type, the formats VisionClient.Analyze expects. It accepts:
//   - raw base64 string: "<base64>"                -> base64, "image/jpeg"
//   - data URL: "data:<mediatype>;base64,<base64>" -> base64, "<mediatype>"
//   - CSS url() wrapper around a data URL: url("data:<mediatype>;base64,<base64>")
//
// For bare base64 with no data URL (e.g. JPEG frames from the camera) the
// media type defaults to "image/jpeg".
func NormalizeImage(image string) (base64Data, mediaType string) {
	s := strings.TrimSpace(image)

	// Unwrap a leading CSS url(...) wrapper, e.g. url("data:...") or url(data:...).
	if strings.HasPrefix(strings.ToLower(s), "url(") {
		s = strings.TrimSpace(s[4:])
		s = strings.TrimSuffix(s, ")")
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "\"'") // strip surrounding " or '
		s = strings.TrimSpace(s)
	}

	// Default media type for bare base64 input (e.g. camera JPEG frames).
	mediaType = "image/jpeg"

	// Strip the data URL prefix and capture its declared media type. Base64 data
	// never contains a comma, so the comma reliably marks the end of the prefix.
	if strings.HasPrefix(strings.ToLower(s), "data:") {
		if idx := strings.Index(s, ","); idx >= 0 {
			mediaType = parseDataURLMediaType(s[:idx])
			s = s[idx+1:]
		}
	}

	return s, mediaType
}

// parseDataURLMediaType extracts the media type from a data-URL header portion
// such as "data:image/png;base64" -> "image/png". Falls back to "image/jpeg"
// when the data URL declares no media type.
func parseDataURLMediaType(header string) string {
	s := strings.ToLower(strings.TrimSpace(header))
	s = strings.TrimPrefix(s, "data:")
	if i := strings.IndexAny(s, ";,"); i >= 0 {
		s = s[:i]
	}
	if s = strings.TrimSpace(s); s != "" {
		return s
	}
	return "image/jpeg"
}

// NormalizeImageBase64 returns just the raw base64 portion of an image input.
// Convenience wrapper around NormalizeImage for callers that don't need the
// media type.
func NormalizeImageBase64(image string) string {
	b, _ := NormalizeImage(image)
	return b
}

// ExtractBase64Image removes the data URL prefix if present. Retained for
// compatibility; it now also handles url()-wrapped values via NormalizeImage.
func ExtractBase64Image(data string) string {
	return NormalizeImageBase64(data)
}

// EncodeFrameToBase64 encodes raw JPEG bytes to base64.
func EncodeFrameToBase64(frame []byte) string {
	return base64.StdEncoding.EncodeToString(frame)
}
