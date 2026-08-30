package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
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

// ErrOnlyInstantRefreshable 仅平台托管(instant)市场项支持手动刷新快照:
// source 类目为用户自行部署形态,平台侧无上游连接,快照由管理员通过编辑接口维护。
var ErrOnlyInstantRefreshable = errors.New("仅开箱即用(平台托管)市场项支持刷新快照")

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

// --- 平台凭证掩码(管理端详情回显 / 保存回填) ---

// secretMask 短凭证(<10 字符)的整体掩码:过短的值连首尾都不保留,避免被整体还原。
const secretMask = "****"

// maskSecret 把凭证值掩码为「首4...尾4」(API Key 展示惯例):保留首尾便于管理员比对是否换了钥匙,
// 中间至少隐藏 2 个字符;不足 10 字符的值整体遮蔽。
func maskSecret(v string) string {
	r := []rune(v)
	if len(r) < 10 {
		return secretMask
	}
	return string(r[:4]) + "..." + string(r[len(r)-4:])
}

// configCredentialKeys config_template 中承载凭证值的两个顶层字段:HTTP 类的 headers、stdio 的 env。
var configCredentialKeys = [2]string{"headers", "env"}

// maskConfigCredentials 返回 config 的浅拷贝,其中 headers/env 每项的**值**替换为掩码;
// url/command/args 等非凭证结构保持明文(编辑 URL 不受影响)。不修改原 map。
func maskConfigCredentials(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	out := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	for _, key := range configCredentialKeys {
		m, ok := out[key].(map[string]interface{})
		if !ok || len(m) == 0 {
			continue
		}
		masked := make(map[string]interface{}, len(m))
		for hk, hv := range m {
			s, isStr := hv.(string)
			if !isStr {
				masked[hk] = hv
				continue
			}
			// Bearer 保留 scheme 前缀、仅掩码 token(如 Bearer tok-...cdef):
			// 前端按 "Bearer " 前缀识别认证类型并重组头,整值掩码会让该逻辑失配。
			if key == "headers" && hk == "Authorization" && strings.HasPrefix(s, "Bearer ") {
				masked[hk] = "Bearer " + maskSecret(strings.TrimPrefix(s, "Bearer "))
				continue
			}
			masked[hk] = maskSecret(s)
		}
		out[key] = masked
	}
	return out
}

// mergeMaskedCredentials 保存时回填:入参 config 里与「库内值的掩码」逐字相同的首尾掩码值,
// 替换回库内明文(管理员未改动凭证、掩码原样传回的场景);输入了新值(≠掩码)则原样保留。
// 就地修改 incoming。
func mergeMaskedCredentials(incoming, stored map[string]interface{}) {
	if incoming == nil || stored == nil {
		return
	}
	for _, key := range configCredentialKeys {
		in, ok := incoming[key].(map[string]interface{})
		if !ok {
			continue
		}
		st, ok := stored[key].(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range in {
			s, isStr := v.(string)
			if !isStr {
				continue
			}
			old, exists := st[k]
			if !exists {
				continue
			}
			oldStr, oldIsStr := old.(string)
			if !oldIsStr {
				continue
			}
			// Bearer:token 部分与库内 token 的掩码相同 → 回填完整原值(与 maskConfigCredentials 对称)
			if key == "headers" && k == "Authorization" &&
				strings.HasPrefix(s, "Bearer ") && strings.HasPrefix(oldStr, "Bearer ") {
				if strings.TrimPrefix(s, "Bearer ") == maskSecret(strings.TrimPrefix(oldStr, "Bearer ")) {
					in[k] = oldStr
				}
				continue
			}
			if s == maskSecret(oldStr) {
				in[k] = oldStr
			}
		}
	}
}

// --- Admin operations ---

// 注:市场项仅支持"从自有服务克隆上架"(CloneFromService),已移除手动创建接口
// (与"添加服务"功能重复,且无法保证 transport/config 可被平台托管运行)。

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
		// 管理端详情回显的是首尾掩码:未改动的凭证以掩码原样传回,先回填库内明文再加密落库;
		// 输入了新值的项(≠库内值的掩码)不受影响。存量明文行 Decrypt 失败回退原值。
		stored := map[string]interface{}{}
		if plain, dErr := common.Decrypt(item.ConfigTemplate); dErr == nil && plain != "" {
			_ = json.Unmarshal([]byte(plain), &stored)
		} else {
			_ = json.Unmarshal([]byte(item.ConfigTemplate), &stored)
		}
		mergeMaskedCredentials(req.ConfigTemplate, stored)
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
	if req.ResourcesSnapshot != nil {
		b, _ := json.Marshal(req.ResourcesSnapshot)
		item.ResourcesSnapshot = string(b)
	}
	if req.PromptsSnapshot != nil {
		b, _ := json.Marshal(req.PromptsSnapshot)
		item.PromptsSnapshot = string(b)
	}
	if req.Status != nil {
		item.Status = *req.Status
	}
	if req.SortOrder != nil {
		item.SortOrder = *req.SortOrder
	}
	// 独占进程切换(仅 stdio 条目):会话池键控方式改变(行键↔条目键),旧会话必须
	// 踢掉按新模式重建,否则共享→独占后旧共享进程仍被全体复用、反向则旧行进程残留。
	kickSessions := req.ConfigTemplate != nil
	if req.IsolatedProcess != nil && item.TransportType == string(transport.TypeStdio) && item.IsolatedProcess != *req.IsolatedProcess {
		item.IsolatedProcess = *req.IsolatedProcess
		kickSessions = true
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
	// 平台上游配置变更后,该市场项全部引用服务的池内会话仍带旧配置/旧凭证,
	// 踢掉待下次调用按新配置重连(引用服务众多,不做预连,按需重连即可)。
	// 进程模式切换(共享↔独占)同理并入同一次踢除。
	if kickSessions && SessionPool != nil {
		SessionPool.RemoveByMarketplaceItem(item.ID)
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

// RefreshItemSnapshots 管理端手动刷新市场项快照:用平台托管的上游配置临时直连上游
// (与服务详情「刷新工具」同源拉取),把 tools/resources/prompts 写回 marketplace_items 快照列。
// 临时连接不落库、不入会话池,用完即关。
func (s *MarketplaceService) RefreshItemSnapshots(itemID int64) (*dto.MarketplaceRefreshResult, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	if item.Category != "instant" {
		return nil, ErrOnlyInstantRefreshable
	}
	// 物化平台上游连接配置(与 materializeMarketplace 一致:解密 config_template、还原真实 transport)
	tmp := &model.McpService{
		Name:          item.Name,
		TransportType: item.TransportType,
		Config:        item.ConfigTemplate,
	}
	if plain, dErr := common.Decrypt(item.ConfigTemplate); dErr == nil && plain != "" {
		tmp.Config = plain
	}
	adapter := bridge.CreateAdapter(tmp)
	if adapter == nil {
		return nil, fmt.Errorf("不支持的传输类型: %s", item.TransportType)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := adapter.Connect(ctx); err != nil {
		return nil, err
	}
	defer adapter.Close()

	toolsJSON, resourcesJSON, promptsJSON := bridge.FetchAdapterCaches(ctx, adapter)
	updates := map[string]interface{}{
		"tools_snapshot":     toolsJSON,
		"resources_snapshot": resourcesJSON,
		"prompts_snapshot":   promptsJSON,
	}
	// 握手拿到的真实服务版本/协议版本一并落库;上游没给时不动旧值
	if v := adapter.GetProtocolVersion(); v != "" {
		updates["protocol_version"] = v
	}
	if si := adapter.GetServerInfo(); si != nil {
		if b, err := json.Marshal(si); err == nil {
			updates["server_info"] = string(b)
		}
	}
	if err := model.DB.Model(&model.MarketplaceItem{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
		return nil, err
	}

	var result dto.MarketplaceRefreshResult
	result.ToolsCount = countJSONItems(toolsJSON)
	var rc struct {
		Resources json.RawMessage `json:"resources"`
		Templates json.RawMessage `json:"templates"`
	}
	_ = json.Unmarshal([]byte(resourcesJSON), &rc)
	result.ResourcesCount = countJSONItems(string(rc.Resources))
	result.TemplatesCount = countJSONItems(string(rc.Templates))
	result.PromptsCount = countJSONItems(promptsJSON)
	return &result, nil
}

// countJSONItems 统计 JSON 数组的元素个数(非数组/解析失败返回 0)。
func countJSONItems(s string) int {
	var arr []interface{}
	if json.Unmarshal([]byte(s), &arr) != nil {
		return 0
	}
	return len(arr)
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

// entryKindLabel 条目种类的中文标签(校验错误文案用)。
func entryKindLabel(kind string) string {
	switch kind {
	case billing.EntryKindResource:
		return "资源"
	case billing.EntryKindPrompt:
		return "提示"
	default:
		return "工具"
	}
}

// SetItemEntryPrices 全量替换市场项的条目级定价(§5.2 条目维度):prices 为管理员期望
// 的完整条目价列表,不在其中的条目按缺省回退(工具→服务统一价,资源/提示→免费);
// 资源/提示显式继承服务价用 billing_type=inherit 行表达(价格强制归零存储)。
// 校验:条目必须存在于快照(条目键与网关计费同口径:工具名/资源上游 URI/提示名;
// 资源模板不在集合中,天然拒绝——按模板展开的具体 URI 读取按资源条目计费,模板价
// 无意义)、价格非负、per_call 须 price>0(同 explicitlyPriced 口径)、(kind,name) 去重。
// 事务删旧插新后失效价格缓存。服务级显式定价门控不受影响(非自用模式启用项仍须
// 服务级显式定价,条目价只是覆盖/补充)。
func (s *MarketplaceService) SetItemEntryPrices(itemID int64, prices []dto.MarketplaceEntryPrice) error {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return fmt.Errorf("市场项不存在")
	}

	// 快照合法键集合(kind+"\x00"+条目名)
	valid := map[string]bool{}
	var tools []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(item.ToolsSnapshot), &tools) == nil {
		for _, t := range tools {
			if t.Name != "" {
				valid[billing.EntryKindTool+"\x00"+t.Name] = true
			}
		}
	}
	var res struct {
		Resources []struct {
			URI string `json:"uri"`
		} `json:"resources"`
	}
	if json.Unmarshal([]byte(item.ResourcesSnapshot), &res) == nil {
		for _, r := range res.Resources {
			if r.URI != "" {
				valid[billing.EntryKindResource+"\x00"+r.URI] = true
			}
		}
	}
	var prompts []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(item.PromptsSnapshot), &prompts) == nil {
		for _, p := range prompts {
			if p.Name != "" {
				valid[billing.EntryKindPrompt+"\x00"+p.Name] = true
			}
		}
	}

	seen := map[string]bool{}
	rows := make([]model.McpToolPrice, 0, len(prices))
	for _, p := range prices {
		if err := validatePrice(p.PricePerCall); err != nil {
			return fmt.Errorf("%w: %s %s", err, entryKindLabel(p.Kind), p.Name)
		}
		if p.BillingType == billing.BillingTypePerCall && p.PricePerCall <= 0 {
			return fmt.Errorf("%s %s: 按次计费单价必须大于 0", entryKindLabel(p.Kind), p.Name)
		}
		key := p.Kind + "\x00" + p.Name
		if seen[key] {
			return fmt.Errorf("重复的条目定价: %s %s", entryKindLabel(p.Kind), p.Name)
		}
		if !valid[key] {
			return fmt.Errorf("条目不存在于服务快照: %s %s(资源模板不可定价)", entryKindLabel(p.Kind), p.Name)
		}
		seen[key] = true
		// inherit 行不带价(解析时回退服务级链),价格强制归零
		price := p.PricePerCall
		if p.BillingType == billing.BillingTypeInherit {
			price = 0
		}
		rows = append(rows, model.McpToolPrice{
			MarketplaceItemID: itemID,
			Kind:              p.Kind,
			ToolName:          p.Name,
			BillingType:       p.BillingType,
			PricePerCall:      price,
			Enabled:           true, // 无 default 的 bool 显式赋值(§GORM 约定)
		})
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("marketplace_item_id = ?", itemID).Delete(&model.McpToolPrice{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			return tx.Create(&rows).Error
		}
		return nil
	}); err != nil {
		return err
	}
	billing.InvalidatePricingCacheItem(itemID)
	return nil
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
	// 标识全局唯一(name 唯一索引):前端选择服务后自动回显标识,重复克隆同一服务时
	// 提前给出可读错误,而非落到唯一索引冲突的晦涩 DB 报错。
	if exists, e := model.MarketplaceItemNameExists(req.Name); e == nil && exists {
		return nil, fmt.Errorf("市场项标识已存在: %s,请修改服务标识", req.Name)
	}

	item := &model.MarketplaceItem{
		AdminID:       adminID,
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		Category:      "instant",
		Version:       "1.0.0",
		TransportType: svc.TransportType,
		// 独占进程开关仅对 stdio 源生效:其余传输无平台子进程概念,恒共享语义
		IsolatedProcess: req.IsolatedProcess && svc.TransportType == string(transport.TypeStdio),
		ConfigTemplate:  encryptConfigTemplate(svc.Config), // 克隆源凭证并加密;前端提示替换为平台凭证
		AuthInstructions: svc.AuthType,
		ConfigTemplateSource: svc.AuthConfig,
		RequiredEnv:   "[]",
		ToolsSnapshot: svc.ToolsCache,
		// 资源/提示快照一并拷贝(形态同 services.resources_cache/prompts_cache)
		ResourcesSnapshot: svc.ResourcesCache,
		PromptsSnapshot:   svc.PromptsCache,
		// 上游握手信息从源服务拷贝(源服务已连接过即有值;否则待手动刷新补齐)
		ServerInfo:      svc.ServerInfo,
		ProtocolVersion: svc.ProtocolVersion,
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

// GetItemByID 管理端详情:额外回传平台上游配置(config_template 解密)供编辑预填,
// 其中 headers/env 的凭证值替换为首尾掩码(如 sk-A...x9z),明文凭证不离开服务端
// (COMMERCIALIZATION「API 返回掩码」约束)。公开路径 GetPublished 不回传该字段。
func (s *MarketplaceService) GetItemByID(itemID int64) (*dto.MarketplaceDetail, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	detail := s.toDetail(item)
	// config_template 加密落库(§4.3):Decrypt 失败的存量明文项回退原值(与 materializeMarketplace 一致)
	plain := item.ConfigTemplate
	if p, dErr := common.Decrypt(item.ConfigTemplate); dErr == nil && p != "" {
		plain = p
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal([]byte(plain), &cfg)
	detail.ConfigTemplate = maskConfigCredentials(cfg)
	return detail, nil
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

	// 命名冲突规避:mcp_services 上 (user_id,name) 唯一。若用户已有同名服务(其他市场引用或自有服务),
	// 自动追加后缀直到唯一,避免落到唯一索引冲突的晦涩错误。最终仍以唯一索引为兜底。
	name := item.Name
	for i := 2; ; i++ {
		exists, e := model.ServiceNameExists(userID, name)
		if e != nil {
			return nil, e
		}
		if !exists {
			break
		}
		name = fmt.Sprintf("%s-%d", item.Name, i)
		if i > 1000 { // 安全上限,理论上不会触达
			break
		}
	}

	svc := &model.McpService{
		UserID:            userID,
		Name:              name,
		DisplayName:       item.DisplayName,
		Description:       item.Description,
		TransportType:     "marketplace", // 哨兵值:resolver 见此改用平台 session(真实 transport 在调用时从 item 注入)
		Config:            "{}",          // 空:不复制上游配置/凭证(平台托管)
		ToolsCache:        item.ToolsSnapshot,
		ResourcesCache:    item.ResourcesSnapshot,
		PromptsCache:      item.PromptsSnapshot,
		// 上游握手信息一并复制,用户侧服务详情无需等网关首次调用即有真实版本
		ServerInfo:      item.ServerInfo,
		ProtocolVersion: item.ProtocolVersion,
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
	var resourcesSnapshot map[string]interface{}
	_ = json.Unmarshal([]byte(item.ResourcesSnapshot), &resourcesSnapshot)
	var promptsSnapshot []interface{}
	_ = json.Unmarshal([]byte(item.PromptsSnapshot), &promptsSnapshot)
	var serverInfo map[string]interface{}
	_ = json.Unmarshal([]byte(item.ServerInfo), &serverInfo)

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

	// 条目级定价(仅 enabled 行;查询失败回退空列表,不影响详情主体)
	entryPrices := []dto.MarketplaceEntryPrice{}
	if rows, e := model.ListToolPricesByItem(item.ID); e == nil {
		for _, r := range rows {
			entryPrices = append(entryPrices, dto.MarketplaceEntryPrice{
				Kind:         r.Kind,
				Name:         r.ToolName,
				BillingType:  r.BillingType,
				PricePerCall: r.PricePerCall,
			})
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
		IsolatedProcess:      item.IsolatedProcess,
		ConfigTemplateSource: configSource,
		AuthInstructions:     item.AuthInstructions,
		RepoURL:              item.RepoURL,
		InstallGuide:         item.InstallGuide,
		RequiredEnv:          requiredEnv,
		InstallCount:         item.InstallCount,
		RatingAvg:            item.RatingAvg,
		RatingCount:          item.RatingCount,
		ToolsSnapshot:        toolsSnapshot,
		ResourcesSnapshot:    resourcesSnapshot,
		PromptsSnapshot:      promptsSnapshot,
		ServerInfo:           serverInfo,
		ProtocolVersion:      item.ProtocolVersion,
		Status:               item.Status,
		CreatedAt:            item.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:            item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		BillingType:          item.BillingType,
		PricePerCall:         item.PricePerCall,
		EntryPrices:          entryPrices,
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
