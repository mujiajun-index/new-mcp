package service

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// setupMarketplaceGroupTest 初始化分组绑定测试环境:TempDir sqlite + 市场项/分组/绑定表。
func setupMarketplaceGroupTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	// McpService:UpdateItem 展示/快照字段变更会同事务同步安装引用行,须有表
	if err := db.AutoMigrate(&model.Option{}, &model.MarketplaceItem{}, &model.MarketplaceGroup{},
		&model.MarketplaceItemGroup{}, &model.MarketplaceItemKey{}, &model.McpToolPrice{},
		&model.McpService{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

// newMarketGroup 建一个市场分组并入库。
func newMarketGroup(t *testing.T, name string, status int) *model.MarketplaceGroup {
	t.Helper()
	g := &model.MarketplaceGroup{Name: name, Status: status, SortOrder: 0}
	if err := g.Insert(); err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return g
}

// newMarketItem 建一个启用的免费市场项(free 绕非自用模式显式定价门)。
func newMarketItem(t *testing.T, name string) *model.MarketplaceItem {
	t.Helper()
	item := &model.MarketplaceItem{
		AdminID: 1, Name: name, Category: "instant",
		BillingType: "free", Status: common.StatusEnabled,
	}
	if err := model.DB.Create(item).Error; err != nil {
		t.Fatalf("create item %s: %v", name, err)
	}
	return item
}

// bindsOf 读某市场项当前的分组绑定(排序后返回,断言用)。
func bindsOf(t *testing.T, itemID int64) []int64 {
	t.Helper()
	var rows []model.MarketplaceItemGroup
	if err := model.DB.Where("item_id = ?", itemID).Find(&rows).Error; err != nil {
		t.Fatalf("load binds of item %d: %v", itemID, err)
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.GroupID)
	}
	slices.Sort(ids)
	return ids
}

func updateItemGroups(t *testing.T, itemID int64, groupIDs []int64) error {
	t.Helper()
	s := &MarketplaceService{}
	req := &dto.UpdateMarketplaceItemReq{GroupIDs: groupIDs}
	return s.UpdateItem(itemID, req)
}

func TestMarketplaceItemGroups(t *testing.T) {
	t.Run("update_replaces_bindings", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		a, b, c := newMarketGroup(t, "A", common.StatusEnabled), newMarketGroup(t, "B", common.StatusEnabled), newMarketGroup(t, "C", common.StatusEnabled)
		item := newMarketItem(t, "item-1")

		if err := updateItemGroups(t, item.ID, []int64{a.ID, b.ID}); err != nil {
			t.Fatalf("set [a,b]: %v", err)
		}
		if got := bindsOf(t, item.ID); len(got) != 2 || got[0] != a.ID || got[1] != b.ID {
			t.Fatalf("after [a,b]: got %v", got)
		}
		if err := updateItemGroups(t, item.ID, []int64{b.ID, c.ID}); err != nil {
			t.Fatalf("set [b,c]: %v", err)
		}
		if got := bindsOf(t, item.ID); len(got) != 2 || got[0] != b.ID || got[1] != c.ID {
			t.Fatalf("after [b,c]: got %v", got)
		}
		if err := updateItemGroups(t, item.ID, []int64{}); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if got := bindsOf(t, item.ID); len(got) != 0 {
			t.Fatalf("after clear: got %v", got)
		}
	})

	t.Run("update_nil_group_ids_untouched", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		a := newMarketGroup(t, "A", common.StatusEnabled)
		item := newMarketItem(t, "item-1")
		if err := updateItemGroups(t, item.ID, []int64{a.ID}); err != nil {
			t.Fatalf("set [a]: %v", err)
		}
		// 不带 group_ids 的更新(如只改名)不得动绑定
		s := &MarketplaceService{}
		if err := s.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{DisplayName: new("新名")}); err != nil {
			t.Fatalf("update without group_ids: %v", err)
		}
		if got := bindsOf(t, item.ID); len(got) != 1 || got[0] != a.ID {
			t.Fatalf("binds changed: got %v", got)
		}
	})

	t.Run("update_dedup_and_nonpositive", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		a := newMarketGroup(t, "A", common.StatusEnabled)
		item := newMarketItem(t, "item-1")
		if err := updateItemGroups(t, item.ID, []int64{a.ID, 0, -3, a.ID}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if got := bindsOf(t, item.ID); len(got) != 1 || got[0] != a.ID {
			t.Fatalf("want cleaned [a]: got %v", got)
		}
	})

	t.Run("update_rejects_disabled_group", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		a := newMarketGroup(t, "A", common.StatusEnabled)
		d := newMarketGroup(t, "D", common.StatusDisabled)
		item := newMarketItem(t, "item-1")
		if err := updateItemGroups(t, item.ID, []int64{a.ID}); err != nil {
			t.Fatalf("set [a]: %v", err)
		}
		err := updateItemGroups(t, item.ID, []int64{d.ID})
		if err == nil || err.Error() != ErrGroupNotFound.Error() {
			t.Fatalf("want ErrGroupNotFound, got %v", err)
		}
		if got := bindsOf(t, item.ID); len(got) != 1 || got[0] != a.ID {
			t.Fatalf("failed update must not touch binds: got %v", got)
		}
	})

	t.Run("update_rejects_unknown_group", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		item := newMarketItem(t, "item-1")
		if err := updateItemGroups(t, item.ID, []int64{99999}); err == nil || err.Error() != ErrGroupNotFound.Error() {
			t.Fatalf("want ErrGroupNotFound, got %v", err)
		}
	})

	t.Run("disable_group_strips_bindings", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		g1, g2 := newMarketGroup(t, "G1", common.StatusEnabled), newMarketGroup(t, "G2", common.StatusEnabled)
		x, y := newMarketItem(t, "x"), newMarketItem(t, "y")
		for _, c := range []struct {
			item *model.MarketplaceItem
			ids  []int64
		}{{x, []int64{g1.ID, g2.ID}}, {y, []int64{g1.ID}}} {
			if err := updateItemGroups(t, c.item.ID, c.ids); err != nil {
				t.Fatalf("bind %s: %v", c.item.Name, err)
			}
		}

		gs := &MarketplaceGroupService{}
		if err := gs.Update(g1.ID, &dto.UpdateMarketplaceGroupReq{Status: new(common.StatusDisabled)}); err != nil {
			t.Fatalf("disable g1: %v", err)
		}
		if got := bindsOf(t, x.ID); len(got) != 1 || got[0] != g2.ID {
			t.Fatalf("x binds after disable: got %v", got)
		}
		if got := bindsOf(t, y.ID); len(got) != 0 {
			t.Fatalf("y binds after disable: got %v", got)
		}
		// 重新启用不恢复绑定
		if err := gs.Update(g1.ID, &dto.UpdateMarketplaceGroupReq{Status: new(common.StatusEnabled)}); err != nil {
			t.Fatalf("re-enable g1: %v", err)
		}
		if got := bindsOf(t, y.ID); len(got) != 0 {
			t.Fatalf("re-enable must not restore binds: got %v", got)
		}
	})

	t.Run("delete_group_hard_deletes_and_strips", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		g := newMarketGroup(t, "G", common.StatusEnabled)
		item := newMarketItem(t, "item-1")
		if err := updateItemGroups(t, item.ID, []int64{g.ID}); err != nil {
			t.Fatalf("bind: %v", err)
		}

		gs := &MarketplaceGroupService{}
		if err := gs.Delete(g.ID); err != nil {
			t.Fatalf("delete group: %v", err)
		}
		var n int64
		if err := model.DB.Unscoped().Model(&model.MarketplaceGroup{}).Where("id = ?", g.ID).Count(&n).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("group row must be physically gone, count=%d", n)
		}
		if got := bindsOf(t, item.ID); len(got) != 0 {
			t.Fatalf("binds after group delete: got %v", got)
		}
	})

	t.Run("same_name_recreate_after_delete", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		gs := &MarketplaceGroupService{}
		g, err := gs.Create(&dto.CreateMarketplaceGroupReq{Name: "同名"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := gs.Delete(g.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := gs.Create(&dto.CreateMarketplaceGroupReq{Name: "同名"}); err != nil {
			t.Fatalf("recreate same name must succeed after hard delete: %v", err)
		}
	})

	t.Run("published_filter_by_group", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		g1, g2 := newMarketGroup(t, "G1", common.StatusEnabled), newMarketGroup(t, "G2", common.StatusEnabled)
		x, y, z := newMarketItem(t, "x"), newMarketItem(t, "y"), newMarketItem(t, "z")
		if err := updateItemGroups(t, x.ID, []int64{g1.ID, g2.ID}); err != nil {
			t.Fatalf("bind x: %v", err)
		}
		if err := updateItemGroups(t, y.ID, []int64{g2.ID}); err != nil {
			t.Fatalf("bind y: %v", err)
		}

		s := &MarketplaceService{}
		names := func(items []dto.MarketplaceListItem) []string {
			out := make([]string, len(items))
			for i, it := range items {
				out[i] = it.Name
			}
			slices.Sort(out)
			return out
		}
		items, total, err := s.ListPublished(1, 20, "", "", 0, "")
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if total != 3 || len(items) != 3 {
			t.Fatalf("no filter: total=%d items=%v", total, names(items))
		}
		items, total, err = s.ListPublished(1, 20, "", "", g2.ID, "")
		if err != nil {
			t.Fatalf("list g2: %v", err)
		}
		// 排序键含 created_at DESC,条目先后不可假设,按集合断言
		if total != 2 || len(items) != 2 || !slices.Equal(names(items), []string{"x", "y"}) {
			t.Fatalf("filter g2: total=%d items=%v", total, names(items))
		}
		items, total, err = s.ListPublished(1, 20, "", "", g1.ID, "")
		if err != nil {
			t.Fatalf("list g1: %v", err)
		}
		if total != 1 || len(items) != 1 || items[0].Name != "x" {
			t.Fatalf("filter g1: total=%d items=%v", total, names(items))
		}
		// 无任何绑定的组过滤结果为空
		g3 := newMarketGroup(t, "G3", common.StatusEnabled)
		_, total, err = s.ListPublished(1, 20, "", "", g3.ID, "")
		if err != nil {
			t.Fatalf("list g3: %v", err)
		}
		if total != 0 {
			t.Fatalf("filter empty group: total=%d", total)
		}
		_ = z // z 未绑定,仅参与无过滤计数
	})

	t.Run("published_filter_by_tag", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		a, b, c := newMarketItem(t, "a"), newMarketItem(t, "b"), newMarketItem(t, "c")
		// 直接落 tags 逗号串(绕开标签字典,筛选只看串本身):a=web,search;b=webgl;c 无
		for _, it := range []*model.MarketplaceItem{a, b} {
			tags := "web,search"
			if it == b {
				tags = "webgl"
			}
			if err := model.DB.Model(it).Update("tags", tags).Error; err != nil {
				t.Fatalf("set tags of %s: %v", it.Name, err)
			}
		}

		s := &MarketplaceService{}
		names := func(items []dto.MarketplaceListItem) []string {
			out := make([]string, len(items))
			for i, it := range items {
				out[i] = it.Name
			}
			slices.Sort(out)
			return out
		}
		// "web" 不得误配 "webgl"(精确词元,非子串)
		items, total, err := s.ListPublished(1, 20, "", "", 0, "web")
		if err != nil {
			t.Fatalf("filter web: %v", err)
		}
		if total != 1 || len(items) != 1 || items[0].Name != "a" {
			t.Fatalf("filter web: total=%d items=%v", total, names(items))
		}
		// 首词 / 尾词 / 中间词与无标签项
		if _, total, err = s.ListPublished(1, 20, "", "", 0, "search"); err != nil || total != 1 {
			t.Fatalf("filter search: total=%d err=%v", total, err)
		}
		if _, total, err = s.ListPublished(1, 20, "", "", 0, "webgl"); err != nil || total != 1 {
			t.Fatalf("filter webgl: total=%d err=%v", total, err)
		}
		if _, total, err = s.ListPublished(1, 20, "", "", 0, "不存在"); err != nil || total != 0 {
			t.Fatalf("filter missing: total=%d err=%v", total, err)
		}
		_ = c
	})

	t.Run("list_fill_group_names_sorted", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		// sortOrder:B(2) < A(1),展示顺序应为 B,A(GetMarketplaceGroupsByIDs 无 ORDER BY,靠 Go 侧排)
		gs := &MarketplaceGroupService{}
		b, err := gs.Create(&dto.CreateMarketplaceGroupReq{Name: "B", SortOrder: 1})
		if err != nil {
			t.Fatalf("create B: %v", err)
		}
		a, err := gs.Create(&dto.CreateMarketplaceGroupReq{Name: "A", SortOrder: 5})
		if err != nil {
			t.Fatalf("create A: %v", err)
		}
		item := newMarketItem(t, "item-1")
		// 入参乱序:A 在前;期望输出按 sort_order:B,A
		if err := updateItemGroups(t, item.ID, []int64{a.ID, b.ID}); err != nil {
			t.Fatalf("bind: %v", err)
		}

		s := &MarketplaceService{}
		items, _, err := s.ListPublished(1, 20, "", "", 0, "")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("want 1 item, got %d", len(items))
		}
		got := items[0]
		if got.GroupIDs == nil || got.GroupNames == nil {
			t.Fatalf("GroupIDs/GroupNames must be non-nil, got %v %v", got.GroupIDs, got.GroupNames)
		}
		if len(got.GroupIDs) != 2 || got.GroupIDs[0] != b.ID || got.GroupIDs[1] != a.ID {
			t.Fatalf("GroupIDs must follow group sort_order: got %v", got.GroupIDs)
		}
		if len(got.GroupNames) != 2 || got.GroupNames[0] != "B" || got.GroupNames[1] != "A" {
			t.Fatalf("GroupNames must follow group sort_order: got %v", got.GroupNames)
		}
	})

	t.Run("detail_fill_empty_not_null", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		item := newMarketItem(t, "item-1")
		s := &MarketplaceService{}
		d, err := s.GetItemByID(item.ID)
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if d.GroupIDs == nil || len(d.GroupIDs) != 0 {
			t.Fatalf("ungrouped detail GroupIDs want empty non-nil, got %v", d.GroupIDs)
		}
		if d.GroupNames == nil || len(d.GroupNames) != 0 {
			t.Fatalf("ungrouped detail GroupNames want empty non-nil, got %v", d.GroupNames)
		}
	})

	t.Run("delete_item_strips_bindings", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		g := newMarketGroup(t, "G", common.StatusEnabled)
		item := newMarketItem(t, "item-1")
		if err := updateItemGroups(t, item.ID, []int64{g.ID}); err != nil {
			t.Fatalf("bind: %v", err)
		}
		s := &MarketplaceService{}
		if err := s.DeleteItem(item.ID); err != nil {
			t.Fatalf("delete item: %v", err)
		}
		if got := bindsOf(t, item.ID); len(got) != 0 {
			t.Fatalf("binds after item delete: got %v", got)
		}
	})

	t.Run("group_item_counts_published_only", func(t *testing.T) {
		setupMarketplaceGroupTest(t)
		g1, g2 := newMarketGroup(t, "G1", common.StatusEnabled), newMarketGroup(t, "G2", common.StatusEnabled)
		newMarketGroup(t, "EMPTY", common.StatusEnabled)
		x, y := newMarketItem(t, "x"), newMarketItem(t, "y")
		// 未上架(禁用)市场项的绑定不计入计数,与广场筛选口径一致
		off := &model.MarketplaceItem{
			AdminID: 1, Name: "off", Category: "instant",
			BillingType: "free", Status: common.StatusDisabled,
		}
		if err := model.DB.Create(off).Error; err != nil {
			t.Fatalf("create off: %v", err)
		}
		for _, c := range []struct {
			item *model.MarketplaceItem
			ids  []int64
		}{{x, []int64{g1.ID, g2.ID}}, {y, []int64{g2.ID}}, {off, []int64{g1.ID}}} {
			if err := updateItemGroups(t, c.item.ID, c.ids); err != nil {
				t.Fatalf("bind %s: %v", c.item.Name, err)
			}
		}

		gs := &MarketplaceGroupService{}
		items, err := gs.ListEnabled()
		if err != nil {
			t.Fatalf("list enabled: %v", err)
		}
		got := map[string]int64{}
		for _, it := range items {
			got[it.Name] = it.ItemCount
		}
		if got["G1"] != 1 || got["G2"] != 2 || got["EMPTY"] != 0 {
			t.Fatalf("want G1=1 G2=2 EMPTY=0, got %v", got)
		}
	})
}
