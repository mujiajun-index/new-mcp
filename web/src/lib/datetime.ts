import dayjs from 'dayjs'
import type { TFunction } from 'i18next'

export function formatRelativeTime(dateStr: string, t: TFunction) {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return '-'
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return t('apiKeys.justNow')
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return t('apiKeys.minutesAgo', { count: minutes })
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('apiKeys.hoursAgo', { count: hours })
  const days = Math.floor(hours / 24)
  if (days < 30) return t('apiKeys.daysAgo', { count: days })
  if (days < 365) return t('apiKeys.monthsAgo', { count: Math.floor(days / 30) })
  return t('apiKeys.yearsAgo', { count: Math.floor(days / 365) })
}

// datetime-local 不接收时区后缀，必须先转到浏览器当地时间再填入。
export function toDateTimeLocalValue(value: string): string {
  if (!value) return ''
  const date = dayjs(value)
  return date.isValid() ? date.format('YYYY-MM-DDTHH:mm') : ''
}
