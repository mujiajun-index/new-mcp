package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/model"
)

// --- 多秘钥管理(/api/services/:id/keys) ---
// 秘钥池仅支持 HTTP 类传输(streamable-http/sse);单秘钥服务保持 config.headers
// 现状不动,多秘钥是显式开启的新模式(无旧数据迁移)。

// applyCreateMultiKey 创建时的多秘钥装配:校验传输/认证/秘钥,推导目标头写入
// AuthConfig,并防御性从 config.headers 剥离同名认证头(值只存池)。
func (s *McpServiceService) applyCreateMultiKey(svc *model.McpService, req *dto.CreateServiceReq) error {
	if svc.TransportType != common.TransportStreamableHTTP && svc.TransportType != common.TransportSSE {
		return fmt.Errorf("多秘钥仅支持 streamable-http / SSE 服务")
	}
	if svc.AuthType == "none" || svc.AuthType == "" {
		return fmt.Errorf("请先选择认证方式再启用多秘钥")
	}
	clean, err := model.CleanKeyValues(req.AuthKeys)
	if err != nil {
		return err
	}
	if len(clean) == 0 {
		return fmt.Errorf("多秘钥模式至少需要一把秘钥")
	}
	headerName := authHeaderForAuthType(svc.AuthType)
	if headerName == "" {
		if req.AuthConfig != nil {
			headerName, _ = req.AuthConfig["header_name"].(string)
		}
		headerName = strings.TrimSpace(headerName)
		if headerName == "" {
			return fmt.Errorf("自定义认证需指定注入的请求头名称")
		}
	}
	if svc.Config != "" {
		var config map[string]interface{}
		_ = json.Unmarshal([]byte(svc.Config), &config)
		if headers, ok := config["headers"].(map[string]interface{}); ok {
			if _, exists := headers[headerName]; exists {
				delete(headers, headerName)
				b, _ := json.Marshal(config)
				svc.Config = string(b)
			}
		}
	}
	svc.AuthConfig = rewriteAuthKeyConfig(svc.AuthConfig, req.KeyMode, headerName)
	return nil
}

// normalizeKeyValues 规整粘贴值:bearer 认证下剥掉误带的 "Bearer " 前缀(池内存裸 token)。
func normalizeKeyValues(svc *model.McpService, values []string) []string {
	if svc.AuthType != "bearer" {
		return values
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.TrimPrefix(v, "Bearer ")
	}
	return out
}

// authHeaderForAuthType 按认证方式推导默认注入头;custom 需调用方显式指定。
func authHeaderForAuthType(authType string) string {
	switch authType {
	case "api_key":
		return "X-API-Key"
	case "bearer":
		return "Authorization"
	}
	return ""
}

// checkKeysManageable 校验服务可管理秘钥:属主匹配 + HTTP 类传输 + 非市场托管。
func checkKeysManageable(svc *model.McpService) error {
	if svc.TransportType != common.TransportStreamableHTTP && svc.TransportType != common.TransportSSE {
		return fmt.Errorf("多秘钥仅支持 streamable-http / SSE 服务")
	}
	if svc.Source == "marketplace" {
		return fmt.Errorf("市场服务由平台托管,不可在此管理秘钥")
	}
	return nil
}

// invalidateKeyRuntime 秘钥池/模式变更后:失效选择器快照、踢会话(下次调用按新池重连)。
func invalidateKeyRuntime(svc *model.McpService) {
	bridge.KeySelectors.Invalidate(svc.ID)
	if SessionPool != nil {
		SessionPool.Remove(svc.ID)
		if svc.Status == common.StatusEnabled {
			go SessionPool.GetOrConnect(context.Background(), svc)
		}
	}
}

// ListKeys 返回秘钥池视图(掩码值,永不回明文)。
func (s *McpServiceService) ListKeys(userID, serviceID int64) (*dto.ServiceKeysResp, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if err := checkKeysManageable(svc); err != nil {
		return nil, err
	}
	cfg := svc.ParseAuthKeyConfig()
	keys, err := model.ListKeysByService(serviceID)
	if err != nil {
		return nil, err
	}
	resp := &dto.ServiceKeysResp{
		KeyMode:       cfg.KeyMode,
		HeaderName:    cfg.HeaderName,
		AuthType:      svc.AuthType,
		TransportType: svc.TransportType,
		Keys:          make([]dto.ServiceKeyItem, 0, len(keys)),
	}
	for _, k := range keys {
		item := dto.ServiceKeyItem{
			ID:             k.ID,
			SortOrder:      k.SortOrder,
			MaskedValue:    maskSecret(k.Value),
			Status:         k.Status,
			DisabledReason: k.DisabledReason,
		}
		if k.DisabledAt != nil {
			item.DisabledAt = k.DisabledAt.Format(time.RFC3339)
		}
		if k.Status == common.StatusEnabled {
			resp.Enabled++
		}
		resp.Keys = append(resp.Keys, item)
	}
	resp.Total = len(keys)
	return resp, nil
}

// UpdateKeys 追加/替换秘钥。追加:对池内已有值去重、保留既有行与状态;
// 替换:事务内整池重排,状态清零。仅多秘钥模式可用。
func (s *McpServiceService) UpdateKeys(userID, serviceID int64, req *dto.UpdateServiceKeysReq) (*dto.UpdateServiceKeysResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if err := checkKeysManageable(svc); err != nil {
		return nil, err
	}
	if !svc.IsMultiKey() {
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
	values := normalizeKeyValues(svc, req.Values)
	switch req.Mode {
	case common.KeyUpdateReplace:
		if err := model.ReplaceKeys(serviceID, values); err != nil {
			return nil, err
		}
		result.Added = len(clean)
	default: // append
		added, err := model.AppendKeys(serviceID, values)
		if err != nil {
			return nil, err
		}
		result.Added = int(added)
		result.Skipped = len(clean) - int(added)
	}
	invalidateKeyRuntime(svc)
	return result, nil
}

// SetKeyStatus 启用/禁用单把秘钥(自动禁用的可手动重新启用,启用即清原因)。
func (s *McpServiceService) SetKeyStatus(userID, serviceID, keyID int64, req *dto.SetServiceKeyStatusReq) error {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return err
	}
	if err := checkKeysManageable(svc); err != nil {
		return err
	}
	status := common.StatusDisabled
	if req.Status == "enabled" {
		status = common.StatusEnabled
	}
	ok, err := model.SetKeyStatus(serviceID, keyID, status, "手动禁用")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("秘钥不存在")
	}
	invalidateKeyRuntime(svc)
	return nil
}

// DeleteKey 删除单把秘钥。
func (s *McpServiceService) DeleteKey(userID, serviceID, keyID int64) error {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return err
	}
	if err := checkKeysManageable(svc); err != nil {
		return err
	}
	ok, err := model.DeleteServiceKey(serviceID, keyID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("秘钥不存在")
	}
	invalidateKeyRuntime(svc)
	return nil
}

// BatchKeys 批量操作:全部启用 / 删除已禁用。
func (s *McpServiceService) BatchKeys(userID, serviceID int64, action string) error {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return err
	}
	if err := checkKeysManageable(svc); err != nil {
		return err
	}
	switch action {
	case "enable_all":
		_, err = model.BatchEnableAllKeys(serviceID)
	case "delete_disabled":
		_, err = model.DeleteDisabledKeys(serviceID)
	default:
		err = fmt.Errorf("未知操作: %s", action)
	}
	if err != nil {
		return err
	}
	invalidateKeyRuntime(svc)
	return nil
}

// UpdateKeyConfig 模式切换(单↔多、随机↔轮询)。
func (s *McpServiceService) UpdateKeyConfig(userID, serviceID int64, req *dto.UpdateServiceKeyConfigReq) (*dto.ServiceKeysResp, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if err := checkKeysManageable(svc); err != nil {
		return nil, err
	}
	if req.KeyMode == "single" {
		if !svc.IsMultiKey() {
			return s.ListKeys(userID, serviceID)
		}
		if err := downgradeToSingleKey(svc); err != nil {
			return nil, err
		}
	} else {
		if err := upgradeToMultiKey(svc, req.KeyMode, req.HeaderName); err != nil {
			return nil, err
		}
	}
	invalidateKeyRuntime(svc)
	return s.ListKeys(userID, serviceID)
}

// upgradeToMultiKey 单→多(或已多秘钥时切换策略 random↔polling):单→多时推导目标头,
// 把 config.headers 里的现有认证值收编为首把秘钥并从 headers 移除该头;策略切换
// 沿用既有注入头,不接受更换。
func upgradeToMultiKey(svc *model.McpService, mode, reqHeader string) error {
	if svc.AuthType == "none" || svc.AuthType == "" {
		return fmt.Errorf("认证方式为「无需认证」时不支持多秘钥")
	}
	cfg := svc.ParseAuthKeyConfig()

	var headerName string
	if svc.IsMultiKey() && cfg.HeaderName != "" {
		// 已是多秘钥 = 策略切换:必须沿用既有注入头——custom 认证没有可推导的
		// 默认头,请求又只在单→多时才带 header_name,按认证方式推导会误报"需指定头名"。
		if reqHeader != "" && reqHeader != cfg.HeaderName {
			return fmt.Errorf("多秘钥模式下不可更换注入头(当前 %s)", cfg.HeaderName)
		}
		headerName = cfg.HeaderName
	} else {
		// 单→多:缺省按认证方式推导;custom 无默认头,回退到认证配置里记录的
		// header_name(详情页保存自定义认证时随 auth_config 落库)。
		headerName = strings.TrimSpace(reqHeader)
		if headerName == "" {
			headerName = authHeaderForAuthType(svc.AuthType)
		}
		if headerName == "" {
			headerName = strings.TrimSpace(cfg.HeaderName)
		}
		if headerName == "" {
			return fmt.Errorf("自定义认证需指定注入的请求头名称")
		}
	}

	authConfig := rewriteAuthKeyConfig(svc.AuthConfig, mode, headerName)

	if !svc.IsMultiKey() {
		// 收编现有静态认证头为首把秘钥;找不到即拒绝切换(避免切完服务立即可用性中断)。
		var config map[string]interface{}
		_ = json.Unmarshal([]byte(svc.Config), &config)
		if config == nil {
			config = map[string]interface{}{}
		}
		headers, _ := config["headers"].(map[string]interface{})
		candidate, _ := headers[headerName].(string)
		if stripBearerPrefix(svc, candidate) == "" {
			return fmt.Errorf("现有配置中未找到认证头 %s 的值,无法切换为多秘钥", headerName)
		}
		if _, err := model.AppendKeys(svc.ID, []string{stripBearerPrefix(svc, candidate)}); err != nil {
			return err
		}
		delete(headers, headerName)
		configJSON, _ := json.Marshal(config)
		svc.Config = string(configJSON)
	}

	svc.AuthConfig = authConfig
	if err := svc.Update(); err != nil {
		return err
	}
	return nil
}

// downgradeToMultiKey 反向:首选启用秘钥写回 config.headers,清空秘钥池。
func downgradeToSingleKey(svc *model.McpService) error {
	cfg := svc.ParseAuthKeyConfig()
	keys, err := model.ListKeysByService(svc.ID)
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

	var config map[string]interface{}
	_ = json.Unmarshal([]byte(svc.Config), &config)
	if config == nil {
		config = map[string]interface{}{}
	}
	headers, ok := config["headers"].(map[string]interface{})
	if !ok {
		headers = map[string]interface{}{}
	}
	if pick != "" {
		headers[cfg.HeaderName] = applyBearerPrefix(svc, pick)
	}
	config["headers"] = headers
	configJSON, _ := json.Marshal(config)
	svc.Config = string(configJSON)
	svc.AuthConfig = rewriteAuthKeyConfig(svc.AuthConfig, "", "")
	if err := svc.Update(); err != nil {
		return err
	}
	return model.DeleteKeysByService(svc.ID)
}

// rewriteAuthKeyConfig 重写 AuthConfig 的多秘钥段,保留其余展示字段。
// mode 为空表示退回单秘钥(两个键一并清除)。
func rewriteAuthKeyConfig(authConfigJSON, mode, headerName string) string {
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(authConfigJSON), &m)
	if m == nil {
		m = map[string]interface{}{}
	}
	if mode == "" {
		delete(m, "key_mode")
		delete(m, "header_name")
	} else {
		m["key_mode"] = mode
		m["header_name"] = headerName
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// applyBearerPrefix / stripBearerPrefix 在 bearer 认证下补/剥 "Bearer " 前缀:
// 秘钥池内存裸 token,写回 config.headers 时需要完整头值。
func applyBearerPrefix(svc *model.McpService, v string) string {
	if svc.AuthType == "bearer" {
		return "Bearer " + v
	}
	return v
}

func stripBearerPrefix(svc *model.McpService, v string) string {
	if svc.AuthType == "bearer" {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return v
}
