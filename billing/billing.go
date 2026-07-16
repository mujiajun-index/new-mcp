package billing

import (
	"errors"
	"sync/atomic"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

// BillingSession 一次市场服务调用的计费会话(§6.3,参考 new-api billing_session 大幅简化)。
type BillingSession struct {
	UserID            int64
	ApiKeyID          int64
	MarketplaceItemID int64
	ConsumedQuota     int64 // 应扣 = 预扣(成本确定,二者相等)
	Trusted           bool   // 命中信任旁路:未实际预扣,成功后由 Confirm 补扣
	Debt              bool   // FailOpen 欠账:计费 DB 异常,放行调用但未扣(仅记录)
	Price             PriceInfo
	RequestID         string

	// 进程内状态机:0=pending / 1=confirmed / 2=refunded。
	// Confirm 与 Refund 互斥(同一会话只结算一次),且各自幂等。
	state int32
}

// ErrInsufficientQuota 余额不足:拒绝本次调用,不禁用 Key(参考 new-api ErrorCodeInsufficientUserQuota)。
var ErrInsufficientQuota = errors.New("insufficient quota")

// PreConsumeRequest 一次预扣的入参。
type PreConsumeRequest struct {
	Price     PriceInfo
	UserID    int64
	ApiKeyID  int64
	UserRole  string // 用于 ChargeAdmin 旁路
	RequestID string // MCP 请求幂等键(防重扣)
}

// BillingService 市场来源服务计费服务。仅 source=marketplace 的分组调用路径触发。
type BillingService struct{}

// NewBillingService 构造计费服务。
func NewBillingService() *BillingService { return &BillingService{} }

// LowQuotaNotifier 低额度提醒钩子(默认 no-op)。由能访问邮件发送的包(如 service)在初始化时注入,
// 以打破 billing → service 的潜在依赖环。
var LowQuotaNotifier = func(userID, currentQuota int64) {}

// PreConsume 预扣(§6.2 插入点 A):校验并原子扣减用户额度 + Key 预算。
//   - 免费 / 管理员(未开启 ChargeAdmin):返回零消费会话,不扣费。
//   - 信任旁路(余额 > TrustQuota 且 Key 预算充足):Trusted=true,不实际预扣,成功后 Confirm 补扣。
//   - 余额或 Key 预算不足:返回 ErrInsufficientQuota(调用方拒绝本次调用,不禁用 Key)。
//   - 计费 DB 异常:按 BillingFailOpen 决定——true 则 Debt=true 放行,nil 错误;false 则返回错误拒绝。
func (s *BillingService) PreConsume(req PreConsumeRequest) (*BillingSession, error) {
	price := req.Price
	sess := &BillingSession{
		UserID:            req.UserID,
		ApiKeyID:          req.ApiKeyID,
		MarketplaceItemID: price.MarketplaceItemID,
		Price:             price,
		RequestID:         req.RequestID,
	}

	// 不计费:免费类型 / 零价 / 管理员(未开 ChargeAdmin)
	if price.BillingType == BillingTypeFree || price.UnitPriceQuota <= 0 {
		return sess, nil
	}
	if !model.GetOptionBool("ChargeAdmin") && common.IsAdminRole(req.UserRole) {
		return sess, nil
	}
	// 软幂等:同一 request_id 已计费成功 → 视为免费(防 MCP 客户端重试重扣)
	if model.HasChargedRequest(req.ApiKeyID, req.RequestID) {
		return sess, nil
	}

	consumed := price.UnitPriceQuota
	sess.ConsumedQuota = consumed

	trustQuota := model.GetOptionInt64("TrustQuota")
	userQuota, err := model.GetUserQuota(req.UserID)
	if err != nil {
		return sess, s.handleBillingDBError(sess, err)
	}
	key, err := model.GetApiKeyByID(req.ApiKeyID)
	if err != nil {
		return sess, s.handleBillingDBError(sess, err)
	}

	// 信任旁路:余额充足且 Key 预算充足 → 不预扣
	keyBudgetOK := key.UnlimitedQuota || (key.Quota-key.UsedQuota) > consumed
	if userQuota > trustQuota && keyBudgetOK {
		sess.Trusted = true
		return sess, nil
	}

	// 余额预检(快速失败,减少无效原子操作)
	if userQuota < consumed {
		return sess, ErrInsufficientQuota
	}

	// 原子扣减用户额度(防透支)
	rows, err := model.DecreaseUserQuotaAtomic(req.UserID, consumed)
	if err != nil {
		return sess, s.handleBillingDBError(sess, err)
	}
	if rows == 0 {
		return sess, ErrInsufficientQuota // 并发下被扣到不足
	}

	// 原子占用 Key 预算(预算 Key);无限 Key 仅记账
	if !key.UnlimitedQuota {
		krows, err := model.DecreaseApiKeyQuotaAtomic(req.ApiKeyID, consumed)
		if err != nil {
			_ = model.IncreaseUserQuota(req.UserID, consumed) // 补偿退还用户额度
			return sess, s.handleBillingDBError(sess, err)
		}
		if krows == 0 {
			_ = model.IncreaseUserQuota(req.UserID, consumed) // 退还用户
			return sess, ErrInsufficientQuota
		}
	} else {
		_ = model.AdjustApiKeyUsedQuota(req.ApiKeyID, consumed)
	}

	return sess, nil
}

// Confirm 成功确认(§6.2 插入点 B):Trusted 时补扣 + 记账;否则仅累加 used 统计(预扣已完成)。
// 与 Refund 互斥、且幂等(同一会话多次 Confirm 只生效一次)。低额度时异步发提醒。
func (s *BillingService) Confirm(sess *BillingSession) error {
	if sess == nil || sess.ConsumedQuota <= 0 {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&sess.state, 0, 1) {
		return nil // 已 Confirm 或已 Refund
	}
	if sess.Debt {
		return nil // 欠账:未扣,仅调用方记 debt 日志
	}

	consumed := sess.ConsumedQuota
	if sess.Trusted {
		// 信任旁路事后补扣(无守卫,接受有界超支)
		_ = model.DecreaseUserQuotaUnguarded(sess.UserID, consumed)
		// 信任路径 Key used 此前未记,此处补记
		_ = model.AdjustApiKeyUsedQuota(sess.ApiKeyID, consumed)
	}
	// 累加用户已用额度(预扣路径与信任路径均记)
	_ = model.AdjustUserUsedQuota(sess.UserID, consumed)

	// 低额度提醒(异步,经钩子解耦邮件发送)
	go func() {
		threshold := model.GetOptionInt64("QuotaRemindThreshold")
		if threshold <= 0 {
			return
		}
		if quota, err := model.GetUserQuota(sess.UserID); err == nil && quota < threshold {
			LowQuotaNotifier(sess.UserID, quota)
		}
	}()
	return nil
}

// Refund 失败退款(§6.2 插入点 B):全额 IncreaseUserQuota(幂等)+ 回退 Key used。
// 与 Confirm 互斥、且幂等。Trusted/Debt 未实际扣,无操作。
func (s *BillingService) Refund(sess *BillingSession) error {
	if sess == nil || sess.ConsumedQuota <= 0 {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&sess.state, 0, 2) {
		return nil // 已 Confirm 或已 Refund
	}
	if sess.Trusted || sess.Debt {
		return nil // 未实际扣,无需退
	}

	consumed := sess.ConsumedQuota
	_ = model.IncreaseUserQuota(sess.UserID, consumed)
	// 回退已用额度(净额反映)+ Key 预算恢复
	_ = model.AdjustUserUsedQuota(sess.UserID, -consumed)
	_ = model.AdjustApiKeyUsedQuota(sess.ApiKeyID, -consumed)
	return nil
}

// handleBillingDBError 计费 DB 异常处理(§6.6):BillingFailOpen=true → Debt 放行(nil);
// 否则返回错误,由调用方拒绝调用。
func (s *BillingService) handleBillingDBError(sess *BillingSession, err error) error {
	if model.GetOptionBool("BillingFailOpen") {
		sess.Debt = true
		sess.ConsumedQuota = 0 // 未实际扣
		sess.Trusted = false
		return nil
	}
	return err
}
