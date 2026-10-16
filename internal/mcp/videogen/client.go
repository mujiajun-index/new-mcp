// Package videogen implements asynchronous OpenAI and OpenRouter video protocols.
package videogen

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
	"strconv"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
)

type Connection struct {
	Protocol    string `json:"protocol,omitempty"`
	EndpointURL string `json:"endpoint_url"`
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
}

type Client struct {
	Connection
	HTTPClient *http.Client
}

type HTTPError struct {
	Status     int
	RetryAfter time.Duration
	Message    string
	Code       string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("video API HTTP %d: %s", e.Status, e.Message) }

const videoQueueFullCode = "video_queue_full"

func (e *HTTPError) submissionRejected() bool {
	// Other 5xx responses can occur after a task was accepted. Only this
	// explicit gateway queue rejection confirms that no generation was queued.
	if e.Status >= 400 && e.Status < 500 {
		return true
	}
	if e.Status != http.StatusServiceUnavailable {
		return false
	}
	// New API can wrap the upstream JSON body in error.message/error rather
	// than exposing its code. Parse that complete body, never match prose.
	return e.Code == videoQueueFullCode || responseErrorCode([]byte(e.Message)) == videoQueueFullCode
}

type Video struct {
	ID       string          `json:"id"`
	TaskID   string          `json:"task_id"`
	Status   string          `json:"status"`
	Progress json.RawMessage `json:"progress"`
	Error    json.RawMessage `json:"error"`
}

const (
	ProtocolOpenAI            = "openai_video"
	ProtocolOpenRouter        = "openrouter_video"
	DefaultOpenAIEndpoint     = "https://api.openai.com/v1"
	DefaultOpenRouterEndpoint = "https://openrouter.ai/api/v1"
)

// Empty snapshots and configurations created before protocol selection retain
// the original gateway protocol.
func ProtocolName(protocol string) string {
	if protocol == "" {
		return ProtocolOpenAI
	}
	return protocol
}

func BaseURL(raw string) (string, error) { return BaseURLForProtocol(raw, ProtocolOpenAI) }

func DefaultEndpoint(protocol string) string {
	switch ProtocolName(protocol) {
	case ProtocolOpenAI:
		return DefaultOpenAIEndpoint
	case ProtocolOpenRouter:
		return DefaultOpenRouterEndpoint
	default:
		return ""
	}
}

func BaseURLForProtocol(raw, protocol string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = DefaultEndpoint(protocol)
	}
	switch ProtocolName(protocol) {
	case ProtocolOpenAI:
	case ProtocolOpenRouter:
		u, err := url.Parse(strings.TrimSpace(raw))
		if err == nil && strings.EqualFold(u.Hostname(), "openrouter.ai") && strings.Trim(u.Path, "/") == "" {
			u.Path = "/api"
			raw = u.String()
		}
	default:
		return "", fmt.Errorf("protocol must be openai_video or openrouter_video")
	}
	return imagegen.BaseURL(raw, "openai")
}

func ValidateProtocolParams(protocol string, p map[string]any) error {
	switch ProtocolName(protocol) {
	case ProtocolOpenAI:
		return nil
	case ProtocolOpenRouter:
		if metadata, _ := p["metadata"].(map[string]any); len(metadata) != 0 {
			return fmt.Errorf("OpenRouter video does not use metadata; use extra_body for resolution, aspect_ratio or provider options")
		}
		if seconds, ok := p["seconds"].(float64); ok && seconds != float64(int(seconds)) {
			return fmt.Errorf("OpenRouter video seconds must be an integer (sent as duration)")
		}
		if extra, _ := p["extra_body"].(map[string]any); extra != nil {
			if duration, exists := extra["duration"]; exists {
				if _, exists := p["seconds"]; exists {
					return fmt.Errorf("use either seconds or extra_body.duration, not both")
				}
				n, ok := duration.(float64)
				if !ok || n <= 0 || n > 3600 || n != float64(int(n)) {
					return fmt.Errorf("extra_body.duration must be an integer from 1 to 3600")
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("protocol must be openai_video or openrouter_video")
	}
}

func WaitSeconds(p map[string]any, fallback int) (int, error) {
	v, exists := p["wait_seconds"]
	if !exists {
		return fallback, nil
	}
	n, ok := v.(float64)
	if !ok || n < 0 || n > 30 || n != float64(int(n)) {
		return 0, fmt.Errorf("wait_seconds must be an integer from 0 to 30")
	}
	return int(n), nil
}

func ValidateParams(p map[string]any, defaults bool) error {
	allowed := map[string]bool{"seconds": true, "size": true, "metadata": true, "extra_body": true}
	if !defaults {
		for _, k := range []string{"prompt", "image", "wait_seconds", "request_id"} {
			allowed[k] = true
		}
	}
	for k := range p {
		if !allowed[k] {
			return fmt.Errorf("unknown video parameter %q; use metadata or extra_body for extensions", k)
		}
	}
	for _, k := range []string{"prompt", "image", "size", "request_id"} {
		if v, ok := p[k]; ok {
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s must be a string", k)
			}
		}
	}
	if v, ok := p["seconds"]; ok {
		n, valid := v.(float64)
		if !valid || n <= 0 || n > 3600 {
			return fmt.Errorf("seconds must be greater than 0 and at most 3600")
		}
	}
	if _, err := WaitSeconds(p, 0); err != nil {
		return err
	}
	if v, ok := p["metadata"]; ok {
		if m, valid := v.(map[string]any); !valid || m == nil {
			return fmt.Errorf("metadata must be an object")
		}
	}
	if v, ok := p["extra_body"]; ok {
		m, valid := v.(map[string]any)
		if !valid || m == nil {
			return fmt.Errorf("extra_body must be an object")
		}
		for k := range m {
			name := strings.ToLower(k)
			if name == "previous_job_id" {
				return fmt.Errorf("extra_body.previous_job_id is not supported; video editing and extension are not enabled")
			}
			if allowed[name] || name == "prompt" || name == "image" || name == "wait_seconds" || name == "request_id" || strings.Contains(name, "authorization") || strings.ReplaceAll(name, "_", "") == "apikey" || name == "headers" || name == "model" || name == "input_reference" || name == "input_references" || name == "frame_images" || name == "images" || name == "image_url" || name == "stream" || name == "extra_body" || name == "return_mode" {
				return fmt.Errorf("extra_body cannot override %s", k)
			}
		}
	}
	if id, _ := p["request_id"].(string); len(id) > 128 {
		return fmt.Errorf("request_id exceeds 128 bytes")
	}
	return nil
}

func (c *Client) BuildSubmit(ctx context.Context, p map[string]any, img *imagegen.Image, format string) (*http.Request, error) {
	if err := ValidateParams(p, false); err != nil {
		return nil, err
	}
	if err := ValidateProtocolParams(c.Protocol, p); err != nil {
		return nil, err
	}
	base, err := BaseURLForProtocol(c.EndpointURL, c.Protocol)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"model": c.Model, "prompt": p["prompt"]}
	openRouter := ProtocolName(c.Protocol) == ProtocolOpenRouter
	for _, k := range []string{"seconds", "size", "metadata"} {
		if v, ok := p[k]; ok {
			if openRouter && k == "seconds" {
				body["duration"] = int(v.(float64))
			} else if !openRouter || k != "metadata" {
				body[k] = v
			}
		}
	}
	if extra, ok := p["extra_body"].(map[string]any); ok {
		for k, v := range extra {
			body[k] = v
		}
	}
	var data bytes.Buffer
	contentType := "application/json"
	if img != nil && format == "multipart" && !openRouter {
		writer := multipart.NewWriter(&data)
		for k, v := range body {
			var value string
			if k == "metadata" {
				b, e := json.Marshal(v)
				if e != nil {
					return nil, e
				}
				value = string(b)
			} else {
				switch v.(type) {
				case map[string]any, []any:
					return nil, fmt.Errorf("complex extension %s requires JSON image_request_format", k)
				}
				value = fmt.Sprint(v)
			}
			if err := writer.WriteField(k, value); err != nil {
				return nil, err
			}
		}
		header := textproto.MIMEHeader{}
		ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[img.MIME]
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="input_reference"; filename="reference.%s"`, ext))
		header.Set("Content-Type", img.MIME)
		part, e := writer.CreatePart(header)
		if e != nil {
			return nil, e
		}
		if _, e = part.Write(img.Bytes); e != nil {
			return nil, e
		}
		if e = writer.Close(); e != nil {
			return nil, e
		}
		contentType = writer.FormDataContentType()
	} else {
		if img != nil {
			reference := "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Bytes)
			if img.URL != "" {
				reference = img.URL
			}
			if openRouter {
				body["frame_images"] = []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": reference}, "frame_type": "first_frame"}}
			} else {
				body["input_reference"] = reference
			}
		}
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/videos", &data)
	if err == nil {
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return req, err
}

func (c *Client) request(req *http.Request) ([]byte, error) {
	hc := &http.Client{}
	if c.HTTPClient != nil {
		copied := *c.HTTPClient
		hc = &copied
	}
	// JSON protocol responses never need a cross-origin redirect.
	hc.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || r.URL.Scheme != req.URL.Scheme || !strings.EqualFold(r.URL.Host, req.URL.Host) {
			return fmt.Errorf("video API redirect changed origin")
		}
		return nil
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video API request failed: %s", c.redact(err.Error()))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, fmt.Errorf("video API response exceeds 4 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry := time.Duration(0)
		if n, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && n > 0 {
			retry = time.Duration(min(n, 3600)) * time.Second
		} else if date, e := http.ParseTime(resp.Header.Get("Retry-After")); e == nil {
			retry = max(0, time.Until(date))
		}
		return nil, &HTTPError{Status: resp.StatusCode, RetryAfter: retry, Message: c.redact(responseError(raw)), Code: c.redact(responseErrorCode(raw))}
	}
	return raw, nil
}

func (c *Client) do(req *http.Request) (*Video, error) {
	raw, err := c.request(req)
	if err != nil {
		return nil, err
	}
	var v Video
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("invalid video API response")
	}
	if v.ID == "" {
		v.ID = v.TaskID
	}
	if len(v.ID) > 512 {
		return nil, fmt.Errorf("upstream task ID exceeds 512 bytes")
	}
	return &v, nil
}

func (c *Client) Submit(req *http.Request) (*Video, error) {
	v, err := c.do(req)
	if err == nil && v.ID == "" {
		return nil, fmt.Errorf("video API response has no task ID")
	}
	return v, err
}

func (c *Client) Query(ctx context.Context, id string) (*Video, error) {
	base, err := BaseURLForProtocol(c.EndpointURL, c.Protocol)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/videos/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	return c.do(req)
}

func (c *Client) Content(ctx context.Context, id string, maxBytes int64) (*http.Response, error) {
	base, err := BaseURLForProtocol(c.EndpointURL, c.Protocol)
	if err != nil {
		return nil, err
	}
	contentURL := base + "/videos/" + url.PathEscape(id) + "/content"
	if ProtocolName(c.Protocol) == ProtocolOpenRouter {
		contentURL += "?index=0"
	}
	resp, err := imagegen.OpenDownload(ctx, contentURL, base, maxBytes, "Bearer "+c.APIKey)
	if err != nil {
		return nil, fmt.Errorf("video content: %s", strings.ReplaceAll(err.Error(), "image", "media"))
	}
	return resp, nil
}

func (c *Client) ListModels(ctx context.Context) ([]imagegen.ModelInfo, error) {
	base, err := BaseURLForProtocol(c.EndpointURL, c.Protocol)
	if err != nil {
		return nil, err
	}
	path := "/models"
	if ProtocolName(c.Protocol) == ProtocolOpenRouter {
		path = "/videos/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	raw, err := c.request(req)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []imagegen.ModelInfo `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Data == nil {
		return nil, fmt.Errorf("invalid video model catalog response")
	}
	for i := range response.Data {
		if response.Data[i].Name == "" {
			response.Data[i].Name = response.Data[i].ID
		}
	}
	return response.Data, nil
}

func (c *Client) redact(s string) string {
	if c.APIKey != "" {
		s = strings.ReplaceAll(s, c.APIKey, "[redacted]")
	}
	if len(s) > 4096 {
		s = strings.ToValidUTF8(s[:4096], "") + "…"
	}
	return s
}

func responseErrorCode(raw []byte) string {
	var v struct {
		Code  string          `json:"code"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	if v.Code != "" {
		return v.Code
	}
	var nested struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(v.Error, &nested) != nil {
		return ""
	}
	return nested.Code
}

func responseError(raw []byte) string {
	var v struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(raw, &v) == nil {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(v.Error, &e) == nil && e.Message != "" {
			return e.Message
		}
		var msg string
		if json.Unmarshal(v.Error, &msg) == nil && msg != "" {
			return msg
		}
		if v.Message != "" {
			return v.Message
		}
	}
	return string(raw[:min(len(raw), 1024)])
}

func (v *Video) ProgressValue() int {
	var n float64
	if json.Unmarshal(v.Progress, &n) != nil {
		var s string
		_ = json.Unmarshal(v.Progress, &s)
		n, _ = strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	}
	return min(100, max(0, int(n)))
}

func NormalizeStatus(s string) string {
	switch strings.ToLower(s) {
	case "queued", "pending", "not_start", "submitted":
		return "queued"
	case "in_progress", "processing", "running":
		return "in_progress"
	case "completed", "succeeded", "success":
		return "completed"
	case "failed", "cancelled", "canceled", "failure", "expired":
		return "failed"
	default:
		return "unknown"
	}
}
