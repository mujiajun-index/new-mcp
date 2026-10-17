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
	"strings"

	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
)

var geminiResourceSegment = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

func geminiModelName(model string) (string, error) {
	name := strings.TrimPrefix(strings.TrimSpace(model), "models/")
	if len(name) > 128 || !geminiResourceSegment.MatchString(name) {
		return "", fmt.Errorf("Gemini video model must be a model ID or models/{model}, without a URL or path")
	}
	return name, nil
}

// Operation names are resource paths, never arbitrary URLs or escaped paths.
// Keep their separators when polling; PathEscape on the whole name is invalid.
func validGeminiOperation(name string) bool {
	if len(name) > 512 {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) == 4 && parts[0] == "models" && geminiResourceSegment.MatchString(parts[1]) {
		parts = parts[2:]
	}
	return len(parts) == 2 && parts[0] == "operations" && geminiResourceSegment.MatchString(parts[1])
}

func validateGeminiParams(p map[string]any) error {
	if metadata, _ := p["metadata"].(map[string]any); len(metadata) != 0 {
		return fmt.Errorf("Gemini video does not use metadata; put native parameters such as aspectRatio and resolution in extra_body")
	}
	if seconds, ok := p["seconds"].(float64); ok && seconds != float64(int(seconds)) {
		return fmt.Errorf("Gemini video seconds must be an integer (sent as durationSeconds)")
	}
	extra, _ := p["extra_body"].(map[string]any)
	for key, value := range extra {
		canonical := strings.ToLower(strings.ReplaceAll(key, "_", ""))
		switch canonical {
		case "samplecount", "numberofvideos":
			if value != float64(1) {
				return fmt.Errorf("Gemini video supports one saved video per task; %s must be 1", key)
			}
		case "instances", "parameters", "prompt", "model", "image", "video", "lastframe", "referenceimages", "inputreferences", "frameimages", "storageuri", "outputgcsuri", "gcsuri", "httpoptions", "config":
			return fmt.Errorf("Gemini extra_body cannot override %s; only generation parameters are supported", key)
		}
	}
	if duration, ok := extra["durationSeconds"]; ok {
		if _, exists := p["seconds"]; exists {
			return fmt.Errorf("use either seconds or extra_body.durationSeconds, not both")
		}
		n, valid := duration.(float64)
		if !valid || n <= 0 || n > 3600 || n != float64(int(n)) {
			return fmt.Errorf("extra_body.durationSeconds must be an integer from 1 to 3600")
		}
	}
	if _, exists := extra["resolution"]; exists {
		if _, exists := p["size"]; exists {
			return fmt.Errorf("use either size or extra_body.resolution, not both")
		}
	}
	return nil
}

func (c *Client) geminiRequest(ctx context.Context, method, target string, body *bytes.Buffer) (*http.Request, error) {
	var req *http.Request
	var err error
	if body == nil {
		req, err = http.NewRequestWithContext(ctx, method, target, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, target, body)
	}
	if err == nil {
		req.Header.Set("x-goog-api-key", c.APIKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	return req, err
}

func (c *Client) buildGeminiSubmit(ctx context.Context, p map[string]any, img *imagegen.Image) (*http.Request, error) {
	base, err := BaseURLForProtocol(c.EndpointURL, ProtocolGemini)
	if err != nil {
		return nil, err
	}
	model, err := geminiModelName(c.Model)
	if err != nil {
		return nil, err
	}
	instance := map[string]any{"prompt": p["prompt"]}
	if img != nil {
		// All references have passed the shared format, ownership and URL checks.
		// Send bytes even for URLs, so Google never fetches a private/local path.
		// Match imageToMldev in Google's official SDK; some guide examples use
		// the different generateContent inlineData shape instead.
		instance["image"] = map[string]any{"mimeType": img.MIME, "bytesBase64Encoded": base64.StdEncoding.EncodeToString(img.Bytes)}
	}
	parameters := map[string]any{"sampleCount": 1}
	if seconds, exists := p["seconds"]; exists {
		parameters["durationSeconds"] = int(seconds.(float64))
	}
	if size, exists := p["size"]; exists {
		parameters["resolution"] = size
	}
	if extra, ok := p["extra_body"].(map[string]any); ok {
		for key, value := range extra {
			canonical := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if canonical == "numberofvideos" || canonical == "samplecount" {
				// SDK-style alias; the REST wire field is sampleCount.
				continue
			}
			parameters[key] = value
		}
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(map[string]any{"instances": []any{instance}, "parameters": parameters}); err != nil {
		return nil, err
	}
	return c.geminiRequest(ctx, http.MethodPost, base+"/models/"+model+":predictLongRunning", &body)
}

type geminiOperation struct {
	Name     string          `json:"name"`
	Done     bool            `json:"done"`
	Error    json.RawMessage `json:"error"`
	Response struct {
		GenerateVideoResponse struct {
			GeneratedSamples []struct {
				Video struct {
					URI string `json:"uri"`
				} `json:"video"`
			} `json:"generatedSamples"`
			FilteredCount   int      `json:"raiMediaFilteredCount"`
			FilteredReasons []string `json:"raiMediaFilteredReasons"`
		} `json:"generateVideoResponse"`
	} `json:"response"`
}

func parseGeminiOperation(raw []byte) (*geminiOperation, error) {
	var op geminiOperation
	if json.Unmarshal(raw, &op) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("invalid Gemini video operation response")
	}
	if op.Name != "" && !validGeminiOperation(op.Name) {
		return nil, fmt.Errorf("invalid Gemini video operation name")
	}
	return &op, nil
}

func (op *geminiOperation) uri() string {
	samples := op.Response.GenerateVideoResponse.GeneratedSamples
	if len(samples) == 0 {
		return ""
	}
	return samples[0].Video.URI
}

func (op *geminiOperation) video() *Video {
	v := &Video{ID: op.Name, Status: "in_progress"}
	if len(op.Error) != 0 && string(op.Error) != "null" {
		v.Status, v.Error = "failed", op.Error
	} else if op.Done {
		v.Status, v.Progress = "completed", json.RawMessage("100")
		if op.uri() == "" {
			message := "Gemini video operation completed without a generated video"
			filtered := op.Response.GenerateVideoResponse
			if filtered.FilteredCount > 0 || len(filtered.FilteredReasons) > 0 {
				message = "Gemini video was blocked by safety filters"
				if len(filtered.FilteredReasons) > 0 {
					message += ": " + strings.Join(filtered.FilteredReasons, "; ")
				}
			}
			v.Status = "failed"
			v.Error, _ = json.Marshal(map[string]string{"message": message})
		}
	}
	return v
}

func (c *Client) queryGemini(ctx context.Context, name string) (*geminiOperation, error) {
	if !validGeminiOperation(name) {
		return nil, fmt.Errorf("invalid Gemini video operation name")
	}
	base, err := BaseURLForProtocol(c.EndpointURL, ProtocolGemini)
	if err != nil {
		return nil, err
	}
	req, err := c.geminiRequest(ctx, http.MethodGet, base+"/"+name, nil)
	if err != nil {
		return nil, err
	}
	raw, err := c.request(req)
	if err != nil {
		return nil, err
	}
	op, err := parseGeminiOperation(raw)
	if err != nil {
		return nil, err
	}
	if op.Name != "" && op.Name != name {
		return nil, fmt.Errorf("Gemini video operation name does not match the task")
	}
	op.Name = name
	return op, nil
}

func (c *Client) geminiContent(ctx context.Context, name string, maxBytes int64) (*http.Response, error) {
	// Recover the file URI from the persisted operation name after restarts and
	// download failures. It never requires resubmitting generation or a new row.
	op, err := c.queryGemini(ctx, name)
	if err != nil {
		return nil, err
	}
	if !op.Done || op.video().Status != "completed" {
		return nil, fmt.Errorf("Gemini video content is unavailable")
	}
	base, _ := BaseURLForProtocol(c.EndpointURL, ProtocolGemini)
	resp, err := imagegen.OpenDownloadWithHeaders(ctx, op.uri(), base+"/", maxBytes, http.Header{"X-Goog-Api-Key": []string{c.APIKey}})
	if err != nil {
		return nil, fmt.Errorf("Gemini video content: %s", c.redact(err.Error()))
	}
	return resp, nil
}

func (c *Client) listGeminiModels(ctx context.Context) ([]imagegen.ModelInfo, error) {
	base, err := BaseURLForProtocol(c.EndpointURL, ProtocolGemini)
	if err != nil {
		return nil, err
	}
	out := []imagegen.ModelInfo{}
	seenModels, seenTokens := map[string]bool{}, map[string]bool{}
	token := ""
	for page := 0; page < 100; page++ {
		query := url.Values{"pageSize": []string{"1000"}}
		if token != "" {
			query.Set("pageToken", token)
		}
		req, err := c.geminiRequest(ctx, http.MethodGet, base+"/models?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		raw, err := c.request(req)
		if err != nil {
			return nil, err
		}
		var response struct {
			Models []struct {
				Name        string   `json:"name"`
				DisplayName string   `json:"displayName"`
				Methods     []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if json.Unmarshal(raw, &response) != nil || response.Models == nil {
			return nil, fmt.Errorf("invalid Gemini video model catalog response")
		}
		for _, model := range response.Models {
			video := false
			for _, method := range model.Methods {
				video = video || method == "predictLongRunning"
			}
			if !video {
				continue
			}
			id, err := geminiModelName(model.Name)
			if err != nil || seenModels[id] {
				continue
			}
			name := model.DisplayName
			if name == "" {
				name = id
			}
			out = append(out, imagegen.ModelInfo{ID: id, Name: name})
			seenModels[id] = true
		}
		token = response.NextPageToken
		if token == "" {
			return out, nil
		}
		if seenTokens[token] {
			return nil, fmt.Errorf("Gemini model catalog repeated a page token")
		}
		seenTokens[token] = true
	}
	return nil, fmt.Errorf("Gemini model catalog exceeds 100 pages")
}
