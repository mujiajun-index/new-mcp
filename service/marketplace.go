package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type MarketplaceService struct{}

// --- Admin operations ---

func (s *MarketplaceService) CreateItem(adminID int64, req *dto.CreateMarketplaceItemReq) (*dto.MarketplaceDetail, error) {
	configJSON, _ := json.Marshal(req.ConfigTemplate)
	configSourceJSON, _ := json.Marshal(req.ConfigTemplateSource)
	requiredEnvJSON, _ := json.Marshal(req.RequiredEnv)
	toolsSnapshotJSON := "[]"
	if req.ToolsSnapshot != nil {
		b, _ := json.Marshal(req.ToolsSnapshot)
		toolsSnapshotJSON = string(b)
	}
	tags := strings.Join(req.Tags, ",")

	item := &model.MarketplaceItem{
		AdminID:              adminID,
		Name:                 req.Name,
		DisplayName:          req.DisplayName,
		Description:          req.Description,
		IconURL:              req.IconURL,
		Category:             req.Category,
		Tags:                 tags,
		Version:              req.Version,
		TransportType:        req.TransportType,
		ConfigTemplate:       string(configJSON),
		AuthInstructions:     req.AuthInstructions,
		RepoURL:              req.RepoURL,
		InstallGuide:         req.InstallGuide,
		ConfigTemplateSource: string(configSourceJSON),
		RequiredEnv:          string(requiredEnvJSON),
		ToolsSnapshot:        toolsSnapshotJSON,
		Status:               common.StatusEnabled,
	}
	if req.Status != nil {
		item.Status = *req.Status
	}
	if item.Version == "" {
		item.Version = "1.0.0"
	}

	if err := item.Insert(); err != nil {
		return nil, err
	}
	return s.toDetail(item), nil
}

func (s *MarketplaceService) UpdateItem(itemID int64, req *dto.UpdateMarketplaceItemReq) error {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return err
	}
	if req.DisplayName != nil {
		item.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.IconURL != nil {
		item.IconURL = *req.IconURL
	}
	if req.Category != nil {
		item.Category = *req.Category
	}
	if req.Tags != nil {
		item.Tags = strings.Join(req.Tags, ",")
	}
	if req.Version != nil {
		item.Version = *req.Version
	}
	if req.TransportType != nil {
		item.TransportType = *req.TransportType
	}
	if req.ConfigTemplate != nil {
		b, _ := json.Marshal(req.ConfigTemplate)
		item.ConfigTemplate = string(b)
	}
	if req.AuthInstructions != nil {
		item.AuthInstructions = *req.AuthInstructions
	}
	if req.RepoURL != nil {
		item.RepoURL = *req.RepoURL
	}
	if req.InstallGuide != nil {
		item.InstallGuide = *req.InstallGuide
	}
	if req.ConfigTemplateSource != nil {
		b, _ := json.Marshal(req.ConfigTemplateSource)
		item.ConfigTemplateSource = string(b)
	}
	if req.RequiredEnv != nil {
		b, _ := json.Marshal(req.RequiredEnv)
		item.RequiredEnv = string(b)
	}
	if req.ToolsSnapshot != nil {
		b, _ := json.Marshal(req.ToolsSnapshot)
		item.ToolsSnapshot = string(b)
	}
	if req.Status != nil {
		item.Status = *req.Status
	}
	if req.SortOrder != nil {
		item.SortOrder = *req.SortOrder
	}
	return item.Update()
}

func (s *MarketplaceService) DeleteItem(itemID int64) error {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return err
	}
	return item.Delete()
}

func (s *MarketplaceService) ListItemsAdmin(page, pageSize int) ([]dto.MarketplaceListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	items, total, err := model.ListAllMarketplaceItems(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return s.toListItems(items), total, nil
}

// --- Public/User browsing ---

func (s *MarketplaceService) ListPublished(page, pageSize int, category, keyword string) ([]dto.MarketplaceListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	items, total, err := model.ListPublishedMarketplaceItems(offset, pageSize, category, keyword)
	if err != nil {
		return nil, 0, err
	}
	return s.toListItems(items), total, nil
}

func (s *MarketplaceService) GetPublished(itemID int64) (*dto.MarketplaceDetail, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	if item.Status != common.StatusEnabled {
		return nil, fmt.Errorf("marketplace item not available")
	}
	return s.toDetail(item), nil
}

func (s *MarketplaceService) GetItemByID(itemID int64) (*dto.MarketplaceDetail, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	return s.toDetail(item), nil
}

// --- User: Install ---

func (s *MarketplaceService) Install(userID int64, req *dto.InstallFromMarketplaceReq) (*dto.InstallResult, error) {
	item, err := model.GetMarketplaceItemByID(req.ItemID)
	if err != nil {
		return nil, fmt.Errorf("marketplace item not found")
	}
	if item.Status != common.StatusEnabled {
		return nil, fmt.Errorf("marketplace item not available")
	}

	// Determine service name: use override or marketplace item name
	svcName := item.Name
	if req.NameOverride != "" {
		svcName = req.NameOverride
	}

	svc := &model.McpService{
		UserID:            userID,
		Name:              svcName,
		DisplayName:       item.DisplayName,
		Description:       item.Description,
		TransportType:     item.TransportType,
		Config:            item.ConfigTemplate,
		ToolsCache:        item.ToolsSnapshot,
		Source:            "marketplace",
		MarketplaceItemID: &item.ID,
		IconURL:           item.IconURL,
		Tags:              item.Tags,
		Status:            common.StatusEnabled,
		HealthStatus:      common.HealthUnknown,
		AuthType:          "none",
	}

	if err := svc.Insert(); err != nil {
		return nil, err
	}

	_ = model.IncrementInstallCount(item.ID)

	return &dto.InstallResult{
		ServiceID: svc.ID,
		Name:      svc.Name,
	}, nil
}

// --- User: Rate/Review ---

func (s *MarketplaceService) CreateReview(userID int64, req *dto.CreateReviewReq) error {
	if _, err := model.GetMarketplaceItemByID(req.ItemID); err != nil {
		return fmt.Errorf("marketplace item not found")
	}

	existing, _ := model.GetUserReviewForItem(userID, req.ItemID)
	if existing != nil {
		existing.Rating = req.Rating
		existing.ReviewText = req.ReviewText
		if err := existing.Update(); err != nil {
			return err
		}
		_ = model.UpdateRating(req.ItemID)
		return nil
	}

	review := &model.MarketplaceReview{
		UserID:   userID,
		ItemID:   req.ItemID,
		Rating:   req.Rating,
		ReviewText: req.ReviewText,
	}
	if err := review.Insert(); err != nil {
		return err
	}

	_ = model.UpdateRating(req.ItemID)
	return nil
}

// --- Helpers ---

func (s *MarketplaceService) toDetail(item *model.MarketplaceItem) *dto.MarketplaceDetail {
	var configSource map[string]interface{}
	_ = json.Unmarshal([]byte(item.ConfigTemplateSource), &configSource)
	var requiredEnv []string
	_ = json.Unmarshal([]byte(item.RequiredEnv), &requiredEnv)
	var toolsSnapshot []interface{}
	_ = json.Unmarshal([]byte(item.ToolsSnapshot), &toolsSnapshot)

	var tags []string
	if item.Tags != "" {
		tags = strings.Split(item.Tags, ",")
	} else {
		tags = []string{}
	}

	return &dto.MarketplaceDetail{
		ID:                   item.ID,
		Name:                 item.Name,
		DisplayName:          item.DisplayName,
		Description:          item.Description,
		IconURL:              item.IconURL,
		Category:             item.Category,
		Tags:                 tags,
		Version:              item.Version,
		TransportType:        item.TransportType,
		ConfigTemplateSource: configSource,
		AuthInstructions:     item.AuthInstructions,
		RepoURL:              item.RepoURL,
		InstallGuide:         item.InstallGuide,
		RequiredEnv:          requiredEnv,
		InstallCount:         item.InstallCount,
		RatingAvg:            item.RatingAvg,
		RatingCount:          item.RatingCount,
		ToolsSnapshot:        toolsSnapshot,
		Status:               item.Status,
		CreatedAt:            item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:            item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (s *MarketplaceService) toListItems(items []model.MarketplaceItem) []dto.MarketplaceListItem {
	result := make([]dto.MarketplaceListItem, len(items))
	for i, item := range items {
		var tags []string
		if item.Tags != "" {
			tags = strings.Split(item.Tags, ",")
		} else {
			tags = []string{}
		}
		result[i] = dto.MarketplaceListItem{
			ID:            item.ID,
			Name:          item.Name,
			DisplayName:   item.DisplayName,
			Description:   item.Description,
			IconURL:       item.IconURL,
			Category:      item.Category,
			Tags:          tags,
			Version:       item.Version,
			TransportType: item.TransportType,
			InstallCount:  item.InstallCount,
			RatingAvg:     item.RatingAvg,
			RatingCount:   item.RatingCount,
			Status:        item.Status,
			SortOrder:     item.SortOrder,
			CreatedAt:     item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return result
}
