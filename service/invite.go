package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// ErrTransferTooSmall 邀请奖励转账低于最小额度(对齐 new-api「转移额度最小为 X」)。
var ErrTransferTooSmall = errors.New("转入金额过小")

type InviteService struct{}

// Overview 返回当前用户的邀请概览(对齐 new-api GetAffCode:惰性补全邀请码)。
func (s *InviteService) Overview(userID int64) (*dto.InviteOverview, error) {
	u, err := model.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	// 惰性生成邀请码(存量用户或早期未分配时),对齐 new-api GetAffCode 的 if AffCode=="" 分支。
	if u.AffCode == "" {
		code, err := model.GenerateAffCode()
		if err != nil {
			return nil, err
		}
		if err := model.DB.Model(&model.User{}).Where("id = ?", userID).Update("aff_code", code).Error; err != nil {
			return nil, err
		}
		u.AffCode = code
	}
	server := strings.TrimRight(model.GetOptionString("ServerAddress"), "/")
	inviteURL := server + "/sign-up?aff=" + u.AffCode
	return &dto.InviteOverview{
		AffCode:         u.AffCode,
		InviteURL:       inviteURL,
		AffCount:        u.AffCount,
		AffQuota:        u.AffQuota,
		AffHistoryQuota: u.AffHistoryQuota,
		QuotaForInviter: model.GetOptionInt64("QuotaForInviter"),
		QuotaForInvitee: model.GetOptionInt64("QuotaForInvitee"),
	}, nil
}

// Transfer 把邀请奖励待提取余额(aff_quota)转入钱包可用额度(quota)。
// 对齐 new-api TransferAffQuotaToQuota:最小转账额 = 1 个货币单位(QuotaPerUnit)。
func (s *InviteService) Transfer(userID int64, req *dto.TransferAffQuotaReq, ip string) (*dto.TransferAffQuotaResp, error) {
	minTransfer := model.GetQuotaPerUnit()
	if req.Quota < minTransfer {
		return nil, fmt.Errorf("%w: 最小转入金额为 %s", ErrTransferTooSmall, model.FormatQuotaCurrency(minTransfer))
	}
	if err := model.TransferAffQuota(userID, req.Quota); err != nil {
		return nil, err
	}
	// 回查最新余额返回(转账已原子落库,select 仅取需要的列,顺带取用户名写日志)。
	var u model.User
	if err := model.DB.Select("username", "quota", "aff_quota").First(&u, userID).Error; err != nil {
		return nil, err
	}
	model.RecordSystemLog(userID, u.Username, fmt.Sprintf("邀请额度转入钱包 %s", model.FormatQuotaCurrency(req.Quota)), req.Quota, ip, map[string]any{"type": "aff_transfer"})
	return &dto.TransferAffQuotaResp{
		Quota:    u.Quota,
		AffQuota: u.AffQuota,
	}, nil
}
