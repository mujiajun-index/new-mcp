package model

import "time"

type McpGroupTool struct {
	ID                 int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	GroupID            int64     `json:"group_id" gorm:"not null;uniqueIndex:idx_group_service_tool"`
	ServiceID          int64     `json:"service_id" gorm:"not null;uniqueIndex:idx_group_service_tool;index"`
	ToolName           string    `json:"tool_name" gorm:"size:255;not null;uniqueIndex:idx_group_service_tool"`
	Enabled            bool      `json:"enabled" gorm:"default:true"`
	NameOverride       string    `json:"name_override" gorm:"size:255"`
	DescriptionOverride string   `json:"description_override" gorm:"type:text"`
	Annotations        string    `json:"annotations" gorm:"type:text;default:'{}'"`
	CreatedAt          time.Time `json:"created_at"`
}

func (McpGroupTool) TableName() string { return "mcp_group_tools" }

func GetGroupTools(groupID int64) ([]McpGroupTool, error) {
	var tools []McpGroupTool
	err := DB.Where("group_id = ?", groupID).Find(&tools).Error
	return tools, err
}

func GetGroupTool(groupID, serviceID int64, toolName string) (*McpGroupTool, error) {
	var tool McpGroupTool
	err := DB.Where("group_id = ? AND service_id = ? AND tool_name = ?", groupID, serviceID, toolName).First(&tool).Error
	return &tool, err
}

func (t *McpGroupTool) Upsert() error {
	return DB.Save(t).Error
}
