package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/installer"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

type McpSession struct {
	key         sessionKey
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
	useMu             sync.Mutex
	inUse             int
}

type SessionPool struct {
	mu         sync.RWMutex
	sessions   map[sessionKey]*McpSession
	maxRetries int
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
		sessions:   make(map[sessionKey]*McpSession),
		maxRetries: 5,
	}
}

func (p *SessionPool) GetOrConnect(ctx context.Context, svc *model.McpService) (*McpSession, error) {
	key := sessionKeyFor(svc)
	p.mu.RLock()
	if session, ok := p.sessions[key]; ok && session.Adapter.IsConnected() {
		session.markUsed()
		p.mu.RUnlock()
		return session, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double check after acquiring write lock
	if session, ok := p.sessions[key]; ok && session.Adapter.IsConnected() {
		session.markUsed()
		return session, nil
	}

	adapter := CreateAdapter(svc)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported transport type: %s", svc.TransportType)
	}

	if err := adapter.Connect(ctx); err != nil {
		_ = adapter.Close()
		// 多秘钥服务建连失败(首把 key 可能已被 401 熔断)时换 key 立即重试一次:
		// OnAuthFailure 已在 RoundTripper 里禁掉坏 key,新 adapter 的 initialize
		// 会选下一把。非鉴权性失败重试一次也无害。
		reconnected := false
		if KeySelectors.Get(svc) != nil {
			if retry := CreateAdapter(svc); retry != nil {
				if err2 := retry.Connect(ctx); err2 == nil {
					adapter = retry
					reconnected = true
				} else {
					_ = retry.Close()
				}
			}
		}
		if !reconnected {
			return nil, err
		}
	}

	session := &McpSession{
		key:               key,
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
		go p.refreshItemCachesWhenLeased(session)
	}

	return session, nil
}

// AcquireSession holds a lease for a session previously returned by this pool.
// A false result means it was concurrently removed and the caller should retry
// through its normal connect path.
func (p *SessionPool) AcquireSession(session *McpSession) (func(), bool) {
	if session == nil {
		return nil, false
	}
	return p.acquireExisting(session.key, session)
}

// Acquire connects a service if necessary and holds a usage lease until the
// returned release function is called. The lease prevents idle cleanup from
// closing a platform stdio process while an upstream request is in flight.
func (p *SessionPool) Acquire(ctx context.Context, svc *model.McpService) (*McpSession, func(), error) {
	for i := 0; i < 2; i++ {
		session, err := p.GetOrConnect(ctx, svc)
		if err != nil {
			return nil, nil, err
		}
		if release, ok := p.acquireExisting(sessionKeyFor(svc), session); ok {
			return session, release, nil
		}
	}
	return nil, nil, fmt.Errorf("service session was released while acquiring")
}

func (p *SessionPool) acquireExisting(key sessionKey, want *McpSession) (func(), bool) {
	p.mu.RLock()
	session, ok := p.sessions[key]
	if !ok || session != want || !session.Adapter.IsConnected() {
		p.mu.RUnlock()
		return nil, false
	}
	session.useMu.Lock()
	session.inUse++
	session.LastUsed = time.Now()
	session.useMu.Unlock()
	p.mu.RUnlock()
	return func() {
		session.useMu.Lock()
		if session.inUse > 0 {
			session.inUse--
		}
		session.LastUsed = time.Now()
		session.useMu.Unlock()
	}, true
}

func (s *McpSession) markUsed() {
	s.useMu.Lock()
	s.LastUsed = time.Now()
	s.useMu.Unlock()
}

func (p *SessionPool) refreshItemCachesWhenLeased(session *McpSession) {
	if release, ok := p.AcquireSession(session); ok {
		defer release()
		p.RefreshItemCaches(context.Background(), session)
	}
}

// StartIdleReaper checks every minute for idle platform-managed stdio sessions.
// timeout is read for every sweep so an administrator's setting takes effect
// without restarting the server.
func (p *SessionPool) StartIdleReaper(ctx context.Context, timeout func() time.Duration) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.ReleaseIdlePlatformStdio(timeout())
		}
	}
}

// ReleaseIdlePlatformStdio removes only marketplace-owned stdio sessions that
// have no active lease. Adapters are closed after releasing the pool lock;
// 每个被释放的进程记一条日志(服务/条目标识 + 空闲时长),便于运维对账。
func (p *SessionPool) ReleaseIdlePlatformStdio(timeout time.Duration) int {
	if timeout <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-timeout)
	var victims []*McpSession
	p.mu.Lock()
	for key, session := range p.sessions {
		if session.Source != "marketplace" || session.Adapter.GetType() != transport.TypeStdio {
			continue
		}
		session.useMu.Lock()
		idle := session.inUse == 0 && session.LastUsed.Before(cutoff)
		session.useMu.Unlock()
		if !idle {
			continue
		}
		delete(p.sessions, key)
		victims = append(victims, session)
	}
	p.mu.Unlock()
	for _, session := range victims {
		// 会话已出池且无人能再租到,LastUsed 可直接读
		idle := time.Since(session.LastUsed).Round(time.Minute)
		target := fmt.Sprintf("service %d", session.ServiceID)
		if session.key.itemID != 0 {
			target = fmt.Sprintf("shared item %d", session.key.itemID)
		}
		log.Printf("[idle-reaper] released marketplace stdio process %q (%s) after %s idle", session.ServiceName, target, idle)
		if err := session.Adapter.Close(); err != nil {
			log.Printf("[idle-reaper] close %q (%s) failed: %v", session.ServiceName, target, err)
		}
	}
	return len(victims)
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

// dynamicAuthOptions 返回多秘钥动态注入的构造选项;单秘钥/stdio 服务为空
// (保持既有静态 headers 行为)。
func dynamicAuthOptions(svc *model.McpService) []transport.AdapterOption {
	sel := KeySelectors.Get(svc)
	if sel == nil {
		return nil
	}
	return []transport.AdapterOption{transport.WithDynamicAuth(sel.HeaderName(), sel)}
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
		return transport.NewStreamableHTTPAdapter(svc.ID, url, h, dynamicAuthOptions(svc)...)

	case transport.TypeSSE:
		url, _ := config["url"].(string)
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string)
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewSSEAdapter(svc.ID, url, h, dynamicAuthOptions(svc)...)

	default:
		return nil
	}
}
