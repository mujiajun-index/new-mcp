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
// mcp_services(安装引用行)与标签字典(tags 校验/字典同步用例)。
func setupMarketplaceSyncTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.Option{}, &model.MarketplaceItem{}, &model.MarketplaceGroup{},
		&model.MarketplaceItemGroup{}, &model.MarketplaceTag{}, &model.McpService{}); err != nil {
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
