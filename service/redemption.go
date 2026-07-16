package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type RedemptionService struct{}

// Generate 批量生成兑换码(§8.1)。count 上限 100,统一面值与有效期。返回含明文 code 的条目(仅此一次可见)。
func (s *RedemptionService) Generate(req *dto.RedemptionCreateReq) ([]dto.RedemptionItem, error) {
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 100 {
		req.Count = 100
	}

	items := make([]dto.RedemptionItem, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		code, err := generateRedemptionCode()
		if err != nil {
			return nil, err
		}
		r := &model.Redemption{
			Code:      code,
			Name:      req.Name,
			Quota:     req.Quota,
			Status:    model.RedemptionStatusAvailable,
			ExpiredAt: req.ExpiredAt,
		}
		if err := r.Insert(); err != nil {
			return nil, err
		}
		items = append(items, toRedemptionItem(r))
	}
	return items, nil
}

func (s *RedemptionService) List(page, pageSize int, keyword string, status int) ([]dto.RedemptionItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	rs, total, err := model.ListRedemptions(offset, pageSize, keyword, status)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.RedemptionItem, len(rs))
	for i, r := range rs {
		out[i] = toRedemptionItem(&r)
	}
	return out, total, nil
}

// UpdateStatus 启停兑换码(仅可在 1↔3 间切换;已兑换(2)不可改)。
func (s *RedemptionService) UpdateStatus(id int64, status int) error {
	r, err := model.GetRedemptionByID(id)
	if err != nil {
		return errors.New("兑换码不存在")
	}
	if r.Status == model.RedemptionStatusRedeemed {
		return errors.New("已兑换的兑换码不可修改状态")
	}
	if status != model.RedemptionStatusAvailable && status != model.RedemptionStatusDisabled {
		return errors.New("无效的状态")
	}
	r.Status = status
	return r.Update()
}

func (s *RedemptionService) Delete(id int64) error {
	r, err := model.GetRedemptionByID(id)
	if err != nil {
		return errors.New("兑换码不存在")
	}
	return r.Delete()
}

// Redeem 用户兑换(§8.2):model.Redeem 原子占领 + 入账。返回入账额度。
func (s *RedemptionService) Redeem(userID int64, code string) (int64, error) {
	r, err := model.GetRedemptionByCode(code)
	if err != nil {
		return 0, errors.New("兑换码无效")
	}
	quota, err := r.Redeem(userID)
	if err != nil {
		return 0, err
	}
	return quota, nil
}

// generateRedemptionCode 生成 32 位十六进制兑换码(16 随机字节)。code 列唯一索引兜底碰撞。
func generateRedemptionCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func toRedemptionItem(r *model.Redemption) dto.RedemptionItem {
	item := dto.RedemptionItem{
		ID:        r.ID,
		Code:      r.Code,
		Name:      r.Name,
		Quota:     r.Quota,
		Status:    r.Status,
		UserID:    r.UserID,
		ExpiredAt: r.ExpiredAt,
		CreatedAt: r.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if r.RedeemedAt != nil {
		item.RedeemedAt = r.RedeemedAt.Format("2006-01-02T15:04:05Z")
	}
	return item
}
