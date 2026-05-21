package service

import (
	"errors"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
)

var ErrUsernameExists = errors.New("用户名已存在")
var ErrInvalidCredentials = errors.New("用户名或密码错误")
var ErrWrongPassword = errors.New("原密码不正确")
var ErrRegisterDisabled = errors.New("注册功能已禁用")
var ErrEmailDomainRestricted = errors.New("该邮箱域名不在允许列表中")

type AuthService struct{}

func (s *AuthService) Register(req *dto.RegisterReq) (*dto.AuthResp, error) {
	if !model.GetOptionBool("RegisterEnabled") {
		return nil, ErrRegisterDisabled
	}

	if req.Email != "" && !model.IsEmailDomainAllowed(req.Email) {
		return nil, ErrEmailDomainRestricted
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
	user, err := model.GetUserByUsername(req.Username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !common.ValidatePasswordAndHash(req.Password, user.Password) {
		return nil, ErrInvalidCredentials
	}
	if user.Status != common.StatusEnabled {
		return nil, ErrInvalidCredentials
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
	if req.Email != nil {
		user.Email = *req.Email
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
