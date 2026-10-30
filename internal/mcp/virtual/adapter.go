package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/imagegentask"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/internal/mcp/videogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual/events"
	"github.com/mujkjk/newmcp/model"
)

// Adapter presents built-in services through the same session contract as an
// external MCP server. One observer serves all watchers of this user service.
type Adapter struct {
	svc          model.McpService
	mu           sync.RWMutex
	connected    bool
	tools        []transport.Tool
	watchers     map[uint64]adapterWatch
	next         uint64
	cancel       context.CancelFunc
	toolsChanged func()
	done         chan struct{}
}
type adapterWatch struct {
	ctx  context.Context
	uris map[string]bool
	emit func(transport.Notification)
}

func NewAdapter(svc *model.McpService) *Adapter {
	return &Adapter{svc: *svc, watchers: map[uint64]adapterWatch{}}
}
func (a *Adapter) resolve() (Binding, *model.McpService, model.VirtualExecution, error) {
	svc := a.svc
	if svc.ID != 0 {
		current, err := model.GetServiceByID(svc.UserID, svc.ID)
		if err != nil || current.Status != common.StatusEnabled {
			return Binding{}, nil, model.VirtualExecution{}, fmt.Errorf("service is disabled or deleted")
		}
		svc = *current
	}
	owner := svc.UserID
	if svc.Source == "marketplace" {
		if svc.MarketplaceItemID == nil {
			return Binding{}, nil, model.VirtualExecution{}, fmt.Errorf("missing marketplace binding")
		}
		item, err := model.GetMarketplaceItemByID(*svc.MarketplaceItemID)
		if err != nil || item.Status != common.StatusEnabled || item.TransportType != "virtual" {
			return Binding{}, nil, model.VirtualExecution{}, fmt.Errorf("marketplace service is unavailable")
		}
		svc.Config = item.ConfigTemplate
		if plain, err := common.Decrypt(svc.Config); err == nil {
			svc.Config = plain
		}
		owner = item.AdminID
	}
	b, err := ParseBinding(svc.Config)
	if err != nil {
		return b, nil, model.VirtualExecution{}, err
	}
	if svc.Source == "marketplace" && b.VirtualType == "camera" {
		return b, nil, model.VirtualExecution{}, fmt.Errorf("camera services cannot be listed")
	}
	source, err := ResolveBinding(b, owner)
	s := model.VirtualExecution{UserID: svc.UserID, OwnerID: owner, ConfigID: b.RefID, ServiceID: svc.ID, Kind: b.VirtualType, MarketplaceItemID: svc.MarketplaceItemID}
	return b, source, s, err
}
func (a *Adapter) Connect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.RLock()
	connected := a.connected
	a.mu.RUnlock()
	if connected {
		return nil
	}
	generation := events.Generation()
	b, source, scope, err := a.resolve()
	if err != nil {
		return err
	}
	var tools []transport.Tool
	if err := json.Unmarshal([]byte(source.ToolsCache), &tools); err != nil {
		return err
	}
	rows, err := taskStamps(b, scope)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connected {
		return nil
	}
	subscription := events.Subscribe(scope)
	if events.Generation() != generation {
		subscription.CatchUp()
	}
	a.tools = tools
	a.connected = true
	observer, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.done = make(chan struct{})
	raw, _ := json.Marshal(a.tools)
	go a.observe(observer, subscription, b, scope, rows, string(raw))
	return nil
}
func (a *Adapter) Close() error {
	a.mu.Lock()
	a.connected = false
	if a.cancel != nil {
		a.cancel()
	}
	done := a.done
	a.watchers = map[uint64]adapterWatch{}
	a.mu.Unlock()
	if done != nil {
		<-done
	}
	return nil
}
func (a *Adapter) IsConnected() bool                { a.mu.RLock(); defer a.mu.RUnlock(); return a.connected }
func (a *Adapter) GetType() transport.TransportType { return transport.TypeVirtual }
func (a *Adapter) GetProtocolVersion() string       { return "2025-11-25" }
func (a *Adapter) GetServerInfo() *transport.ServerInfo {
	return &transport.ServerInfo{Name: a.svc.Name, Version: "1.0.0"}
}
func (a *Adapter) GetStdioProcess() *transport.StdioProcessInfo { return nil }
func (a *Adapter) GetTools() []transport.Tool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]transport.Tool(nil), a.tools...)
}
func (a *Adapter) SetToolsChangedHandler(f func()) { a.mu.Lock(); a.toolsChanged = f; a.mu.Unlock() }
func (a *Adapter) RefreshTools(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, source, _, err := a.resolve()
	if err != nil {
		return err
	}
	var tools []transport.Tool
	if err = json.Unmarshal([]byte(source.ToolsCache), &tools); err != nil {
		return err
	}
	a.mu.Lock()
	a.tools = tools
	a.mu.Unlock()
	return nil
}
func (a *Adapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	switch method {
	case "resources/list":
		return a.ListResources(ctx)
	case "resources/templates/list":
		return a.ListResourceTemplates(ctx)
	case "prompts/list":
		return a.ListPrompts(ctx)
	case "resources/read", "prompts/get":
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		var p struct {
			URI       string            `json:"uri"`
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		if method == "resources/read" {
			return a.ReadResource(ctx, p.URI)
		}
		return a.GetPrompt(ctx, p.Name, p.Arguments)
	}
	if method == "ping" {
		_, _, _, err := a.resolve()
		return json.RawMessage(`{}`), err
	}
	if method == "tools/list" {
		if err := a.RefreshTools(ctx); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"tools": a.GetTools()})
	}
	if method != "tools/call" {
		return nil, fmt.Errorf("unsupported MCP method: %s", method)
	}
	b, source, scope, err := a.resolve()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err = json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	var tools []transport.Tool
	_ = json.Unmarshal([]byte(source.ToolsCache), &tools)
	found := false
	for _, t := range tools {
		if t.Name == p.Name {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("tool not found: %s", p.Name)
	}
	handlers := map[string]VirtualToolHandler{"vision": VisionHandler, "camera": CameraHandler, "systemone": SystemOneHandler, "imagegen": ImageGenHandler, "videogen": VideoGenHandler, "audiogen": AudioGenHandler}
	ctx = WithCallerUserID(model.WithVirtualExecution(ctx, scope), scope.UserID)
	return handlers[b.VirtualType](ctx, source.ID, ParseConfig(source.Config), p.Name, p.Arguments)
}
func (a *Adapter) ListPrompts(ctx context.Context) (json.RawMessage, error) {
	_, _, _, err := a.resolve()
	return json.RawMessage(`{"prompts":[]}`), err
}
func (a *Adapter) GetPrompt(ctx context.Context, name string, args map[string]string) (json.RawMessage, error) {
	return nil, fmt.Errorf("prompt not found: %s", name)
}
func taskTable(kind string) string {
	return map[string]string{"imagegen": "imagegen_tasks", "videogen": "videogen_tasks", "audiogen": "audiogen_tasks"}[kind]
}
func (a *Adapter) ListResourceTemplates(ctx context.Context) (json.RawMessage, error) {
	b, _, _, err := a.resolve()
	if err != nil {
		return nil, err
	}
	templates := []any{}
	if taskTable(b.VirtualType) != "" {
		templates = append(templates, map[string]any{"uriTemplate": "tasks://" + b.VirtualType + "/{task_id}", "name": "Generation task", "mimeType": "application/json"})
	}
	return json.Marshal(map[string]any{"resourceTemplates": templates})
}

type taskStamp struct {
	ID             string
	Status         string
	ArtifactStatus string
	UpdatedAt      time.Time
}

func (a *Adapter) tasks() (Binding, model.VirtualExecution, []taskStamp, error) {
	b, _, scope, err := a.resolve()
	if err != nil {
		return b, scope, nil, err
	}
	rows, err := taskStamps(b, scope)
	return b, scope, rows, err
}
func taskStamps(b Binding, scope model.VirtualExecution) ([]taskStamp, error) {
	rows := []taskStamp{}
	if table := taskTable(b.VirtualType); table != "" && scope.ServiceID != 0 {
		err := model.ScopeVirtualTasks(model.DB.Table(table), scope).Select("id, status, artifact_status, updated_at").Order("created_at DESC, id DESC").Find(&rows).Error
		return rows, err
	}
	return rows, nil
}
func (a *Adapter) ListResources(ctx context.Context) (json.RawMessage, error) {
	if _, _, _, err := a.resolve(); err != nil {
		return nil, err
	}
	// Task IDs are returned by generation tools. The resource template permits
	// reading a known task without publishing the entire task history as a list.
	return json.RawMessage(`{"resources":[]}`), nil
}
func (a *Adapter) ReadResource(ctx context.Context, uri string) (json.RawMessage, error) {
	b, _, scope, err := a.resolve()
	if err != nil {
		return nil, err
	}
	prefix := "tasks://" + b.VirtualType + "/"
	id := strings.TrimPrefix(uri, prefix)
	if id == uri || id == "" || strings.ContainsAny(id, "/?#") || taskTable(b.VirtualType) == "" {
		return nil, fmt.Errorf("resource not found")
	}
	q := model.ScopeVirtualTasks(model.DB.Table(taskTable(b.VirtualType)), scope).Where("id = ?", id)
	var view any
	switch b.VirtualType {
	case "imagegen":
		var t model.ImageGenTask
		err = q.First(&t).Error
		if err == nil {
			var views []imagegentask.TaskView
			views, err = imagegentask.Views([]model.ImageGenTask{t})
			if err == nil && len(views) > 0 {
				view = views[0]
			}
		}
	case "videogen":
		var t model.VideoGenTask
		err = q.First(&t).Error
		if err == nil {
			view = videogen.TaskView(&t)
		}
	case "audiogen":
		var t model.AudioGenTask
		err = q.First(&t).Error
		if err == nil && AudioManager != nil {
			view = AudioManager.View(&t)
		}
	}
	if err != nil || view == nil {
		return nil, fmt.Errorf("resource not found")
	}
	text, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"contents": []any{map[string]any{"uri": uri, "mimeType": "application/json", "text": string(text)}}})
}
func (a *Adapter) NotificationCapabilities() transport.NotificationCapabilities {
	b, _, _, err := a.resolve()
	if err != nil {
		return transport.NotificationCapabilities{}
	}
	tasks := taskTable(b.VirtualType) != ""
	return transport.NotificationCapabilities{ToolsListChanged: true, ResourceSubscribe: tasks}
}
func (a *Adapter) WatchNotifications(ctx context.Context, uris []string, emit func(transport.Notification)) (func(), error) {
	if emit == nil {
		return nil, fmt.Errorf("notification listener is required")
	}
	for _, uri := range uris {
		if _, err := a.ReadResource(ctx, uri); err != nil {
			return nil, err
		}
	}
	child, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.next++
	id := a.next
	set := map[string]bool{}
	for _, u := range uris {
		set[u] = true
	}
	if !a.connected {
		a.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("adapter is not connected")
	}
	done := a.done
	queue := newNotificationQueue(child, done, emit)
	a.watchers[id] = adapterWatch{child, set, queue.enqueue}
	a.mu.Unlock()
	stop := func() { cancel(); a.mu.Lock(); delete(a.watchers, id); a.mu.Unlock() }
	go func() {
		select {
		case <-child.Done():
		case <-done:
			cancel()
		}
		a.mu.Lock()
		delete(a.watchers, id)
		a.mu.Unlock()
	}()
	return stop, nil
}
func (a *Adapter) observe(ctx context.Context, subscription *events.Subscription, initial Binding, scope model.VirtualExecution, rows []taskStamp, oldTools string) {
	defer func() { subscription.Close() }()
	defer close(a.done)
	defer func() {
		a.mu.Lock()
		a.connected = false
		if a.cancel != nil {
			a.cancel()
		}
		a.mu.Unlock()
	}()
	previous := map[string]string{}
	for _, r := range rows {
		stamp, _ := json.Marshal(r)
		previous["tasks://"+initial.VirtualType+"/"+r.ID] = string(stamp)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-subscription.Ready():
		}
		tasksChanged, configChanged := subscription.Drain()
		if configChanged {
			generation := events.Generation()
			binding, _, nextScope, err := a.resolve()
			if err != nil {
				return
			}
			if scope.Kind != nextScope.Kind || scope.ConfigID != nextScope.ConfigID || scope.ServiceID != nextScope.ServiceID || scope.UserID != nextScope.UserID || scope.OwnerID != nextScope.OwnerID {
				next := events.Subscribe(nextScope)
				if events.Generation() != generation {
					next.CatchUp()
				}
				subscription.Close()
				subscription = next
				scope = nextScope
				initial = binding
				tasksChanged = true
			}
		}
		b := initial
		current := previous
		if tasksChanged {
			var err error
			b, _, rows, err = a.tasks()
			if err != nil {
				return
			}
			current = map[string]string{}
			for _, r := range rows {
				stamp, _ := json.Marshal(r)
				current["tasks://"+b.VirtualType+"/"+r.ID] = string(stamp)
			}
		}
		raw := oldTools
		if configChanged {
			if err := a.RefreshTools(ctx); err != nil {
				return
			}
			tools, _ := json.Marshal(a.GetTools())
			raw = string(tools)
		}
		notices := []transport.Notification{}
		if configChanged && oldTools != raw {
			notices = append(notices, transport.Notification{Method: "notifications/tools/list_changed", Params: json.RawMessage(`{}`)})
			a.mu.RLock()
			f := a.toolsChanged
			a.mu.RUnlock()
			if f != nil {
				f()
			}
		}
		if tasksChanged {
			for uri, stamp := range current {
				before := previous[uri]
				if before != stamp {
					p, _ := json.Marshal(map[string]string{"uri": uri})
					notices = append(notices, transport.Notification{Method: "notifications/resources/updated", Params: p})
				}
			}
			for uri := range previous {
				if _, exists := current[uri]; !exists {
					p, _ := json.Marshal(map[string]string{"uri": uri})
					notices = append(notices, transport.Notification{Method: "notifications/resources/updated", Params: p})
				}
			}
		}
		previous = current
		oldTools = raw
		a.mu.RLock()
		watchers := []adapterWatch{}
		for _, w := range a.watchers {
			watchers = append(watchers, w)
		}
		a.mu.RUnlock()
		for _, n := range notices {
			for _, w := range watchers {
				if w.ctx.Err() != nil {
					continue
				}
				if n.Method == "notifications/resources/updated" {
					var p struct {
						URI string `json:"uri"`
					}
					_ = json.Unmarshal(n.Params, &p)
					if !w.uris[p.URI] {
						continue
					}
				}
				w.emit(n)
			}
		}
	}
}

var _ transport.TransportAdapter = (*Adapter)(nil)

// PublicResourceSnapshot describes capabilities, never a user's task inventory.
func PublicResourceSnapshot(kind string) string {
	templates := []any{}
	if taskTable(kind) != "" {
		templates = append(templates, map[string]any{"uriTemplate": "tasks://" + kind + "/{task_id}", "name": "Generation task", "mimeType": "application/json"})
	}
	raw, _ := json.Marshal(map[string]any{"resources": []any{}, "templates": templates})
	return string(raw)
}
