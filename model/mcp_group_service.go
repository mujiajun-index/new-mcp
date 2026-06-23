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

// GetEnabledGroupServicesByGroupIDs returns enabled memberships for many groups in a
// single query, ordered by (group_id, sort_order) for stable group/service pairing.
func GetEnabledGroupServicesByGroupIDs(groupIDs []int64) ([]McpGroupService, error) {
	var items []McpGroupService
	err := DB.Where("group_id IN ? AND enabled = ?", groupIDs, true).Order("group_id, sort_order").Find(&items).Error
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

// GroupServicePair pairs a service with one of the groups through which it is enabled.
type GroupServicePair struct {
	Group   McpGroup
	Service McpService
}

// ResolveEnabledServicesForGroups returns every enabled service reachable through the
// given groups, each paired with the group it was reached through. It uses two batched
// queries (memberships, then services) instead of one query per group and per service.
// Order is stable: groups in the given order, services in intra-group sort_order.
func ResolveEnabledServicesForGroups(groups []McpGroup) ([]GroupServicePair, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	groupIDs := make([]int64, len(groups))
	for i, g := range groups {
		groupIDs[i] = g.ID
	}

	memberships, err := GetEnabledGroupServicesByGroupIDs(groupIDs)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, nil
	}

	// Unique service IDs -> fetch all services in one query.
	seen := make(map[int64]struct{}, len(memberships))
	for _, m := range memberships {
		seen[m.ServiceID] = struct{}{}
	}
	svcIDs := make([]int64, 0, len(seen))
	for id := range seen {
		svcIDs = append(svcIDs, id)
	}
	services, err := GetServicesByIDs(svcIDs)
	if err != nil {
		return nil, err
	}
	svcByID := make(map[int64]*McpService, len(services))
	for i := range services {
		svcByID[services[i].ID] = &services[i]
	}

	// Pair services with their groups, preserving group + sort order.
	byGroup := make(map[int64][]int64, len(groups))
	for _, m := range memberships {
		byGroup[m.GroupID] = append(byGroup[m.GroupID], m.ServiceID)
	}
	result := make([]GroupServicePair, 0, len(memberships))
	for _, g := range groups {
		for _, sid := range byGroup[g.ID] {
			if svc := svcByID[sid]; svc != nil {
				result = append(result, GroupServicePair{Group: g, Service: *svc})
			}
		}
	}
	return result, nil
}
