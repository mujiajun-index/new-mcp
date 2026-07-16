package model

import (
	"time"

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

// GetBillingLogsByUser 返回用户的消费明细(仅计费相关行,排除 skipped),按时间倒序分页。
func GetBillingLogsByUser(userID int64, offset, limit int) ([]McpCallLog, int64, error) {
	q := DB.Model(&McpCallLog{}).Where("user_id = ? AND billing_status != ?", userID, "skipped")
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []McpCallLog
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// DeleteCallLogsBefore 删除 created_at 早于指定时间的调用日志(TTL 清理,§4.5)。
// 返回删除行数。三库通用(DELETE ... WHERE created_at < ?)。
func DeleteCallLogsBefore(t time.Time) (int64, error) {
	res := DB.Where("created_at < ?", t).Delete(&McpCallLog{})
	return res.RowsAffected, res.Error
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
	Keyword     string
}

func applyLogFilter(query *gorm.DB, f *LogFilter) *gorm.DB {
	if f == nil {
		return query
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
	query := DB.Model(&McpCallLog{})
	query = applyLogFilter(query, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var successCount int64
	DB.Model(&McpCallLog{}).Scopes(func(db *gorm.DB) *gorm.DB {
		return applyLogFilter(db, filter)
	}).Where("response_status = ?", "success").Count(&successCount)

	var avgDuration float64
	db := DB.Model(&McpCallLog{})
	db = applyLogFilter(db, filter)
	db.Where("tool_name NOT IN ?", []string{"mcp.search", "mcp.describe"}).Select("COALESCE(AVG(duration_ms), 0)").Scan(&avgDuration)

	today := time.Now().Truncate(24 * time.Hour)
	var todayCount int64
	DB.Model(&McpCallLog{}).Where("created_at >= ?", today).Count(&todayCount)

	return &LogStatsResult{
		TotalCalls:    total,
		SuccessCalls:  successCount,
		FailedCalls:   total - successCount,
		AvgDurationMs: avgDuration,
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
	todayQuery.Where("created_at >= ?", today).Count(&todayCount)

	return &LogStatsResult{
		TotalCalls:    total,
		SuccessCalls:  successCount,
		FailedCalls:   total - successCount,
		AvgDurationMs: avgDuration,
		CallsToday:    todayCount,
	}, nil
}
