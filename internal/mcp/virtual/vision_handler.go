package virtual

import (
	"bytes"
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
		Image  string `json:"image"`
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Image == "" {
		return nil, fmt.Errorf("image parameter is required")
	}

	// Decode the input into real image bytes first. If it isn't a valid image,
	// fail here — never send bad data to the upstream model. This is the same
	// form the camera path feeds Analyze: raw bytes that get re-encoded below.
	imgBytes, mediaType, err := DecodeImage(params.Image)
	if err != nil {
		return nil, fmt.Errorf("invalid image: %w", err)
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
			systemPrompt = "You are a precise image analysis assistant. Examine the provided image and identify the objects, text, and scenes it contains. Be accurate, objective, and thorough."
		}
		if params.Prompt != "" {
			userPrompt = params.Prompt
		} else {
			userPrompt = "Analyze this image in detail. Identify and describe every object, transcribe any visible text, and explain the scenes depicted. Return a structured breakdown of all recognized elements."
		}
	case strings.HasSuffix(toolName, "describe_scene") || toolName == "vision.describe_scene":
		systemPrompt = vc.SystemPrompt
		if systemPrompt == "" {
			systemPrompt = "You are a scene description assistant. Describe the overall content and context of the provided image in clear, natural language."
		}
		if params.Prompt != "" {
			userPrompt = params.Prompt
		} else {
			userPrompt = "Describe the scene and overall content of this image in natural language. Summarize what is happening, including the setting, subjects, and their actions."
		}
	default:
		return nil, fmt.Errorf("unknown vision tool: %s", toolName)
	}

	// Re-encode to clean standard base64, exactly like the camera path, so both
	// feeds hand Analyze the same well-formed data URL.
	imageBase64 := EncodeFrameToBase64(imgBytes)
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

// DecodeImage decodes an image input into its raw bytes and detected media
// type — the same form the camera path feeds Analyze. It accepts:
//   - raw base64 (standard or URL-safe, padded or not)
//   - a data URL: "data:<mediatype>;base64,<base64>"
//   - a CSS url() wrapper around either of the above
//
// The decoded bytes are authoritative for the media type: a wrong or missing
// label is overridden by sniffing the real format from the magic bytes. An
// error is returned when the input is not valid base64 or not a recognized
// image, so callers fail fast rather than burning an upstream request.
func DecodeImage(image string) (imgBytes []byte, mediaType string, err error) {
	b64 := extractBase64Text(image)
	if b64 == "" {
		return nil, "", fmt.Errorf("image data is empty")
	}

	imgBytes, err = decodeBase64Loose(b64)
	if err != nil {
		return nil, "", err
	}
	if len(imgBytes) == 0 {
		return nil, "", fmt.Errorf("decoded image is empty")
	}

	mediaType = sniffMediaType(imgBytes)
	if mediaType == "" {
		return nil, "", fmt.Errorf("unsupported or unrecognized image format")
	}
	return imgBytes, mediaType, nil
}

// extractBase64Text reduces any supported image input to its bare base64 text
// by stripping a CSS url() wrapper, the data: URL header, and any whitespace.
func extractBase64Text(image string) string {
	s := strings.TrimSpace(image)

	// Unwrap a leading CSS url(...) wrapper, e.g. url("data:...") or url(data:...).
	if strings.HasPrefix(strings.ToLower(s), "url(") {
		s = strings.TrimSpace(s[4:])
		s = strings.TrimSuffix(s, ")")
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "\"'")
		s = strings.TrimSpace(s)
	}

	// Strip the data URL header. Base64 data never contains a comma, so the
	// comma reliably marks the end of the header.
	if strings.HasPrefix(strings.ToLower(s), "data:") {
		if idx := strings.Index(s, ","); idx >= 0 {
			s = s[idx+1:]
		}
	}

	return stripBase64Whitespace(s)
}

// decodeBase64Loose decodes base64 that may be standard or URL-safe, padded or
// not. Different sources produce different variants; trying each in turn is
// cheap and maximizes what we accept.
func decodeBase64Loose(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("not valid base64")
}

// sniffMediaType identifies a common image format from its magic bytes. The
// bytes are authoritative — more reliable than any label the sender attached.
func sniffMediaType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}): // \x89PNG
		return "image/png"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "image/gif"
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}

// stripBase64Whitespace removes spaces, tabs, and newlines from a base64
// string. Base64 only contains [A-Za-z0-9+/=] (and -_= for URL-safe), so
// whitespace is never significant and only breaks the data URL we build.
func stripBase64Whitespace(s string) string {
	if !strings.ContainsAny(s, " \t\r\n") {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		default:
			return r
		}
	}, s)
}

// EncodeFrameToBase64 encodes raw image bytes to standard base64. Used by both
// the camera path (on captured frames) and the vision path (after decoding),
// so both feed Analyze the same clean form.
func EncodeFrameToBase64(frame []byte) string {
	return base64.StdEncoding.EncodeToString(frame)
}
