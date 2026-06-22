import { useParams } from '@tanstack/react-router'
import { toast } from 'sonner'
import { Phone, PhoneOff, SwitchCamera, Loader2, Video, Copy } from 'lucide-react'
import { useCameraStream } from './use-camera-stream'

export function CameraStreamPage() {
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
      toast.success('链接已复制，可发送到手机/平板打开')
    } catch {
      toast.error('复制失败，请手动复制地址栏链接')
    }
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
                <p className="mt-4 text-lg font-medium">摄像头视频</p>
                <p className="mt-1 text-sm text-white/50">
                  {!s.mediaSupported ? '需要 HTTPS 或 localhost 才能访问摄像头' : token ? '点击接通开始推流' : '缺少授权 token，无法推流'}
                </p>
              </div>

              <div className="flex flex-col items-center gap-2">
                <button
                  type="button"
                  onClick={s.open}
                  disabled={!canAnswer}
                  className="flex h-20 w-20 items-center justify-center rounded-full bg-emerald-500 text-white shadow-lg transition hover:bg-emerald-600 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  {s.opening ? <Loader2 className="h-8 w-8 animate-spin" /> : <Phone className="h-8 w-8" />}
                </button>
                <span className="text-sm text-white/60">{s.opening ? '正在接通...' : '接通'}</span>
              </div>
            </div>

            {/* top-right copy link — rendered last + z-10 so it stays clickable above the full-screen overlay */}
            <button
              type="button"
              onClick={handleCopyLink}
              title="复制链接"
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
                {s.streaming ? '推流中' : '连接中'}
              </span>
              <span className="rounded-full bg-black/50 px-3 py-1 text-xs backdrop-blur">
                {s.facingMode === 'user' ? '前置' : '后置'}
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
              <ControlButton label="链接" onClick={handleCopyLink}>
                <Copy className="h-5 w-5" />
              </ControlButton>
              <button
                type="button"
                onClick={s.close}
                title="挂断"
                className="flex flex-col items-center gap-1.5"
              >
                <span className="flex h-16 w-16 items-center justify-center rounded-full bg-red-500 text-white shadow-lg transition hover:bg-red-600">
                  <PhoneOff className="h-7 w-7" />
                </span>
                <span className="text-[11px] text-white/80">挂断</span>
              </button>
              <ControlButton
                label="翻转"
                onClick={s.switchCamera}
                disabled={s.switching || !s.hasMultipleCameras}
                title={s.hasMultipleCameras ? '切换前后镜头' : '仅检测到一个摄像头'}
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
