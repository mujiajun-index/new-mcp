import i18n from '@/i18n/config'

// 商业化展示 helper(§3/§5)。市场价格 price_per_call 已是展示货币 decimal,直接展示;
// 用户额度按货币单位展示(quota / QuotaPerUnit,对齐 reference/new-api formatQuota)。

/**
 * 市场服务价格文本:免费 / ¥0.05/次。displayCurrency 默认 CNY。
 * item 形如 { billing_type, price_per_call }。
 */
export function priceLabel(billingType: string, pricePerCall: number, displayCurrency = 'CNY'): string {
  if (billingType === 'free') return i18n.t('billing.free')
  if (!pricePerCall || pricePerCall <= 0) {
    // per_call 但未定价(非自用模式应已拒绝上架,这里兜底显示"未定价")
    return i18n.t('billing.unpriced')
  }
  const symbol = currencySymbol(displayCurrency)
  return `${symbol}${pricePerCall.toFixed(2)}${i18n.t('billing.perCall')}`
}

export function currencySymbol(currency: string): string {
  switch ((currency || 'CNY').toUpperCase()) {
    case 'USD':
      return '$'
    case 'EUR':
      return '€'
    default:
      return '¥'
  }
}

/**
 * 把原始 quota 整数换算成展示货币文本:quota / QuotaPerUnit。
 * 参考 reference/new-api formatQuotaWithCurrency。单位额度非法(<=0)时回退 500000。
 * unit 缺省时从 i18n 无法读 store,故由调用方注入(来自 system-config-store)。
 */
export function formatQuotaCurrency(quota: number, quotaPerUnit: number, currency = 'CNY'): string {
  const unit = quotaPerUnit > 0 ? quotaPerUnit : 500000
  const amount = quota / unit
  const symbol = currencySymbol(currency)
  // 按量级选精度上限:>=1 最多 2 位,<1 最多 4 位(避免小额被四舍五入为 0),再去掉尾部多余的 0(0.0100 → 0.01)。
  const maxDigits = amount >= 1 ? 2 : 4
  const trimmed = amount.toFixed(maxDigits).replace(/\.?0+$/, '')
  return `${symbol}${trimmed || '0'}`
}

/** 是否已显式定价(§5.6):free 或 (per_call 且 price>0)。 */
export function isExplicitlyPriced(billingType: string, pricePerCall: number): boolean {
  if (billingType === 'free') return true
  return billingType !== 'free' && pricePerCall > 0
}

/**
 * 计费状态徽标文本(§4.5):
 * skipped(自有/未启用/免费) / charged(已扣) / refunded(失败退款) / blocked(余额不足拒绝) / debt(FailOpen 欠账)
 */
export function billingStatusKey(status: string): string {
  switch (status) {
    case 'charged':
      return 'billing.statusCharged'
    case 'refunded':
      return 'billing.statusRefunded'
    case 'blocked':
      return 'billing.statusBlocked'
    case 'debt':
      return 'billing.statusDebt'
    case 'skipped':
    default:
      return 'billing.statusSkipped'
  }
}

/** 计费状态对应的 tailwind 徽标颜色类。 */
export function billingStatusClass(status: string): string {
  switch (status) {
    case 'charged':
      return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
    case 'refunded':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
    case 'blocked':
      return 'bg-red-500/10 text-red-600 dark:text-red-400'
    case 'debt':
      return 'bg-orange-500/10 text-orange-600 dark:text-orange-400'
    case 'skipped':
    default:
      return 'bg-muted text-muted-foreground'
  }
}

/** 定价来源层级文本:tool/service/marketplace/global。 */
export function priceScopeKey(scope: string): string {
  switch (scope) {
    case 'tool':
      return 'billing.scopeTool'
    case 'service':
      return 'billing.scopeService'
    case 'marketplace':
      return 'billing.scopeMarketplace'
    case 'global':
      return 'billing.scopeGlobal'
    default:
      return 'billing.scopeGlobal'
  }
}
