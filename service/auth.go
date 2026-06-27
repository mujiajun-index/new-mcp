package service

import (
	"errors"
	"strings"

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

func (s *AuthService) Register(req *dto.RegisterReq) (*dto.AuthResp, error) {
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

	user := &model.User{
		Username: req.Username,
		Password: hash,
		Email:    req.Email,
		Role:     common.RoleCommonUser,
		Status:   common.StatusEnabled,
		Group:    "default",
	}
	if err := user.Insert(); err != nil {
		return nil, err
	}

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

func (s *AuthService) Login(req *dto.LoginReq) (*dto.AuthResp, error) {
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
