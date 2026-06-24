package vision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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
}

func (c *VisionClient) Analyze(ctx context.Context, systemPrompt, userPrompt, base64Image, mediaType string) (string, error) {
	switch c.Provider {
	case "anthropic":
		return c.analyzeAnthropic(ctx, systemPrompt, userPrompt, base64Image, mediaType)
	case "gemini":
		return c.analyzeGemini(ctx, systemPrompt, userPrompt, base64Image, mediaType)
	default:
		return c.analyzeOpenAI(ctx, systemPrompt, userPrompt, base64Image, mediaType)
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

func (c *VisionClient) analyzeOpenAI(ctx context.Context, systemPrompt, userPrompt, base64Image, mediaType string) (string, error) {
	content := []openAIContent{
		{Type: "text", Text: userPrompt},
		{Type: "image_url", ImageURL: &openAIImage{URL: "data:" + mediaType + ";base64," + base64Image}},
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
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
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

func (c *VisionClient) analyzeAnthropic(ctx context.Context, systemPrompt, userPrompt, base64Image, mediaType string) (string, error) {
	blocks := []anthropicBlock{
		{Type: "text", Text: userPrompt},
		{Type: "image", Source: &anthropicSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64Image,
		}},
	}

	reqBody := anthropicRequest{
		Model:     c.ModelName,
		MaxTokens: c.MaxTokens,
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
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
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

func (c *VisionClient) analyzeGemini(ctx context.Context, systemPrompt, userPrompt, base64Image, mediaType string) (string, error) {
	parts := []geminiPart{
		{Text: userPrompt},
		{InlineData: &geminiInlineData{MimeType: mediaType, Data: base64Image}},
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

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
