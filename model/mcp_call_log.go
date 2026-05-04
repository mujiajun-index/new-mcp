package model

import "time"

type McpCallLog struct {
	ID              int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID          *int64    `json:"user_id" gorm:"index"`
	ApiKeyID        *int64    `json:"api_key_id"`
	DeviceID        *int64    `json:"device_id"`
	GroupID         *int64    `json:"group_id" gorm:"index"`
	ServiceID       *int64    `json:"service_id" gorm:"index"`
	ToolName        string    `json:"tool_name" gorm:"size:255;not null;index"`
	Method          string    `json:"method" gorm:"size:64"`
	RequestPayload  string    `json:"request_payload" gorm:"type:mediumtext"`
	ResponseStatus  string    `json:"response_status" gorm:"size:16"`
	ResponsePayload string    `json:"response_payload" gorm:"type:mediumtext"`
	DurationMs      int       `json:"duration_ms" gorm:"default:0"`
	ErrorMessage    string    `json:"error_message" gorm:"type:text"`
	ClientIP        string    `json:"client_ip" gorm:"size:64"`
	UserAgent       string    `json:"user_agent" gorm:"size:512"`
	CreatedAt       time.Time `json:"created_at" gorm:"index"`
}

func (McpCallLog) TableName() string { return "mcp_call_logs" }

func (l *McpCallLog) Insert() error {
	return DB.Create(l).Error
}
