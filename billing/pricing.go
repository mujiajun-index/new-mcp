package billing

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/model"
)

// 计费类型(§5.1)。仅 free / per_call 两种,不存在 per_token。
const (
	BillingTypeFree    = "free"
	BillingTypePerCall = "per_call"
)

// 条目种类(条目级定价的维度)。三种 kind 条目未设价时的缺省:工具继承服务价,
// 资源/提示免费;资源/提示要继承服务价须显式存 inherit 行。
const (
	EntryKindTool     = "tool"
	EntryKindResource = "resource"
	EntryKindPrompt   = "prompt"
)

// BillingTypeInherit 条目价标记:显式继承服务统一价(仅 mcp_tool_prices 条目行使用,
// 服务级定价恒为 free/per_call)。效果与"未设条目价的工具"一致(回退服务级链)。
const BillingTypeInherit = "inherit"

// PriceScopeEntry 条目级价格来源:资源/提示条目命中(工具命中沿用存量 "tool" 口径)。
const PriceScopeEntry = "entry"

// PriceInfo 一次市场服务调用的定价解析结果。
type PriceInfo struct {
	BillingType       string  // free / per_call
	UnitPriceQuota    int64   // 最终单价(已乘分组倍率,整数 quota);free 时为 0
	UnitPriceDecimal  float64 // 单价快照(展示货币,用于日志展示,未乘倍率)
	Scope             string  // tool / entry / service / global / free
	MarketplaceItemID int64
	ToolName          string // 条目名(工具名/资源上游 URI/提示名)
}

// ErrPriceNotConfigured 非自用模式下市场项未显式定价、也无法解析到有效价格(参考 new-api "价格未配置")。
var ErrPriceNotConfigured = errors.New("marketplace service price not configured")

// --- 价格缓存(§5.4):服务级 + 工具级价格 1 分钟 TTL ---

type cachedItemPricing struct {
	billingType  string
	pricePerCall float64                       // 服务级单价(展示货币)
	entryPrices  map[string]model.McpToolPrice // "kind\x00条目名" → 条目级定价(enabled)
	loadedAt     time.Time
}

var (
	pricingCacheMu  sync.RWMutex
	pricingCache    = make(map[int64]*cachedItemPricing)
	pricingCacheTTL = 60 * time.Second
)

// loadItemPricing 取(必要时刷新)某市场项的服务级 + 工具级价格(double-check locking)。
func loadItemPricing(itemID int64) (*cachedItemPricing, error) {
	pricingCacheMu.RLock()
	if c, ok := pricingCache[itemID]; ok && time.Since(c.loadedAt) < pricingCacheTTL {
		pricingCacheMu.RUnlock()
		return c, nil
	}
	pricingCacheMu.RUnlock()

	pricingCacheMu.Lock()
	defer pricingCacheMu.Unlock()
	if c, ok := pricingCache[itemID]; ok && time.Since(c.loadedAt) < pricingCacheTTL {
		return c, nil
	}

	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	toolPrices, _ := model.ListToolPricesByItem(itemID)
	tpMap := make(map[string]model.McpToolPrice, len(toolPrices))
	for i := range toolPrices {
		tpMap[toolPrices[i].Kind+"\x00"+toolPrices[i].ToolName] = toolPrices[i]
	}

	c := &cachedItemPricing{
		billingType:  item.BillingType,
		pricePerCall: item.PricePerCall,
		entryPrices:  tpMap,
		loadedAt:     time.Now(),
	}
	pricingCache[itemID] = c
	return c, nil
}

// InvalidatePricingCache 清空全部价格缓存。管理员改价/换凭证后调用,使变更即时生效(§5.4)。
func InvalidatePricingCache() {
	pricingCacheMu.Lock()
	pricingCache = make(map[int64]*cachedItemPricing)
	pricingCacheMu.Unlock()
}

// InvalidatePricingCacheItem 仅失效单个市场项的缓存。
func InvalidatePricingCacheItem(itemID int64) {
	pricingCacheMu.Lock()
	delete(pricingCache, itemID)
	pricingCacheMu.Unlock()
}

// ResolveMarketplaceEntryPrice 条目级定价解析(§5.2 的条目维度扩展):
//   - 第 1 级条目级 mcp_tool_prices[item, kind, name] —— 命中即用(最高优先;
//     工具 scope=tool,资源/提示 scope=entry);billing_type=inherit 行回退第 2 级。
//   - 未命中条目价的缺省:工具继承服务级,资源/提示免费(**不回退服务价**,
//     恒不返回 ErrPriceNotConfigured);三种 kind 显式 inherit 后都走服务级链——
//     1. 服务级 marketplace_items.billing_type / price_per_call(已显式定价才用)
//     2. 全局默认 BillingDefaultType / BillingDefaultPricePerCall(**仅自用模式生效**)
//     非自用模式且无法解析到有效价 → ErrPriceNotConfigured(仅走服务级链的条目)。
//
// 结果乘 userGroup 的分组倍率。BillingEnabled=false → 免费。
func ResolveMarketplaceEntryPrice(itemID int64, kind, entryName, userGroup string) (PriceInfo, error) {
	base := PriceInfo{BillingType: BillingTypeFree, Scope: "free", MarketplaceItemID: itemID, ToolName: entryName}

	if !model.GetOptionBool("BillingEnabled") {
		return base, nil // 总开关关闭:市场服务也跳过计费
	}

	c, err := loadItemPricing(itemID)
	if err != nil {
		// 价格加载失败:返回免费 + 错误,由计费层按 BillingFailOpen 决定放行与否
		return base, err
	}

	ratio := groupRatio(userGroup)
	quotaPerUnit := model.GetQuotaPerUnit()

	// 第 1 级:条目级(三种 kind 同表,命中即用;inherit 行回退服务级链)
	explicitInherit := false
	if tp, ok := c.entryPrices[kind+"\x00"+entryName]; ok {
		if tp.BillingType == BillingTypeInherit {
			explicitInherit = true
		} else if kind == EntryKindTool {
			return priceResult(tp.BillingType, tp.PricePerCall, ratio, quotaPerUnit, "tool", itemID, entryName), nil
		} else {
			return priceResult(tp.BillingType, tp.PricePerCall, ratio, quotaPerUnit, PriceScopeEntry, itemID, entryName), nil
		}
	}
	// 资源/提示无条目价且非显式继承 → 免费(缺省),不回退服务价
	if kind != EntryKindTool && !explicitInherit {
		return base, nil
	}

	// 第 2 级服务级(工具缺省/条目显式继承都走这里;已显式定价才用)
	if c.billingType == BillingTypeFree {
		return priceResult(BillingTypeFree, 0, ratio, quotaPerUnit, "service", itemID, entryName), nil
	}
	if c.billingType == BillingTypePerCall && c.pricePerCall > 0 {
		return priceResult(BillingTypePerCall, c.pricePerCall, ratio, quotaPerUnit, "service", itemID, entryName), nil
	}
	// 第 3 级:全局默认(仅自用模式生效,§5.6)
	if model.GetOptionBool("SelfUseModeEnabled") {
		defType := model.GetOptionString("BillingDefaultType")
		if defType == "" {
			defType = BillingTypePerCall
		}
		if defType == BillingTypeFree {
			return priceResult(BillingTypeFree, 0, ratio, quotaPerUnit, "global", itemID, entryName), nil
		}
		if defPrice := model.GetOptionFloat("BillingDefaultPricePerCall"); defPrice > 0 {
			return priceResult(BillingTypePerCall, defPrice, ratio, quotaPerUnit, "global", itemID, entryName), nil
		}
	}

	// 兜底:非自用模式且未显式定价 → 调用时报错(上架时应已被门控拦截)
	return base, ErrPriceNotConfigured
}

// priceResult 组装 PriceInfo,统一应用"单价 × 分组倍率"换算。
func priceResult(billingType string, priceDecimal float64, ratio float64, quotaPerUnit int64, scope string, itemID int64, toolName string) PriceInfo {
	pi := PriceInfo{
		BillingType:       billingType,
		UnitPriceDecimal:  priceDecimal,
		Scope:             scope,
		MarketplaceItemID: itemID,
		ToolName:          toolName,
	}
	if billingType == BillingTypePerCall && priceDecimal > 0 {
		base := priceToQuota(priceDecimal, quotaPerUnit)
		pi.UnitPriceQuota = applyRatio(base, ratio)
	}
	return pi
}

func groupRatio(userGroup string) float64 {
	r := model.GetGroupRatio()[userGroup]
	if r <= 0 {
		r = 1.0
	}
	return r
}

// priceToQuota 展示货币单价 → 整数 quota(四舍五入,避免浮点精度问题)。
func priceToQuota(priceDecimal float64, quotaPerUnit int64) int64 {
	if priceDecimal <= 0 {
		return 0
	}
	return int64(math.Round(priceDecimal * float64(quotaPerUnit)))
}

// applyRatio 对整数 quota 应用分组倍率(四舍五入)。
func applyRatio(base int64, ratio float64) int64 {
	if base <= 0 || ratio == 1.0 {
		return base
	}
	return int64(math.Round(float64(base) * ratio))
}
