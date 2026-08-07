package model

import "encoding/json"

// 日志类型常量(对齐 new-api,显式整数而非 iota,值永不改变)。
const (
	LogTypeAll     = 0 // 仅作"全部"筛选哨兵,不落库
	LogTypeTopup   = 1 // 兑换码充值(用户发起的入账)
	LogTypeConsume = 2 // MCP 调用消费
	LogTypeManage  = 3 // 管理员操作
	LogTypeSystem  = 4 // 系统事件(用户注册等)
	LogTypeError   = 5 // 预留(错误仍作为 Consume 行,用 response_status 区分)
	LogTypeRefund  = 6 // 预留(退款仍作为 Consume 行,用 billing_status=refunded 区分)
	LogTypeLogin   = 7 // 用户登录
)

// Operator 描述一次管理/系统操作的操作者,序列化进日志 extra.operator 供审计;
// 普通用户视图经 StripAuditExtra 剥离,不泄露管理员身份。
type Operator struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	IP       string `json:"ip"`
}

// RecordLog 通用日志写入。call 专用列(user/api_key/tool/group/billing 等)留空,
// 仅填充 Type/UserID/Username/Content/QuotaConsumed/ClientIP/Extra。
// 写失败忽略(与网关 recordLog 的 _ = log.Insert() 一致),不影响业务事务。
func RecordLog(userID int64, username string, logType int, content string, quota int64, ip string, extra map[string]any) {
	l := &McpCallLog{
		Type:          logType,
		UserID:        userID,
		Username:      username,
		Content:       content,
		QuotaConsumed: quota,
		ClientIP:      ip,
		Extra:         marshalExtra(extra),
	}
	_ = l.Insert()
}

// RecordTopupLog 用户发起的入账(兑换码兑换)。
func RecordTopupLog(userID int64, username, content string, quota int64, ip string, extra map[string]any) {
	RecordLog(userID, username, LogTypeTopup, content, quota, ip, extra)
}

// RecordManageLog 以目标用户为 owner 写管理日志;操作者信息放入 extra.operator。
// owner=目标用户,使该用户能在自己日志里看到「管理员调整了我的额度」。
func RecordManageLog(targetUserID int64, targetUsername, content string, quota int64, actor Operator, extra map[string]any) {
	if extra == nil {
		extra = map[string]any{}
	}
	extra["operator"] = actor
	RecordLog(targetUserID, targetUsername, LogTypeManage, content, quota, actor.IP, extra)
}

// RecordSystemLog 系统事件(用户注册等)。
func RecordSystemLog(userID int64, username, content string, quota int64, ip string, extra map[string]any) {
	RecordLog(userID, username, LogTypeSystem, content, quota, ip, extra)
}

// RecordLoginLog 用户登录。
func RecordLoginLog(userID int64, username, ip string, extra map[string]any) {
	RecordLog(userID, username, LogTypeLogin, "用户登录", 0, ip, extra)
}

// StripAuditExtra 从 extra JSON 中移除 operator 等审计字段,供普通用户视图脱敏。
// 解析失败时原样返回,避免因脏数据影响列表展示。
func StripAuditExtra(extra string) string {
	if extra == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(extra), &m); err != nil {
		return extra
	}
	delete(m, "operator")
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

func marshalExtra(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	b, err := json.Marshal(extra)
	if err != nil {
		return ""
	}
	return string(b)
}
