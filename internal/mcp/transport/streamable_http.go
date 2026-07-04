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
	sessionID string // 由服务端在 initialize 响应中下发，后续请求必须回传
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

	// 1. initialize
	_, headers, err := a.request(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "newmcp", "version": "1.0.0"},
	}, false)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// MCP 规范：服务端在 initialize 响应里返回 Mcp-Session-Id 时，客户端后续请求必须带上。
	if sid := headers.Get("Mcp-Session-Id"); sid != "" {
		a.sessionID = sid
	}

	// 2. notifications/initialized（通知，无响应体；部分服务端强校验，缺失会导致后续请求 -32602）
	if _, _, err := a.request(ctx, "notifications/initialized", nil, true); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}

	// 3. tools/list
	if toolsResp, _, err := a.request(ctx, "tools/list", nil, false); err == nil {
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
	a.sessionID = ""
	return nil
}

func (a *StreamableHTTPAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	result, _, err := a.request(ctx, method, params, false)
	return result, err
}

// request 发起一次 JSON-RPC 请求或通知，返回解析后的 result、响应头（用于读取 Mcp-Session-Id）。
// notification=true 时表示通知（无 id、无响应体，服务端通常回 202）。
func (a *StreamableHTTPAdapter) request(ctx context.Context, method string, params interface{}, notification bool) (json.RawMessage, http.Header, error) {
	id := 0
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if !notification {
		id = a.nextID
		req["id"] = id
		a.nextID++
	}
	if params != nil {
		req["params"] = params
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if a.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", a.sessionID)
	}
	for k, v := range a.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}

	// 200 正常响应；通知通常返回 202 Accepted。
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	// 通知不期望响应体。
	if notification {
		return nil, resp.Header, nil
	}

	// 服务端可能返回纯 JSON 或 SSE 流，根据 Content-Type 判断。
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		respBody = extractSSEData(respBody, id)
	}

	var jsonResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &jsonResp); err != nil {
		return nil, nil, fmt.Errorf("parse response: %w", err)
	}
	if jsonResp.Error != nil {
		return nil, nil, fmt.Errorf("MCP error %d: %s", jsonResp.Error.Code, jsonResp.Error.Message)
	}
	return jsonResp.Result, resp.Header, nil
}

// extractSSEData 从 SSE 流中提取与本次请求对应的 JSON-RPC 响应。
// 一个流可能含多个事件（通知、ping、最终响应）；优先返回 id 匹配的 data，
// 退而求其次取首个含 result/error 的事件，避免误取到前置通知。
func extractSSEData(raw []byte, requestID int) []byte {
	var fallback []byte
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	// 工具列表可能很大，放宽单 token 上限到 10MB，避免默认 64KB 截断。
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := []byte(strings.TrimPrefix(line, "data: "))
		if len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		var probe struct {
			ID     *int           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &probe) == nil {
			if probe.ID != nil && *probe.ID == requestID {
				return data
			}
			if (len(probe.Result) > 0 || len(probe.Error) > 0) && fallback == nil {
				fallback = data
			}
		}
	}
	if fallback != nil {
		return fallback
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
