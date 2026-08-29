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
	sessions    map[sessionKey]*McpSession
	idleTimeout time.Duration
	maxRetries  int
}

// sessionKey 会话池键:默认按服务行键控(自有服务/独占市场引用一行一会话);
// 共享市场 stdio 条目按条目键控(itemID≠0,serviceID 恒 0),全部安装用户复用
// 同一条目会话与子进程。svc.SharedProcess 由 materialize 从条目 isolated_process
// 反算(仅市场 stdio 条目会置 true),市场行必先 materialize 再入池,键控可靠。
type sessionKey struct {
	serviceID int64
	itemID    int64
}

func sessionKeyFor(svc *model.McpService) sessionKey {
	if svc.SharedProcess && svc.MarketplaceItemID != nil {
		return sessionKey{itemID: *svc.MarketplaceItemID}
	}
	return sessionKey{serviceID: svc.ID}
}

func NewSessionPool() *SessionPool {
	return &SessionPool{
		sessions:    make(map[sessionKey]*McpSession),
		idleTimeout: 10 * time.Minute,
		maxRetries:  5,
	}
}

func (p *SessionPool) GetOrConnect(ctx context.Context, svc *model.McpService) (*McpSession, error) {
	key := sessionKeyFor(svc)
	p.mu.RLock()
	if session, ok := p.sessions[key]; ok && session.Adapter.IsConnected() {
		session.LastUsed = time.Now()
		p.mu.RUnlock()
		return session, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double check after acquiring write lock
	if session, ok := p.sessions[key]; ok && session.Adapter.IsConnected() {
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

	p.sessions[key] = session

	// 共享条目预热用的是内存行(ID=0,不落库):跳过 DB 回写与缓存预热,
	// 引用行的 tools/握手信息由真实调用路径或刷新补齐。
	if svc.ID != 0 {
		updateSessionRow(session, adapter)
		go p.RefreshItemCaches(context.Background(), session)
	}

	return session, nil
}

// updateSessionRow 连接成功后把 tools 缓存 + 上游握手信息回写服务行。
func updateSessionRow(session *McpSession, adapter transport.TransportAdapter) {
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
	model.DB.Model(&model.McpService{}).Where("id = ?", session.ServiceID).Updates(updates)
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
	return p.sessions[sessionKey{serviceID: serviceID}]
}

// GetByItem 共享市场条目的平台会话(条目键控);共享条目在池内至多一条。
// 不校验连接状态,调用方自行判定(与 Get 同口径)。
func (p *SessionPool) GetByItem(itemID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessions[sessionKey{itemID: itemID}]
}

// GetSessionsByItem 某市场条目在池内的全部会话(按会话携带的 item 归属过滤,
// 与键控方式无关):独占条目=各安装用户的行会话,共享条目=至多一条条目会话。
// 供条目级进程视图/健康判定枚举。
func (p *SessionPool) GetSessionsByItem(itemID int64) []*McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []*McpSession
	for _, s := range p.sessions {
		if s.MarketplaceItemID != nil && *s.MarketplaceItemID == itemID {
			out = append(out, s)
		}
	}
	return out
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
	key := sessionKey{serviceID: serviceID}
	if s, ok := p.sessions[key]; ok {
		s.Adapter.Close()
		delete(p.sessions, key)
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
