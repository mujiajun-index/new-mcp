package service

import (
	"errors"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// MarketplaceGroupService 市场分组(业务分类)管理,管理员全局范围(§11)。
type MarketplaceGroupService struct{}

// ErrGroupNameExists 分组名称已存在。
var ErrGroupNameExists = errors.New("分组名称已存在")

func (s *MarketplaceGroupService) List(status, page, pageSize int) ([]dto.MarketplaceGroupItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	groups, total, err := model.ListAllMarketplaceGroups(status, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return s.toList(groups), total, nil
}

// ListAllForAdmin 返回所有分组（不分页，仅供管理 UI 用，避免 MaxPageSize 截断丢失新条目）。
func (s *MarketplaceGroupService) ListAllForAdmin() ([]dto.MarketplaceGroupItem, error) {
	groups, err := model.ListAllMarketplaceGroupsForAdmin()
	if err != nil {
		return nil, err
	}
	return s.toList(groups), nil
}

// ListEnabled 返回启用分组(供广场公开端点 / 左侧筛选)。
func (s *MarketplaceGroupService) ListEnabled() ([]dto.MarketplaceGroupItem, error) {
	groups, err := model.ListEnabledMarketplaceGroups()
	if err != nil {
		return nil, err
	}
	return s.toList(groups), nil
}

func (s *MarketplaceGroupService) Get(id int64) (*dto.MarketplaceGroupItem, error) {
	g, err := model.GetMarketplaceGroupByID(id)
	if err != nil {
		return nil, err
	}
	return s.toItem(g), nil
}

func (s *MarketplaceGroupService) Create(req *dto.CreateMarketplaceGroupReq) (*dto.MarketplaceGroupItem, error) {
	exists, err := model.CheckMarketplaceGroupNameExists(req.Name, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrGroupNameExists
	}
	g := &model.MarketplaceGroup{
		Name:        req.Name,
		Description: req.Description,
		IconURL:     req.IconURL,
		SortOrder:   req.SortOrder,
		Status:      common.StatusEnabled,
	}
	if req.Status != nil {
		g.Status = *req.Status
	}
	if err := g.Insert(); err != nil {
		return nil, err
	}
	return s.toItem(g), nil
}

func (s *MarketplaceGroupService) Update(id int64, req *dto.UpdateMarketplaceGroupReq) error {
	g, err := model.GetMarketplaceGroupByID(id)
	if err != nil {
		return err
	}
	wasEnabled := g.Status == common.StatusEnabled
	if req.Name != nil {
		exists, err := model.CheckMarketplaceGroupNameExists(*req.Name, id)
		if err != nil {
			return err
		}
		if exists {
			return ErrGroupNameExists
		}
		g.Name = *req.Name
	}
	if req.Description != nil {
		g.Description = *req.Description
	}
	if req.IconURL != nil {
		g.IconURL = *req.IconURL
	}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}
	if req.Status != nil {
		g.Status = *req.Status
	}
	// 引用同步(与分组更新同事务):绑定行按 ID 引用,改名无需同步;禁用须摘除,
	// 维持"市场项绑定 ⊆ 启用分组"不变量(否则存量绑定项再次保存时报 ErrGroupNotFound)。
	// 重新启用不恢复已摘除的绑定,由管理员重新勾选。
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(g).Error; err != nil {
			return err
		}
		if wasEnabled && g.Status != common.StatusEnabled {
			return model.DeleteMarketplaceItemGroupsByGroupID(tx, g.ID)
		}
		return nil
	})
}

func (s *MarketplaceGroupService) Delete(id int64) error {
	g, err := model.GetMarketplaceGroupByID(id)
	if err != nil {
		return err
	}
	// 硬删:name 唯一索引含软删行,软删后同名重建会撞唯一键;绑定表按 ID 引用,同事务摘除。
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.DeleteMarketplaceItemGroupsByGroupID(tx, g.ID); err != nil {
			return err
		}
		return tx.Unscoped().Delete(g).Error
	})
}

func (s *MarketplaceGroupService) toList(groups []model.MarketplaceGroup) []dto.MarketplaceGroupItem {
	items := make([]dto.MarketplaceGroupItem, len(groups))
	for i, g := range groups {
		items[i] = *s.toItem(&g)
	}
	return items
}

func (s *MarketplaceGroupService) toItem(g *model.MarketplaceGroup) *dto.MarketplaceGroupItem {
	return &dto.MarketplaceGroupItem{
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		IconURL:     g.IconURL,
		SortOrder:   g.SortOrder,
		Status:      g.Status,
		CreatedAt:   g.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
