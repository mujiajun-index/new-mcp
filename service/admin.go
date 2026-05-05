package service

import (
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type AdminService struct{}

func (s *AdminService) ListUsers(page, pageSize int) ([]dto.UserListItem, int64, error) {
	var users []model.User
	var total int64
	offset := common.GetOffset(page, pageSize)
	query := model.DB.Model(&model.User{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(pageSize).Order("id ASC").Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.UserListItem, len(users))
	for i, u := range users {
		items[i] = dto.UserListItem{
			ID:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			Role:      u.Role,
			Status:    u.Status,
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
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
	return model.DB.Save(&user).Error
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

func (s *AdminService) GetLogs(filter *dto.LogFilter, page, pageSize int) ([]dto.LogItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	modelFilter := dtoToModelFilter(filter)

	logs, total, err := model.GetCallLogs(modelFilter, offset, pageSize)
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

func (s *AdminService) GetLogStats(filter *dto.LogFilter) (*dto.LogStats, error) {
	modelFilter := dtoToModelFilter(filter)
	stats, err := model.GetCallLogStats(modelFilter)
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

func (s *AdminService) GetUserLogs(userID int64, filter *dto.LogFilter, page, pageSize int) ([]dto.LogItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	modelFilter := dtoToModelFilter(filter)

	logs, total, err := model.GetCallLogsByUser(userID, modelFilter, offset, pageSize)
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

func (s *AdminService) GetUserLogStats(userID int64, filter *dto.LogFilter) (*dto.LogStats, error) {
	modelFilter := dtoToModelFilter(filter)
	stats, err := model.GetCallLogStatsByUser(userID, modelFilter)
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
