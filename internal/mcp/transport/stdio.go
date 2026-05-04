package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type StdioAdapter struct {
	serviceID int64
	command   string
	args      []string
	env       []string

	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	tools     []Tool
	connected bool
	mu        sync.Mutex
	nextID    int
}

func NewStdioAdapter(serviceID int64, command string, args []string, env map[string]string) *StdioAdapter {
	var envList []string
	for k, v := range env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	return &StdioAdapter{
		serviceID: serviceID,
		command:   command,
		args:      args,
		env:       envList,
		nextID:    1,
	}
}

func (a *StdioAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cmd = exec.CommandContext(ctx, a.command, a.args...)
	a.cmd.Env = append(a.cmd.Environ(), a.env...)

	stdin, err := a.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	a.stdin = stdin

	stdout, err := a.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	a.stdout = bufio.NewReader(stdout)

	if err := a.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// Send initialize
	initResp, err := a.callUnlocked(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "newmcp", "version": "1.0.0"},
	})
	if err != nil {
		a.cmd.Process.Kill()
		return fmt.Errorf("initialize: %w", err)
	}

	// Send initialized notification
	_ = a.notifyUnlocked("notifications/initialized", nil)

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
	_ = initResp
	return nil
}

func (a *StdioAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connected = false
	if a.stdin != nil {
		a.stdin.Close()
	}
	if a.cmd != nil && a.cmd.Process != nil {
		return a.cmd.Process.Kill()
	}
	return nil
}

func (a *StdioAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.callUnlocked(ctx, method, params)
}

func (a *StdioAdapter) callUnlocked(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if !a.connected {
		return nil, fmt.Errorf("not connected")
	}

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

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	if _, err := a.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	line, err := a.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (a *StdioAdapter) notifyUnlocked(method string, params interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	_, err := a.stdin.Write(data)
	return err
}

func (a *StdioAdapter) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

func (a *StdioAdapter) GetType() TransportType { return TypeStdio }
func (a *StdioAdapter) GetTools() []Tool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tools
}
