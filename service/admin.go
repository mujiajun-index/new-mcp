package service

import (
	"fmt"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type AdminService struct{}

func (s *AdminService) ListUsers(page, pageSize int, keyword string) ([]dto.UserListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	users, total, err := model.ListUsersWithPaged(offset, pageSize, keyword)
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

func (s *AdminService) UpdateUser(userID int64, req *dto.AdminUpdateUserReq) error {
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		return err
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
