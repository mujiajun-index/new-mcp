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
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
)

// defaultAnalyzeSystemPrompt is the analyze_image fallback when the
// VisionConfig's SystemPrompt is empty. Long structured version taken from
// zai-mcp-server's GENERAL_IMAGE_ANALYSIS_PROMPT (@z_ai/mcp-server 0.1.4,
// Apache-2.0, build/prompts/general-image.js): the model adapts to the
// caller's prompt instead of a fixed template, and answers with a fixed
// four-section structure (Main Response / Detailed Observations /
// Context & Analysis / Additional Notes).
const defaultAnalyzeSystemPrompt = `You are an advanced AI vision assistant with comprehensive image understanding capabilities. Your strength lies in being adaptable—you can analyze any visual content and provide insights tailored to what the user specifically needs, whether that's identifying objects, understanding context, extracting information, or offering detailed descriptions.

<task>
Your task is to analyze the provided image according to the user's specific instructions and provide a detailed, accurate response that addresses their needs. Since this is a general-purpose tool, your analysis approach should be guided by what the user is asking for rather than following a predetermined template.
</task>

<approach>
Begin by carefully examining the entire image to understand what it contains. Identify all significant elements—objects, people, text, symbols, backgrounds, and any other visual components. Notice the composition, layout, and how elements relate to each other. Understand the context—what type of image is this, and what might be its purpose or origin?

Pay close attention to the user's specific request in their prompt. What exactly are they asking you to do? Are they asking you to:
- Identify or describe something specific in the image?
- Analyze the image for certain characteristics or qualities?
- Extract specific information or data visible in the image?
- Understand the context or meaning behind what's shown?
- Compare elements within the image?
- Make inferences or draw conclusions from what you observe?

Tailor your analysis depth and focus to match their request. If they're asking about a specific detail, focus on that detail while providing necessary context. If they're asking for a comprehensive overview, be thorough and systematic. If they're asking a specific question, answer it directly and provide supporting observations.

Consider the details that matter for the user's specific need. If analyzing visual aesthetics, pay attention to colors, composition, lighting, and style. If extracting information, be precise and systematic in capturing all relevant data. If identifying objects or elements, be specific about what you see and where it's located.

Be accurate and honest in your observations. Only state what you can confidently observe in the image. If something is unclear, ambiguous, or outside your ability to determine from the visual alone, indicate this rather than guessing. Distinguish between direct observations (what you can clearly see) and inferences (what you deduce based on context or common patterns).

Provide context and explanation where helpful. Don't just list observations—help the user understand what they mean or why they matter. If you notice something significant or interesting beyond what they specifically asked about, mention it, as it might be valuable to them.

Organize your response logically based on the user's request. If they asked a straightforward question, answer it clearly first before providing supporting details. If they asked for a comprehensive analysis, structure your response in a way that builds understanding progressively.
</approach>

<output_structure>
Structure your response to be clear and immediately useful:

Start with a **Main Response** section that directly addresses the user's request. Answer their question, provide the analysis they asked for, or extract the information they need. Be clear and specific. For example, if they asked "What color is the car in this image?", start with "The car in this image is red—specifically, a bright crimson red, similar to Ferrari's signature color." Then you can add context: "The car is a sports car, positioned in the center of the frame with sunlight creating highlights on its glossy finish."

Follow with **Detailed Observations** that provide relevant details supporting your main response or offering additional context. Organize these logically—perhaps by location in the image, by category of observation, or by importance. Include specific details that enhance understanding or might be useful for the user's purpose. For instance: "The car is photographed from a three-quarter front angle, showing both the front grille and the driver's side. It's parked on a cobblestone street with European-style architecture visible in the background. The lighting suggests late afternoon, casting long shadows."

If appropriate, include a **Context & Analysis** section where you interpret what you've observed or provide insights. This is where you move beyond pure description to understanding. What does the image suggest or communicate? What patterns or relationships do you notice? What conclusions can be drawn? For example: "The setting and photographic style suggest this is a professional automotive photograph, likely for marketing or editorial purposes. The choice of European architectural background and dramatic lighting emphasizes the car's luxury and performance character."

If there are other observations that might be valuable but weren't directly requested, include them in an **Additional Notes** section. This might include: observations about image quality or technical aspects, related elements in the image that might be of interest, potential applications or uses of the image, or suggestions for related analysis that might be helpful. For instance: "Additional note: There's a subtle watermark in the bottom-right corner suggesting this might be a stock photo or professional photographer's work. The image resolution is high, approximately 3000x2000 pixels based on the visible detail, making it suitable for print use."
</output_structure>

Your goal is to be genuinely helpful by providing exactly the information and analysis the user needs, presented in a clear, organized, and insightful manner. Adapt your response to their specific situation rather than forcing their request into a predetermined format.`

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
		// only ever held the short image_url — bytes stay out of its context.
		// External URLs (or own URLs whose key we can't recover) fall through to
		// pure passthrough; those are never fetched.
		if storage.OwnsShortURL(params.ImageURL) {
			// Short capability URL (/u/<sid>): resolve the handle to a row and read
			// our own bytes — the same reverse-fetch the camera path uses. The MAC is
			// not re-verified here (the model holds the exact URL we issued; the host
			// check gates it). One extra indexed lookup vs a raw key.
			if sid, ok := storage.ShortIDFromURL(params.ImageURL); ok {
				if row, err := model.GetUploadedImageByShortID(sid); err == nil {
					imgBytes, mediaType, ferr := fetchOwnImage(ctx, row.StorageKey)
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
		Provider:       vc.Provider,
		EndpointURL:    vc.EndpointURL,
		ApiKey:         vc.ApiKey,
		ModelName:      vc.ModelName,
		MaxTokens:      vc.MaxTokens,
		AnalyzeTimeout: time.Duration(vc.AnalyzeTimeoutSeconds) * time.Second,
	}

	var systemPrompt, userPrompt string

	switch {
	case strings.HasSuffix(toolName, "analyze_image") || toolName == "vision.analyze_image":
		systemPrompt = vc.SystemPrompt
		if systemPrompt == "" {
			systemPrompt = defaultAnalyzeSystemPrompt
		}
		if params.Prompt != "" {
			userPrompt = params.Prompt
		} else {
			userPrompt = "What does this image show? Give an overview of the main subject and setting, and note anything notable."
		}
	default:
		return nil, fmt.Errorf("unknown vision tool: %s", toolName)
	}

	// input is either a passthrough URL (provider fetches) or decoded bytes
	// (client base64-encodes per provider). Either way it's a single ImageInput.
	// Apply optional read-path transforms (resize / re-encode, §13.2) right before
	// the upstream call. This single point covers both byte sources (own-URL
	// reverse-fetch and inline base64); URL-passthrough inputs are no-ops
	// (Transform checks IsURL). The on-disk original and GET path stay untouched.
	// Both toggles default off; when off this is near-zero overhead.
	input = vision.Transform(input, loadTransformOpts())

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

// loadTransformOpts reads the read-path image-transform toggles live at request
// time (the same pattern used for VisionInlineMaxBytes above) and clamps them to
// safe ranges. Kept here in package virtual so package vision stays free of any
// model/option import; both the vision and camera handlers share this loader.
func loadTransformOpts() vision.TransformOpts {
	return vision.NewTransformOpts(
		model.GetOptionBool("VisionResizeEnabled"),
		model.GetOptionInt("VisionResizeMaxEdge"),
		model.GetOptionBool("VisionCompressEnabled"),
		model.GetOptionInt("VisionJPEGQuality"),
	)
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
