import i18n from '@/i18n/config'
import { toast } from 'sonner'

/**
 * 复制文本到剪贴板。非安全上下文（如 HTTP 部署）回退到 execCommand，
 * 保证手机端也能复制。成功/失败均以 toast 提示（i18n 用 common.copySuccess/copyFailed）。
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.focus()
      ta.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(ta)
      if (!ok) throw new Error('execCommand failed')
    }
    toast.success(i18n.t('common.copySuccess'))
    return true
  } catch {
    toast.error(i18n.t('common.copyFailed'))
    return false
  }
}
