package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mujkjk/newmcp/common"
)

const eventExtension = "io.newmcp/events"

func requestMeta(req *JSONRPCRequest) map[string]json.RawMessage {
	var params struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	_ = json.Unmarshal(req.Params, &params)
	return params.Meta
}

// RequestProtocolVersion reads per-request metadata, never connection state.
func RequestProtocolVersion(req *JSONRPCRequest) string {
	var version string
	_ = json.Unmarshal(requestMeta(req)[mcp.MetaKeyProtocolVersion], &version)
	return version
}

func IsModernRequest(req *JSONRPCRequest) bool {
	return RequestProtocolVersion(req) >= latestProtocolVersion
}

func rpcFailure(id any, code int, message string, data any) *JSONRPCResponse {
	return &JSONRPCResponse{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message, Data: data}}
}

// ValidateRequest is shared by HTTP, WebSocket, and embedded gateway callers.
func ValidateRequest(req *JSONRPCRequest) *JSONRPCResponse {
	if req.JSONRPC != "2.0" || req.Method == "" {
		return rpcFailure(req.ID, -32600, "Invalid JSON-RPC request", nil)
	}
	if req.ID == nil && !strings.HasPrefix(req.Method, "notifications/") {
		return rpcFailure(nil, -32600, "A request ID is required", nil)
	}
	if req.ID != nil {
		if strings.HasPrefix(req.Method, "notifications/") {
			return rpcFailure(nil, -32600, "A notification must not have an ID", nil)
		}
		switch id := req.ID.(type) {
		case string, int, int64:
		case json.Number:
			if _, err := id.Int64(); err != nil {
				return rpcFailure(nil, -32600, "A numeric request ID must be an integer", nil)
			}
		case float64:
			if math.IsNaN(id) || math.IsInf(id, 0) || id != math.Trunc(id) {
				return rpcFailure(nil, -32600, "A numeric request ID must be an integer", nil)
			}
		default:
			return rpcFailure(nil, -32600, "Invalid request ID", nil)
		}
	}
	if len(req.Params) > 0 && string(req.Params) != "null" {
		var params map[string]json.RawMessage
		if json.Unmarshal(req.Params, &params) != nil || params == nil {
			return rpcFailure(req.ID, -32602, "params must be an object", nil)
		}
	}
	meta := requestMeta(req)
	version := RequestProtocolVersion(req)
	if raw, exists := meta[mcp.MetaKeyProtocolVersion]; exists && (version == "" || string(raw) == "null") {
		return rpcFailure(req.ID, -32602, "Invalid protocol version metadata", nil)
	}
	if version != "" && !slices.Contains(supportedProtocolVersions, version) {
		return rpcFailure(req.ID, -32022, "Unsupported protocol version", map[string]any{"requested": version, "supported": supportedProtocolVersions})
	}
	if IsModernRequest(req) {
		var capabilities map[string]json.RawMessage
		if json.Unmarshal(meta[mcp.MetaKeyClientCapabilities], &capabilities) != nil || capabilities == nil {
			return rpcFailure(req.ID, -32602, "clientCapabilities metadata must be an object", nil)
		}
		var typedCapabilities mcp.ClientCapabilities
		if json.Unmarshal(meta[mcp.MetaKeyClientCapabilities], &typedCapabilities) != nil {
			return rpcFailure(req.ID, -32602, "Invalid clientCapabilities metadata", nil)
		}
		if raw, ok := meta[mcp.MetaKeyClientInfo]; ok {
			var info mcp.Implementation
			if json.Unmarshal(raw, &info) != nil || info.Name == "" || info.Version == "" {
				return rpcFailure(req.ID, -32602, "clientInfo must include name and version", nil)
			}
		}
		switch req.Method {
		case "initialize", "notifications/initialized", "ping", "resources/subscribe", "resources/unsubscribe", "logging/setLevel", "notifications/roots/list_changed":
			return rpcFailure(req.ID, -32601, "Method removed in protocol "+latestProtocolVersion+": "+req.Method, nil)
		}
	} else if req.Method == "server/discover" || req.Method == "subscriptions/listen" {
		return rpcFailure(req.ID, -32601, "Method requires per-request protocol metadata for "+latestProtocolVersion, nil)
	}
	return nil
}

func ValidateHTTPHeaders(req *JSONRPCRequest, headers http.Header) *JSONRPCResponse {
	version := RequestProtocolVersion(req)
	headerVersion := headers.Get("MCP-Protocol-Version")
	if headerVersion != "" && !slices.Contains(supportedProtocolVersions, headerVersion) {
		return rpcFailure(req.ID, -32022, "Unsupported protocol version", map[string]any{"requested": headerVersion, "supported": supportedProtocolVersions})
	}
	if version == "" && headerVersion >= latestProtocolVersion {
		return rpcFailure(req.ID, -32602, "Missing protocol version in params._meta", nil)
	}
	if version != "" {
		if headerVersion == "" || headerVersion != version {
			return rpcFailure(req.ID, -32020, "MCP-Protocol-Version must match params._meta", nil)
		}
	}
	if !IsModernRequest(req) {
		return nil
	}
	if headers.Get("Mcp-Method") != req.Method {
		return rpcFailure(req.ID, -32020, "Mcp-Method must match the request method", nil)
	}
	var p struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}
	_ = json.Unmarshal(req.Params, &p)
	name, needed := "", false
	switch req.Method {
	case "tools/call", "prompts/get":
		name, needed = p.Name, true
	case "resources/read":
		name, needed = p.URI, true
	}
	decodedName, validName := decodeHeaderValue(headers.Get("Mcp-Name"))
	if needed && (len(headers.Values("Mcp-Name")) != 1 || !validName || decodedName != name) {
		return rpcFailure(req.ID, -32020, "Mcp-Name must match the requested name or URI", nil)
	}
	if !needed && headers.Get("Mcp-Name") != "" {
		return rpcFailure(req.ID, -32020, "Mcp-Name is not valid for this method", nil)
	}
	return nil
}

func decodeHeaderValue(value string) (string, bool) {
	if !strings.HasPrefix(value, "=?base64?") {
		return value, true
	}
	if !strings.HasSuffix(value, "?=") {
		return "", false
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSuffix(strings.TrimPrefix(value, "=?base64?"), "?="))
	return string(raw), err == nil
}

// ValidateToolHeaders validates schema-declared Mcp-Param-* mirrors before
// executing a tool. Catalog resolution uses the same scope as tools/list.
func (h *GatewayHandler) ValidateToolHeaders(ctx context.Context, req *JSONRPCRequest, headers http.Header, logCtx *LogContext) *JSONRPCResponse {
	if !IsModernRequest(req) || req.Method != "tools/call" {
		return nil
	}
	taskDone, err := h.startGatewayTask()
	if err != nil {
		return rpcFailure(req.ID, -32603, err.Error(), nil)
	}
	defer taskDone()
	var p struct {
		Name      string                     `json:"name"`
		Arguments map[string]json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(req.Params, &p) != nil {
		return nil
	}
	response := h.handleToolsList(ctx, &JSONRPCRequest{JSONRPC: "2.0", ID: req.ID, Method: "tools/list"}, logCtx)
	if response == nil || response.Error != nil {
		return nil
	}
	raw, _ := json.Marshal(response.Result)
	var catalog struct {
		Tools []struct {
			Name        string          `json:"name"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if json.Unmarshal(raw, &catalog) != nil {
		return nil
	}
	for _, tool := range catalog.Tools {
		if tool.Name != p.Name {
			continue
		}
		var schema headerSchema
		if json.Unmarshal(tool.InputSchema, &schema) != nil {
			return nil
		}
		if err := validateParameterHeaders(schema.Properties, p.Arguments, headers); err != nil {
			return rpcFailure(req.ID, -32020, err.Error(), nil)
		}
	}
	return nil
}

type headerSchema struct {
	Header     string                  `json:"x-mcp-header"`
	Properties map[string]headerSchema `json:"properties"`
}

func validateParameterHeaders(properties map[string]headerSchema, arguments map[string]json.RawMessage, headers http.Header) error {
	for name, property := range properties {
		raw, present := arguments[name]
		if property.Header != "" {
			header := "Mcp-Param-" + property.Header
			values := headers.Values(header)
			if !present || string(raw) == "null" {
				if len(values) > 0 {
					return fmt.Errorf("Unexpected %s for absent or null argument", header)
				}
			} else {
				actual, valid := decodeHeaderValue(headers.Get(header))
				if len(values) != 1 || !valid || !primitiveHeaderMatches(raw, actual) {
					return fmt.Errorf("%s must match its tool argument", header)
				}
			}
		}
		if len(property.Properties) > 0 {
			var nested map[string]json.RawMessage
			_ = json.Unmarshal(raw, &nested)
			if err := validateParameterHeaders(property.Properties, nested, headers); err != nil {
				return err
			}
		}
	}
	return nil
}

func primitiveHeaderValue(raw json.RawMessage) (string, bool) {
	if strings.TrimSpace(string(raw)) == "null" {
		return "", false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, true
	}
	var value bool
	if json.Unmarshal(raw, &value) == nil && string(raw) != "null" {
		return strconv.FormatBool(value), true
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		if n, err := number.Float64(); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && n >= -(1<<53-1) && n <= 1<<53-1 {
			return strconv.FormatInt(int64(n), 10), true
		}
	}
	return "", false
}

func primitiveHeaderMatches(raw json.RawMessage, actual string) bool {
	expected, valid := primitiveHeaderValue(raw)
	if !valid {
		return false
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "" && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')) {
		n, err := strconv.ParseFloat(actual, 64)
		return err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && n >= -(1<<53-1) && n <= 1<<53-1 && strconv.FormatInt(int64(n), 10) == expected
	}
	return actual == expected
}

func HTTPStatus(req *JSONRPCRequest, response *JSONRPCResponse) int {
	if response == nil {
		return http.StatusAccepted
	}
	if response.Error != nil && (response.Error.Code == -32020 || response.Error.Code == -32021 || response.Error.Code == -32022 || response.Error.Code == -32600) {
		return http.StatusBadRequest
	}
	if response.Error == nil || !IsModernRequest(req) {
		return http.StatusOK
	}
	switch response.Error.Code {
	case -32601:
		return http.StatusNotFound
	case -32600, -32602, -32020, -32021, -32022:
		return http.StatusBadRequest
	default:
		return http.StatusOK
	}
}

func (h *GatewayHandler) gatewayCapabilities(logCtx *LogContext) map[string]any {
	caps := map[string]any{
		"tools":      map[string]any{},
		"extensions": map[string]any{eventExtension: map[string]any{"experimental": true}},
	}
	if h.nativeItemsAllowed(logCtx) {
		caps["tools"] = map[string]any{"listChanged": true}
		caps["resources"] = map[string]any{"subscribe": true, "listChanged": true}
		caps["prompts"] = map[string]any{"listChanged": true}
	}
	return caps
}

func (h *GatewayHandler) handleDiscover(req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	return &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
		"supportedVersions": supportedProtocolVersions,
		"capabilities":      h.gatewayCapabilities(logCtx),
		"instructions":      h.smartModeInstructions(logCtx),
	}}
}

// CompleteResponse adds the modern result discriminator without rewriting the
// upstream content or losing numeric precision in arbitrary tool output.
func (h *GatewayHandler) CompleteResponse(req *JSONRPCRequest, response *JSONRPCResponse) {
	if !IsModernRequest(req) || response == nil || response.Error != nil {
		return
	}
	raw, err := json.Marshal(response.Result)
	if err != nil {
		return
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil || result == nil {
		result = map[string]json.RawMessage{}
	}
	if _, exists := result["resultType"]; !exists {
		result["resultType"] = json.RawMessage(`"complete"`)
	}
	var resultType string
	_ = json.Unmarshal(result["resultType"], &resultType)
	if resultType == "complete" {
		switch req.Method {
		case "server/discover", "tools/list", "resources/list", "resources/templates/list", "resources/read", "prompts/list":
			// Every gateway result is scoped to the current principal and group.
			result["ttlMs"] = json.RawMessage(`0`)
			result["cacheScope"] = json.RawMessage(`"private"`)
		}
	}
	meta := map[string]json.RawMessage{}
	_ = json.Unmarshal(result["_meta"], &meta)
	if meta == nil {
		meta = map[string]json.RawMessage{}
	}
	meta[mcp.MetaKeyServerInfo], _ = json.Marshal(map[string]string{"name": "newmcp", "version": common.Version})
	result["_meta"], _ = json.Marshal(meta)
	response.Result = result
}
