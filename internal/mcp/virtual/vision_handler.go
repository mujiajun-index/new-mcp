package virtual

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/internal/storage"
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

	// Built-in upload_image tool: dispatched through this same handler (per-config,
	// like analyze_image) rather than as a global tool. It short-circuits before the
	// image/image_url param parsing below because its argument is local_path. The
	// caller's userID (for upload ownership + quota) is injected into ctx by the
	// gateway at the virtual dispatch sites; fall back to the config owner if absent.
	if strings.HasSuffix(toolName, "upload_image") || toolName == UploadImageToolName {
		uid := CallerUserID(ctx)
		if uid == 0 {
			uid = vc.UserID
		}
		return handleUploadImage(ctx, uid, args)
	}

	var params struct {
		Image    string `json:"image"`
		ImageURL string `json:"image_url"`
		Prompt   string `json:"prompt"`
	}
	_ = json.Unmarshal(args, &params)

	// Two transports, mutually exclusive. image_url is preferred: the bytes
	// bypass the calling LLM's context entirely (upload once, pass the signed
	// URL) and the upstream model fetches it — new-mcp never downloads it, so
	// there is no SSRF surface. image (base64) is kept for back-compat, now
	// capped by VisionUploadMaxBytes after decode.
	var input vision.ImageInput
	switch {
	case params.ImageURL != "":
		u, err := url.Parse(params.ImageURL)
		// Both http and https are accepted. For external URLs new-mcp never
		// fetches — it is pure passthrough to the upstream provider, so there is
		// no SSRF surface regardless of scheme. Allowing http also covers local
		// / non-TLS deployments where ServerAddress is http. Own-storage URLs
		// (below) are read off local disk / the bucket, also not a fetch of an
		// external host.
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, fmt.Errorf("image_url must be an http(s) URL")
		}
		// An own-storage URL is reverse-fetched here and forwarded as base64 to
		// the upstream — the camera path, not URL passthrough. This matters for
		// local deployments: the upstream cannot reach ServerAddress (localhost
		// or otherwise private), and Gemini's native file_uri fetch is flaky for
		// arbitrary URLs. Reading our own bytes has no SSRF surface (OwnsURL
		// already verified the URL is one we issued), and the calling LLM still
		// only ever held the short file_url — bytes stay out of its context.
		// External URLs (or own URLs whose key we can't recover) fall through to
		// pure passthrough; those are never fetched.
		if UploadStore != nil && UploadStore.OwnsURL(params.ImageURL) {
			if key, ok := UploadStore.KeyFromURL(params.ImageURL); ok {
				imgBytes, mediaType, ferr := fetchOwnImage(ctx, key)
				if ferr != nil {
					return nil, ferr
				}
				input.Bytes, input.MediaType = imgBytes, mediaType
			} else {
				input.URL = params.ImageURL
			}
		} else {
			input.URL = params.ImageURL
		}
	case params.Image != "":
		imgBytes, mediaType, err := DecodeImage(params.Image)
		if err != nil {
			return nil, fmt.Errorf("invalid image: %w", err)
		}
		// V1.1 soft cap: a large base64 bloats the calling LLM's context (~400
		// token/KB, generated as output). Guide it to the upload path instead.
		// VisionInlineMaxBytes=0 disables this (back to V1.0 behavior).
		if inline := model.GetOptionInt64("VisionInlineMaxBytes"); inline > 0 && int64(len(imgBytes)) > inline {
			return nil, fmt.Errorf("image is %d bytes, above the %d-byte inline threshold (VisionInlineMaxBytes); use vision.upload_image → curl → image_url instead (same result, far less context); set VisionInlineMaxBytes=0 to allow inlining", len(imgBytes), inline)
		}
		if max := model.GetOptionInt64("VisionUploadMaxBytes"); max > 0 && int64(len(imgBytes)) > max {
			return nil, fmt.Errorf("image exceeds the %d-byte limit", max)
		}
		input.Bytes, input.MediaType = imgBytes, mediaType
	default:
		return nil, fmt.Errorf("either image or image_url is required")
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

	// input is either a passthrough URL (provider fetches) or decoded bytes
	// (client base64-encodes per provider). Either way it's a single ImageInput.
	result, err := client.Analyze(ctx, systemPrompt, userPrompt, input)
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

	mediaType = vision.SniffMediaType(imgBytes)
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

// fetchOwnImage reads an own-storage object by key, enforces the hard size cap
// (VisionUploadMaxBytes) before materializing it, and returns its bytes + sniffed
// media type — the same shape DecodeImage and the camera path produce. Called
// only for URLs OwnsURL/KeyFromURL confirmed belong to this backend, so there is
// no SSRF surface: we read our own object, never an arbitrary external URL.
func fetchOwnImage(ctx context.Context, key string) ([]byte, string, error) {
	oi, err := UploadStore.Stat(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, "", fmt.Errorf("image_url refers to an upload that was never received — run the upload_command from vision.upload_image first")
		}
		return nil, "", fmt.Errorf("read uploaded image: %w", err)
	}
	// Size-check before Get: reject oversized uploads without streaming them in.
	// VisionInlineMaxBytes (the calling-LLM-context soft cap) does NOT apply here:
	// these bytes go new-mcp→upstream, never into the calling LLM's context.
	if max := model.GetOptionInt64("VisionUploadMaxBytes"); max > 0 && oi.Size > max {
		return nil, "", fmt.Errorf("uploaded image (%d bytes) exceeds the %d-byte limit", oi.Size, max)
	}
	rc, err := UploadStore.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, "", fmt.Errorf("image_url refers to an upload that was never received — run the upload_command from vision.upload_image first")
		}
		return nil, "", fmt.Errorf("open uploaded image: %w", err)
	}
	defer rc.Close()
	imgBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", fmt.Errorf("read uploaded image bytes: %w", err)
	}
	mediaType := vision.SniffMediaType(imgBytes)
	if mediaType == "" {
		return nil, "", fmt.Errorf("uploaded image is not a recognized image format")
	}
	return imgBytes, mediaType, nil
}
