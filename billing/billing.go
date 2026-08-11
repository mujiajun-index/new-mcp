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
	ConsumedQuota     int64 // 实际应扣(单价;成功后据此结算并累加 used)
	PreConsumedQuota  int64 // 实际预扣(=Max(单价, PreConsumedQuota 下限));成功后退还(预扣-应扣)差额
	Trusted           bool  // 命中信任旁路:未实际预扣,成功后由 Confirm 补扣
	Debt              bool  // FailOpen 欠账:计费 DB 异常,放行调用但未扣(仅记录)
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
//   - 免费 / 零价:返回零消费会话,不扣费。
//   - 管理员:默认同样计费(ChargeAdmin=true)。仅当显式关闭 ChargeAdmin 时豁免管理员本人的平台托管服务调用。
//   - 信任旁路(用户余额 > TrustQuota 且 Key 余额 > TrustQuota 或无限 Key):Trusted=true,不实际预扣,成功后 Confirm 补扣。
//   - 预扣额 = Max(实际单价, PreConsumedQuota 下限);高于单价的部分在 Confirm 成功后退还(对齐 new-api PreConsumedQuota)。
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

	// 不计费:免费类型 / 零价。
	if price.BillingType == BillingTypeFree || price.UnitPriceQuota <= 0 {
		return sess, nil
	}
	// 管理员豁免:ChargeAdmin 默认开启(对管理员计费);仅当显式关闭时,豁免管理员本人的平台托管服务调用。
	if !model.GetOptionBool("ChargeAdmin") && common.IsAdminRole(req.UserRole) {
		return sess, nil
	}
	// 软幂等:同一 request_id 已计费成功 → 视为免费(防 MCP 客户端重试重扣)
	if model.HasChargedRequest(req.ApiKeyID, req.RequestID) {
		return sess, nil
	}

	consumed := price.UnitPriceQuota // 实际应扣(单价)
	sess.ConsumedQuota = consumed
	// 预扣下限(对齐 new-api PreConsumedQuota):预扣额不低于此值,成功后由 Confirm 退还差额。
	preConsumed := consumed
	if floor := model.GetPreConsumedQuota(); floor > preConsumed {
		preConsumed = floor
	}
	sess.PreConsumedQuota = preConsumed

	trustQuota := model.GetTrustQuota()
	userQuota, err := model.GetUserQuota(req.UserID)
	if err != nil {
		return sess, s.handleBillingDBError(sess, err)
	}
	key, err := model.GetApiKeyByID(req.ApiKeyID)
	if err != nil {
		return sess, s.handleBillingDBError(sess, err)
	}

	// 信任旁路(对齐 reference/new-api service.PreConsumeQuota):用户余额 > trustQuota
	// 且 Key 余额 > trustQuota(或无限 Key)→ 不预扣,成功后由 Confirm 补扣。
	// new-api 对 user 与 token 使用同一 trustQuota 门槛(=10*QuotaPerUnit);
	// 任一不满足则落入下方正常预扣路径——仅当余额 < 本次消费时才拒绝。
	keyTrustOK := key.UnlimitedQuota || (key.Quota-key.UsedQuota) > trustQuota
	if userQuota > trustQuota && keyTrustOK {
		sess.Trusted = true
		return sess, nil
	}

	// 余额预检(用预扣额,对齐 new-api userQuota-preConsumed<0 → 拒绝;减少无效原子操作)
	if userQuota < preConsumed {
		return sess, ErrInsufficientQuota
	}

	// 原子扣减用户额度(按预扣额,防透支)
	rows, err := model.DecreaseUserQuotaAtomic(req.UserID, preConsumed)
	if err != nil {
		return sess, s.handleBillingDBError(sess, err)
	}
	if rows == 0 {
		return sess, ErrInsufficientQuota // 并发下被扣到不足
	}

	// 原子占用 Key 预算(预算 Key,按预扣额);无限 Key 仅记账
	if !key.UnlimitedQuota {
		krows, err := model.DecreaseApiKeyQuotaAtomic(req.ApiKeyID, preConsumed)
		if err != nil {
			_ = model.IncreaseUserQuota(req.UserID, preConsumed) // 补偿退还用户额度
			return sess, s.handleBillingDBError(sess, err)
		}
		if krows == 0 {
			_ = model.IncreaseUserQuota(req.UserID, preConsumed) // 退还用户
			return sess, ErrInsufficientQuota
		}
	} else {
		_ = model.AdjustApiKeyUsedQuota(req.ApiKeyID, preConsumed)
	}

	return sess, nil
}

// Confirm 成功确认(§6.2 插入点 B):
//   - Trusted:事后补扣实际单价 + 记账。
//   - 非信任:预扣已完成,退还(预扣-单价)差额(预消耗下限高于单价时),并累加 used。
//
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

	consumed := sess.ConsumedQuota // 实际应扣(单价)
	if sess.Trusted {
		// 信任旁路事后补扣(无守卫,接受有界超支);Key used 此前未记,此处补记
		_ = model.DecreaseUserQuotaUnguarded(sess.UserID, consumed)
		_ = model.AdjustApiKeyUsedQuota(sess.ApiKeyID, consumed)
	} else if excess := sess.PreConsumedQuota - consumed; excess > 0 {
		// 非信任:预扣 > 单价(命中预消耗下限),退还差额到用户余额与 Key 预算
		_ = model.IncreaseUserQuota(sess.UserID, excess)
		_ = model.AdjustApiKeyUsedQuota(sess.ApiKeyID, -excess)
	}
	// 累加用户累计已用额度(实际消费;预扣与信任路径均记)
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

// Refund 失败退款(§6.2 插入点 B):全额退还实际预扣额(幂等)+ 恢复 Key 预算。
// 与 Confirm 互斥、且幂等。Trusted/Debt 未实际预扣,无操作。
// 注:不调整 user used_quota——失败未实际消费,累计已用不应变化。
func (s *BillingService) Refund(sess *BillingSession) error {
	if sess == nil || sess.ConsumedQuota <= 0 {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&sess.state, 0, 2) {
		return nil // 已 Confirm 或已 Refund
	}
	if sess.Trusted || sess.Debt {
		return nil // 未实际预扣,无需退
	}

	// 全额退还实际预扣(预消耗下限包含在内);Key 预算同步恢复
	preConsumed := sess.PreConsumedQuota
	_ = model.IncreaseUserQuota(sess.UserID, preConsumed)
	_ = model.AdjustApiKeyUsedQuota(sess.ApiKeyID, -preConsumed)
	return nil
}

// handleBillingDBError 计费 DB 异常处理(§6.6):BillingFailOpen=true → Debt 放行(nil);
// 否则返回错误,由调用方拒绝调用。
func (s *BillingService) handleBillingDBError(sess *BillingSession, err error) error {
	if model.GetOptionBool("BillingFailOpen") {
		sess.Debt = true
		sess.ConsumedQuota = 0    // 未实际扣
		sess.PreConsumedQuota = 0 // 未实际扣
		sess.Trusted = false
		return nil
	}
	return err
}
