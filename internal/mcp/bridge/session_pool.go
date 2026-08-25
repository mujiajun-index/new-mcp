package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/installer"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

type McpSession struct {
	ServiceID   int64
	ServiceName string
	UserID      int64
	// 商业化:服务来源(user/admin/marketplace)。计费 hook 据此判定是否扣费(§6)。
	Source            string
	MarketplaceItemID *int64
	Adapter           transport.TransportAdapter
	Tools             []transport.Tool
	LastUsed          time.Time
	LastRefresh       time.Time
	Health            string
	failCount         int
}

type SessionPool struct {
	mu          sync.RWMutex
	sessions    map[int64]*McpSession
	idleTimeout time.Duration
	maxRetries  int
}

func NewSessionPool() *SessionPool {
	return &SessionPool{
		sessions:    make(map[int64]*McpSession),
		idleTimeout: 10 * time.Minute,
		maxRetries:  5,
	}
}

func (p *SessionPool) GetOrConnect(ctx context.Context, svc *model.McpService) (*McpSession, error) {
	p.mu.RLock()
	if session, ok := p.sessions[svc.ID]; ok && session.Adapter.IsConnected() {
		session.LastUsed = time.Now()
		p.mu.RUnlock()
		return session, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double check after acquiring write lock
	if session, ok := p.sessions[svc.ID]; ok && session.Adapter.IsConnected() {
		session.LastUsed = time.Now()
		return session, nil
	}

	adapter := CreateAdapter(svc)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported transport type: %s", svc.TransportType)
	}

	if err := adapter.Connect(ctx); err != nil {
		return nil, err
	}

	session := &McpSession{
		ServiceID:         svc.ID,
		ServiceName:       svc.Name,
		UserID:            svc.UserID,
		Source:            svc.Source,
		MarketplaceItemID: svc.MarketplaceItemID,
		Adapter:           adapter,
		Tools:             adapter.GetTools(),
		LastUsed:          time.Now(),
		LastRefresh:       time.Now(),
		Health:            "healthy",
	}

	p.sessions[svc.ID] = session

	// Update tools cache + handshake info in database
	now := time.Now()
	updates := map[string]interface{}{
		"tools_updated_at": now,
		"health_status":    "healthy",
	}
	if toolsData, err := json.Marshal(session.Tools); err == nil {
		updates["tools_cache"] = string(toolsData)
	}
	// 上游握手拿到的真实协议版本/serverInfo 一并落库;拿不到时不动旧值
	if v := adapter.GetProtocolVersion(); v != "" {
		updates["protocol_version"] = v
	}
	if si := adapter.GetServerInfo(); si != nil {
		if b, err := json.Marshal(si); err == nil {
			updates["server_info"] = string(b)
		}
	}
	model.DB.Model(&model.McpService{}).Where("id = ?", svc.ID).Updates(updates)

	// 资源/提示缓存异步预热:不阻塞连接路径(tools/call 热路径不为列表枚举多等两次上游往返),
	// 失败静默(缓存留空,由"刷新"或下次重连补齐)。
	go p.RefreshItemCaches(context.Background(), session)

	return session, nil
}

// RefreshItemCaches 拉取上游 resources/templates/prompts 并回写 mcp_services 缓存列。
// 服务详情/分组勾选 UI 读这些缓存;上游未声明能力时对应列表为空。
func (p *SessionPool) RefreshItemCaches(ctx context.Context, session *McpSession) {
	resourcesJSON := fetchResourcesCache(ctx, session.Adapter)
	promptsJSON := fetchPromptsCache(ctx, session.Adapter)
	model.DB.Model(&model.McpService{}).Where("id = ?", session.ServiceID).Updates(map[string]interface{}{
		"resources_cache": resourcesJSON,
		"prompts_cache":   promptsJSON,
	})
}

// fetchResourcesCache 合并静态资源与模板为 {"resources":[...],"templates":[...]}。
func fetchResourcesCache(ctx context.Context, adapter transport.TransportAdapter) string {
	combined := map[string]json.RawMessage{
		"resources": json.RawMessage(`[]`),
		"templates": json.RawMessage(`[]`),
	}
	if raw, err := adapter.ListResources(ctx); err == nil {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil && m["resources"] != nil {
			combined["resources"] = m["resources"]
		}
	}
	if raw, err := adapter.ListResourceTemplates(ctx); err == nil {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil && m["resourceTemplates"] != nil {
			combined["templates"] = m["resourceTemplates"]
		}
	}
	b, err := json.Marshal(combined)
	if err != nil {
		return `{"resources":[],"templates":[]}`
	}
	return string(b)
}

// fetchPromptsCache 返回提示裸数组(与 tools_cache 的裸数组形态一致)。
func fetchPromptsCache(ctx context.Context, adapter transport.TransportAdapter) string {
	raw, err := adapter.ListPrompts(ctx)
	if err != nil {
		return "[]"
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) == nil && m["prompts"] != nil {
		if b, err := json.Marshal(m["prompts"]); err == nil {
			return string(b)
		}
	}
	return "[]"
}

// FetchAdapterCaches 一次性拉取上游 tools/resources/prompts 缓存 JSON(形态与 mcp_services 缓存列一致)。
// 供临时连接场景使用(如市场项快照手动刷新):不落库,由调用方决定写入位置。
func FetchAdapterCaches(ctx context.Context, adapter transport.TransportAdapter) (toolsJSON, resourcesJSON, promptsJSON string) {
	toolsJSON = "[]"
	if toolsData, err := json.Marshal(adapter.GetTools()); err == nil {
		toolsJSON = string(toolsData)
	}
	return toolsJSON, fetchResourcesCache(ctx, adapter), fetchPromptsCache(ctx, adapter)
}

func (p *SessionPool) Get(serviceID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessions[serviceID]
}

func (p *SessionPool) GetByName(serviceName string) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		if s.ServiceName == serviceName {
			return s
		}
	}
	return nil
}

func (p *SessionPool) GetByNameForUser(serviceName string, userID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		if s.ServiceName == serviceName && s.UserID == userID {
			return s
		}
	}
	return nil
}

func (p *SessionPool) Remove(serviceID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.sessions[serviceID]; ok {
		s.Adapter.Close()
		delete(p.sessions, serviceID)
	}
}

// RemoveByMarketplaceItem 踢掉某市场项全部引用服务的池内会话(市场项平台上游配置变更后调用),
// 下次调用时按新 config_template 重新物化连接。只清内存池,不查 DB:无引用的项自然无会话可踢。
func (p *SessionPool) RemoveByMarketplaceItem(itemID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, s := range p.sessions {
		if s.MarketplaceItemID != nil && *s.MarketplaceItemID == itemID {
			s.Adapter.Close()
			delete(p.sessions, id)
		}
	}
}

func (p *SessionPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, s := range p.sessions {
		s.Adapter.Close()
		delete(p.sessions, id)
	}
}

func (p *SessionPool) FindByToolName(toolName string) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		for _, t := range s.Tools {
			if t.Name == toolName {
				return s
			}
		}
	}
	return nil
}

func (p *SessionPool) FindByToolNameForUser(toolName string, userID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		if s.UserID != userID {
			continue
		}
		for _, t := range s.Tools {
			if t.Name == toolName {
				return s
			}
		}
	}
	return nil
}

func (p *SessionPool) GetAllSessions() []*McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*McpSession, 0, len(p.sessions))
	for _, s := range p.sessions {
		result = append(result, s)
	}
	return result
}

func CreateAdapter(svc *model.McpService) transport.TransportAdapter {
	var config map[string]interface{}
	_ = json.Unmarshal([]byte(svc.Config), &config)

	switch transport.TransportType(svc.TransportType) {
	case transport.TypeStdio:
		cmd, _ := config["command"].(string)
		argsRaw, _ := config["args"].([]interface{})
		args := make([]string, len(argsRaw))
		for i, a := range argsRaw {
			args[i], _ = a.(string)
		}
		env, _ := config["env"].(map[string]interface{})
		envMap := make(map[string]string)
		for k, v := range env {
			envMap[k], _ = v.(string)
		}
		// 将所选包管理源镜像注入子进程环境变量（npx→NPM_CONFIG_REGISTRY；
		// uvx→UV_DEFAULT_INDEX+PIP_INDEX_URL）。用户在 env 里显式写过的同名变量优先。
		if registry, _ := config["registry"].(string); registry != "" {
			for k, v := range installer.RegistryEnv(installer.ClassifyCommand(cmd), registry) {
				if _, exists := envMap[k]; !exists {
					envMap[k] = v
				}
			}
		}
		return transport.NewStdioAdapter(svc.ID, cmd, args, envMap)

	case transport.TypeStreamableHTTP:
		url, _ := config["url"].(string)
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string)
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewStreamableHTTPAdapter(svc.ID, url, h)

	case transport.TypeSSE:
		url, _ := config["url"].(string)
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string)
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewSSEAdapter(svc.ID, url, h)

	default:
		return nil
	}
}
