// Shared helpers for the uploads UI (user + admin pages).

export function formatBytes(n: number): string {
  if (!n || n < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  // Bytes are integers; larger units get one decimal place.
  return i === 0 ? `${v} ${units[i]}` : `${v.toFixed(1)} ${units[i]}`
}

export function copyText(text: string): Promise<void> {
  if (navigator.clipboard) {
    return navigator.clipboard.writeText(text)
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  return Promise.resolve()
}

// Short, locale-aware relative time for the created/expires columns.
export function formatRelative(dateStr: string, t: (k: string, opts?: any) => string): string {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  const diffMs = date.getTime() - new Date().getTime()
  const future = diffMs > 0
  const abs = Math.abs(diffMs)
  const mins = Math.floor(abs / 60000)
  const hours = Math.floor(mins / 60)
  const days = Math.floor(hours / 24)
  // expires_at (future) and created_at (past) reuse the same buckets; the
  // suffix flips meaning via the i18n key chosen by the caller.
  let key: string
  let count = 0
  if (mins < 1) key = future ? 'uploads.inMoments' : 'uploads.justNow'
  else if (mins < 60) {
    key = future ? 'uploads.inMinutes' : 'uploads.minutesAgo'
    count = mins
  } else if (hours < 24) {
    key = future ? 'uploads.inHours' : 'uploads.hoursAgo'
    count = hours
  } else {
    key = future ? 'uploads.inDays' : 'uploads.daysAgo'
    count = days
  }
  return count > 0 ? t(key, { count }) : t(key)
}

export function formatDateTime(dateStr: string, locale: string): string {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString(locale)
}
