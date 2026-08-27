package service

import (
	"errors"
	"fmt"
	"strings"

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
var ErrCannotManageTarget = errors.New("无权操作该用户")
var ErrInvalidQuotaMode = errors.New("无效的调额模式")
var ErrCannotDeleteSelf = errors.New("不能删除自己的账号")
var ErrUserNotDeleted = errors.New("该用户未被删除,无需恢复")

type AdminService struct{}

func (s *AdminService) ListUsers(actorRole string, page, pageSize int, keyword, role string, status int, deletedOnly bool) ([]dto.UserListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	// 普通管理员看不到超级管理员（id=1）。
	var excludeID int64
	if actorRole != common.RoleSuperAdmin {
		excludeID = common.SuperAdminUserID
	}
	users, total, err := model.ListUsersWithPaged(offset, pageSize, keyword, excludeID, role, status, deletedOnly)
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

func (s *AdminService) UpdateUser(actor model.Operator, userID int64, req *dto.AdminUpdateUserReq) error {
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		return err
	}

	// 超级管理员保护：目标为超级管理员（id=1 或 role=super_admin）。
	targetIsSuper := user.ID == common.SuperAdminUserID || user.Role == common.RoleSuperAdmin
	if targetIsSuper {
		// 普通管理员不能修改超级管理员的任何信息。
		if actor.Role != common.RoleSuperAdmin {
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

	// 审计:边应用变更边收集本次改动的字段。
	var changes []string
	extra := map[string]any{}
	if req.Status != nil {
		user.Status = *req.Status
		changes = append(changes, "状态")
		extra["status"] = *req.Status
	}
	if req.Role != nil {
		user.Role = *req.Role
		changes = append(changes, "角色")
		extra["role"] = *req.Role
	}
	if req.Email != nil {
		user.Email = *req.Email
		changes = append(changes, "邮箱")
		extra["email"] = *req.Email
	}
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
		changes = append(changes, "昵称")
	}
	if req.Quota != nil {
		user.Quota = *req.Quota
		changes = append(changes, "额度")
		extra["quota"] = *req.Quota
	}
	if req.Group != nil {
		user.Group = *req.Group
		changes = append(changes, "分组")
		extra["group"] = *req.Group
	}
	if req.Remark != nil {
		user.Remark = *req.Remark
		changes = append(changes, "备注")
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := common.Password2Hash(*req.Password)
		if err != nil {
			return err
		}
		user.Password = hash
		changes = append(changes, "密码")
	}
	if err := model.DB.Save(&user).Error; err != nil {
		return err
	}
	if len(changes) > 0 {
		model.RecordManageLog(userID, user.Username, "管理员更新用户:"+strings.Join(changes, "、"), 0, actor, extra)
	}
	return nil
}

func (s *AdminService) CreateUser(actor model.Operator, req *dto.AdminCreateUserReq) (*dto.UserListItem, error) {
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

	model.RecordManageLog(user.ID, user.Username, "管理员创建用户", req.Quota, actor, map[string]any{
		"role":          role,
		"group":         group,
		"initial_quota": req.Quota,
	})

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

// AdjustUserQuota 管理员调额(D13):mode=add/sub/set,value 单位 quota。
// 权限(canManageTargetRole):普通管理员不得操作超级管理员或同级管理员;超级管理员可操作任意用户。
// 审计:写入 Manage 日志,额度变动量 = 调整后 - 调整前。
// 返回调整后的最新额度。
func (s *AdminService) AdjustUserQuota(actor model.Operator, targetID int64, req *dto.AdminAdjustQuotaReq) (int64, error) {
	user, err := model.GetUserByID(targetID)
	if err != nil {
		return 0, ErrUserNotFound
	}

	// canManageTargetRole
	targetIsSuper := user.ID == common.SuperAdminUserID || user.Role == common.RoleSuperAdmin
	if targetIsSuper && actor.Role != common.RoleSuperAdmin {
		return 0, ErrSuperAdminProtected
	}
	if user.Role == common.RoleAdminUser && actor.Role != common.RoleSuperAdmin {
		return 0, ErrCannotManageTarget // 普通管理员不能操作同级管理员
	}

	value := req.Value
	if value < 0 {
		value = -value
	}

	oldQuota := user.Quota
	var newQuota int64
	switch req.Mode {
	case "add":
		if err := model.IncreaseUserQuota(targetID, value); err != nil {
			return 0, err
		}
		newQuota = user.Quota + value
	case "sub":
		if user.Quota >= value {
			if err := model.DecreaseUserQuotaUnguarded(targetID, value); err != nil {
				return 0, err
			}
			newQuota = user.Quota - value
		} else {
			if err := model.SetUserQuota(targetID, 0); err != nil { // 不足则清零,不允许负额度
				return 0, err
			}
			newQuota = 0
		}
	case "set":
		if err := model.SetUserQuota(targetID, value); err != nil {
			return 0, err
		}
		newQuota = value
	default:
		return 0, ErrInvalidQuotaMode
	}

	modeText := req.Mode
	switch req.Mode {
	case "add":
		modeText = "增加"
	case "sub":
		modeText = "减少"
	case "set":
		modeText = "设置"
	}
	model.RecordManageLog(targetID, user.Username,
		fmt.Sprintf("管理员调整额度(%s %s),%s → %s",
			modeText, model.FormatQuotaCurrency(value), model.FormatQuotaCurrency(oldQuota), model.FormatQuotaCurrency(newQuota)),
		newQuota-oldQuota, actor,
		map[string]any{"mode": req.Mode, "value": value, "remark": req.Remark, "before": oldQuota, "after": newQuota})
	return newQuota, nil
}

// DeleteUser 软删除用户(管理员):仅置 deleted_at,名下数据(API Key/分组/服务/额度/
// 日志)全部保留,可在"已删除"筛选中恢复(RestoreUser)。删除期间登录与 API Key 认证
// 立即失效(默认作用域查无此用户),用户名继续占用唯一索引以防抢注。禁止删除自己、
// 超级管理员;普通管理员仅能删除普通用户。
func (s *AdminService) DeleteUser(actor model.Operator, targetID int64) error {
	if actor.ID == targetID {
		return ErrCannotDeleteSelf
	}
	user, err := model.GetUserByID(targetID)
	if err != nil {
		return ErrUserNotFound
	}
	// canManageTargetRole:超级管理员受保护;普通管理员不能操作管理员
	targetIsSuper := user.ID == common.SuperAdminUserID || user.Role == common.RoleSuperAdmin
	if targetIsSuper {
		return ErrSuperAdminProtected
	}
	if user.Role == common.RoleAdminUser && actor.Role != common.RoleSuperAdmin {
		return ErrCannotManageTarget
	}
	if err := model.SoftDeleteUserByID(targetID); err != nil {
		return err
	}
	model.RecordManageLog(targetID, user.Username,
		fmt.Sprintf("管理员删除用户(%s,软删除可恢复)", user.Username),
		0, actor,
		map[string]any{"action": "delete", "username": user.Username})
	return nil
}

// RestoreUser 恢复软删除的用户:清空 deleted_at。名下数据在删除期间从未被清理,
// 恢复即可用。权限守卫与删除一致(超管受保护;普通管理员仅能恢复普通用户)。
func (s *AdminService) RestoreUser(actor model.Operator, targetID int64) error {
	// 目标是软删除行,须用 Unscoped 读取做权限校验
	user, err := model.GetUserByIDUnscoped(targetID)
	if err != nil {
		return ErrUserNotFound
	}
	targetIsSuper := user.ID == common.SuperAdminUserID || user.Role == common.RoleSuperAdmin
	if targetIsSuper {
		return ErrSuperAdminProtected
	}
	if user.Role == common.RoleAdminUser && actor.Role != common.RoleSuperAdmin {
		return ErrCannotManageTarget
	}
	restored, err := model.RestoreUserByID(targetID)
	if err != nil {
		return err
	}
	if !restored {
		return ErrUserNotDeleted
	}
	model.RecordManageLog(targetID, user.Username,
		fmt.Sprintf("管理员恢复用户(%s)", user.Username),
		0, actor,
		map[string]any{"action": "restore", "username": user.Username})
	return nil
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
			ID:                l.ID,
			UserID:            l.UserID,
			Username:          l.Username,
			ApiKeyID:          l.ApiKeyID,
			ApiKeyName:        l.ApiKeyName,
			GroupID:           l.GroupID,
			GroupName:         l.GroupName,
			ServiceID:         l.ServiceID,
			ServiceName:       l.ServiceName,
			ToolName:          l.ToolName,
			Method:            l.Method,
			RequestID:         l.RequestID,
			ResponseStatus:    l.ResponseStatus,
			DurationMs:        l.DurationMs,
			ErrorMessage:      l.ErrorMessage,
			ClientIP:          l.ClientIP,
			CreatedAt:         l.CreatedAt.Format("2006-01-02T15:04:05Z"),
			BillingStatus:     l.BillingStatus,
			BillingType:       l.BillingType,
			UnitPrice:         l.UnitPrice,
			QuotaConsumed:     l.QuotaConsumed,
			PriceScope:        l.PriceScope,
			MarketplaceItemID: l.MarketplaceItemID,
			Type:              l.Type,
			Content:           l.Content,
			Extra:             l.Extra,
		}
		// 普通用户视图剥离 operator 等审计字段。
		if !isAdmin {
			items[i].Extra = model.StripAuditExtra(l.Extra)
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
		ApiKeyName:  f.ApiKeyName,
		Keyword:     f.Keyword,
		Type:        f.Type,
	}
}
