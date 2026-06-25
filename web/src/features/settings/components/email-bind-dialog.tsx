import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from 'sonner'

const COUNTDOWN_SECONDS = 60

interface EmailBindDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentEmail?: string
  // When SMTP is configured, binding/changing requires a verification code.
  requireVerify: boolean
  onSuccess: () => void
}

// Modal for binding or changing the account email. Mirrors reference/new-api's
// email-bind-dialog, adapted to this project's verify-only-when-SMTP-configured rule.
export function EmailBindDialog({
  open,
  onOpenChange,
  currentEmail,
  requireVerify,
  onSuccess,
}: EmailBindDialogProps) {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [countdown, setCountdown] = useState(0)
  const [sendingCode, setSendingCode] = useState(false)
  const [loading, setLoading] = useState(false)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Reset transient state whenever the dialog closes.
  useEffect(() => {
    if (!open) {
      setEmail('')
      setCode('')
      setCountdown(0)
      if (timerRef.current) {
        clearInterval(timerRef.current)
        timerRef.current = null
      }
    }
  }, [open])

  // Clear the countdown timer on unmount.
  useEffect(() => {
    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [])

  const startCountdown = () => {
    setCountdown(COUNTDOWN_SECONDS)
    if (timerRef.current) clearInterval(timerRef.current)
    timerRef.current = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          if (timerRef.current) {
            clearInterval(timerRef.current)
            timerRef.current = null
          }
          return 0
        }
        return prev - 1
      })
    }, 1000)
  }

  const handleSendCode = async () => {
    if (!email || !email.includes('@')) {
      toast.error(t('auth.enterEmailFirst'))
      return
    }
    setSendingCode(true)
    try {
      await api.get('/auth/profile/email-code', {
        params: { email },
        disableDuplicate: true,
      } as any)
      toast.success(t('auth.verificationSent'))
      startCountdown()
    } catch {
      // error toast handled by the axios interceptor
    } finally {
      setSendingCode(false)
    }
  }

  const handleBind = async () => {
    if (!email) {
      toast.error(t('auth.enterEmailFirst'))
      return
    }
    if (requireVerify && !code) {
      toast.error(t('auth.codeRequired'))
      return
    }
    setLoading(true)
    try {
      await api.put('/auth/profile', {
        email,
        ...(requireVerify ? { email_verification_code: code } : {}),
      })
      toast.success('邮箱绑定成功')
      onOpenChange(false)
      onSuccess()
    } catch {
      // error toast handled by the axios interceptor
    } finally {
      setLoading(false)
    }
  }

  const isChange = !!currentEmail

  return (
    <Dialog open={open} onOpenChange={(o) => !loading && onOpenChange(o)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{isChange ? '更换邮箱' : '绑定邮箱'}</DialogTitle>
          <DialogDescription>
            {isChange
              ? `当前邮箱：${currentEmail}。输入新邮箱${requireVerify ? '并完成验证' : ''}以更换。`
              : '为你的账号绑定一个邮箱地址。'}
            {!requireVerify && ' 当前未配置 SMTP 邮件，可直接绑定，无需验证码。'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="bind-email">邮箱地址</Label>
            <Input
              id="bind-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="请输入邮箱"
              disabled={loading}
            />
          </div>
          {requireVerify && (
            <div className="space-y-2">
              <Label htmlFor="bind-code">{t('auth.verificationCode')}</Label>
              <div className="flex gap-2">
                <Input
                  id="bind-code"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder={t('auth.verificationCode')}
                  maxLength={6}
                  disabled={loading}
                />
                <Button
                  type="button"
                  variant="outline"
                  className="shrink-0"
                  disabled={sendingCode || countdown > 0 || !email}
                  onClick={handleSendCode}
                >
                  {sendingCode ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : countdown > 0 ? (
                    t('auth.resendIn', { seconds: countdown })
                  ) : (
                    t('auth.sendCode')
                  )}
                </Button>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
            取消
          </Button>
          <Button
            type="button"
            onClick={handleBind}
            disabled={loading || !email || (requireVerify && !code)}
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {loading ? '绑定中...' : isChange ? '更换邮箱' : '绑定'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
