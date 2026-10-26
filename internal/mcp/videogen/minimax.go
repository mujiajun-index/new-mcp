package videogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
)

var miniMaxTaskID = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,511}$`)

func miniMaxBaseURL(raw string) (string, error) {
	// Validate before manipulating the path so query strings, credentials and
	// fragments cannot be discarded accidentally. Preserve reverse proxy paths.
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("endpoint_url must be an http(s) API base URL without credentials or query")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	for _, version := range []string{"/v2", "/v1"} {
		if strings.HasSuffix(u.Path, version) {
			u.Path = strings.TrimSuffix(u.Path, version)
			break
		}
	}
	u.Path += "/v2"
	u.RawPath = ""
	return u.String(), nil
}

func validateMiniMaxParams(p map[string]any) error {
	if metadata, _ := p["metadata"].(map[string]any); len(metadata) != 0 {
		return fmt.Errorf("MiniMax video does not use metadata; put native generation options in extra_body")
	}
	if seconds, ok := p["seconds"].(float64); ok && seconds != float64(int(seconds)) {
		return fmt.Errorf("MiniMax video seconds must be an integer (sent as duration)")
	}
	extra, _ := p["extra_body"].(map[string]any)
	for key := range extra {
		canonical := strings.ToLower(strings.ReplaceAll(key, "_", ""))
		switch canonical {
		case "content", "model", "prompt", "image", "images", "imageurl", "videourl", "audiourl", "firstframeimage", "lastframeimage", "referenceimages", "referencevideos", "referenceaudios", "subjectreference", "inputreference", "inputreferences", "frameimages", "video", "taskid", "requestid", "waitseconds", "retrydownload", "endpointurl", "baseurl":
			return fmt.Errorf("MiniMax extra_body cannot override %s; only generation options are supported", key)
		}
	}
	if duration, exists := extra["duration"]; exists {
		if _, exists := p["seconds"]; exists {
			return fmt.Errorf("use either seconds or extra_body.duration, not both")
		}
		n, ok := duration.(float64)
		if !ok || n <= 0 || n > 3600 || n != float64(int(n)) {
			return fmt.Errorf("extra_body.duration must be an integer from 1 to 3600")
		}
	}
	if _, exists := extra["resolution"]; exists {
		if _, exists := p["size"]; exists {
			return fmt.Errorf("use either size or extra_body.resolution, not both")
		}
	}
	for _, key := range []string{"resolution", "ratio"} {
		if value, exists := extra[key]; exists {
			if s, ok := value.(string); !ok || strings.TrimSpace(s) == "" {
				return fmt.Errorf("extra_body.%s must be a non-empty string", key)
			}
		}
	}
	if size, exists := p["size"].(string); exists && strings.TrimSpace(size) == "" {
		return fmt.Errorf("MiniMax video size must be a non-empty resolution")
	}
	return nil
}

func (c *Client) buildMiniMaxSubmit(ctx context.Context, p map[string]any, img *imagegen.Image) (*http.Request, error) {
	base, err := BaseURLForProtocol(c.EndpointURL, ProtocolMiniMax)
	if err != nil {
		return nil, err
	}
	content := []any{map[string]any{"type": "text", "text": p["prompt"]}}
	if img != nil {
		// Use validated bytes, including for external URLs, to keep private URLs
		// and caller-local paths out of the native multimodal input.
		ref := "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Bytes)
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": ref}, "role": "first_frame"})
	}
	body := map[string]any{"model": c.Model, "content": content, "duration": 5, "resolution": "768P"}
	if img == nil {
		body["ratio"] = "16:9"
	} else {
		body["ratio"] = "adaptive"
	}
	if seconds, exists := p["seconds"]; exists {
		body["duration"] = int(seconds.(float64))
	}
	if size, exists := p["size"]; exists {
		body["resolution"] = size
	}
	if extra, ok := p["extra_body"].(map[string]any); ok {
		for key, value := range extra {
			body[key] = value
		}
	}
	var data bytes.Buffer
	if err := json.NewEncoder(&data).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/video_generation", &data)
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return req, err
}

type miniMaxTask struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Progress json.RawMessage `json:"progress"`
	Error    json.RawMessage `json:"error"`
	Content  struct {
		URL string `json:"url"`
	} `json:"content"`
}

func (task *miniMaxTask) video() *Video {
	return &Video{ID: task.ID, Status: task.Status, Progress: task.Progress, Error: task.Error}
}

func (c *Client) parseMiniMaxResponse(raw []byte) (*miniMaxTask, error) {
	var response struct {
		TaskID string          `json:"task_id"`
		Task   *miniMaxTask    `json:"task"`
		Type   string          `json:"type"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &response) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("invalid MiniMax video response")
	}
	if response.Type == "error" {
		// Some compatible deployments wrap an explicit rejection in HTTP 200.
		// Only trust a declared error status; otherwise submission stays unknown.
		var detail struct {
			HTTPCode json.RawMessage `json:"http_code"`
		}
		_ = json.Unmarshal(response.Error, &detail)
		code, _ := strconv.Atoi(strings.Trim(string(detail.HTTPCode), `"`))
		if code < 400 || code > 599 {
			code = http.StatusInternalServerError
		}
		return nil, &HTTPError{Status: code, Message: c.redact(responseError(raw))}
	}
	task := response.Task
	if task == nil {
		task = &miniMaxTask{ID: response.TaskID, Status: "queued"}
	}
	if !miniMaxTaskID.MatchString(task.ID) {
		return nil, fmt.Errorf("MiniMax video response has an invalid task ID")
	}
	return task, nil
}

func (c *Client) queryMiniMax(ctx context.Context, id string) (*miniMaxTask, error) {
	if !miniMaxTaskID.MatchString(id) {
		return nil, fmt.Errorf("invalid MiniMax video task ID")
	}
	base, err := BaseURLForProtocol(c.EndpointURL, ProtocolMiniMax)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/query/video_generation/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	raw, err := c.request(req)
	if err != nil {
		return nil, err
	}
	task, err := c.parseMiniMaxResponse(raw)
	if err != nil {
		return nil, err
	}
	if task.ID != id {
		return nil, fmt.Errorf("MiniMax video task ID does not match the task")
	}
	return task, nil
}

func (c *Client) miniMaxContent(ctx context.Context, id string, maxBytes int64) (*http.Response, error) {
	// Re-query the persisted ID to recover expiring CDN links after restarts or
	// save failures. Never submit another paid generation when saving a result.
	task, err := c.queryMiniMax(ctx, id)
	if err != nil {
		return nil, err
	}
	if NormalizeStatus(task.Status) != "completed" || task.Content.URL == "" {
		return nil, fmt.Errorf("MiniMax video content is unavailable")
	}
	base, _ := BaseURLForProtocol(c.EndpointURL, ProtocolMiniMax)
	resp, err := imagegen.OpenDownload(ctx, task.Content.URL, base+"/", maxBytes, "Bearer "+c.APIKey)
	if err != nil {
		return nil, fmt.Errorf("MiniMax video content: %s", c.redact(err.Error()))
	}
	return resp, nil
}
