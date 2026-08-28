package service

import (
	"sync"
	"time"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// 管理端市场页的平台级健康:按日志表 marketplace_item_id 列(有索引,tools/call
// 计费路径与 resources/prompts/mcp.read 回填路径都会写)聚合每个市场条目全部
// 用户的真实调用,窗口/分桶口径与总览被动健康一致(20 桶 × 10 分钟,见
// health_snapshot.go)。不枚举 mcp_services 引用行——IN 列表恒为条目数,不随
// 用户安装量增长,用户卸载后遗留的历史日志照样计入。自带 30s 缓存,只在管理员
// 打开市场管理页时触发,与总览(各用户自己服务行口径)互不影响。

const marketplaceHealthCacheTTL = 30 * time.Second

var marketplaceHealthCache struct {
	sync.Mutex
	builtAt time.Time
	data    map[int64]*dto.MarketplaceItemHealth
}

// GetMarketplaceItemHealth 全部市场条目的平台级健康(条目 ID → 健康)。构建
// 出错时沿用上次结果(可能为 nil),页面降级为不展示健康列内容。
func GetMarketplaceItemHealth() map[int64]*dto.MarketplaceItemHealth {
	marketplaceHealthCache.Lock()
	defer marketplaceHealthCache.Unlock()

	now := time.Now()
	if marketplaceHealthCache.data != nil &&
		now.Sub(marketplaceHealthCache.builtAt) < marketplaceHealthCacheTTL {
		return marketplaceHealthCache.data
	}
	if data := buildMarketplaceItemHealth(now); data != nil {
		marketplaceHealthCache.builtAt = now
		marketplaceHealthCache.data = data
	}
	return marketplaceHealthCache.data
}

// buildMarketplaceItemHealth 两次查询完成聚合:条目 id+status 窄行 → 200 分钟
// 窗口窄行日志(按 marketplace_item_id,行内 ServiceID 字段承载条目 ID 的别名,
// 恒等键聚合)。健康实时推导(derivePassiveHealth):条目禁用→未知,任一引用
// 服务有活跃连接→健康,否则看窗口成败(与总览非 stdio 服务同规则)。
func buildMarketplaceItemHealth(now time.Time) map[int64]*dto.MarketplaceItemHealth {
	items, err := model.ListMarketplaceItemStatuses()
	if err != nil || len(items) == 0 {
		return nil
	}
	itemStatus := make(map[int64]int, len(items))
	itemIDs := make([]int64, 0, len(items))
	for _, it := range items {
		itemIDs = append(itemIDs, it.ID)
		itemStatus[it.ID] = it.Status
	}

	firstBucket := firstHealthBucket(now)
	rows, err := model.GetCallLogRowsForItems(itemIDs, firstBucket)
	if err != nil {
		return nil
	}
	snaps := buildSnapshotsFromRows(rows, nil, itemIDs, firstBucket)

	// 会话自带 item 归属(session.MarketplaceItemID),任一引用服务有活跃连接
	// 即视作条目连接中,无需反查服务行
	connectedItems := map[int64]bool{}
	if SessionPool != nil {
		for _, sess := range SessionPool.GetAllSessions() {
			if sess.Adapter.IsConnected() && sess.MarketplaceItemID != nil {
				connectedItems[*sess.MarketplaceItemID] = true
			}
		}
	}

	out := make(map[int64]*dto.MarketplaceItemHealth, len(itemIDs))
	for _, itemID := range itemIDs {
		snap := snaps[itemID]
		h := &dto.MarketplaceItemHealth{
			Buckets:          snap.Buckets,
			LastErrorMessage: snap.LastErrorMessage,
			LastErrorAt:      snap.LastErrorUnix,
		}
		// 窗口内有调用直接取快照时间;空窗条目点查全史,给"暂无调用"一个时间锚点
		if snap.LastCallUnix > 0 {
			h.LastCallAt = snap.LastCallUnix
		} else if lt := model.GetLastConsumeTimeForItem(itemID); lt != nil {
			h.LastCallAt = lt.Unix()
		}
		h.HealthStatus = derivePassiveHealth(itemStatus[itemID], connectedItems[itemID], snap.Buckets)
		out[itemID] = h
	}
	return out
}
