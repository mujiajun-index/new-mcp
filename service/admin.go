package service

import (
	"errors"
	"fmt"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// 超级管理员保护：id 为 1 的账号（super_admin）不可被改角色、禁用或删除，
// 普通管理员也不能修改其任何信息。
var ErrSuperAdminProtected = errors.New("普通管理员不能修改超级管理员的信息")
var ErrSuperAdminRoleProtected = errors.New("超级管理员的角色不可修改")
var ErrSuperAdminStatusProtected = errors.New("超级管理员不可禁用")
var ErrSuperAdminRoleReserved = errors.New("超级管理员角色不可分配")
var ErrUserNotFound = errors.New("用户不存在")

type AdminService struct{}

func (s *AdminService) ListUsers(actorRole string, page, pageSize int, keyword string) ([]dto.UserListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	// 普通管理员看不到超级管理员（id=1）。
	var excludeID int64
	if actorRole != common.RoleSuperAdmin {
		excludeID = common.SuperAdminUserID
	}
	users, total, err := model.ListUsersWithPaged(offset, pageSize, keyword, excludeID)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.UserListItem, len(users))
	for i, u := range users {
		items[i] = dto.UserListItem{
			ID:           u.ID,
			Username:     u.Username,
			DisplayName:  u.DisplayName,
			Email:        u.Email,
			Role:         u.Role,
			Status:       u.Status,
			Quota:        u.Quota,
			UsedQuota:    u.UsedQuota,
			RequestCount: u.RequestCount,
			Group:        u.Group,
			Remark:       u.Remark,
			CreatedAt:    u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, total, nil
}

// GetUserDetail 返回单个用户的详情（含审计字段）。普通管理员查询超级管理员时按隐藏规则返回 404。
func (s *AdminService) GetUserDetail(actorRole string, userID int64) (*dto.UserDetailResp, error) {
	user, err := model.GetUserByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	// 普通管理员看不到超级管理员（与列表隐藏一致），直接当作不存在。
	targetIsSuper := user.ID == common.SuperAdminUserID || user.Role == common.RoleSuperAdmin
	if targetIsSuper && actorRole != common.RoleSuperAdmin {
		return nil, ErrUserNotFound
	}

	resp := &dto.UserDetailResp{
		ID:           user.ID,
		Username:     user.Username,
		DisplayName:  user.DisplayName,
		Email:        user.Email,
		Role:         user.Role,
		Status:       user.Status,
		Quota:        user.Quota,
		UsedQuota:    user.UsedQuota,
		RequestCount: user.RequestCount,
		Group:        user.Group,
		Remark:       user.Remark,
		CreatedAt:    user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		RegisterIP:   user.RegisterIP,
		LastLoginIP:  user.LastLoginIP,
	}
	if user.LastLoginAt != nil {
		resp.LastLoginAt = user.LastLoginAt.Format("2006-01-02T15:04:05Z")
	}
	return resp, nil
}

func (s *AdminService) UpdateUser(actorRole string, userID int64, req *dto.AdminUpdateUserReq) error {
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		return err
	}

	// 超级管理员保护：目标为超级管理员（id=1 或 role=super_admin）。
	targetIsSuper := user.ID == common.SuperAdminUserID || user.Role == common.RoleSuperAdmin
	if targetIsSuper {
		// 普通管理员不能修改超级管理员的任何信息。
		if actorRole != common.RoleSuperAdmin {
			return ErrSuperAdminProtected
		}
		// 超级管理员本人也不能改自己的角色或把自己禁用（防自锁）；其余字段可正常修改。
		if req.Role != nil && *req.Role != user.Role {
			return ErrSuperAdminRoleProtected
		}
		if req.Status != nil && *req.Status != common.StatusEnabled {
			return ErrSuperAdminStatusProtected
		}
	} else if req.Role != nil && *req.Role == common.RoleSuperAdmin {
		// super_admin 固定为 id=1 独有，任何人都不能把别人提升为超级管理员。
		return ErrSuperAdminRoleReserved
	}

	if req.Status != nil {
		user.Status = *req.Status
	}
	if req.Role != nil {
		user.Role = *req.Role
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.Quota != nil {
		user.Quota = *req.Quota
	}
	if req.Group != nil {
		user.Group = *req.Group
	}
	if req.Remark != nil {
		user.Remark = *req.Remark
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := common.Password2Hash(*req.Password)
		if err != nil {
			return err
		}
		user.Password = hash
	}
	return model.DB.Save(&user).Error
}

func (s *AdminService) CreateUser(req *dto.AdminCreateUserReq) (*dto.UserListItem, error) {
	// super_admin 固定为 id=1 独有，禁止通过后台创建该角色。
	if req.Role == common.RoleSuperAdmin {
		return nil, ErrSuperAdminRoleReserved
	}
	if _, err := model.GetUserByUsername(req.Username); err == nil {
		return nil, fmt.Errorf("用户名已存在")
	}

	hash, err := common.Password2Hash(req.Password)
	if err != nil {
		return nil, err
	}

	role := req.Role
	if role == "" {
		role = common.RoleCommonUser
	}
	group := req.Group
	if group == "" {
		group = "default"
	}

	user := &model.User{
		Username:    req.Username,
		Password:    hash,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Role:        role,
		Status:      common.StatusEnabled,
		Quota:       req.Quota,
		Group:       group,
	}
	if err := user.Insert(); err != nil {
		return nil, err
	}

	return &dto.UserListItem{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Role:        user.Role,
		Status:      user.Status,
		Quota:       user.Quota,
		Group:       user.Group,
		CreatedAt:   user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (s *AdminService) GetStats() (*dto.AdminStats, error) {
	var usersCount, servicesCount, groupsCount, connectionsCount int64
	model.DB.Model(&model.User{}).Count(&usersCount)
	model.DB.Model(&model.McpService{}).Count(&servicesCount)
	model.DB.Model(&model.McpGroup{}).Count(&groupsCount)
	model.DB.Model(&model.CloudEndpoint{}).Count(&connectionsCount)

	stats, err := model.GetCallLogStats(nil)
	if err != nil {
		return nil, err
	}

	var successRate float64
	if stats.TotalCalls > 0 {
		successRate = float64(stats.SuccessCalls) / float64(stats.TotalCalls) * 100
	}

	return &dto.AdminStats{
		UsersCount:       usersCount,
		ServicesCount:    servicesCount,
		GroupsCount:      groupsCount,
		ConnectionsCount: connectionsCount,
		CallsToday:       stats.CallsToday,
		CallsSuccessRate: successRate,
		AvgLatencyMs:     stats.AvgDurationMs,
	}, nil
}

func (s *AdminService) GetLogsForUser(userID int64, isAdmin bool, filter *dto.LogFilter, page, pageSize int) ([]dto.LogItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	modelFilter := dtoToModelFilter(filter)

	logs, total, err := model.GetCallLogsForUser(userID, isAdmin, modelFilter, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.LogItem, len(logs))
	for i, l := range logs {
		items[i] = dto.LogItem{
			ID:             l.ID,
			UserID:         l.UserID,
			Username:       l.Username,
			ApiKeyID:       l.ApiKeyID,
			ApiKeyName:     l.ApiKeyName,
			GroupID:        l.GroupID,
			GroupName:      l.GroupName,
			ServiceID:      l.ServiceID,
			ServiceName:    l.ServiceName,
			ToolName:       l.ToolName,
			Method:         l.Method,
			ResponseStatus: l.ResponseStatus,
			DurationMs:     l.DurationMs,
			ErrorMessage:   l.ErrorMessage,
			ClientIP:       l.ClientIP,
			CreatedAt:      l.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, total, nil
}

func (s *AdminService) GetLogStatsForUser(userID int64, isAdmin bool, filter *dto.LogFilter) (*dto.LogStats, error) {
	modelFilter := dtoToModelFilter(filter)
	stats, err := model.GetCallLogStatsForUser(userID, isAdmin, modelFilter)
	if err != nil {
		return nil, err
	}
	return &dto.LogStats{
		TotalCalls:    stats.TotalCalls,
		SuccessCalls:  stats.SuccessCalls,
		FailedCalls:   stats.FailedCalls,
		AvgDurationMs: stats.AvgDurationMs,
		CallsToday:    stats.CallsToday,
	}, nil
}

func dtoToModelFilter(f *dto.LogFilter) *model.LogFilter {
	if f == nil {
		return nil
	}
	return &model.LogFilter{
		StartDate:   f.StartDate,
		EndDate:     f.EndDate,
		Status:      f.Status,
		ToolName:    f.ToolName,
		GroupName:   f.GroupName,
		Username:    f.Username,
		ServiceName: f.ServiceName,
		Keyword:     f.Keyword,
	}
}
