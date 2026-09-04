package service

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// setupMarketplaceSyncTest 初始化引用行同步测试环境:分组绑定测试环境之外补
// mcp_services(安装引用行)、标签字典(tags 校验/字典同步用例)与用户分组四表
// (下架联动禁用+清分组用例)。
func setupMarketplaceSyncTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.Option{}, &model.MarketplaceItem{}, &model.MarketplaceGroup{},
		&model.MarketplaceItemGroup{}, &model.MarketplaceTag{}, &model.McpService{},
		&model.McpGroup{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{},
		&model.MarketplaceItemKey{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

// newRefRow 建一条市场安装引用行(name 可偏离条目名,模拟 -2 冲突后缀)。
func newRefRow(t *testing.T, userID int64, item *model.MarketplaceItem, name string) *model.McpService {
	t.Helper()
	svc := &model.McpService{
		UserID: userID, Name: name,
		DisplayName: item.DisplayName, Description: item.Description,
		TransportType: "marketplace", Config: "{}",
		Source: "marketplace", MarketplaceItemID: &item.ID,
		Status: common.StatusEnabled, HealthStatus: common.HealthUnknown, AuthType: "none",
	}
	if err := model.DB.Create(svc).Error; err != nil {
		t.Fatalf("create ref row (user %d): %v", userID, err)
	}
	return svc
}

func loadRef(t *testing.T, id int64) *model.McpService {
	t.Helper()
	var svc model.McpService
	if err := model.DB.Where("id = ?", id).First(&svc).Error; err != nil {
		t.Fatalf("load ref %d: %v", id, err)
	}
	return &svc
}

func loadItem(t *testing.T, id int64) *model.MarketplaceItem {
	t.Helper()
	item, err := model.GetMarketplaceItemByID(id)
	if err != nil {
		t.Fatalf("load item %d: %v", id, err)
	}
	return item
}

// TestUpdateItemSyncsRefRows 管理端修改市场项的展示/快照字段须同事务同步到全部
// 安装引用行;mcp.search/tools/list 等读路径取引用行快照,不同步即过期。
func TestUpdateItemSyncsRefRows(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref1 := newRefRow(t, 101, item, "item-1")
	ref2 := newRefRow(t, 202, item, "item-1-2") // 冲突后缀行:name 不得被同步覆盖

	s := &MarketplaceService{}
	req := &dto.UpdateMarketplaceItemReq{
		DisplayName:   new("新显示名"),
		Description:   new("新描述"),
		IconURL:       new("https://cdn.example/x.png"),
		ToolsSnapshot: []interface{}{map[string]interface{}{"name": "tool_a", "description": "工具A"}},
	}
	if err := s.UpdateItem(item.ID, req); err != nil {
		t.Fatalf("update item: %v", err)
	}

	fresh := loadItem(t, item.ID)
	for _, id := range []int64{ref1.ID, ref2.ID} {
		ref := loadRef(t, id)
		if ref.DisplayName != "新显示名" || ref.Description != "新描述" || ref.IconURL != "https://cdn.example/x.png" {
			t.Fatalf("ref %d 展示字段未同步: %+v", id, ref)
		}
		if ref.ToolsCache != fresh.ToolsSnapshot {
			t.Fatalf("ref %d tools_cache 未同步: got %q want %q", id, ref.ToolsCache, fresh.ToolsSnapshot)
		}
		if ref.ToolsUpdatedAt == nil {
			t.Fatalf("ref %d tools_updated_at 未刷新", id)
		}
		if ref.TransportType != "marketplace" {
			t.Fatalf("ref %d transport_type 哨兵被破坏: %q", id, ref.TransportType)
		}
	}
	if got := loadRef(t, ref1.ID).Name; got != "item-1" {
		t.Fatalf("ref1 name 被改动: %q", got)
	}
	if got := loadRef(t, ref2.ID).Name; got != "item-1-2" {
		t.Fatalf("ref2 name 被改动: %q", got)
	}
}

// TestUpdateItemWithoutDisplayFieldsKeepsRefs 只改分组/价格等非同步字段的更新
// 不得动引用行(指针部分更新语义回归)。
func TestUpdateItemWithoutDisplayFieldsKeepsRefs(t *testing.T) {
	setupMarketplaceSyncTest(t)
	a := newMarketGroup(t, "A", common.StatusEnabled)
	item := newMarketItem(t, "item-1")
	ref := newRefRow(t, 101, item, "item-1")

	s := &MarketplaceService{}
	if err := s.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{
		GroupIDs:    []int64{a.ID},
		PricePerCall: new(0.0),
	}); err != nil {
		t.Fatalf("update groups/pricing: %v", err)
	}

	got := loadRef(t, ref.ID)
	if got.DisplayName != item.DisplayName || got.Description != item.Description {
		t.Fatalf("非展示字段更新不得动引用行: %+v", got)
	}
	if got.ToolsCache != "" || got.ToolsUpdatedAt != nil {
		t.Fatalf("非快照更新不得动引用行缓存: cache=%q updated_at=%v", got.ToolsCache, got.ToolsUpdatedAt)
	}
}

// TestBatchSetGroupsTagsSyncsRefs 批量设置 tags 须同步到所选条目全部安装引用行;
// 只传 group_ids(tags=nil)不动引用行。
func TestBatchSetGroupsTagsSyncsRefs(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref1 := newRefRow(t, 101, item, "item-1")
	ref2 := newRefRow(t, 202, item, "item-1-2")
	if err := model.DB.Create(&model.MarketplaceTag{Name: "web", Status: common.StatusEnabled}).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	s := &MarketplaceService{}
	if _, err := s.BatchSetGroupsTags(&dto.BatchGroupsTagsReq{
		IDs:  []int64{item.ID},
		Tags: []string{"web"},
	}); err != nil {
		t.Fatalf("batch set tags: %v", err)
	}
	for _, id := range []int64{ref1.ID, ref2.ID} {
		if got := loadRef(t, id).Tags; got != "web" {
			t.Fatalf("ref %d tags 未同步: %q", id, got)
		}
	}

	// 标签字典改名:条目与引用行 tags 一并整词替换
	if err := model.ReplaceMarketplaceTagName(model.DB, "web", "search"); err != nil {
		t.Fatalf("rename tag: %v", err)
	}
	if got := loadItem(t, item.ID).Tags; got != "search" {
		t.Fatalf("item tags after rename: %q", got)
	}
	for _, id := range []int64{ref1.ID, ref2.ID} {
		if got := loadRef(t, id).Tags; got != "search" {
			t.Fatalf("ref %d tags after rename: %q", id, got)
		}
	}

	// 标签字典删除:条目与引用行 tags 一并摘除
	if err := model.RemoveMarketplaceTagName(model.DB, "search"); err != nil {
		t.Fatalf("remove tag: %v", err)
	}
	if got := loadItem(t, item.ID).Tags; got != "" {
		t.Fatalf("item tags after remove: %q", got)
	}
	for _, id := range []int64{ref1.ID, ref2.ID} {
		if got := loadRef(t, id).Tags; got != "" {
			t.Fatalf("ref %d tags after remove: %q", id, got)
		}
	}

	// 只传 group_ids(tags=nil):引用行 tags 不动
	g := newMarketGroup(t, "G", common.StatusEnabled)
	if _, err := s.BatchSetGroupsTags(&dto.BatchGroupsTagsReq{IDs: []int64{item.ID}, GroupIDs: []int64{g.ID}}); err != nil {
		t.Fatalf("batch set groups only: %v", err)
	}
	for _, id := range []int64{ref1.ID, ref2.ID} {
		if got := loadRef(t, id).Tags; got != "" {
			t.Fatalf("ref %d tags 应保持空: %q", id, got)
		}
	}

	// 全缺省=无操作,拒绝
	if _, err := s.BatchSetGroupsTags(&dto.BatchGroupsTagsReq{IDs: []int64{item.ID}}); err == nil {
		t.Fatalf("全缺省应拒绝(ErrBatchNothingToUpdate)")
	}
}

// --- 下架联动:引用行自动禁用 + 分组清除 + 启用拦截 ---

// newUserGroup 建一个用户分组并入库(endpoint_slug 仅为非空唯一,直接用名)。
func newUserGroup(t *testing.T, userID int64, name string) *model.McpGroup {
	t.Helper()
	g := &model.McpGroup{UserID: userID, Name: name, EndpointSlug: name, Status: common.StatusEnabled}
	if err := model.DB.Create(g).Error; err != nil {
		t.Fatalf("create user group %s: %v", name, err)
	}
	return g
}

// seedGroupRows 给某服务在某分组里补齐三张关联表各一行(成员/工具覆盖/条目覆盖)。
func seedGroupRows(t *testing.T, groupID, serviceID int64) {
	t.Helper()
	rows := []interface{}{
		&model.McpGroupService{GroupID: groupID, ServiceID: serviceID, Enabled: true},
		&model.McpGroupTool{GroupID: groupID, ServiceID: serviceID, ToolName: "tool_a", Enabled: true},
		&model.McpGroupItem{GroupID: groupID, ServiceID: serviceID, ItemKind: "resource", ItemKey: "newmcp://x", Enabled: true},
	}
	for _, r := range rows {
		if err := model.DB.Create(r).Error; err != nil {
			t.Fatalf("seed group row (%T): %v", r, err)
		}
	}
}

// countGroupRows 数某服务在指定分组表中的关联行数。
func countGroupRows(t *testing.T, table interface{}, serviceID int64) int64 {
	t.Helper()
	var n int64
	if err := model.DB.Model(table).Where("service_id = ?", serviceID).Count(&n).Error; err != nil {
		t.Fatalf("count %T: %v", table, err)
	}
	return n
}

// assertRefsPurged 断言引用行全部禁用且三张分组表清空,无关服务的关联行不动。
func assertRefsPurged(t *testing.T, refIDs []int64, bystanderID int64) {
	t.Helper()
	for _, id := range refIDs {
		if got := loadRef(t, id).Status; got != common.StatusDisabled {
			t.Fatalf("ref %d 应已禁用: status=%d", id, got)
		}
		for _, table := range []interface{}{&model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}} {
			if n := countGroupRows(t, table, id); n != 0 {
				t.Fatalf("ref %d 在 %T 仍有 %d 行残留", id, table, n)
			}
		}
	}
	for _, table := range []interface{}{&model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}} {
		if n := countGroupRows(t, table, bystanderID); n != 1 {
			t.Fatalf("无关服务 %d 在 %T 的关联行被误删: n=%d", bystanderID, table, n)
		}
	}
}

// TestUpdateItemDisablePurgesRefsAndGroups 管理端下架须把全部安装引用行置禁用并
// 清出用户分组(与手动禁用服务同语义);路由屏蔽真正依赖的就是这三张关联行。
func TestUpdateItemDisablePurgesRefsAndGroups(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref1 := newRefRow(t, 101, item, "item-1")
	ref2 := newRefRow(t, 202, item, "item-1-2")
	// 两个用户各自的分组 + 引用行关联;另有一个无关服务挂在同分组里,不得被误删
	g1 := newUserGroup(t, 101, "G1")
	g2 := newUserGroup(t, 202, "G2")
	bystander := &model.McpService{UserID: 999, Name: "own", TransportType: "sse", Config: "{}", Source: "user", Status: common.StatusEnabled}
	if err := model.DB.Create(bystander).Error; err != nil {
		t.Fatalf("create bystander: %v", err)
	}
	seedGroupRows(t, g1.ID, ref1.ID)
	seedGroupRows(t, g2.ID, ref2.ID)
	seedGroupRows(t, g1.ID, bystander.ID)

	s := &MarketplaceService{}
	if err := s.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{Status: new(common.StatusDisabled)}); err != nil {
		t.Fatalf("disable item: %v", err)
	}
	assertRefsPurged(t, []int64{ref1.ID, ref2.ID}, bystander.ID)
}

// TestUpdateItemEnableKeepsRefsDisabled 重新上架不得自动恢复引用行(单向下行):
// 用户须自行启用或从市场重新添加;分组关联同样不复活。
func TestUpdateItemEnableKeepsRefsDisabled(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref := newRefRow(t, 101, item, "item-1")
	g := newUserGroup(t, 101, "G")
	seedGroupRows(t, g.ID, ref.ID)

	s := &MarketplaceService{}
	if err := s.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{Status: new(common.StatusDisabled)}); err != nil {
		t.Fatalf("disable item: %v", err)
	}
	if err := s.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{Status: new(common.StatusEnabled)}); err != nil {
		t.Fatalf("re-enable item: %v", err)
	}
	if got := loadRef(t, ref.ID).Status; got != common.StatusDisabled {
		t.Fatalf("重新上架不得自动恢复引用行: status=%d", got)
	}
	for _, table := range []interface{}{&model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}} {
		if n := countGroupRows(t, table, ref.ID); n != 0 {
			t.Fatalf("分组关联不得复活(%T): n=%d", table, n)
		}
	}
}

// TestDeleteItemDisablesRefsAndPurgesGroups 删除条目(软删)同下架联动:引用行保留
// 但禁用、分组清空,避免残留行把旧 tools_cache 泄入 mcp.search。
func TestDeleteItemDisablesRefsAndPurgesGroups(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref := newRefRow(t, 101, item, "item-1")
	bystander := &model.McpService{UserID: 999, Name: "own", TransportType: "sse", Config: "{}", Source: "user", Status: common.StatusEnabled}
	if err := model.DB.Create(bystander).Error; err != nil {
		t.Fatalf("create bystander: %v", err)
	}
	g := newUserGroup(t, 101, "G")
	seedGroupRows(t, g.ID, ref.ID)
	seedGroupRows(t, g.ID, bystander.ID)

	s := &MarketplaceService{}
	if err := s.DeleteItem(item.ID); err != nil {
		t.Fatalf("delete item: %v", err)
	}
	if _, err := model.GetMarketplaceItemByID(item.ID); err == nil {
		t.Fatalf("条目应已软删")
	}
	// 引用行本身保留(供用户侧显示已下架),但已禁用+清分组
	if got := loadRef(t, ref.ID).Status; got != common.StatusDisabled {
		t.Fatalf("ref 应已禁用: status=%d", got)
	}
	for _, table := range []interface{}{&model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}} {
		if n := countGroupRows(t, table, ref.ID); n != 0 {
			t.Fatalf("ref 在 %T 仍有 %d 行残留", table, n)
		}
		if n := countGroupRows(t, table, bystander.ID); n != 1 {
			t.Fatalf("无关服务在 %T 的关联行被误删: n=%d", table, n)
		}
	}
}

// TestAddToMyServicesReenablesDisabledRef 条目启用时重复"添加到我的服务"须把已禁用
// 引用行恢复启用;条目下架时添加直接拒绝。
func TestAddToMyServicesReenablesDisabledRef(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref := newRefRow(t, 101, item, "item-1")
	if err := model.DB.Model(ref).Update("status", common.StatusDisabled).Error; err != nil {
		t.Fatalf("disable ref: %v", err)
	}

	s := &MarketplaceService{}
	res, err := s.AddToMyServices(101, item.ID)
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if res.ServiceID != ref.ID {
		t.Fatalf("去重应返回原引用行: got %d want %d", res.ServiceID, ref.ID)
	}
	if got := loadRef(t, ref.ID).Status; got != common.StatusEnabled {
		t.Fatalf("重复添加应恢复启用: status=%d", got)
	}

	// 条目下架时添加直接拒绝(去重分支之前的状态门控)
	if err := s.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{Status: new(common.StatusDisabled)}); err != nil {
		t.Fatalf("disable item: %v", err)
	}
	if err := model.DB.Model(ref).Update("status", common.StatusDisabled).Error; err != nil {
		t.Fatalf("disable ref again: %v", err)
	}
	if _, err := s.AddToMyServices(101, item.ID); err == nil || err.Error() != "marketplace item not available" {
		t.Fatalf("下架条目应拒绝添加: %v", err)
	}
	if got := loadRef(t, ref.ID).Status; got != common.StatusDisabled {
		t.Fatalf("拒绝添加不得改动引用行: status=%d", got)
	}
}

// TestUpdateServiceEnableRejectedWhenItemOffline 平台下架/删除的市场引用行不可手动
// 启用(ErrMarketplaceItemOffline);条目启用时启用正常。
func TestUpdateServiceEnableRejectedWhenItemOffline(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref := newRefRow(t, 101, item, "item-1")
	svc := &McpServiceService{}

	// 条目启用:启用引用行正常
	if err := svc.Update(101, ref.ID, &dto.UpdateServiceReq{Status: new(1)}); err != nil {
		t.Fatalf("启用(条目在线)不应报错: %v", err)
	}

	// 条目下架:拒绝
	ms := &MarketplaceService{}
	if err := ms.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{Status: new(common.StatusDisabled)}); err != nil {
		t.Fatalf("disable item: %v", err)
	}
	if err := svc.Update(101, ref.ID, &dto.UpdateServiceReq{Status: new(1)}); err != ErrMarketplaceItemOffline {
		t.Fatalf("下架条目应拒绝启用: %v", err)
	}

	// 条目软删:同样拒绝
	if err := model.DB.Delete(&model.MarketplaceItem{}, item.ID).Error; err != nil {
		t.Fatalf("soft delete item: %v", err)
	}
	if err := svc.Update(101, ref.ID, &dto.UpdateServiceReq{Status: new(1)}); err != ErrMarketplaceItemOffline {
		t.Fatalf("软删条目应拒绝启用: %v", err)
	}
}

// TestResolveEnabledServicesForGroupsExcludesDisabled 路由解析兜底:服务行非启用的
// 分组成员不得进入配对结果(防残留关联行把禁用服务泄入 mcp.search/网关路由)。
func TestResolveEnabledServicesForGroupsExcludesDisabled(t *testing.T) {
	setupMarketplaceSyncTest(t)
	item := newMarketItem(t, "item-1")
	ref := newRefRow(t, 101, item, "item-1")
	g := newUserGroup(t, 101, "G")
	if err := model.AddServicesToGroup(g.ID, []int64{ref.ID}); err != nil {
		t.Fatalf("add to group: %v", err)
	}

	if err := model.DB.Model(ref).Update("status", common.StatusDisabled).Error; err != nil {
		t.Fatalf("disable ref: %v", err)
	}
	pairs, err := model.ResolveEnabledServicesForGroups([]model.McpGroup{*g})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("禁用服务不得进入路由配对: %d 对", len(pairs))
	}

	if err := model.DB.Model(ref).Update("status", common.StatusEnabled).Error; err != nil {
		t.Fatalf("enable ref: %v", err)
	}
	pairs, err = model.ResolveEnabledServicesForGroups([]model.McpGroup{*g})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(pairs) != 1 || pairs[0].Service.ID != ref.ID {
		t.Fatalf("启用服务应进入路由配对: %+v", pairs)
	}
}
