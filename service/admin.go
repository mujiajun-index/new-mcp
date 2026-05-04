package service

import (
	"time"

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

	var callsToday int64
	today := time.Now().Truncate(24 * time.Hour)
	model.DB.Model(&model.McpCallLog{}).Where("created_at >= ?", today).Count(&callsToday)

	return &dto.AdminStats{
		UsersCount:       usersCount,
		ServicesCount:    servicesCount,
		GroupsCount:      groupsCount,
		ConnectionsCount: connectionsCount,
		CallsToday:       callsToday,
	}, nil
}

func (s *AdminService) GetLogs(page, pageSize int) ([]dto.LogItem, int64, error) {
	var logs []model.McpCallLog
	var total int64
	offset := common.GetOffset(page, pageSize)
	query := model.DB.Model(&model.McpCallLog{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.LogItem, len(logs))
	for i, l := range logs {
		items[i] = dto.LogItem{
			ID:             l.ID,
			UserID:         l.UserID,
			ServiceID:      l.ServiceID,
			GroupID:        l.GroupID,
			ToolName:       l.ToolName,
			ResponseStatus: l.ResponseStatus,
			DurationMs:     l.DurationMs,
			ClientIP:       l.ClientIP,
			CreatedAt:      l.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, total, nil
}
