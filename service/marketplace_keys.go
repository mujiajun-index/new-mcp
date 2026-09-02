package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/model"
)

// --- 条目级多秘钥管理(/admin/marketplace/:id/keys) ---
// 市场条目的平台级秘钥池(仅 HTTP 类 instant 条目):一份池对全部安装用户全局轮换,
// 坏 key 一次禁光;凭证存 marketplace_item_keys,多秘钥态模板 headers 不含认证头。
// 交互与服务级(/services/:id/keys)对齐,DTO 复用。

// checkItemKeysManageable 校验条目可管理秘钥池:平台托管(instant)+ HTTP 类传输。
// 属主无需校验(AdminAuth 已把关);source 类条目无平台上游连接,拒绝。
func checkItemKeysManageable(item *model.MarketplaceItem) error {
	if item.Category != "instant" {
		return fmt.Errorf("仅平台托管(开箱即用)市场项支持秘钥池")
	}
	if item.TransportType != common.TransportStreamableHTTP && item.TransportType != common.TransportSSE {
		return fmt.Errorf("多秘钥仅支持 streamable-http / SSE 条目")
	}
	return nil
}

// plainItemConfig 解密条目模板为 map(Decrypt 失败的存量明文回退原值,与 materialize 一致)。
func plainItemConfig(item *model.MarketplaceItem) map[string]interface{} {
	plain := item.ConfigTemplate
	if p, err := common.Decrypt(item.ConfigTemplate); err == nil && p != "" {
		plain = p
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal([]byte(plain), &cfg)
	return cfg
}

// inferTemplateAuth 从模板 headers 反推认证形态:返回认证类型与认证头名(无认证头时
// authType="none")。与前端管理详情 UpstreamConfigCard 的反推规则保持一致。
func inferTemplateAuth(cfg map[string]interface{}) (authType, headerName string) {
	headers, _ := cfg["headers"].(map[string]interface{})
	if _, ok := headers["Authorization"]; ok {
		return "bearer", "Authorization"
	}
	if _, ok := headers["X-API-Key"]; ok {
		return "api_key", "X-API-Key"
	}
	for k := range headers {
		return "custom", k // 任意首个其他头 = 自定义认证
	}
	return "none", ""
}

// headerAuthType 按注入头名推导认证类型展示(条目无 AuthType,多秘钥态用)。
func headerAuthType(headerName string) string {
	switch headerName {
	case "Authorization":
		return "bearer"
	case "X-API-Key":
		return "api_key"
	}
	return "custom"
}

// rewriteItemAuthKeyConfig 重写条目 AuthConfig 的多秘钥段(比服务版多一个 bearer 位:
// 条目无 AuthType,值补/剥 "Bearer " 前缀的依据显式落库)。mode 为空表示退回单秘钥。
func rewriteItemAuthKeyConfig(authConfigJSON, mode, headerName string, bearer bool) string {
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(authConfigJSON), &m)
	if m == nil {
		m = map[string]interface{}{}
	}
	if mode == "" {
		delete(m, "key_mode")
		delete(m, "header_name")
		delete(m, "bearer")
	} else {
		m["key_mode"] = mode
		m["header_name"] = headerName
		m["bearer"] = bearer
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// invalidateItemRuntime 条目池/模式变更后:失效选择器快照、踢该条目全部引用会话。
// 安装用户众多,不做预连(与 UpdateItem 改模板后的踢法一致,按需重连)。
func invalidateItemRuntime(itemID int64) {
	bridge.KeySelectors.InvalidateItem(itemID)
	if SessionPool != nil {
		SessionPool.RemoveByMarketplaceItem(itemID)
	}
}

// normalizeItemKeyValues 规整粘贴值:bearer 池下剥掉误带的 "Bearer " 前缀(池内存裸 token)。
func normalizeItemKeyValues(bearer bool, values []string) []string {
	if !bearer {
		return values
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.TrimPrefix(v, "Bearer ")
	}
	return out
}

// ListKeys 返回条目秘钥池视图(掩码值,永不回明文)。
func (s *MarketplaceService) ListKeys(itemID int64) (*dto.ServiceKeysResp, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	if err := checkItemKeysManageable(item); err != nil {
		return nil, err
	}
	cfg := item.ParseAuthKeyConfig()
	keys, err := model.ListKeysByItem(itemID)
	if err != nil {
		return nil, err
	}
	authType := headerAuthType(cfg.HeaderName)
	if cfg.KeyMode == "" {
		// 单秘钥态:认证形态从模板 headers 反推
		authType, _ = inferTemplateAuth(plainItemConfig(item))
	}
	resp := &dto.ServiceKeysResp{
		KeyMode:       cfg.KeyMode,
		HeaderName:    cfg.HeaderName,
		AuthType:      authType,
		TransportType: item.TransportType,
		Keys:          make([]dto.ServiceKeyItem, 0, len(keys)),
	}
	for _, k := range keys {
		keyItem := dto.ServiceKeyItem{
			ID:             k.ID,
			SortOrder:      k.SortOrder,
			MaskedValue:    maskSecret(k.Value),
			Status:         k.Status,
			DisabledReason: k.DisabledReason,
		}
		if k.DisabledAt != nil {
			keyItem.DisabledAt = k.DisabledAt.Format(time.RFC3339)
		}
		if k.Status == common.StatusEnabled {
			resp.Enabled++
		}
		resp.Keys = append(resp.Keys, keyItem)
	}
	resp.Total = len(keys)
	return resp, nil
}

// UpdateKeys 追加/替换条目秘钥(语义同服务级:追加去重保状态,替换整池重排清状态)。
func (s *MarketplaceService) UpdateKeys(itemID int64, req *dto.UpdateServiceKeysReq) (*dto.UpdateServiceKeysResult, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	if err := checkItemKeysManageable(item); err != nil {
		return nil, err
	}
	if !item.IsMultiKey() {
		return nil, fmt.Errorf("请先在秘钥设置中切换为多秘钥模式")
	}
	clean, err := model.CleanKeyValues(req.Values)
	if err != nil {
		return nil, err
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("未输入有效秘钥(空行/重复行已忽略)")
	}
	result := &dto.UpdateServiceKeysResult{}
	values := normalizeItemKeyValues(item.ParseAuthKeyConfig().Bearer, req.Values)
	switch req.Mode {
	case common.KeyUpdateReplace:
		if err := model.ReplaceItemKeys(itemID, values); err != nil {
			return nil, err
		}
		result.Added = len(clean)
	default: // append
		added, err := model.AppendItemKeys(itemID, values)
		if err != nil {
			return nil, err
		}
		result.Added = int(added)
		result.Skipped = len(clean) - int(added)
	}
	invalidateItemRuntime(itemID)
	return result, nil
}

// SetKeyStatus 启用/禁用单把条目秘钥(自动禁用的可手动重新启用,启用即清原因)。
func (s *MarketplaceService) SetKeyStatus(itemID, keyID int64, req *dto.SetServiceKeyStatusReq) error {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return err
	}
	if err := checkItemKeysManageable(item); err != nil {
		return err
	}
	status := common.StatusDisabled
	if req.Status == "enabled" {
		status = common.StatusEnabled
	}
	ok, err := model.SetItemKeyStatus(itemID, keyID, status, "手动禁用")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("秘钥不存在")
	}
	invalidateItemRuntime(itemID)
	return nil
}

// DeleteKey 删除单把条目秘钥。
func (s *MarketplaceService) DeleteKey(itemID, keyID int64) error {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return err
	}
	if err := checkItemKeysManageable(item); err != nil {
		return err
	}
	ok, err := model.DeleteItemKey(itemID, keyID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("秘钥不存在")
	}
	invalidateItemRuntime(itemID)
	return nil
}

// BatchKeys 批量操作:全部启用 / 删除已禁用。
func (s *MarketplaceService) BatchKeys(itemID int64, action string) error {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return err
	}
	if err := checkItemKeysManageable(item); err != nil {
		return err
	}
	switch action {
	case "enable_all":
		_, err = model.BatchEnableAllItemKeys(itemID)
	case "delete_disabled":
		_, err = model.DeleteDisabledItemKeys(itemID)
	default:
		err = fmt.Errorf("未知操作: %s", action)
	}
	if err != nil {
		return err
	}
	invalidateItemRuntime(itemID)
	return nil
}

// UpdateKeyConfig 条目模式切换(单↔多、随机↔轮询)。
func (s *MarketplaceService) UpdateKeyConfig(itemID int64, req *dto.UpdateServiceKeyConfigReq) (*dto.ServiceKeysResp, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	if err := checkItemKeysManageable(item); err != nil {
		return nil, err
	}
	if req.KeyMode == "single" {
		if !item.IsMultiKey() {
			return s.ListKeys(itemID)
		}
		if err := downgradeItemToSingleKey(item); err != nil {
			return nil, err
		}
	} else {
		if err := upgradeItemToMultiKey(item, req.KeyMode, req.HeaderName); err != nil {
			return nil, err
		}
	}
	invalidateItemRuntime(itemID)
	return s.ListKeys(itemID)
}

// upgradeItemToMultiKey 单→多(或已多秘钥时切换策略):单→多时目标头 = 显式指定 >
// 模板 headers 反推(Authorization/X-API-Key/首个自定义头),把模板现有认证值收编为
// 首把秘钥并从模板剥掉该头;bearer 位按库内值是否带 "Bearer " 前缀推导后显式落库,
// 注入形态与切换前一致。策略切换沿用既有注入头,不接受更换。
func upgradeItemToMultiKey(item *model.MarketplaceItem, mode, reqHeader string) error {
	cfg := item.ParseAuthKeyConfig()
	singleToMulti := !item.IsMultiKey()

	var headerName string
	var bearer bool
	if !singleToMulti {
		// 已是多秘钥 = 策略切换:沿用既有注入头(模板已无认证头,无从再推导)。
		if reqHeader != "" && reqHeader != cfg.HeaderName {
			return fmt.Errorf("多秘钥模式下不可更换注入头(当前 %s)", cfg.HeaderName)
		}
		headerName = cfg.HeaderName
		bearer = cfg.Bearer
	} else {
		headerName = strings.TrimSpace(reqHeader)
		if headerName == "" {
			_, headerName = inferTemplateAuth(plainItemConfig(item))
		}
		if headerName == "" {
			return fmt.Errorf("模板中未找到认证头,请先在平台上游配置中填写认证凭证")
		}
	}

	if singleToMulti {
		// 收编模板现有认证头值为首把秘钥(找不到即拒绝,避免切完即断供)。
		config := plainItemConfig(item)
		headers, _ := config["headers"].(map[string]interface{})
		candidate, _ := headers[headerName].(string)
		bearer = headerName == "Authorization" && strings.HasPrefix(candidate, "Bearer ")
		token := strings.TrimPrefix(candidate, "Bearer ")
		if token == "" {
			return fmt.Errorf("现有配置中未找到认证头 %s 的值,无法切换为多秘钥", headerName)
		}
		if _, err := model.AppendItemKeys(item.ID, []string{token}); err != nil {
			return err
		}
		delete(headers, headerName)
		configJSON, _ := json.Marshal(config)
		item.ConfigTemplate = encryptConfigTemplate(string(configJSON))
	}

	item.AuthConfig = rewriteItemAuthKeyConfig(item.AuthConfig, mode, headerName, bearer)
	return item.Update()
}

// downgradeItemToSingleKey 多→单:首选启用秘钥(全禁用退化取第一把)按 bearer 位
// 补前缀写回模板 headers,清条目多秘钥配置并清池。
func downgradeItemToSingleKey(item *model.MarketplaceItem) error {
	cfg := item.ParseAuthKeyConfig()
	keys, err := model.ListKeysByItem(item.ID)
	if err != nil {
		return err
	}
	pick := ""
	for _, k := range keys {
		if k.Status == common.StatusEnabled {
			pick = k.Value
			break
		}
	}
	if pick == "" && len(keys) > 0 {
		pick = keys[0].Value // 全禁用时退化取第一把,避免无凭证
	}

	config := plainItemConfig(item)
	headers, ok := config["headers"].(map[string]interface{})
	if !ok {
		headers = map[string]interface{}{}
	}
	if pick != "" && cfg.Bearer {
		pick = "Bearer " + pick
	}
	if pick != "" {
		headers[cfg.HeaderName] = pick
	}
	config["headers"] = headers
	configJSON, _ := json.Marshal(config)
	item.ConfigTemplate = encryptConfigTemplate(string(configJSON))
	item.AuthConfig = rewriteItemAuthKeyConfig(item.AuthConfig, "", "", false)
	if err := item.Update(); err != nil {
		return err
	}
	return model.DeleteKeysByItem(item.ID)
}

// --- 克隆整拷(多秘钥源服务 → 条目级池) ---

// stripAuthHeaderConfig 克隆整拷用:源服务 config 的 headers 剥掉认证头,
// 凭证值只存条目池(模板不再含该头)。
func stripAuthHeaderConfig(svc *model.McpService) (string, error) {
	cfg := svc.ParseAuthKeyConfig()
	if cfg.HeaderName == "" {
		return "", fmt.Errorf("源服务多秘钥配置缺少注入头,无法克隆")
	}
	var config map[string]interface{}
	_ = json.Unmarshal([]byte(svc.Config), &config)
	if config == nil {
		config = map[string]interface{}{}
	}
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		delete(headers, cfg.HeaderName)
	}
	b, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// itemAuthConfigFromService 服务多秘钥配置转条目 AuthConfig(bearer 位取服务认证类型)。
func itemAuthConfigFromService(svc *model.McpService) string {
	cfg := svc.ParseAuthKeyConfig()
	b, _ := json.Marshal(model.ItemAuthKeyConfig{
		KeyMode:    cfg.KeyMode,
		HeaderName: cfg.HeaderName,
		Bearer:     svc.AuthType == "bearer",
	})
	return string(b)
}

// servicePoolValues 取服务秘钥池全部值(克隆整拷;空池报错,应先补秘钥或降级单秘钥)。
func servicePoolValues(serviceID int64) ([]string, error) {
	keys, err := model.ListKeysByService(serviceID)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("源服务秘钥池为空,无法克隆(请先添加秘钥或降级为单秘钥)")
	}
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, k.Value)
	}
	return values, nil
}
