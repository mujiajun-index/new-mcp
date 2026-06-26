import { useParams } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Phone, PhoneOff, SwitchCamera, Loader2, Video, Copy } from 'lucide-react'
import { useCameraStream } from './use-camera-stream'

export function CameraStreamPage() {
  const { t } = useTranslation()
  const { id } = useParams({ strict: false }) as { id: string }
  const cameraId = Number(id)
  const token =
    typeof window !== 'undefined'
      ? new URLSearchParams(window.location.search).get('token') || localStorage.getItem('newmcp-token') || ''
      : ''

  const s = useCameraStream(cameraId, token || undefined)

  const handleCopyLink = async () => {
    const url = window.location.href
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(url)
      } else {
        // 非安全上下文（如 HTTP 部署）回退到 execCommand，保证手机端也能复制
        const ta = document.createElement('textarea')
        ta.value = url
        ta.style.position = 'fixed'
        ta.style.opacity = '0'
        document.body.appendChild(ta)
        ta.focus()
        ta.select()
        const ok = document.execCommand('copy')
        document.body.removeChild(ta)
        if (!ok) throw new Error('execCommand failed')
      }
      toast.success(t('cameras.stream.copySuccess'))
    } catch {
      toast.error(t('cameras.stream.copyFailed'))
    }
  }

  const handleAnswer = async () => {
    // 推流前预检（WS 握手被拒时前端只能拿到通用失败，这里给出明确提示）：
    // 1) 摄像头是否已禁用；2) 是否已有其他连接正在推流。
    if (token) {
      try {
        const base = (import.meta.env.BASE_URL ?? '/').replace(/\/$/, '')
        const res = await fetch(`${base}/api/v1/cameras/${cameraId}`, {
          headers: { Authorization: `Bearer ${token}` },
        })
        if (res.ok) {
          const json = await res.json()
          if (json?.data) {
            if (json.data.auto_register === false) {
              toast.error(t('cameras.stream.streamDisabled'))
              return
            }
            if (json.data.streaming === true) {
              toast.error(t('cameras.stream.streamingInUse'))
              return
            }
          }
        }
      } catch {
        // 查询失败不阻塞，交给 WS 握手处理
      }
    }
    s.open()
  }

  const canAnswer = !!token && s.mediaSupported && !s.opening

  return (
    <div className="flex min-h-[100dvh] w-full items-center justify-center bg-neutral-950 text-white">
      <div className="relative h-[100dvh] w-full overflow-hidden bg-black
                      md:h-auto md:max-h-[92dvh] md:w-[420px] md:aspect-[3/4] md:rounded-3xl md:shadow-2xl">
        {/* Local camera preview (fills the call window) */}
        <video ref={s.videoRef} className="absolute inset-0 h-full w-full object-cover" playsInline muted />
        {/* Hidden canvas for frame capture */}
        <canvas ref={s.canvasRef} className="hidden" />

        {/* Idle / calling screen */}
        {!s.active && (
          <>
            <div className="absolute inset-0 bg-gradient-to-b from-neutral-900 via-neutral-950 to-black" />

            <div className="absolute inset-0 flex flex-col items-center justify-between py-16">
              <div className="mt-6 flex flex-col items-center text-center">
                <div className="flex h-24 w-24 items-center justify-center rounded-full bg-white/10">
                  <Video className="h-10 w-10 text-white/70" />
                </div>
                <p className="mt-4 text-lg font-medium">{t('cameras.stream.title')}</p>
                <p className="mt-1 text-sm text-white/50">
                  {!s.mediaSupported ? t('cameras.stream.httpsRequired') : token ? t('cameras.stream.clickConnect') : t('cameras.stream.missingToken')}
                </p>
              </div>

              <div className="flex flex-col items-center gap-2">
                <button
                  type="button"
                  onClick={handleAnswer}
                  disabled={!canAnswer}
                  className="flex h-20 w-20 items-center justify-center rounded-full bg-emerald-500 text-white shadow-lg transition hover:bg-emerald-600 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  {s.opening ? <Loader2 className="h-8 w-8 animate-spin" /> : <Phone className="h-8 w-8" />}
                </button>
                <span className="text-sm text-white/60">{s.opening ? t('cameras.stream.connecting') : t('cameras.stream.connect')}</span>
              </div>
            </div>

            {/* top-right copy link — rendered last + z-10 so it stays clickable above the full-screen overlay */}
            <button
              type="button"
              onClick={handleCopyLink}
              title={t('cameras.stream.copyLink')}
              className="absolute right-4 top-4 z-10 inline-flex h-9 w-9 items-center justify-center rounded-full bg-white/10 text-white/80 transition hover:bg-white/20"
            >
              <Copy className="h-4 w-4" />
            </button>
          </>
        )}

        {/* Active / in-call screen */}
        {s.active && (
          <>
            {/* top status bar */}
            <div className="absolute inset-x-0 top-0 flex items-center justify-between p-4">
              <span className="inline-flex items-center gap-1.5 rounded-full bg-black/50 px-3 py-1 text-xs backdrop-blur">
                <span className={`h-2 w-2 rounded-full ${s.streaming ? 'bg-emerald-400' : 'bg-amber-400'}`} />
                {s.streaming ? t('cameras.stream.streaming') : t('cameras.stream.connecting2')}
              </span>
              <span className="rounded-full bg-black/50 px-3 py-1 text-xs backdrop-blur">
                {s.facingMode === 'user' ? t('cameras.stream.front') : t('cameras.stream.back')}
              </span>
            </div>

            {/* switching overlay */}
            {s.switching && (
              <div className="absolute inset-0 flex items-center justify-center bg-black/40">
                <Loader2 className="h-8 w-8 animate-spin text-white" />
              </div>
            )}

            {/* bottom controls (WeChat-call style) */}
            <div className="absolute inset-x-0 bottom-0 flex items-end justify-center gap-10 bg-gradient-to-t from-black/70 to-transparent px-6 pb-8 pt-16">
              <ControlButton label={t('cameras.stream.linkBtn')} onClick={handleCopyLink}>
                <Copy className="h-5 w-5" />
              </ControlButton>
              <button
                type="button"
                onClick={s.close}
                title={t('cameras.stream.hangupTitle')}
                className="flex flex-col items-center gap-1.5"
              >
                <span className="flex h-16 w-16 items-center justify-center rounded-full bg-red-500 text-white shadow-lg transition hover:bg-red-600">
                  <PhoneOff className="h-7 w-7" />
                </span>
                <span className="text-[11px] text-white/80">{t('cameras.stream.hangup')}</span>
              </button>
              <ControlButton
                label={t('cameras.stream.flipBtn')}
                onClick={s.switchCamera}
                disabled={s.switching || !s.hasMultipleCameras}
                title={s.hasMultipleCameras ? t('cameras.stream.flipTitle') : t('cameras.stream.oneCamOnly')}
              >
                {s.switching ? <Loader2 className="h-5 w-5 animate-spin" /> : <SwitchCamera className="h-5 w-5" />}
              </ControlButton>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function ControlButton({
  label,
  onClick,
  disabled,
  title,
  children,
}: {
  label: string
  onClick: () => void
  disabled?: boolean
  title?: string
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="flex flex-col items-center gap-1.5 text-white/80 disabled:opacity-40"
    >
      <span className="flex h-12 w-12 items-center justify-center rounded-full bg-white/15 backdrop-blur transition hover:bg-white/25">
        {children}
      </span>
      <span className="text-[11px]">{label}</span>
    </button>
  )
}
