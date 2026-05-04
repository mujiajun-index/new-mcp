package model

import "time"

type McpGroupService struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	GroupID    int64     `json:"group_id" gorm:"not null;uniqueIndex:idx_group_service"`
	ServiceID  int64     `json:"service_id" gorm:"not null;uniqueIndex:idx_group_service;index"`
	Enabled    bool      `json:"enabled" gorm:"default:true"`
	SortOrder  int       `json:"sort_order" gorm:"default:0"`
	CreatedAt  time.Time `json:"created_at"`
}

func (McpGroupService) TableName() string { return "mcp_group_services" }

func GetGroupServices(groupID int64) ([]McpGroupService, error) {
	var items []McpGroupService
	err := DB.Where("group_id = ?", groupID).Order("sort_order ASC").Find(&items).Error
	return items, err
}

func GetEnabledGroupServices(groupID int64) ([]McpGroupService, error) {
	var items []McpGroupService
	err := DB.Where("group_id = ? AND enabled = ?", groupID, true).Order("sort_order ASC").Find(&items).Error
	return items, err
}

func AddServicesToGroup(groupID int64, serviceIDs []int64) error {
	items := make([]McpGroupService, len(serviceIDs))
	for i, sid := range serviceIDs {
		items[i] = McpGroupService{GroupID: groupID, ServiceID: sid}
	}
	return DB.Create(&items).Error
}

func RemoveServiceFromGroup(groupID, serviceID int64) error {
	return DB.Where("group_id = ? AND service_id = ?", groupID, serviceID).Delete(&McpGroupService{}).Error
}
