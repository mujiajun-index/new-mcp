package service

import (
	"errors"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// MarketplaceTagService 市场标签字典管理(§11)。市场项 tags 字段值须存在于本库启用记录。
type MarketplaceTagService struct{}

// ErrTagNameExists 标签已存在。
var ErrTagNameExists = errors.New("标签已存在")

func (s *MarketplaceTagService) List(status, page, pageSize int) ([]dto.MarketplaceTagItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	tags, total, err := model.ListAllMarketplaceTags(status, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return s.toList(tags), total, nil
}

// ListEnabled 返回启用标签(供管理员编辑市场项时多选)。
func (s *MarketplaceTagService) ListEnabled() ([]dto.MarketplaceTagItem, error) {
	tags, err := model.ListEnabledMarketplaceTags()
	if err != nil {
		return nil, err
	}
	return s.toList(tags), nil
}

func (s *MarketplaceTagService) Get(id int64) (*dto.MarketplaceTagItem, error) {
	t, err := model.GetMarketplaceTagByID(id)
	if err != nil {
		return nil, err
	}
	return s.toItem(t), nil
}

func (s *MarketplaceTagService) Create(req *dto.CreateMarketplaceTagReq) (*dto.MarketplaceTagItem, error) {
	exists, err := model.CheckMarketplaceTagNameExists(req.Name, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrTagNameExists
	}
	t := &model.MarketplaceTag{
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
		Status:      common.StatusEnabled,
	}
	if req.Status != nil {
		t.Status = *req.Status
	}
	if err := t.Insert(); err != nil {
		return nil, err
	}
	return s.toItem(t), nil
}

func (s *MarketplaceTagService) Update(id int64, req *dto.UpdateMarketplaceTagReq) error {
	t, err := model.GetMarketplaceTagByID(id)
	if err != nil {
		return err
	}
	if req.Name != nil {
		exists, err := model.CheckMarketplaceTagNameExists(*req.Name, id)
		if err != nil {
			return err
		}
		if exists {
			return ErrTagNameExists
		}
		t.Name = *req.Name
	}
	if req.Description != nil {
		t.Description = *req.Description
	}
	if req.SortOrder != nil {
		t.SortOrder = *req.SortOrder
	}
	if req.Status != nil {
		t.Status = *req.Status
	}
	return t.Update()
}

func (s *MarketplaceTagService) Delete(id int64) error {
	t, err := model.GetMarketplaceTagByID(id)
	if err != nil {
		return err
	}
	return t.Delete()
}

func (s *MarketplaceTagService) toList(tags []model.MarketplaceTag) []dto.MarketplaceTagItem {
	items := make([]dto.MarketplaceTagItem, len(tags))
	for i, t := range tags {
		items[i] = *s.toItem(&t)
	}
	return items
}

func (s *MarketplaceTagService) toItem(t *model.MarketplaceTag) *dto.MarketplaceTagItem {
	return &dto.MarketplaceTagItem{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		SortOrder:   t.SortOrder,
		Status:      t.Status,
		CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
