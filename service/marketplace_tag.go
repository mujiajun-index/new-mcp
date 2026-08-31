package service

import (
	"errors"
	"regexp"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// MarketplaceTagService 市场标签字典管理(§11)。市场项 tags 字段值须存在于本库启用记录。
type MarketplaceTagService struct{}

// ErrTagNameExists 标签已存在。
var ErrTagNameExists = errors.New("标签已存在")

// tagColorRe 标签颜色格式:#RRGGBB。
var tagColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// normalizeTagColor 规范化标签颜色(空=默认灰;非法格式报错)。
func normalizeTagColor(color string) (string, error) {
	color = strings.TrimSpace(color)
	if color == "" {
		return "", nil
	}
	if !tagColorRe.MatchString(color) {
		return "", errors.New("颜色格式须为 #RRGGBB")
	}
	return strings.ToLower(color), nil
}

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
	color, err := normalizeTagColor(req.Color)
	if err != nil {
		return nil, err
	}
	t := &model.MarketplaceTag{
		Name:        req.Name,
		Description: req.Description,
		Color:       color,
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
	oldName, wasEnabled := t.Name, t.Status == common.StatusEnabled
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
	if req.Color != nil {
		color, err := normalizeTagColor(*req.Color)
		if err != nil {
			return err
		}
		t.Color = color
	}
	if req.SortOrder != nil {
		t.SortOrder = *req.SortOrder
	}
	if req.Status != nil {
		t.Status = *req.Status
	}
	// 引用同步(与字典更新同事务):市场项 tags 按名引用本字典,改名须整词替换、
	// 改禁用须摘除,否则存量引用项再次保存时报"标签不在标签库中"。
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(t).Error; err != nil {
			return err
		}
		if t.Name != oldName {
			if err := model.ReplaceMarketplaceTagName(tx, oldName, t.Name); err != nil {
				return err
			}
		}
		if wasEnabled && t.Status != common.StatusEnabled {
			return model.RemoveMarketplaceTagName(tx, t.Name)
		}
		return nil
	})
}

func (s *MarketplaceTagService) Delete(id int64) error {
	t, err := model.GetMarketplaceTagByID(id)
	if err != nil {
		return err
	}
	// 硬删:name 唯一索引含软删行,软删后同名重建会撞唯一键;标签无 ID 级引用
	// (市场项按名引用,同事务摘除),硬删安全。摘除维持"市场项 tags ⊆ 启用字典"不变量。
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(t).Error; err != nil {
			return err
		}
		return model.RemoveMarketplaceTagName(tx, t.Name)
	})
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
		Color:       t.Color,
		SortOrder:   t.SortOrder,
		Status:      t.Status,
		CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
