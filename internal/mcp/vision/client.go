package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultMaxTokensAnthropic is substituted when MaxTokens <= 0 ("unlimited").
// Anthropic's API requires a non-zero max_tokens and rejects 0/omitted values,
// so we send a high safe cap. 8192 is the output ceiling for Claude 3.5
// Sonnet/Haiku and a valid bound for the Claude 4 family.
const defaultMaxTokensAnthropic = 8192

type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type VisionClient struct {
	Provider    string
	EndpointURL string
	ApiKey      string
	ModelName   string
	MaxTokens   int
	// AnalyzeTimeout bounds each Analyze call (the upstream POST). <=0 means use
	// the 30s default. ListModels (doGet) is unaffected — it keeps its own 15s.
	AnalyzeTimeout time.Duration
}

// ImageInput is the discriminated union for the image fed to Analyze. Exactly
// one transport is used:
//   - Bytes: raw image bytes; each provider base64-encodes them inline in its
//     own request shape (OpenAI data URL, Anthropic base64 source, Gemini
//     inline_data). MediaType must be set on this path.
//   - URL: a URL the upstream model fetches itself. new-mcp never downloads it
//     (no SSRF, the bytes never touch this process); the URL is passed through
//     verbatim in the provider's native url field. MediaType is optional here.
type ImageInput struct {
	Bytes     []byte
	MediaType string
	URL       string
}

// IsURL reports whether the URL transport is selected.
func (in ImageInput) IsURL() bool { return in.URL != "" }

func (c *VisionClient) Analyze(ctx context.Context, systemPrompt, userPrompt string, in ImageInput) (string, error) {
	switch c.Provider {
	case "anthropic":
		return c.analyzeAnthropic(ctx, systemPrompt, userPrompt, in)
	case "gemini":
		return c.analyzeGemini(ctx, systemPrompt, userPrompt, in)
	default:
		return c.analyzeOpenAI(ctx, systemPrompt, userPrompt, in)
	}
}

func (c *VisionClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	switch c.Provider {
	case "anthropic":
		return c.listAnthropicModels(ctx)
	case "gemini":
		return c.listGeminiModels(ctx)
	default:
		return c.listOpenAIModels(ctx)
	}
}

// ========== OpenAI ==========

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type openAIMessage struct {
	Role    string          `json:"role"`
	Content []openAIContent `json:"content"`
}

type openAIContent struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *openAIImage `json:"image_url,omitempty"`
}

type openAIImage struct {
	URL string `json:"url"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (c *VisionClient) analyzeOpenAI(ctx context.Context, systemPrompt, userPrompt string, in ImageInput) (string, error) {
	// OpenAI's image_url accepts either a data URL (bytes inline) or a plain URL
	// the model fetches itself. Passthrough for URLs means new-mcp never
	// downloads the image.
	imageURL := in.URL
	if !in.IsURL() {
		imageURL = "data:" + in.MediaType + ";base64," + base64.StdEncoding.EncodeToString(in.Bytes)
	}
	content := []openAIContent{
		{Type: "text", Text: userPrompt},
		{Type: "image_url", ImageURL: &openAIImage{URL: imageURL}},
	}

	messages := []openAIMessage{{Role: "user", Content: content}}
	if systemPrompt != "" {
		messages = append([]openAIMessage{{Role: "system", Content: []openAIContent{{Type: "text", Text: systemPrompt}}}}, messages...)
	}

	reqBody := openAIRequest{Model: c.ModelName, Messages: messages, MaxTokens: c.MaxTokens}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.EndpointURL, "/") + "/v1/chat/completions"
	respBody, err := c.doPost(ctx, url, body, func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
		if c.ApiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.ApiKey)
		}
	})
	if err != nil {
		return "", err
	}

	var resp openAIResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("API error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from vision model")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *VisionClient) listOpenAIModels(ctx context.Context) ([]ModelInfo, error) {
	url := strings.TrimRight(c.EndpointURL, "/") + "/v1/models"
	respBody, err := c.doGet(ctx, url, func(req *http.Request) {
		if c.ApiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.ApiKey)
		}
	})
	if err != nil {
		return nil, err
	}
	var resp openAIModelsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	models := make([]ModelInfo, len(resp.Data))
	for i, d := range resp.Data {
		models[i] = ModelInfo{ID: d.ID, Name: d.ID}
	}
	return models, nil
}

// ========== Anthropic ==========

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicBlock struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *anthropicSource `json:"source,omitempty"`
}

type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type anthropicModelsResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"display_name"`
	} `json:"data"`
}

func (c *VisionClient) analyzeAnthropic(ctx context.Context, systemPrompt, userPrompt string, in ImageInput) (string, error) {
	// Anthropic image sources are either "base64" (inline) or "url" (the
	// provider fetches). URLs pass through verbatim — no download here.
	src := &anthropicSource{}
	if in.IsURL() {
		src.Type = "url"
		src.URL = in.URL
	} else {
		src.Type = "base64"
		src.MediaType = in.MediaType
		src.Data = base64.StdEncoding.EncodeToString(in.Bytes)
	}
	blocks := []anthropicBlock{
		{Type: "text", Text: userPrompt},
		{Type: "image", Source: src},
	}

	// Anthropic requires max_tokens > 0; map "unlimited" (0) to a high safe cap.
	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokensAnthropic
	}

	reqBody := anthropicRequest{
		Model:     c.ModelName,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: blocks}},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.EndpointURL, "/") + "/v1/messages"
	respBody, err := c.doPost(ctx, url, body, func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("anthropic-version", "2023-06-01")
		if c.ApiKey != "" {
			req.Header.Set("x-api-key", c.ApiKey)
		}
	})
	if err != nil {
		return "", err
	}

	var resp anthropicResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("API error: %s", resp.Error.Message)
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("no response from vision model")
	}
	return resp.Content[0].Text, nil
}

func (c *VisionClient) listAnthropicModels(ctx context.Context) ([]ModelInfo, error) {
	url := strings.TrimRight(c.EndpointURL, "/") + "/v1/models"
	respBody, err := c.doGet(ctx, url, func(req *http.Request) {
		req.Header.Set("anthropic-version", "2023-06-01")
		if c.ApiKey != "" {
			req.Header.Set("x-api-key", c.ApiKey)
		}
	})
	if err != nil {
		return nil, err
	}
	var resp anthropicModelsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	models := make([]ModelInfo, len(resp.Data))
	for i, d := range resp.Data {
		name := d.Name
		if name == "" {
			name = d.ID
		}
		models[i] = ModelInfo{ID: d.ID, Name: name}
	}
	return models, nil
}

// ========== Gemini ==========

type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
	FileData   *geminiFileData   `json:"file_data,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

// geminiFileData references an image by URI via Gemini's file_data part, used
// for the URL passthrough path. NOTE: Gemini's native file_uri targets its own
// Files API / GCS URIs, so an arbitrary https URL may not be accepted by every
// Gemini-compatible endpoint — the OpenAI-compatible and Anthropic URL paths
// are the reliable ones for V1; this is implemented for completeness.
type geminiFileData struct {
	FileURI  string `json:"file_uri"`
	MimeType string `json:"mime_type,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type geminiModelsResponse struct {
	Models []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"models"`
}

func (c *VisionClient) analyzeGemini(ctx context.Context, systemPrompt, userPrompt string, in ImageInput) (string, error) {
	var imgPart geminiPart
	if in.IsURL() {
		imgPart.FileData = &geminiFileData{FileURI: in.URL, MimeType: in.MediaType}
	} else {
		imgPart.InlineData = &geminiInlineData{MimeType: in.MediaType, Data: base64.StdEncoding.EncodeToString(in.Bytes)}
	}
	parts := []geminiPart{
		{Text: userPrompt},
		imgPart,
	}

	reqBody := geminiRequest{
		Contents:         []geminiContent{{Role: "user", Parts: parts}},
		GenerationConfig: &geminiGenConfig{MaxOutputTokens: c.MaxTokens},
	}
	if systemPrompt != "" {
		reqBody.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: systemPrompt}}}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	base := strings.TrimRight(c.EndpointURL, "/")
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", base, c.ModelName, c.ApiKey)
	respBody, err := c.doPost(ctx, url, body, func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
	})
	if err != nil {
		return "", err
	}

	var resp geminiResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("API error: %s", resp.Error.Message)
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from vision model")
	}
	return resp.Candidates[0].Content.Parts[0].Text, nil
}

func (c *VisionClient) listGeminiModels(ctx context.Context) ([]ModelInfo, error) {
	base := strings.TrimRight(c.EndpointURL, "/")
	url := fmt.Sprintf("%s/v1beta/models?key=%s", base, c.ApiKey)
	respBody, err := c.doGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	var resp geminiModelsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	models := make([]ModelInfo, len(resp.Models))
	for i, m := range resp.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		name := m.DisplayName
		if name == "" {
			name = id
		}
		models[i] = ModelInfo{ID: id, Name: name}
	}
	return models, nil
}

// ========== HTTP helpers ==========

func (c *VisionClient) doPost(ctx context.Context, url string, body []byte, setupReq func(*http.Request)) ([]byte, error) {
	// Per-config analysis timeout (set by the vision/camera handlers from
	// VisionConfig.AnalyzeTimeoutSeconds). >0 bounds the upstream call; <=0 means
	// "no timeout" — the request then runs under whatever deadline the caller's
	// ctx already carries (TestVision's 15s, or none for MCP tool calls).
	var cancel context.CancelFunc
	if c.AnalyzeTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.AnalyzeTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if setupReq != nil {
		setupReq(req)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (c *VisionClient) doGet(ctx context.Context, url string, setupReq func(*http.Request)) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if setupReq != nil {
		setupReq(req)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
