// Package imagegen implements synchronous image APIs without model-name allowlists.
package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

type Client struct {
	Provider, EndpointURL, APIKey, Model string
	EditRequestFormat                    string
	HTTPClient                           *http.Client
	MaxResponseBytes                     int64
}

const (
	ProviderOpenRouter                 = "openrouter"
	EditFormatMultipart                = "multipart"
	EditFormatGenerationsJSON          = "generations_json"
	EditFormatGenerationsJSONExtraBody = "generations_json_extra_body"
)

// An empty value preserves the standard edits API for existing configurations.
func ValidateEditRequestFormat(provider, format string) error {
	switch format {
	case "", EditFormatMultipart:
		return nil
	case EditFormatGenerationsJSON, EditFormatGenerationsJSONExtraBody:
		if provider == "openai" || provider == ProviderOpenRouter {
			return nil
		}
	}
	return fmt.Errorf("unsupported edit_request_format %q for %s", format, provider)
}

func SupportsMask(provider, format string) bool {
	return provider == "openai" && (format == "" || format == EditFormatMultipart)
}

type Image struct {
	Bytes                    []byte
	MIME, URL, RevisedPrompt string
}
type Result struct {
	Images []Image
	Text   string
	Usage  json.RawMessage
}
type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func DefaultEndpoint(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta"
	case ProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	default:
		return ""
	}
}

// BaseURL accepts root URLs, reverse-proxy prefixes and versioned roots.
func BaseURL(raw, provider string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = DefaultEndpoint(provider)
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("endpoint_url must be an http(s) API base URL without credentials or query")
	}
	if provider != "openai" && provider != "gemini" && provider != ProviderOpenRouter {
		return "", fmt.Errorf("unsupported image provider: %s", provider)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if provider == ProviderOpenRouter && strings.EqualFold(u.Hostname(), "openrouter.ai") && u.Path == "" {
		u.Path = "/api"
	}
	for _, version := range []string{"/v1beta", "/v1"} {
		if strings.HasSuffix(u.Path, version) {
			u.Path = strings.TrimSuffix(u.Path, version)
			break
		}
	}
	version := "/v1"
	if provider == "gemini" {
		version = "/v1beta"
	}
	u.Path += version
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func (c *Client) Endpoint(operation string) (string, error) {
	if err := ValidateEditRequestFormat(c.Provider, c.EditRequestFormat); err != nil {
		return "", err
	}
	base, err := BaseURL(c.EndpointURL, c.Provider)
	if err != nil {
		return "", err
	}
	if c.Provider == ProviderOpenRouter {
		return base + "/images", nil
	}
	if c.Provider == "gemini" {
		return base + "/models/" + url.PathEscape(strings.TrimPrefix(c.Model, "models/")) + ":generateContent", nil
	}
	if operation == "edit" && SupportsMask(c.Provider, c.EditRequestFormat) {
		return base + "/images/edits", nil
	}
	return base + "/images/generations", nil
}

// MergeParams deep-merges defaults and invocation parameters; protected input
// fields and streaming are always supplied by the adapter, never extra_body.
func MergeParams(defaults, args map[string]any) (map[string]any, error) {
	merged := clone(defaults)
	merge(merged, args)
	return merged, nil
}
func clone(m map[string]any) map[string]any { out := map[string]any{}; merge(out, m); return out }
func merge(dst, src map[string]any) {
	for k, v := range src {
		if child, ok := v.(map[string]any); ok {
			old, _ := dst[k].(map[string]any)
			copied := clone(old)
			merge(copied, child)
			dst[k] = copied
		} else {
			dst[k] = v
		}
	}
}
func ValidateParams(provider string, p map[string]any, defaults bool) error {
	allowed := map[string]bool{"n": true, "size": true, "quality": true, "background": true, "output_format": true, "aspect_ratio": true, "image_size": true, "extra_body": true}
	if provider == ProviderOpenRouter {
		allowed["resolution"] = true
	}
	if !defaults {
		for _, k := range []string{"prompt", "images", "mask", "return_mode"} {
			allowed[k] = true
		}
	}
	for k := range p {
		if !allowed[k] {
			return fmt.Errorf("unknown image parameter %q; use extra_body for upstream extensions", k)
		}
	}
	if extra, present := p["extra_body"]; present {
		obj, ok := extra.(map[string]any)
		if !ok {
			return fmt.Errorf("extra_body must be an object")
		}
		protected := map[string]bool{"model": true, "prompt": true, "image": true, "images": true, "mask": true, "stream": true, "partial_images": true, "contents": true, "return_mode": true, "api_key": true, "authorization": true, "input_references": true, "models": true, "messages": true}
		for k := range obj {
			if protected[strings.ToLower(k)] {
				return fmt.Errorf("extra_body cannot override %s", k)
			}
			if allowed[k] && k != "extra_body" {
				return fmt.Errorf("use the top-level %s parameter instead of extra_body", k)
			}
		}
		// Some compatible gateways require a literal nested extra_body on the wire.
		// It may carry extensions, but never a second source of fixed inputs.
		if nested, exists := obj["extra_body"]; exists {
			fields, ok := nested.(map[string]any)
			if !ok {
				return fmt.Errorf("extra_body.extra_body must be an object")
			}
			for k := range fields {
				if protected[strings.ToLower(k)] || strings.EqualFold(k, "extra_body") {
					return fmt.Errorf("extra_body.extra_body cannot override %s", k)
				}
			}
		}
		if value, exists := obj["generationConfig"]; exists {
			g, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("generationConfig must be an object")
			}
			for k := range g {
				if strings.EqualFold(strings.ReplaceAll(k, "_", ""), "responseModalities") {
					return fmt.Errorf("extra_body cannot override responseModalities")
				}
			}
		}
		if provider == "gemini" {
			if _, exists := obj["generation_config"]; exists {
				return fmt.Errorf("use generationConfig for Gemini extensions")
			}
		}
	}
	for _, k := range []string{"size", "quality", "background", "output_format", "aspect_ratio", "image_size", "resolution"} {
		if v, ok := p[k]; ok {
			if _, valid := v.(string); !valid {
				return fmt.Errorf("%s must be a string", k)
			}
		}
	}
	if n, ok := p["n"]; ok {
		value, valid := n.(float64)
		if !valid || value < 1 || value > 10 || value != float64(int(value)) {
			return fmt.Errorf("n must be an integer from 1 to 10")
		}
		if provider == "gemini" && value != 1 {
			return fmt.Errorf("Gemini supports one generation request; n must be 1")
		}
	}
	if provider == "gemini" {
		for _, k := range []string{"size", "quality", "background", "output_format"} {
			if _, ok := p[k]; ok {
				return fmt.Errorf("%s is an OpenAI parameter; use Gemini aspect_ratio/image_size or extra_body", k)
			}
		}
	} else if provider == ProviderOpenRouter {
		if _, ok := p["image_size"]; ok {
			return fmt.Errorf("image_size is a Gemini parameter; use OpenRouter resolution or size")
		}
		if format, _ := p["output_format"].(string); strings.EqualFold(strings.TrimSpace(format), "svg") {
			return fmt.Errorf("SVG output is not supported by image storage; request png, jpeg or webp")
		}
	} else {
		for _, k := range []string{"aspect_ratio", "image_size"} {
			if _, ok := p[k]; ok {
				return fmt.Errorf("%s is a Gemini parameter; use OpenAI size", k)
			}
		}
	}
	return nil
}

func (c *Client) Run(ctx context.Context, operation string, p map[string]any, images []Image, mask *Image) (*Result, error) {
	if err := ValidateParams(c.Provider, p, false); err != nil {
		return nil, err
	}
	prompt, _ := p["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	endpoint, err := c.Endpoint(operation)
	if err != nil {
		return nil, err
	}
	if mask != nil && c.Provider == "openai" && !SupportsMask(c.Provider, c.EditRequestFormat) {
		return nil, fmt.Errorf("mask is not supported by this editing interface; use the OpenAI multipart edits interface with a model that supports masks")
	}
	if mask != nil && c.Provider == ProviderOpenRouter {
		return nil, fmt.Errorf("OpenRouter Image API does not support mask; use reference images and a prompt")
	}
	if operation == "edit" && len(images) == 0 {
		return nil, fmt.Errorf("images are required for editing")
	}
	extra, _ := p["extra_body"].(map[string]any)
	body := clone(extra)
	var reader io.Reader
	contentType := "application/json"
	if c.Provider == "gemini" {
		if mask != nil {
			return nil, fmt.Errorf("Gemini does not support mask")
		}
		parts := []any{map[string]any{"text": prompt}}
		for _, img := range images {
			parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": img.MIME, "data": base64.StdEncoding.EncodeToString(img.Bytes)}})
		}
		body["contents"] = []any{map[string]any{"role": "user", "parts": parts}}
		gen, _ := body["generationConfig"].(map[string]any)
		gen = clone(gen)
		gen["responseModalities"] = []string{"TEXT", "IMAGE"}
		imageCfg, _ := gen["imageConfig"].(map[string]any)
		imageCfg = clone(imageCfg)
		for param, key := range map[string]string{"aspect_ratio": "aspectRatio", "image_size": "imageSize"} {
			if v, ok := p[param]; ok && v != "" {
				imageCfg[key] = v
			}
		}
		if len(imageCfg) > 0 {
			gen["imageConfig"] = imageCfg
		}
		body["generationConfig"] = gen
	} else {
		body["model"], body["prompt"] = c.Model, prompt
		for _, key := range []string{"n", "size", "quality", "background", "output_format"} {
			if v, ok := p[key]; ok && v != "" {
				body[key] = v
			}
		}
		if _, ok := body["n"]; !ok {
			body["n"] = 1
		}
		if c.Provider == ProviderOpenRouter {
			body["stream"] = false
			for _, key := range []string{"aspect_ratio", "resolution"} {
				if v, ok := p[key]; ok && v != "" {
					body[key] = v
				}
			}
			if operation == "edit" {
				refs := make([]any, 0, len(images))
				for _, img := range images {
					refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Bytes)}})
				}
				body["input_references"] = refs
			}
		}
	}
	if c.Provider == "openai" && operation == "edit" && !SupportsMask(c.Provider, c.EditRequestFormat) {
		inputs := make([]string, len(images))
		for i, img := range images {
			// Forward validated bytes, never signed localhost URLs or private links.
			inputs[i] = "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Bytes)
		}
		if c.EditRequestFormat == EditFormatGenerationsJSONExtraBody {
			nested, _ := body["extra_body"].(map[string]any)
			nested = clone(nested)
			nested["image"] = inputs
			body["extra_body"] = nested
		} else {
			body["image"] = inputs
		}
	}
	if c.Provider == "openai" && operation == "edit" && SupportsMask(c.Provider, c.EditRequestFormat) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		for key, val := range body {
			var text string
			if v, ok := val.(string); ok {
				text = v
			} else {
				data, e := json.Marshal(val)
				if e != nil {
					return nil, e
				}
				text = string(data)
			}
			if e := writer.WriteField(key, text); e != nil {
				return nil, e
			}
		}
		field := "image"
		if len(images) > 1 {
			field = "image[]"
		}
		for i, img := range images {
			if err := writeImage(writer, field, fmt.Sprintf("reference-%d", i), img); err != nil {
				return nil, err
			}
		}
		if mask != nil {
			if err := writeImage(writer, "mask", "mask", *mask); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		contentType = writer.FormDataContentType()
		reader = &buf
	} else {
		data, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		reader = bytes.NewReader(data)
	}
	raw, err := c.request(ctx, http.MethodPost, endpoint, reader, contentType)
	if err != nil {
		return nil, err
	}
	var result *Result
	if c.Provider == "gemini" {
		result, err = parseGemini(raw)
	} else {
		result, err = parseOpenAI(raw)
	}
	if err != nil {
		return nil, fmt.Errorf("%s", c.redact(err.Error()))
	}
	return result, nil
}
func writeImage(w *multipart.Writer, field, name string, img Image) error {
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[img.MIME]
	if ext == "" {
		return fmt.Errorf("unsupported reference image MIME: %s", img.MIME)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s.%s"`, field, name, ext))
	h.Set("Content-Type", img.MIME)
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = part.Write(img.Bytes)
	return err
}
func (c *Client) request(ctx context.Context, method, endpoint string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Provider == "gemini" {
		req.Header.Set("x-goog-api-key", c.APIKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image API request failed: %w", err)
	}
	defer resp.Body.Close()
	limit := c.MaxResponseBytes
	if limit <= 0 {
		limit = 150 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("image API response exceeds %d bytes", limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("image API HTTP %d: %s", resp.StatusCode, c.redact(errorMessage(raw)))
	}
	var failure struct {
		Error json.RawMessage `json:"error"`
	}
	_ = json.Unmarshal(raw, &failure)
	if len(failure.Error) > 0 && string(failure.Error) != "null" {
		return nil, fmt.Errorf("image API error: %s", c.redact(errorMessage(raw)))
	}
	return raw, nil
}
func (c *Client) redact(s string) string {
	if c.APIKey != "" {
		s = strings.ReplaceAll(s, c.APIKey, "[redacted]")
	}
	return s
}
func errorMessage(raw []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &e) == nil {
		if e.Error.Message != "" {
			return e.Error.Message
		}
		if e.Message != "" {
			return e.Message
		}
	}
	if len(raw) > 1024 {
		raw = raw[:1024]
	}
	return string(raw)
}
func parseOpenAI(raw []byte) (*Result, error) {
	var response struct {
		Data  json.RawMessage `json:"data"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("invalid OpenAI image response: %w", err)
	}
	var entries []struct {
		URL     string `json:"url"`
		Base64  string `json:"b64_json"`
		Revised string `json:"revised_prompt"`
		MIME    string `json:"media_type"`
	}
	data := bytes.TrimSpace(response.Data)
	if len(data) > 0 && data[0] == '{' {
		data = append(append([]byte{'['}, data...), ']')
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("invalid image data: %w", err)
	}
	result := &Result{Usage: response.Usage}
	for _, e := range entries {
		img := Image{URL: e.URL, RevisedPrompt: e.Revised, MIME: e.MIME}
		if e.Base64 != "" {
			b, err := base64.StdEncoding.DecodeString(e.Base64)
			if err != nil {
				return nil, fmt.Errorf("invalid output base64: %w", err)
			}
			img.Bytes = b
		}
		if len(img.Bytes) > 0 || img.URL != "" {
			result.Images = append(result.Images, img)
		}
	}
	if len(result.Images) == 0 {
		return nil, fmt.Errorf("image API returned no images")
	}
	return result, nil
}
func parseGemini(raw []byte) (*Result, error) {
	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text   string `json:"text"`
					Inline *struct {
						MIME string `json:"mimeType"`
						Data string `json:"data"`
					} `json:"inlineData"`
					Snake *struct {
						MIME string `json:"mime_type"`
						Data string `json:"data"`
					} `json:"inline_data"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		Usage    json.RawMessage `json:"usageMetadata"`
		Feedback json.RawMessage `json:"promptFeedback"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	result := &Result{Usage: response.Usage}
	var texts []string
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
			var b64, mime string
			if part.Inline != nil {
				b64, mime = part.Inline.Data, part.Inline.MIME
			} else if part.Snake != nil {
				b64, mime = part.Snake.Data, part.Snake.MIME
			}
			if b64 != "" && strings.HasPrefix(mime, "image/") {
				b, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return nil, fmt.Errorf("invalid Gemini image base64: %w", err)
				}
				result.Images = append(result.Images, Image{Bytes: b, MIME: mime})
			}
		}
	}
	result.Text = strings.Join(texts, "\n")
	if len(result.Images) == 0 {
		return nil, fmt.Errorf("Gemini returned no image: %s %s", result.Text, string(response.Feedback))
	}
	return result, nil
}
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	base, err := BaseURL(c.EndpointURL, c.Provider)
	if err != nil {
		return nil, err
	}
	path := "/models"
	if c.Provider == ProviderOpenRouter {
		path = "/images/models"
	}
	raw, err := c.request(ctx, http.MethodGet, base+path, nil, "")
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		Models []struct {
			Name    string `json:"name"`
			Display string `json:"displayName"`
		} `json:"models"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	out := []ModelInfo{}
	for _, m := range response.Data {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		out = append(out, ModelInfo{m.ID, name})
	}
	for _, m := range response.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		name := m.Display
		if name == "" {
			name = id
		}
		out = append(out, ModelInfo{id, name})
	}
	return out, nil
}
