package model

import (
	"encoding/json"
	"math"
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
	"BillingEnabled":             "false",  // 总开关,false 时市场服务也跳过计费
	"QuotaPerUnit":               "500000", // 1 货币单位 = 多少 quota
	"DisplayCurrency":            "CNY",
	"BillingDefaultType":         "per_call",                       // 全局默认计费类型(仅 free/per_call)
	"BillingDefaultPricePerCall": "0",                              // 全局默认按次单价(市场服务第 3 级,展示货币)
	"GroupRatio":                 `{"default":1,"vip":1,"svip":1}`, // 分组倍率 JSON
	"TrustQuota":                 "0",                              // 信任额度旁路阈值;0=按 10*QuotaPerUnit 动态缩放(对齐 new-api)
	"PreConsumedQuota":           "500",                            // 预消耗配额下限(quota):每次预扣不低于此值,成功后退还差额(对齐 new-api PreConsumedQuota);0=禁用
	"ChargeAdmin":                "true",                           // 是否对管理员计费(默认开启:管理员本人调用平台托管服务同样扣费;关闭则豁免)
	"ChargeOnClientError":        "false",                          // 客户端参数错误是否收费
	"ChargeOnTimeout":            "false",                          // 超时是否收费
	"BillingFailOpen":            "true",                           // 计费 DB 异常时是否放行(记欠账)
	// --- 额度 ---
	"QuotaForNewUser":      "0", // 新用户赠送额度
	"QuotaForInviter":      "0", // 邀请者奖励(→ 邀请人 aff_quota 待提取,对齐 new-api QuotaForInviter)
	"QuotaForInvitee":      "0", // 受邀者奖励(→ 受邀者钱包 quota 直接可用,对齐 new-api QuotaForInvitee)
	"QuotaRemindThreshold": "0", // 低额度邮件提醒阈值(0=不提醒)
	// --- 日志 ---
	"LogPayloadEnabled": "true", // 是否落 request/response_payload
	// --- 自有服务 / 自用模式 ---
	"UserOwnedServicesEnabled": "true",  // 是否允许用户添加/调用自有服务(false=纯市场模式)
	"SelfUseModeEnabled":       "false", // 自用模式可用全局默认;非自用(默认)上架必须显式定价
	// --- 兑换 ---
	"RedemptionEnabled": "true",
	// --- 支付(V2)---
	"PaymentEnabled": "false",
	"EpayEndpoint":   "",
	"EpayPID":        "",
	"EpayKey":        "",
	// --- 视觉图片上传 / 对象存储(base64→URL 透传,字节绕开 LLM 上下文)---
	// StorageBackend=local 开箱即用零依赖;=s3 走 minio-go 兼容桶(S3/R2/OSS/COS)。
	"StorageBackend":               "local",      // local | s3
	"StorageLocalPath":             "./data/uploads", // local 磁盘根目录
	"StoragePathPrefix":            "vision",     // 磁盘/桶内的物理隔离前缀
	"StorageEndpoint":              "",           // s3 兼容端点(如 s3.amazonaws.com 或 R2 的 <acct>.r2.cloudflarestorage.com)
	"StorageRegion":                "",           // s3 region
	"StorageBucket":                "",           // s3 bucket 名
	"StorageAccessKey":             "",           // s3 access key(sensitive)
	"StorageSecretKey":             "",           // s3 secret key(sensitive)
	"StorageUseSSL":                "true",       // s3 端点是否走 https
	"VisionUploadMaxBytes":         "10485760",   // 单图字节上限(解码后),10MB
	"SignedURLTTLSeconds":          "3600",       // 签名 URL 有效期(秒),1h
	"UploadRetentionHours":         "24",         // 上传文件保留时长(小时),须 > SignedURLTTL
	"UploadCleanupIntervalMinutes": "60",         // 过期清理扫描间隔(分钟)
	// 市场平台托管的 stdio 进程空闲回收；0 表示关闭自动回收。
	"SharedStdioIdleTimeoutMinutes": "60",
	// 市场共享 stdio 条目的并发调用上限(每条目独立,全体用户合计);超出立即
	// 返回"负载较高,请稍后重试"。0 表示不限。
	"SharedStdioMaxConcurrency": "10",
	// --- V1.1: shell 直传 / 图片管理 / 入参择优 ---
	"PresignedPutTTLSeconds": "600",   // 预签名 PUT URL 有效期(秒),直传路径;须 < SignedURLTTL
	"MaxUploadsPerUser":      "0",     // 每用户活跃上传数上限(护栏);0=不限
	"VisionInlineMaxBytes":   "10240", // base64 内联软阈值(字节);超过引导走上传;0=关闭(退回 V1.0)
	// --- 图像读取时变换(resize / 重压缩;§13.2 网关侧 resize)---
	// 仅作用于 Bytes 路径,在 base64 编码前、client.Analyze 前;磁盘原图与 GET 不受影响。默认全关,
	// 管理员显式开启;出错 fail-open 回退原图(绝不阻塞识别)。
	"VisionResizeEnabled":   "false", // 大图缩放总开关;开启后仅当长边 > VisionResizeMaxEdge 才等比缩
	"VisionResizeMaxEdge":   "1568",  // 长边像素阈值,与 OpenAI/Claude 上游内部长边上限对齐
	"VisionCompressEnabled": "false", // 高保真重编码总开关(非真无损;JPEG/WebP 按 JPEGQuality 重编码,PNG/GIF 不动)
	"VisionJPEGQuality":     "85",    // JPEG/WebP→JPEG 重编码质量 1-100
}

var sensitiveKeys = map[string]bool{
	"SMTPToken":       true,
	"EpayKey":         true,
	"StorageAccessKey": true,
	"StorageSecretKey": true,
}

var publicKeys = map[string]bool{
	"SystemName":               true,
	"Footer":                   true,
	"ServerAddress":            true,
	"RegisterEnabled":          true,
	"EmailVerificationEnabled": true,
	"UserGroupOptions":         true,
	"BillingEnabled":           true,
	"DisplayCurrency":          true,
	"QuotaPerUnit":             true, // 供管理员调额界面按货币换算(对齐 reference/new-api /api/status 暴露)
	"SelfUseModeEnabled":       true,
	"RedemptionEnabled":        true,
	"UserOwnedServicesEnabled": true,
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

// DefaultQuotaPerUnit 是"单位额度"的默认值:1 个货币单位对应的 quota 数量。
// 与 reference/new-api 的 common.QuotaPerUnit(500*1000.0)保持一致。
const DefaultQuotaPerUnit int64 = 500000

// GetQuotaPerUnit 读取"单位额度"(QuotaPerUnit),未配置或非法(<=0)时回退到
// DefaultQuotaPerUnit。集中此处,替代各业务包里重复的硬编码 500000 回退逻辑。
func GetQuotaPerUnit() int64 {
	if v := GetOptionInt64("QuotaPerUnit"); v > 0 {
		return v
	}
	return DefaultQuotaPerUnit
}

// CurrencyToQuota 把展示货币金额(如 1 元)按当前 QuotaPerUnit 换算为整数额度(四舍五入)。
// 与 billing/pricing.go 的 priceToQuota 同算法,集中在此供 model 层调用(billing 依赖 model,
// 反向不可),避免循环依赖与重复硬编码。
func CurrencyToQuota(amount float64) int64 {
	if amount <= 0 {
		return 0
	}
	return int64(math.Round(amount * float64(GetQuotaPerUnit())))
}

// QuotaToCurrency 把整数 quota 按当前 QuotaPerUnit 换算为展示货币金额(浮点)。
func QuotaToCurrency(quota int64) float64 {
	return float64(quota) / float64(GetQuotaPerUnit())
}

// FormatQuotaCurrency 把整数 quota 换算为带币种符号的展示货币文本(如 ¥1、¥0.05),
// 写日志文案时用它替代裸 quota 数字,用户全程只见金额不见额度。
// 精度对齐前端 web/src/lib/billing.ts 的 formatQuotaCurrency:
// 金额 >=1 保留最多 2 位小数,<1 保留最多 4 位(避免小额舍成 0),再去掉尾部多余的 0。
func FormatQuotaCurrency(quota int64) string {
	amount := QuotaToCurrency(quota)
	digits := 2
	if amount < 1 && amount > -1 {
		digits = 4
	}
	s := strconv.FormatFloat(amount, 'f', digits, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		s = "0"
	}
	return CurrencySymbol(GetOptionString("DisplayCurrency")) + s
}

// CurrencySymbol 把展示币种代码映射为符号(对齐前端 web/src/lib/billing.ts 的 currencySymbol)。
// 未知币种回退到 ¥(CNY)。
func CurrencySymbol(currency string) string {
	switch strings.ToUpper(currency) {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	default:
		return "¥"
	}
}

// GetTrustQuota 读取"信任额度旁路阈值"。未配置或非法(<=0)时按 10*QuotaPerUnit 动态缩放,
// 与 reference/new-api 的 common.GetTrustQuota()(=10*QuotaPerUnit)完全一致——
// 信任阈值始终等价于 10 个货币单位,随单位额度变化而缩放。
// 管理员仍可在选项里显式设置一个具体 quota 值来覆盖该动态默认。
func GetTrustQuota() int64 {
	if v := GetOptionInt64("TrustQuota"); v > 0 {
		return v
	}
	return 10 * GetQuotaPerUnit()
}

// GetPreConsumedQuota 读取"预消耗配额下限"(PreConsumedQuota):每次市场调用预扣的最低 quota。
// 与 reference/new-api 的 common.PreConsumedQuota(=500,LLM token 估算下限)对齐——new-mcp 为按次计价,
// 故此处直接以 quota 为单位。默认 500(≈0.001 元);管理员置 0 表示禁用下限(预扣额=实际单价)。
func GetPreConsumedQuota() int64 {
	return GetOptionInt64("PreConsumedQuota")
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
