package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
)

var ErrUsernameExists = errors.New("用户名已存在")
var ErrInvalidCredentials = errors.New("用户名或密码错误")
var ErrUserDisabled = errors.New("用户已被禁用")
var ErrWrongPassword = errors.New("原密码不正确")
var ErrRegisterDisabled = errors.New("注册功能已禁用")
var ErrEmailDomainRestricted = errors.New("该邮箱域名不在允许列表中")
var ErrEmailVerificationRequired = errors.New("请完成邮箱验证")
var ErrVerificationCodeInvalid = errors.New("验证码错误或已过期")
var ErrEmailAlreadyBound = errors.New("该邮箱已被其他账号绑定")

type AuthService struct{}

func (s *AuthService) Register(registerIP string, req *dto.RegisterReq) (*dto.AuthResp, error) {
	if !model.GetOptionBool("RegisterEnabled") {
		return nil, ErrRegisterDisabled
	}

	if req.Email != "" && !model.IsEmailDomainAllowed(req.Email) {
		return nil, ErrEmailDomainRestricted
	}

	// When email verification is enabled, the email + a valid verification
	// code are mandatory. Verified on the email actually being registered.
	if model.GetOptionBool("EmailVerificationEnabled") {
		if req.Email == "" || req.VerificationCode == "" {
			return nil, ErrEmailVerificationRequired
		}
		if !common.VerifyCodeWithKey(req.Email, req.VerificationCode, common.EmailVerificationPurpose) {
			return nil, ErrVerificationCodeInvalid
		}
		common.DeleteKey(req.Email, common.EmailVerificationPurpose)
	}

	if _, err := model.GetUserByUsername(req.Username); err == nil {
		return nil, ErrUsernameExists
	}

	hash, err := common.Password2Hash(req.Password)
	if err != nil {
		return nil, err
	}

	// 解析邀请人(对齐 new-api:GetUserIdByAffCode;空/坏码 → 0,不阻断注册)。
	inviterID, _ := model.GetUserIDByAffCode(req.InviteCode)

	// 注册赠送额度(§7.1 QuotaForNewUser,可配)。默认 0=不赠送。
	newUserQuota := model.GetOptionInt64("QuotaForNewUser")
	if newUserQuota < 0 {
		newUserQuota = 0
	}
	user := &model.User{
		Username:   req.Username,
		Password:   hash,
		Email:      req.Email,
		Role:       common.RoleCommonUser,
		Status:     common.StatusEnabled,
		Group:      "default",
		Quota:      newUserQuota,
		InviterID:  inviterID,
		RegisterIP: registerIP,
	}
	if err := user.Insert(); err != nil {
		return nil, err
	}

	// 邀请奖励(对齐 new-api finishInsert;无支付合规门禁——配置>0 即发放)。
	s.grantInviteRewards(user, inviterID, registerIP)

	// 系统日志:记录注册(若有注册赠送额度,一并体现在文案与额度变动量)。
	registerContent := "用户注册"
	if newUserQuota > 0 {
		registerContent = fmt.Sprintf("用户注册(赠送 %d 额度)", newUserQuota)
	}
	model.RecordSystemLog(user.ID, user.Username, registerContent, newUserQuota, registerIP, map[string]any{
		"email":       req.Email,
		"register_ip": registerIP,
		"inviter_id":  inviterID,
	})

	token, err := middleware.GenerateToken(user)
	if err != nil {
		return nil, err
	}

	return &dto.AuthResp{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
		Token:    token,
	}, nil
}

// grantInviteRewards 发放邀请奖励(对齐 new-api finishInsert 的邀请分支;无支付合规门禁)。
//   - 受邀者:QuotaForInvitee > 0 时直接进受邀者钱包 quota(IncreaseUserQuota);
//   - 邀请者:inviterID != 0 且 QuotaForInviter > 0 时,进邀请者 aff_quota 待提取(RewardInviter)。
//
// 奖励发放失败不阻断注册(仅记日志),与 new-api 的 _ = IncreaseUserQuota(...) 容错一致。
func (s *AuthService) grantInviteRewards(user *model.User, inviterID int64, registerIP string) {
	if quotaForInvitee := model.GetOptionInt64("QuotaForInvitee"); quotaForInvitee > 0 {
		_ = model.IncreaseUserQuota(user.ID, quotaForInvitee)
		model.RecordSystemLog(user.ID, user.Username,
			fmt.Sprintf("使用邀请码赠送 %d 额度", quotaForInvitee),
			quotaForInvitee, registerIP, map[string]any{"type": "invitee_reward"})
	}
	if inviterID != 0 {
		if quotaForInviter := model.GetOptionInt64("QuotaForInviter"); quotaForInviter > 0 {
			if err := model.RewardInviter(inviterID, quotaForInviter); err != nil {
				return
			}
			// 邀请人日志:取邀请人用户名,失败则留空(日志仍以邀请人 id 为 owner)。
			inviterUsername := ""
			if inviter, err := model.GetUserByID(inviterID); err == nil {
				inviterUsername = inviter.Username
			}
			model.RecordSystemLog(inviterID, inviterUsername,
				fmt.Sprintf("邀请用户赠送 %d 额度(待提取)", quotaForInviter),
				quotaForInviter, registerIP, map[string]any{"type": "inviter_reward", "invitee_id": user.ID})
		}
	}
}

func (s *AuthService) Login(loginIP string, req *dto.LoginReq) (*dto.AuthResp, error) {
	// Login accepts either a username or an email address.
	user, err := model.GetUserByUsernameOrEmail(strings.TrimSpace(req.Username))
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !common.ValidatePasswordAndHash(req.Password, user.Password) {
		return nil, ErrInvalidCredentials
	}
	// 密码正确但账号被禁用：返回明确的禁用提示，而非"用户名或密码错误"。
	// 仅在凭据校验通过后才暴露状态，避免攻击者借此枚举有效用户名。
	if user.Status != common.StatusEnabled {
		return nil, ErrUserDisabled
	}

	// 记录最后登录时间与 IP；定向更新，避免覆盖并发变更的 quota/request_count。
	now := time.Now()
	_ = model.DB.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"last_login_at": now,
		"last_login_ip": loginIP,
	})

	// 登录日志。
	model.RecordLoginLog(user.ID, user.Username, loginIP, nil)

	token, err := middleware.GenerateToken(user)
	if err != nil {
		return nil, err
	}

	return &dto.AuthResp{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
		Token:    token,
	}, nil
}

func (s *AuthService) GetProfile(userID int64) (*dto.ProfileResp, error) {
	user, err := model.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	return &dto.ProfileResp{
		ID:           user.ID,
		Username:     user.Username,
		DisplayName:  user.DisplayName,
		Email:        user.Email,
		Role:         user.Role,
		AvatarURL:    user.AvatarURL,
		Status:       user.Status,
		Quota:        user.Quota,
		UsedQuota:    user.UsedQuota,
		RequestCount: user.RequestCount,
		Group:        user.Group,
		CreatedAt:    user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (s *AuthService) UpdateProfile(userID int64, req *dto.UpdateProfileReq) error {
	user, err := model.GetUserByID(userID)
	if err != nil {
		return err
	}
	// Email binding / change. When SMTP is configured the new address must be
	// verified via a code sent to it; otherwise it may be set directly.
	if req.Email != nil && *req.Email != user.Email {
		newEmail := *req.Email
		if !model.IsEmailDomainAllowed(newEmail) {
			return ErrEmailDomainRestricted
		}
		if model.IsEmailAlreadyTaken(newEmail) {
			return ErrEmailAlreadyBound
		}
		if model.IsSMTPConfigured() {
			if req.EmailVerificationCode == "" {
				return ErrEmailVerificationRequired
			}
			if !common.VerifyCodeWithKey(newEmail, req.EmailVerificationCode, common.EmailBindPurpose) {
				return ErrVerificationCodeInvalid
			}
			common.DeleteKey(newEmail, common.EmailBindPurpose)
		}
		user.Email = newEmail
	}
	if req.AvatarURL != nil {
		user.AvatarURL = *req.AvatarURL
	}
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	return user.Update()
}

func (s *AuthService) ChangePassword(userID int64, req *dto.ChangePasswordReq) error {
	user, err := model.GetUserByID(userID)
	if err != nil {
		return err
	}
	if !common.ValidatePasswordAndHash(req.OldPassword, user.Password) {
		return ErrWrongPassword
	}
	hash, err := common.Password2Hash(req.NewPassword)
	if err != nil {
		return err
	}
	user.Password = hash
	return user.Update()
}
