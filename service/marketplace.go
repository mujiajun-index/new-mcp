package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type MarketplaceService struct{}

// ErrExplicitPricingRequired 非自用模式下市场上架/启用须显式定价(§5.6)。
var ErrExplicitPricingRequired = errors.New("非自用模式下,市场上架/启用必须显式定价(设置价格或标记免费)")

// explicitlyPriced 判断是否"已显式定价":free 或 (per_call 且 price>0)。
func explicitlyPriced(billingType string, price float64) bool {
	if billingType == "free" {
		return true
	}
	return billingType == "per_call" && price > 0
}

// requireExplicitPricingIfNotSelfUse 非自用模式时,校验市场项已显式定价(上架/启用门控,§5.6)。
// 自用模式不校验(可继承全局默认)。
func requireExplicitPricingIfNotSelfUse(billingType string, price float64) error {
	if model.GetOptionBool("SelfUseModeEnabled") {
		return nil
	}
	if !explicitlyPriced(billingType, price) {
		return ErrExplicitPricingRequired
	}
	return nil
}

// encryptConfigTemplate 加密平台上游配置/凭证后落库(§4.3)。
// common.Encrypt 在未配置 CRYPTO_SECRET 时退化为 base64(与 vision_configs 等一致),不影响功能。
func encryptConfigTemplate(plain string) string {
	enc, err := common.Encrypt(plain)
	if err != nil {
		return plain // 加密失败保留明文,避免阻断上架(调用仍可用,仅 at-rest 保护降级)
	}
	return enc
}

// --- Admin operations ---

func (s *MarketplaceService) CreateItem(adminID int64, req *dto.CreateMarketplaceItemReq) (*dto.MarketplaceDetail, error) {
	billingType := req.BillingType
	if billingType == "" {
		billingType = "per_call"
	}
	// 非自用模式强制定价门控(§5.6)
	if err := requireExplicitPricingIfNotSelfUse(billingType, req.PricePerCall); err != nil {
		return nil, err
	}

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
		ConfigTemplate:       encryptConfigTemplate(string(configJSON)), // 平台凭证加密落库
		AuthInstructions:     req.AuthInstructions,
		RepoURL:              req.RepoURL,
		InstallGuide:         req.InstallGuide,
		ConfigTemplateSource: string(configSourceJSON),
		RequiredEnv:          string(requiredEnvJSON),
		ToolsSnapshot:        toolsSnapshotJSON,
		BillingType:          billingType,
		PricePerCall:         req.PricePerCall,
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
	billing.InvalidatePricingCacheItem(item.ID)
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
		item.ConfigTemplate = encryptConfigTemplate(string(b)) // 平台凭证加密落库
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
	// 商业化定价
	if req.BillingType != nil {
		item.BillingType = *req.BillingType
	}
	if req.PricePerCall != nil {
		item.PricePerCall = *req.PricePerCall
	}
	if item.BillingType == "" {
		item.BillingType = "per_call"
	}
	// 启用状态下非自用模式须显式定价(§5.6);下架(非启用)不校验
	if item.Status == common.StatusEnabled {
		if err := requireExplicitPricingIfNotSelfUse(item.BillingType, item.PricePerCall); err != nil {
			return err
		}
	}
	if err := item.Update(); err != nil {
		return err
	}
	billing.InvalidatePricingCacheItem(item.ID)
	return nil
}

func (s *MarketplaceService) DeleteItem(itemID int64) error {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return err
	}
	if err := item.Delete(); err != nil {
		return err
	}
	// 硬删除(软删):已添加引用的 mcp_services 行保留(resolver 调用时会因 item 不可用而失败退款)。
	// V2 提供显式级联清理 + 引用悬空检测(§11)。
	billing.InvalidatePricingCacheItem(itemID)
	return nil
}

// BatchUpdatePricing 批量设置已上架市场服务价格(§5.5)。非自用模式逐条校验显式定价。
func (s *MarketplaceService) BatchUpdatePricing(items []dto.BatchPricingItem) (int64, error) {
	if !model.GetOptionBool("SelfUseModeEnabled") {
		for _, it := range items {
			if !explicitlyPriced(it.BillingType, it.PricePerCall) {
				return 0, fmt.Errorf("%w: 市场项 id=%d", ErrExplicitPricingRequired, it.ID)
			}
		}
	}
	updates := make([]model.MarketplacePricingUpdate, len(items))
	for i, it := range items {
		updates[i] = model.MarketplacePricingUpdate{
			ID:           it.ID,
			BillingType:  it.BillingType,
			PricePerCall: it.PricePerCall,
		}
	}
	affected, err := model.UpdateMarketplacePricing(updates)
	if err != nil {
		return 0, err
	}
	// 批量改价后统一失效缓存,使变更即时对所有引用生效
	for _, it := range items {
		billing.InvalidatePricingCacheItem(it.ID)
	}
	return affected, nil
}

// CloneFromService 从自有服务克隆上架(D14/§11):深拷贝 transport/config/auth/tools,
// 与源服务无关联。保留源凭证但调用方应替换为平台凭证(前端高亮提示)。非自用模式须显式定价。
func (s *MarketplaceService) CloneFromService(adminID int64, req *dto.CloneMarketplaceReq) (*dto.MarketplaceDetail, error) {
	svc, err := model.GetServiceByIDWithoutUser(req.FromServiceID)
	if err != nil {
		return nil, fmt.Errorf("源服务不存在")
	}
	billingType := req.BillingType
	if billingType == "" {
		billingType = "per_call"
	}
	if err := requireExplicitPricingIfNotSelfUse(billingType, req.PricePerCall); err != nil {
		return nil, err
	}

	item := &model.MarketplaceItem{
		AdminID:       adminID,
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		Category:      "instant",
		Version:       "1.0.0",
		TransportType: svc.TransportType,
		ConfigTemplate: encryptConfigTemplate(svc.Config), // 克隆源凭证并加密;前端提示替换为平台凭证
		AuthInstructions: svc.AuthType,
		ConfigTemplateSource: svc.AuthConfig,
		RequiredEnv:   "[]",
		ToolsSnapshot: svc.ToolsCache,
		BillingType:   billingType,
		PricePerCall:  req.PricePerCall,
		Status:        common.StatusEnabled,
	}
	if item.DisplayName == "" {
		item.DisplayName = svc.DisplayName
	}
	if item.Description == "" {
		item.Description = svc.Description
	}

	if err := item.Insert(); err != nil {
		return nil, err
	}
	billing.InvalidatePricingCacheItem(item.ID)
	return s.toDetail(item), nil
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

// --- User: 引用式安装(AddToMyServices,§4.2/§11)---

// AddToMyServices 把市场项添加为用户的**引用服务**:在 mcp_services 建一条 source=marketplace
// 引用行,**config 留空(不复制上游配置/凭证)**、transport_type 置 "marketplace" 哨兵、复制 tools_cache 快照。
// 上游连接/凭证仍由平台在 marketplace_items 侧托管;调用时 resolver 按 marketplace_item_id 注入平台 session(§6.1)。
// 去重:同一用户对同一市场项仅一份引用,重复添加返回已有引用。
func (s *MarketplaceService) AddToMyServices(userID, itemID int64) (*dto.InstallResult, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, fmt.Errorf("marketplace item not found")
	}
	if item.Status != common.StatusEnabled {
		return nil, fmt.Errorf("marketplace item not available")
	}

	// 去重:已存在引用则直接返回
	if existing, e := model.GetMarketplaceReferenceByUser(userID, itemID); e == nil && existing != nil {
		return &dto.InstallResult{ServiceID: existing.ID, Name: existing.Name}, nil
	}

	svc := &model.McpService{
		UserID:            userID,
		Name:              item.Name,
		DisplayName:       item.DisplayName,
		Description:       item.Description,
		TransportType:     "marketplace", // 哨兵值:resolver 见此改用平台 session(真实 transport 在调用时从 item 注入)
		Config:            "{}",          // 空:不复制上游配置/凭证(平台托管)
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
		BillingType:          item.BillingType,
		PricePerCall:         item.PricePerCall,
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
			BillingType:   item.BillingType,
			PricePerCall:  item.PricePerCall,
		}
	}
	return result
}
