package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey;size:128"`
	Value string `json:"value" gorm:"type:text"`
}

func (Option) TableName() string { return "options" }

var (
	OptionMap      = make(map[string]string)
	OptionMapMutex sync.RWMutex
)

var defaultOptions = map[string]string{
	"SystemName":                    "NewMCP",
	"ServerAddress":                 "http://localhost:3000",
	"Footer":                        "MCP Protocol Gateway",
	"RegisterEnabled":               "true",
	"EmailVerificationEnabled":      "false",
	"EmailDomainRestrictionEnabled": "false",
	"EmailDomainWhitelist":          "",
	"UserGroupOptions":              "default,vip,svip",
	"RateLimitEnabled":              "false",
	"RateLimitMaxRequests":          "60",
	"RateLimitWindowMinutes":        "1",
	"RateLimitGroupConfig":          "{}",
	"SMTPServer":                    "",
	"SMTPPort":                      "587",
	"SMTPAccount":                   "",
	"SMTPToken":                     "",
	"SMTPFrom":                      "",
	"SMTPSSLEnabled":                "false",
	"CloudflareProxyEnabled":        "false",

	// --- 商业化计费(§15)---
	"BillingEnabled":              "false", // 总开关,false 时市场服务也跳过计费
	"QuotaPerUnit":                "500000", // 1 货币单位 = 多少 quota
	"DisplayCurrency":             "CNY",
	"BillingDefaultType":          "per_call", // 全局默认计费类型(仅 free/per_call)
	"BillingDefaultPricePerCall":  "0",        // 全局默认按次单价(市场服务第 3 级,展示货币)
	"GroupRatio":                  `{"default":1,"vip":1,"svip":1}`, // 分组倍率 JSON
	"TrustQuota":                  "5000000", // 信任额度旁路阈值(默认 10 元)
	"ChargeAdmin":                 "false",   // 是否对管理员计费
	"ChargeOnClientError":         "false",   // 客户端参数错误是否收费
	"ChargeOnTimeout":             "false",   // 超时是否收费
	"BillingFailOpen":             "true",    // 计费 DB 异常时是否放行(记欠账)
	// --- 额度 ---
	"QuotaForNewUser":             "0",   // 新用户赠送额度
	"QuotaRemindThreshold":        "0",   // 低额度邮件提醒阈值(0=不提醒)
	// --- 日志 ---
	"LogPayloadEnabled":           "true",  // 是否落 request/response_payload
	"LogRetentionDays":            "30",    // 调用日志 TTL(天),0=永久
	// --- 自有服务 / 自用模式 ---
	"UserOwnedServicesEnabled":    "true",  // 是否允许用户添加/调用自有服务(false=纯市场模式)
	"SelfUseModeEnabled":          "false", // 自用模式可用全局默认;非自用(默认)上架必须显式定价
	// --- 兑换 ---
	"RedemptionEnabled":           "true",
	// --- 支付(V2)---
	"PaymentEnabled":              "false",
	"EpayEndpoint":                "",
	"EpayPID":                     "",
	"EpayKey":                     "",
}

var sensitiveKeys = map[string]bool{
	"SMTPToken": true,
	"EpayKey":   true,
}

var publicKeys = map[string]bool{
	"SystemName":                  true,
	"Footer":                      true,
	"ServerAddress":               true,
	"RegisterEnabled":             true,
	"EmailVerificationEnabled":    true,
	"UserGroupOptions":            true,
	"BillingEnabled":              true,
	"DisplayCurrency":             true,
	"SelfUseModeEnabled":          true,
	"RedemptionEnabled":           true,
	"UserOwnedServicesEnabled":    true,
}

func InitOptionMap() {
	OptionMapMutex.Lock()
	defer OptionMapMutex.Unlock()

	OptionMap = make(map[string]string)
	for k, v := range defaultOptions {
		OptionMap[k] = v
	}

	var options []Option
	DB.Find(&options)
	for _, opt := range options {
		OptionMap[opt.Key] = opt.Value
	}
}

func UpdateOption(key string, value string) error {
	option := Option{Key: key}
	DB.FirstOrCreate(&option, Option{Key: key})
	option.Value = value
	if err := DB.Save(&option).Error; err != nil {
		return err
	}

	OptionMapMutex.Lock()
	OptionMap[key] = value
	OptionMapMutex.Unlock()
	return nil
}

func GetOptionString(key string) string {
	OptionMapMutex.RLock()
	defer OptionMapMutex.RUnlock()
	return OptionMap[key]
}

func GetOptionBool(key string) bool {
	OptionMapMutex.RLock()
	v := OptionMap[key]
	OptionMapMutex.RUnlock()
	return v == "true"
}

func GetOptionInt(key string) int {
	OptionMapMutex.RLock()
	v := OptionMap[key]
	OptionMapMutex.RUnlock()
	n, _ := strconv.Atoi(v)
	return n
}

// GetOptionInt64 读取大额类配置(quota/阈值),避免 32 位溢出。
func GetOptionInt64(key string) int64 {
	OptionMapMutex.RLock()
	v := OptionMap[key]
	OptionMapMutex.RUnlock()
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// GetOptionFloat 读取浮点配置(单价等)。
func GetOptionFloat(key string) float64 {
	OptionMapMutex.RLock()
	v := OptionMap[key]
	OptionMapMutex.RUnlock()
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

// GetGroupRatio 解析 "GroupRatio"(JSON),返回 group→倍率。缺失 key 默认 1.0。
// 解析失败返回仅含 "default":1.0 的安全默认。
func GetGroupRatio() map[string]float64 {
	raw := GetOptionString("GroupRatio")
	if raw == "" {
		return map[string]float64{"default": 1.0}
	}
	m := make(map[string]float64)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]float64{"default": 1.0}
	}
	if _, ok := m["default"]; !ok {
		m["default"] = 1.0
	}
	return m
}

// GetUserGroupOptions parses the comma-separated "UserGroupOptions" setting
// into a deduplicated list of group names. It always contains at least
// "default", so user creation/editing can always pick a valid group.
func GetUserGroupOptions() []string {
	raw := GetOptionString("UserGroupOptions")
	var opts []string
	seen := make(map[string]bool)
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		opts = append(opts, p)
	}
	if len(opts) == 0 {
		opts = []string{"default"}
	}
	return opts
}

// RateLimitGroupRule is one group's rate-limit override: at most Max requests
// per WindowMinutes.
type RateLimitGroupRule struct {
	Max           int
	WindowMinutes int
}

// GetRateLimitGroupConfig parses the "RateLimitGroupConfig" option, stored as
// JSON shaped {"group": {"max": N, "window": M}, ...} with window in minutes.
// Returns nil when unset or invalid (callers then fall back to the global
// RateLimitMaxRequests / RateLimitWindowMinutes defaults).
func GetRateLimitGroupConfig() map[string]RateLimitGroupRule {
	raw := GetOptionString("RateLimitGroupConfig")
	if raw == "" {
		return nil
	}
	var m map[string]struct {
		Max    int `json:"max"`
		Window int `json:"window"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	out := make(map[string]RateLimitGroupRule, len(m))
	for k, v := range m {
		out[k] = RateLimitGroupRule{Max: v.Max, WindowMinutes: v.Window}
	}
	return out
}

func IsSensitiveKey(key string) bool {
	return sensitiveKeys[key]
}

func IsPublicKey(key string) bool {
	return publicKeys[key]
}

func IsEmailDomainAllowed(email string) bool {
	if !GetOptionBool("EmailDomainRestrictionEnabled") {
		return true
	}
	if email == "" {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])
	whitelist := GetOptionString("EmailDomainWhitelist")
	if whitelist == "" {
		return false
	}
	for _, d := range strings.Split(whitelist, ",") {
		if strings.TrimSpace(strings.ToLower(d)) == domain {
			return true
		}
	}
	return false
}

// IsSMTPConfigured reports whether SMTP sending is configured (server + account
// set). It gates whether email binding/changing requires verification: when
// SMTP is configured, the new address must be verified via a code.
func IsSMTPConfigured() bool {
	return GetOptionString("SMTPServer") != "" && GetOptionString("SMTPAccount") != ""
}
