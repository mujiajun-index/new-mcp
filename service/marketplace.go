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

// ErrVirtualServiceNotListable 虚拟服务(vision/camera 等,transport_type='virtual')仅自有配置免费使用,
// 不可上架市场:其 config/凭证绑定配置者私有资源(如 vision_configs.ref_id),克隆或手动上架后无法作为
// 平台托管服务运行。与定价/自用模式无关的上架硬约束(D16/§11)。
var ErrVirtualServiceNotListable = errors.New("虚拟服务(视觉/摄像头等)仅支持自有配置免费使用,不可上架到服务市场")

// ErrServiceNotOwned 克隆上架的源服务不属于当前管理员(§11):管理员只能上架自己账户下的自有服务,
// 不得克隆/上架其他用户的服务。
var ErrServiceNotOwned = errors.New("无权克隆该服务:仅可克隆自己账户下的自有服务")

// ErrNegativePrice 价格不能为负数(§5.5)。
var ErrNegativePrice = errors.New("价格不能为负数")

// ErrTagNotInDictionary 提交的标签不在启用标签库中(§11)。
var ErrTagNotInDictionary = errors.New("标签不在标签库中,请先在标签库中创建")

// ErrGroupNotFound 市场分组不存在或未启用(§11)。
var ErrGroupNotFound = errors.New("市场分组不存在或未启用")

// validatePrice 校验价格非负(§5.5)。
func validatePrice(price float64) error {
	if price < 0 {
		return ErrNegativePrice
	}
	return nil
}

// validateTags 校验标签均存在于启用标签库(去重后匹配,§11)。
func validateTags(tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	unique := make([]string, 0, len(tags))
	for _, t := range tags {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		unique = append(unique, t)
	}
	if len(unique) == 0 {
		return nil
	}
	count, err := model.CountEnabledTagsByNames(unique)
	if err != nil {
		return err
	}
	if int64(len(unique)) != count {
		return ErrTagNotInDictionary
	}
	return nil
}

// validateGroupID 校验分组存在且启用;groupID 为 nil 或 <=0 视为未分组(允许)。
func validateGroupID(groupID *int64) error {
	if groupID == nil || *groupID <= 0 {
		return nil
	}
	if _, err := model.GetEnabledMarketplaceGroupByID(*groupID); err != nil {
		return ErrGroupNotFound
	}
	return nil
}

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
	// 虚拟服务(vision/camera 等)不可上架(D16/§11):virtual 是内置 handler,非平台可托管传输类型。
	if req.TransportType == "virtual" {
		return nil, ErrVirtualServiceNotListable
	}
	if err := validatePrice(req.PricePerCall); err != nil {
		return nil, err
	}
	if err := validateTags(req.Tags); err != nil {
		return nil, err
	}
	if err := validateGroupID(req.GroupID); err != nil {
		return nil, err
	}
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
		GroupID:              req.GroupID,
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
	// 校验(仅对传入字段,§11/§5.5)
	if req.PricePerCall != nil {
		if err := validatePrice(*req.PricePerCall); err != nil {
			return err
		}
	}
	if req.Tags != nil {
		if err := validateTags(req.Tags); err != nil {
			return err
		}
	}
	if req.GroupID != nil {
		if err := validateGroupID(req.GroupID); err != nil {
			return err
		}
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
	if req.GroupID != nil {
		item.GroupID = req.GroupID
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
	for _, it := range items {
		if err := validatePrice(it.PricePerCall); err != nil {
			return 0, fmt.Errorf("%w: 市场项 id=%d", err, it.ID)
		}
	}
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

// CloneFromService 从**管理员自己账户下**的自有服务克隆上架(D14/§11):深拷贝 transport/config/auth/tools,
// 与源服务无关联。仅允许克隆 svc.UserID==adminID 的服务(不得上架其他用户的服务);虚拟服务(virtual)拒绝。
// 保留源凭证但调用方应替换为平台凭证(前端高亮提示)。非自用模式须显式定价。
func (s *MarketplaceService) CloneFromService(adminID int64, req *dto.CloneMarketplaceReq) (*dto.MarketplaceDetail, error) {
	svc, err := model.GetServiceByIDWithoutUser(req.FromServiceID)
	if err != nil {
		return nil, fmt.Errorf("源服务不存在")
	}
	// 仅允许克隆自己账户下的服务(§11):管理员不得上架其他用户的服务。
	if svc.UserID != adminID {
		return nil, ErrServiceNotOwned
	}
	// 虚拟服务(vision/camera 等)不可上架(D16/§11):其 config 指向配置者私有资源(如 vision_configs.ref_id),
	// 克隆后无法作为平台托管服务运行。
	if svc.TransportType == "virtual" {
		return nil, ErrVirtualServiceNotListable
	}
	if err := validatePrice(req.PricePerCall); err != nil {
		return nil, err
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

func (s *MarketplaceService) ListPublished(page, pageSize int, category, keyword string, groupID int64) ([]dto.MarketplaceListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	items, total, err := model.ListPublishedMarketplaceItems(offset, pageSize, category, keyword, groupID)
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

	groupName := ""
	if item.GroupID != nil {
		if g, e := model.GetMarketplaceGroupByID(*item.GroupID); e == nil {
			groupName = g.Name
		}
	}

	return &dto.MarketplaceDetail{
		ID:                   item.ID,
		Name:                 item.Name,
		DisplayName:          item.DisplayName,
		Description:          item.Description,
		IconURL:              item.IconURL,
		Category:             item.Category,
		GroupID:              item.GroupID,
		GroupName:            groupName,
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
	// 批量取分组名(避免 N+1)
	groupIDs := make([]int64, 0, len(items))
	for _, it := range items {
		if it.GroupID != nil {
			groupIDs = append(groupIDs, *it.GroupID)
		}
	}
	groupNameByID := make(map[int64]string)
	if len(groupIDs) > 0 {
		if groups, e := model.GetMarketplaceGroupsByIDs(groupIDs); e == nil {
			for _, g := range groups {
				groupNameByID[g.ID] = g.Name
			}
		}
	}

	result := make([]dto.MarketplaceListItem, len(items))
	for i, item := range items {
		var tags []string
		if item.Tags != "" {
			tags = strings.Split(item.Tags, ",")
		} else {
			tags = []string{}
		}
		var groupName string
		if item.GroupID != nil {
			groupName = groupNameByID[*item.GroupID]
		}
		result[i] = dto.MarketplaceListItem{
			ID:            item.ID,
			Name:          item.Name,
			DisplayName:   item.DisplayName,
			Description:   item.Description,
			IconURL:       item.IconURL,
			Category:      item.Category,
			GroupID:       item.GroupID,
			GroupName:     groupName,
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
