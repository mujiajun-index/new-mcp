package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type StreamableHTTPAdapter struct {
	serviceID int64
	url       string
	headers   map[string]string
	client    *http.Client
	tools     []Tool
	connected bool
	mu        sync.Mutex
	nextID    int
}

func NewStreamableHTTPAdapter(serviceID int64, url string, headers map[string]string) *StreamableHTTPAdapter {
	return &StreamableHTTPAdapter{
		serviceID: serviceID,
		url:       url,
		headers:   headers,
		client:    &http.Client{Timeout: 30 * time.Second},
		nextID:    1,
	}
}

func (a *StreamableHTTPAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Send initialize
	_, err := a.callUnlocked(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "newmcp", "version": "1.0.0"},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// Fetch tools
	toolsResp, err := a.callUnlocked(ctx, "tools/list", nil)
	if err == nil {
		var result struct {
			Tools []Tool `json:"tools"`
		}
		if json.Unmarshal(toolsResp, &result) == nil {
			a.tools = result.Tools
		}
	}

	a.connected = true
	return nil
}

func (a *StreamableHTTPAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connected = false
	return nil
}

func (a *StreamableHTTPAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.callUnlocked(ctx, method, params)
}

func (a *StreamableHTTPAdapter) callUnlocked(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := a.nextID
	a.nextID++

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range a.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	// 服务端可能返回纯 JSON 或 SSE 流，根据 Content-Type 判断
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		respBody = extractSSEData(respBody)
	}

	var jsonResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &jsonResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if jsonResp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", jsonResp.Error.Code, jsonResp.Error.Message)
	}
	return jsonResp.Result, nil
}

// extractSSEData 从 SSE 流中提取第一个 data 字段的 JSON 内容
func extractSSEData(raw []byte) []byte {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: "))
		}
	}
	return raw
}

func (a *StreamableHTTPAdapter) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

func (a *StreamableHTTPAdapter) GetType() TransportType { return TypeStreamableHTTP }
func (a *StreamableHTTPAdapter) GetTools() []Tool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tools
}
