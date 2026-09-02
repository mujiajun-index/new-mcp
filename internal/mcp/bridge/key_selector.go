package bridge

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

// authFailureReason 自动禁用的标准原因(写入 disabled_reason 与系统日志)。
const authFailureReason = "upstream 401/403"

// errNoEnabledKeys 池空或全部禁用时的选 key 错误。
var errNoEnabledKeys = errors.New("no enabled secret keys")

// 选择器归属:服务自有池(按服务 ID)或市场条目池(按条目 ID,一份池全局共享)。
const (
	selectorKindService = "service"
	selectorKindItem    = "item"
)

// keyEntry 是池快照的中性条目,服务池(mcp_service_keys)与条目池
// (marketplace_item_keys)共用,选择器不关心行来自哪张表。
type keyEntry struct {
	ID        int64
	SortOrder int
	Value     string
	Status    int
}

// KeySelector 持有某服务/市场条目的秘钥池快照,按随机/轮询策略供认证头值,并在上游
// 401/403 时熔断对应秘钥。进程内与会话生命周期解耦:池编辑后 Invalidate,
// 下次建连按新池重建;轮询游标为内存态,进程重启归零(仅影响轮换起点,无害)。
// 实现 transport.DynamicAuth,由 headerRoundTripper 按上游请求调用。
type KeySelector struct {
	owner   int64  // serviceID 或 itemID(按 kind)
	kind    string // selectorKindService | selectorKindItem:熔断落库与日志文案分流
	svcName string // 日志展示名(服务 name / 条目 display_name)
	target  string // 目标头名
	bearer  bool   // 值是否加 "Bearer " 前缀
	mode    string // common.KeyModeRandom | common.KeyModePolling

	mu     sync.Mutex
	keys   []keyEntry
	cursor int // 轮询游标(keys 下标,指向下一次选择位置)
}

// HeaderName 返回注入目标头名(CreateAdapter 接线用)。
func (s *KeySelector) HeaderName() string { return s.target }

// Pick 按策略返回一把启用秘钥:池内序号(1 起)+ 完整头值。
// 随机=启用集合均匀抽取;轮询=从游标起找下一个启用并推进游标。
func (s *KeySelector) Pick() (int, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pick *keyEntry
	switch s.mode {
	case common.KeyModeRandom:
		enabled := s.enabledLocked()
		if len(enabled) == 0 {
			return 0, "", errNoEnabledKeys
		}
		pick = enabled[rand.Intn(len(enabled))]
	default: // polling
		if s.cursor < 0 || s.cursor >= len(s.keys) {
			s.cursor = 0
		}
		for i := 0; i < len(s.keys); i++ {
			idx := (s.cursor + i) % len(s.keys)
			if s.keys[idx].Status == common.StatusEnabled {
				pick = &s.keys[idx]
				s.cursor = (idx + 1) % len(s.keys)
				break
			}
		}
		if pick == nil {
			return 0, "", errNoEnabledKeys
		}
	}
	if s.bearer {
		return pick.SortOrder, "Bearer " + pick.Value, nil
	}
	return pick.SortOrder, pick.Value, nil
}

// OnAuthFailure 熔断一把秘钥(上游 401/403):内存集同步 + 落库 + 系统日志。
// 已禁用的重复上报、未知序号直接忽略,保证幂等。
func (s *KeySelector) OnAuthFailure(keyIndex int) {
	s.mu.Lock()
	var victim *keyEntry
	for i := range s.keys {
		if s.keys[i].SortOrder == keyIndex {
			victim = &s.keys[i]
			break
		}
	}
	if victim == nil || victim.Status != common.StatusEnabled {
		s.mu.Unlock()
		return
	}
	victim.Status = common.StatusAutoDisabled
	keyID := victim.ID
	hasEnabled := len(s.enabledLocked()) > 0
	s.mu.Unlock()

	// DB 写与日志在锁外,避免慢查询阻塞选 key 路径。
	var err error
	subject := "服务"
	if s.kind == selectorKindItem {
		subject = "市场条目"
		_, err = model.SetItemKeyStatus(s.owner, keyID, common.StatusAutoDisabled, authFailureReason)
	} else {
		_, err = model.SetKeyStatus(s.owner, keyID, common.StatusAutoDisabled, authFailureReason)
	}
	if err != nil {
		log.Printf("key selector: disable key #%d of %s %d failed: %v", keyIndex, s.kind, s.owner, err)
	}
	if hasEnabled {
		model.RecordSystemLog(0, "", fmt.Sprintf("%s %s(#%d) 秘钥 #%d 因上游 401/403 自动禁用",
			subject, s.svcName, s.owner, keyIndex), 0, "", map[string]any{
			"owner_kind": s.kind, "owner_id": s.owner, "key_index": keyIndex, "action": "key_auto_disable",
		})
	} else {
		model.RecordSystemLog(0, "", fmt.Sprintf("%s %s(#%d) 全部秘钥均已禁用(最后:#%d 上游 401/403),调用将持续失败直至重新启用",
			subject, s.svcName, s.owner, keyIndex), 0, "", map[string]any{
			"owner_kind": s.kind, "owner_id": s.owner, "key_index": keyIndex, "action": "key_pool_exhausted",
		})
	}
}

// HasEnabledKeys 报告池内是否还有启用秘钥(建连失败换 key 重试用)。
func (s *KeySelector) HasEnabledKeys() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.enabledLocked()) > 0
}

// enabledLocked 返回启用秘钥的指针列表(调用方持锁)。
// 指针指向 s.keys 元素,仅作只读;禁用/删除经由重新构建快照生效。
func (s *KeySelector) enabledLocked() []*keyEntry {
	out := make([]*keyEntry, 0, len(s.keys))
	for i := range s.keys {
		if s.keys[i].Status == common.StatusEnabled {
			out = append(out, &s.keys[i])
		}
	}
	return out
}

// --- 进程级注册表 ---

// KeySelectors 是进程级秘钥选择器注册表,与 SessionPool 并列:服务自有池按服务 ID、
// 市场条目池按条目 ID(一份池全局共享,全部安装用户同一选择器)。
// 会话重建沿用同一 selector(游标连续);池编辑/模式切换后 Invalidate / InvalidateItem。
var KeySelectors = &keySelectorRegistry{}

type keySelectorRegistry struct {
	mu    sync.Mutex
	m     map[int64]*KeySelector
	items map[int64]*KeySelector
}

// Get 返回多秘钥选择器:市场引用行走条目级池(按 itemID),其余走服务级池;
// 单秘钥、非 HTTP 传输、缺目标头或池加载失败均返回 nil(调用方退回静态 headers 行为)。
func (r *keySelectorRegistry) Get(svc *model.McpService) *KeySelector {
	// 市场引用行 AuthType 恒为 none、AuthConfig 为空,服务级路径必然 miss;
	// 在此分流后 session_pool 的两个调用点(建连重试判断、dynamicAuthOptions)零改动。
	if svc.Source == "marketplace" && svc.MarketplaceItemID != nil {
		return r.getItem(*svc.MarketplaceItemID)
	}
	cfg := svc.ParseAuthKeyConfig()
	if cfg.KeyMode != common.KeyModeRandom && cfg.KeyMode != common.KeyModePolling {
		return nil
	}
	if svc.TransportType != common.TransportStreamableHTTP && svc.TransportType != common.TransportSSE {
		return nil
	}
	if cfg.HeaderName == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[int64]*KeySelector{}
	}
	if sel, ok := r.m[svc.ID]; ok {
		return sel
	}
	keys, err := model.ListKeysByService(svc.ID)
	if err != nil || len(keys) == 0 {
		return nil
	}
	entries := make([]keyEntry, len(keys))
	for i, k := range keys {
		entries[i] = keyEntry{ID: k.ID, SortOrder: k.SortOrder, Value: k.Value, Status: k.Status}
	}
	sel := &KeySelector{
		owner:   svc.ID,
		kind:    selectorKindService,
		svcName: svc.Name,
		target:  cfg.HeaderName,
		bearer:  svc.AuthType == "bearer",
		mode:    cfg.KeyMode,
		keys:    entries,
	}
	r.m[svc.ID] = sel
	return sel
}

// getItem 构建/返回条目级选择器(池快照惰性加载,按 itemID 全局共享)。
func (r *keySelectorRegistry) getItem(itemID int64) *KeySelector {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = map[int64]*KeySelector{}
	}
	if sel, ok := r.items[itemID]; ok {
		return sel
	}
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil
	}
	cfg := item.ParseAuthKeyConfig()
	if cfg.KeyMode != common.KeyModeRandom && cfg.KeyMode != common.KeyModePolling {
		return nil
	}
	if item.TransportType != common.TransportStreamableHTTP && item.TransportType != common.TransportSSE {
		return nil
	}
	if cfg.HeaderName == "" {
		return nil
	}
	keys, err := model.ListKeysByItem(itemID)
	if err != nil || len(keys) == 0 {
		return nil
	}
	entries := make([]keyEntry, len(keys))
	for i, k := range keys {
		entries[i] = keyEntry{ID: k.ID, SortOrder: k.SortOrder, Value: k.Value, Status: k.Status}
	}
	sel := &KeySelector{
		owner:   itemID,
		kind:    selectorKindItem,
		svcName: item.DisplayName,
		target:  cfg.HeaderName,
		bearer:  cfg.Bearer,
		mode:    cfg.KeyMode,
		keys:    entries,
	}
	r.items[itemID] = sel
	return sel
}

// Invalidate 失效某服务的选择器(池编辑/模式切换后调用;下次 Get 重建)。
func (r *keySelectorRegistry) Invalidate(serviceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, serviceID)
}

// InvalidateItem 失效某条目的选择器(条目池编辑/模式切换/删条目后调用)。
func (r *keySelectorRegistry) InvalidateItem(itemID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, itemID)
}
