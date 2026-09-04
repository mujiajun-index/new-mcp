package model

import (
	"time"

	"github.com/mujkjk/newmcp/common"
)

type McpGroupService struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	GroupID    int64     `json:"group_id" gorm:"not null;uniqueIndex:idx_group_service"`
	ServiceID  int64     `json:"service_id" gorm:"not null;uniqueIndex:idx_group_service;index"`
	// 无 default 标签(GORM 对带 default 的 bool 零值 INSERT 时省略该列);
	// AddServicesToGroup 显式置 true,未来「禁用成员」写 false 也能落库。
	Enabled    bool      `json:"enabled"`
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
		items[i] = McpGroupService{GroupID: groupID, ServiceID: sid, Enabled: true}
	}
	return DB.Create(&items).Error
}

func RemoveServiceFromGroup(groupID, serviceID int64) error {
	return DB.Where("group_id = ? AND service_id = ?", groupID, serviceID).Delete(&McpGroupService{}).Error
}

// DeleteGroupServicesByServiceID 将服务从所属的全部分组中移除（解除关联）。
// 用于禁用/删除服务时清理，避免向分组继续暴露已停用的服务。
func DeleteGroupServicesByServiceID(serviceID int64) error {
	return DB.Where("service_id = ?", serviceID).Delete(&McpGroupService{}).Error
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
// Services with a non-enabled row status are skipped defensively: normal disable paths
// delete the membership rows, but any leftover row must not leak a disabled service
// into mcp.search / gateway routing / resources-prompts aggregation.
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
			// 防御性排除:分组聚合只看成员行 enabled,不看服务行 status——正常禁用
			// 路径已删关联行,此处兜底阻止残留行把禁用服务泄入路由/搜索文档。
			if svc := svcByID[sid]; svc != nil && svc.Status == common.StatusEnabled {
				result = append(result, GroupServicePair{Group: g, Service: *svc})
			}
		}
	}
	return result, nil
}
