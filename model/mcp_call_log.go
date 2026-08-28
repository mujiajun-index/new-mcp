package model

import (
	"fmt"
	"math"
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

type McpCallLog struct {
	ID              int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID          int64     `json:"user_id" gorm:"index"`
	Username        string    `json:"username" gorm:"size:128;index;default:''"`
	ApiKeyID        int64     `json:"api_key_id" gorm:"index;default:0"`
	ApiKeyName      string    `json:"api_key_name" gorm:"size:128;default:''"`
	DeviceID        *int64    `json:"device_id"`
	GroupID         int64     `json:"group_id" gorm:"index;default:0"`
	GroupName       string    `json:"group_name" gorm:"size:128;index;default:''"`
	ServiceID       int64     `json:"service_id" gorm:"index;default:0"`
	ServiceName     string    `json:"service_name" gorm:"size:128;default:''"`
	ToolName        string    `json:"tool_name" gorm:"size:255;not null;index"`
	Method          string    `json:"method" gorm:"size:64"`
	RequestID       string    `json:"request_id" gorm:"size:64;index;default:''"`
	RequestPayload  string    `json:"request_payload" gorm:"type:mediumtext"`
	ResponseStatus  string    `json:"response_status" gorm:"size:16;index"`
	ResponsePayload string    `json:"response_payload" gorm:"type:mediumtext"`
	DurationMs      int       `json:"duration_ms" gorm:"default:0"`
	ErrorMessage    string    `json:"error_message" gorm:"type:text"`
	// 商业化计费列(§4.5):skipped(自有/未启用/免费) / charged(已扣) / refunded(失败退款) / blocked(余额不足拒绝) / debt(FailOpen 欠账)
	BillingStatus     string  `json:"billing_status" gorm:"size:16;default:skipped;index"`
	BillingType       string  `json:"billing_type" gorm:"size:16"`        // 本次解析到的计费类型 free/per_call
	UnitPrice         float64 `json:"unit_price" gorm:"type:decimal(10,6)"` // 本次单价快照(展示货币)
	QuotaConsumed     int64   `json:"quota_consumed" gorm:"default:0"`    // 本次实扣额度(quota)
	PriceScope        string  `json:"price_scope" gorm:"size:16"`         // tool/service/marketplace/global
	MarketplaceItemID *int64  `json:"marketplace_item_id" gorm:"index"`   // 市场来源服务关联的市场项 ID
	ClientIP        string    `json:"client_ip" gorm:"size:64"`
	UserAgent       string    `json:"user_agent" gorm:"size:512"`
	CreatedAt       time.Time `json:"created_at" gorm:"index"`
	// 统一日志扩展:区分日志类型 + 人类可读描述 + 结构化元数据(见 model/log.go)。
	// 历史行经回填后 type=Consume;新写入由各 Record* 助手显式赋值。
	Type    int    `json:"type" gorm:"index;default:2"`        // 日志类型(LogType*);默认 Consume
	Content string `json:"content" gorm:"size:512;default:''"` // 非消费类日志的人类可读描述
	Extra   string `json:"extra" gorm:"type:text"`             // 结构化元数据 JSON(operator/action/params 等)
}

func (McpCallLog) TableName() string { return "mcp_call_logs" }

func (l *McpCallLog) Insert() error {
	return DB.Create(l).Error
}

// HasChargedRequest 软幂等检查:该 (api_key_id, request_id) 是否已有计费成功的日志,
// 防止 MCP 客户端重试导致重扣(§6.3)。仅 V1 软检查;硬幂等需 request_id 唯一索引(V1.7+ 硬化)。
func HasChargedRequest(apiKeyID int64, requestID string) bool {
	if requestID == "" || apiKeyID == 0 {
		return false
	}
	var count int64
	DB.Model(&McpCallLog{}).
		Where("api_key_id = ? AND request_id = ? AND billing_status = ?", apiKeyID, requestID, "charged").
		Count(&count)
	return count > 0
}

// SumQuotaConsumedByUser 统计用户在某时间点之后的市场来源服务消费额度(quota_consumed 之和)。
func SumQuotaConsumedByUser(userID int64, since time.Time) (int64, error) {
	var sum int64
	q := DB.Model(&McpCallLog{}).Where("user_id = ? AND billing_status = ?", userID, "charged")
	if !since.IsZero() {
		q = q.Where("created_at >= ?", since)
	}
	err := q.Select("COALESCE(SUM(quota_consumed), 0)").Scan(&sum).Error
	return sum, err
}

// LogFilter holds query parameters for log listing
type LogFilter struct {
	StartDate   string
	EndDate     string
	Status      string // "success" or "error"
	ToolName    string
	GroupName   string
	Username    string
	ServiceName string
	ApiKeyName  string // 令牌名称(含手动测试占位 tool-test)
	Keyword     string
	Type        int // 0=全部(哨兵),否则按 LogType 过滤
}

func applyLogFilter(query *gorm.DB, f *LogFilter) *gorm.DB {
	if f == nil {
		return query
	}
	if f.Type != 0 {
		query = query.Where("type = ?", f.Type)
	}
	if f.StartDate != "" {
		if t, err := time.Parse("2006-01-02", f.StartDate); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if f.EndDate != "" {
		if t, err := time.Parse("2006-01-02", f.EndDate); err == nil {
			query = query.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}
	if f.Status == "success" {
		query = query.Where("response_status = ?", "success")
	} else if f.Status == "error" {
		query = query.Where("response_status != ?", "success")
	}
	if f.ToolName != "" {
		query = query.Where("tool_name LIKE ?", "%"+f.ToolName+"%")
	}
	if f.GroupName != "" {
		query = query.Where("group_name LIKE ?", "%"+f.GroupName+"%")
	}
	if f.Username != "" {
		query = query.Where("username LIKE ?", "%"+f.Username+"%")
	}
	if f.ServiceName != "" {
		query = query.Where("service_name LIKE ?", "%"+f.ServiceName+"%")
	}
	if f.ApiKeyName != "" {
		query = query.Where("api_key_name LIKE ?", "%"+f.ApiKeyName+"%")
	}
	if f.Keyword != "" {
		query = query.Where("tool_name LIKE ? OR group_name LIKE ? OR service_name LIKE ? OR username LIKE ? OR error_message LIKE ?",
			"%"+f.Keyword+"%", "%"+f.Keyword+"%", "%"+f.Keyword+"%", "%"+f.Keyword+"%", "%"+f.Keyword+"%")
	}
	return query
}

func GetCallLogs(filter *LogFilter, offset, limit int) ([]McpCallLog, int64, error) {
	var logs []McpCallLog
	var total int64
	query := DB.Model(&McpCallLog{})
	query = applyLogFilter(query, filter)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error
	return logs, total, err
}

// CallLogRow 是健康聚合用的窄行:只取 5 列,不触碰 mediumtext 的 payload 列。
// ErrorMessage 供健康快照取"窗口内最近一次错误";成功行为空串,几乎无额外开销。
type CallLogRow struct {
	ServiceID      int64
	ResponseStatus string
	DurationMs     int
	ErrorMessage   string
	CreatedAt      time.Time
}

// GetLastConsumeTime 服务全历史最近一次消费调用时间(无则 nil);总览给空窗
// (近 200 分钟无调用)服务显示"上次调用"的时间锚点。MAX(created_at) 在
// glebarez/sqlite 下返回字符串而非 time.Time,经 flexTime 兼容扫描。
func GetLastConsumeTime(serviceID int64) *time.Time {
	row := DB.Model(&McpCallLog{}).
		Where("service_id = ? AND type = ?", serviceID, LogTypeConsume).
		Select("MAX(created_at)").Row()
	var ft flexTime
	if err := row.Scan(&ft); err != nil || !ft.ok {
		return nil
	}
	return &ft.t
}

// GetLastConsumeTimeForItem 市场条目全历史最近一次消费调用时间(无则 nil)。
// 市场管理页给空窗条目显示"上次调用"锚点——条目健康按日志表 marketplace_item_id
// 聚合,锚点同口径(marketplace_item_id 有索引,点查即索引定位)。
func GetLastConsumeTimeForItem(itemID int64) *time.Time {
	row := DB.Model(&McpCallLog{}).
		Where("marketplace_item_id = ? AND type = ?", itemID, LogTypeConsume).
		Select("MAX(created_at)").Row()
	var ft flexTime
	if err := row.Scan(&ft); err != nil || !ft.ok {
		return nil
	}
	return &ft.t
}

// flexTime 兼容聚合 MAX(created_at) 在不同驱动的返回:MySQL/PG 给 time.Time,
// glebarez/sqlite 给 "2006-01-02 15:04:05.9999999+08:00" 形态的字符串(带时区偏移)。
// 用原生 rows/Row.Scan 走 database/sql 的 Scanner 约定;GORM 的 .Scan() 会把
// 自定义类型字段当关系解析,不可用于 struct 字段。
type flexTime struct {
	t  time.Time
	ok bool
}

func (f *flexTime) Scan(v any) error {
	switch val := v.(type) {
	case nil:
		return nil
	case time.Time:
		f.t, f.ok = val, true
		return nil
	case string:
		return f.parse(val)
	case []byte:
		return f.parse(string(val))
	default:
		return fmt.Errorf("flexTime: unsupported driver value %T", v)
	}
}

func (f *flexTime) parse(s string) error {
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			f.t, f.ok = t, true
			return nil
		}
	}
	return fmt.Errorf("flexTime: unparsable time %q", s)
}

// GetCallLogRowsForServices 取一批服务 since 之后的调用窄行(created_at 有索引)。
// 只统计消费行(type=LogTypeConsume),与 GetCallLogStats 口径一致;时间分桶在
// Go 侧完成,避免 SQLite/MySQL/PG 三方言的时间表达式分叉。Limit 兜底防止病态
// 数据集撑爆内存(真实 24h 非 stdio 调用量远低于此,触顶时快照略微少计)。
func GetCallLogRowsForServices(serviceIDs []int64, since time.Time) ([]CallLogRow, error) {
	if len(serviceIDs) == 0 {
		return nil, nil
	}
	var rows []CallLogRow
	err := DB.Model(&McpCallLog{}).
		Where("service_id IN ? AND created_at >= ? AND type = ?", serviceIDs, since, LogTypeConsume).
		Select("service_id, response_status, duration_ms, error_message, created_at").
		Limit(200000).
		Find(&rows).Error
	return rows, err
}

// GetCallLogRowsForItems 取一批市场条目 since 之后的调用窄行,ServiceID 字段承载
// marketplace_item_id(SELECT 别名)——与 GetCallLogRowsForServices 同构,聚合核心
// (buildSnapshotsFromRows)按恒等键直接复用。marketplace_item_id 有索引,IN 列表
// 恒为条目数(小),不随用户安装量增长;且不依赖 mcp_services 引用行,用户卸载后
// 遗留的历史日志照样计入条目健康。
func GetCallLogRowsForItems(itemIDs []int64, since time.Time) ([]CallLogRow, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var rows []CallLogRow
	err := DB.Model(&McpCallLog{}).
		Where("marketplace_item_id IN ? AND created_at >= ? AND type = ?", itemIDs, since, LogTypeConsume).
		Select("marketplace_item_id AS service_id, response_status, duration_ms, error_message, created_at").
		Limit(200000).
		Find(&rows).Error
	return rows, err
}

// CountRecentFailures 统计某服务 since 之后的失败调用数。仅在健康回写的失败
// 路径触发(失败是少数事件),用于"5 分钟内失败 ≥3 → unhealthy"的判定。
func CountRecentFailures(serviceID int64, since time.Time) (int64, error) {
	var n int64
	err := DB.Model(&McpCallLog{}).
		Where("service_id = ? AND created_at >= ? AND response_status <> ? AND type = ?",
			serviceID, since, "success", LogTypeConsume).
		Count(&n).Error
	return n, err
}

// ApplyHealthWriteBack 依据真实调用结果回写 mcp_services.health_status。此前
// 只有池连接成功时写 healthy、从不写 unhealthy(常量定义了却无人写),此处补齐:
//   - 失败:5 分钟内失败 ≥3 → unhealthy(幂等,已是目标值不写)
//   - 成功:仅当前为 unhealthy 时恢复 healthy
//
// 网关调用(internal/mcp/handler recordLog/recordLogs)与服务详情页手动测试
// (service recordManualTestLog)共用此逻辑;调用方自行决定是否异步执行。
func ApplyHealthWriteBack(log *McpCallLog) {
	if log.ServiceID == 0 {
		return
	}
	if log.ResponseStatus == "success" {
		UpdateHealthStatusIfChanged(log.ServiceID, common.HealthHealthy)
		return
	}
	since := time.Now().Add(-5 * time.Minute)
	if n, err := CountRecentFailures(log.ServiceID, since); err == nil && n >= 3 {
		if UpdateHealthStatusIfChanged(log.ServiceID, common.HealthUnhealthy) {
			RecordSystemLog(0, "", "服务 "+log.ServiceName+" 连续失败,健康状态标记为 unhealthy", 0, "",
				map[string]any{"service_id": log.ServiceID, "recent_failures": n})
		}
	}
}

func GetCallLogsByUser(userID int64, filter *LogFilter, offset, limit int) ([]McpCallLog, int64, error) {
	var logs []McpCallLog
	var total int64
	query := DB.Model(&McpCallLog{}).Where("user_id = ?", userID)
	query = applyLogFilter(query, filter)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error
	return logs, total, err
}

type LogStatsResult struct {
	TotalCalls    int64
	SuccessCalls  int64
	FailedCalls   int64
	AvgDurationMs float64
	CallsToday    int64
}

func GetCallLogStats(filter *LogFilter) (*LogStatsResult, error) {
	// 统计恒为消费行,忽略调用方传入的 type 过滤,避免登录/注册/管理行污染调用统计。
	statFilter := &LogFilter{}
	if filter != nil {
		*statFilter = *filter
	}
	statFilter.Type = LogTypeConsume

	query := DB.Model(&McpCallLog{})
	query = applyLogFilter(query, statFilter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var successCount int64
	DB.Model(&McpCallLog{}).Scopes(func(db *gorm.DB) *gorm.DB {
		return applyLogFilter(db, statFilter)
	}).Where("response_status = ?", "success").Count(&successCount)

	var avgDuration float64
	db := DB.Model(&McpCallLog{})
	db = applyLogFilter(db, statFilter)
	db.Where("tool_name NOT IN ?", []string{"mcp.search", "mcp.describe"}).Select("COALESCE(AVG(duration_ms), 0)").Scan(&avgDuration)

	today := time.Now().Truncate(24 * time.Hour)
	var todayCount int64
	DB.Model(&McpCallLog{}).Where("type = ?", LogTypeConsume).Where("created_at >= ?", today).Count(&todayCount)

	return &LogStatsResult{
		TotalCalls:    total,
		SuccessCalls:  successCount,
		FailedCalls:   total - successCount,
		AvgDurationMs: math.Round(avgDuration*100) / 100,
		CallsToday:    todayCount,
	}, nil
}

func GetCallLogStatsByUser(userID int64, filter *LogFilter) (*LogStatsResult, error) {
	return getCallLogStatsInternal(userID, filter)
}

func GetCallLogsForUser(userID int64, isAdmin bool, filter *LogFilter, offset, limit int) ([]McpCallLog, int64, error) {
	var logs []McpCallLog
	var total int64
	query := DB.Model(&McpCallLog{})
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	query = applyLogFilter(query, filter)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error
	return logs, total, err
}

func GetCallLogStatsForUser(userID int64, isAdmin bool, filter *LogFilter) (*LogStatsResult, error) {
	if isAdmin {
		return getCallLogStatsInternal(0, filter)
	}
	return getCallLogStatsInternal(userID, filter)
}

func getCallLogStatsInternal(userID int64, filter *LogFilter) (*LogStatsResult, error) {
	userFilter := &LogFilter{}
	if filter != nil {
		*userFilter = *filter
	}
	// 统计恒为消费行,忽略调用方的 type 过滤。
	userFilter.Type = LogTypeConsume

	baseQuery := DB.Model(&McpCallLog{})
	if userID > 0 {
		baseQuery = baseQuery.Where("user_id = ?", userID)
	}
	q := applyLogFilter(baseQuery, userFilter)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}

	successQuery := DB.Model(&McpCallLog{})
	if userID > 0 {
		successQuery = successQuery.Where("user_id = ?", userID)
	}
	var successCount int64
	successQuery.Scopes(func(db *gorm.DB) *gorm.DB {
		return applyLogFilter(db, userFilter)
	}).Where("response_status = ?", "success").Count(&successCount)

	avgQuery := DB.Model(&McpCallLog{})
	if userID > 0 {
		avgQuery = avgQuery.Where("user_id = ?", userID)
	}
	var avgDuration float64
	applyLogFilter(avgQuery, userFilter).Where("tool_name NOT IN ?", []string{"mcp.search", "mcp.describe"}).Select("COALESCE(AVG(duration_ms), 0)").Scan(&avgDuration)

	today := time.Now().Truncate(24 * time.Hour)
	todayQuery := DB.Model(&McpCallLog{})
	if userID > 0 {
		todayQuery = todayQuery.Where("user_id = ?", userID)
	}
	var todayCount int64
	todayQuery.Where("type = ?", LogTypeConsume).Where("created_at >= ?", today).Count(&todayCount)

	return &LogStatsResult{
		TotalCalls:    total,
		SuccessCalls:  successCount,
		FailedCalls:   total - successCount,
		AvgDurationMs: math.Round(avgDuration*100) / 100,
		CallsToday:    todayCount,
	}, nil
}
