import { useState, useRef, useCallback, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Video, VideoOff, Loader2, SwitchCamera } from 'lucide-react'
import { toast } from 'sonner'

type FacingMode = 'user' | 'environment'

interface CameraCaptureProps {
  cameraId: number
  onStreamingChange?: (streaming: boolean) => void
}

function buildWebSocketUrl(cameraId: number): string {
  const loc = window.location
  const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:'
  const base: string = import.meta.env.BASE_URL ?? '/'
  const apiBase = base.endsWith('/') ? base.slice(0, -1) : base
  const token = localStorage.getItem('newmcp-token') || ''
  return `${protocol}//${loc.host}${apiBase}/api/v1/cameras/${cameraId}/stream?token=${encodeURIComponent(token)}`
}

function dataUrlToBinary(dataUrl: string): Uint8Array<ArrayBuffer> {
  const base64 = dataUrl.split(',')[1] ?? ''
  const binaryStr = atob(base64)
  const bytes = new Uint8Array(binaryStr.length)
  for (let i = 0; i < binaryStr.length; i++) {
    bytes[i] = binaryStr.charCodeAt(i)
  }
  return bytes
}

async function openStream(facingMode: FacingMode): Promise<MediaStream> {
  return navigator.mediaDevices.getUserMedia({ video: { facingMode } })
}

export function CameraCapture({ cameraId, onStreamingChange }: CameraCaptureProps) {
  const mediaSupported = typeof navigator !== 'undefined' && !!navigator.mediaDevices?.getUserMedia

  const [active, setActive] = useState(false)
  const [streaming, setStreaming] = useState(false)
  const [opening, setOpening] = useState(false)
  const [switching, setSwitching] = useState(false)
  const [facingMode, setFacingMode] = useState<FacingMode>('environment')
  const [hasMultipleCameras, setHasMultipleCameras] = useState(false)

  const videoRef = useRef<HTMLVideoElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const streamRef = useRef<MediaStream | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const facingRef = useRef<FacingMode>('environment')

  // 保持 ref 与 state 同步，供异步回调读取最新朝向
  useEffect(() => {
    facingRef.current = facingMode
  }, [facingMode])

  const cleanup = useCallback(() => {
    // Stop interval
    if (intervalRef.current) {
      clearInterval(intervalRef.current)
      intervalRef.current = null
    }

    // Close WebSocket
    if (wsRef.current) {
      const ws = wsRef.current
      wsRef.current = null
      if (ws.readyState === WebSocket.OPEN) {
        ws.close()
      } else if (ws.readyState === WebSocket.CONNECTING) {
        ws.onopen = () => ws.close()
      }
    }

    // Stop media stream tracks
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((track) => track.stop())
      streamRef.current = null
    }

    // Clear video source
    if (videoRef.current) {
      videoRef.current.srcObject = null
    }

    setActive(false)
    setStreaming(false)
    setHasMultipleCameras(false)
    onStreamingChange?.(false)
  }, [onStreamingChange])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      cleanup()
    }
  }, [cleanup])

  // 探测可用视频输入设备数量（需在已获得摄像头权限后调用，标签才会填充）
  const detectMultipleCameras = useCallback(async () => {
    try {
      const devices = await navigator.mediaDevices.enumerateDevices()
      const videoInputs = devices.filter((d) => d.kind === 'videoinput')
      setHasMultipleCameras(videoInputs.length > 1)
    } catch {
      setHasMultipleCameras(false)
    }
  }, [])

  const handleOpen = useCallback(async () => {
    if (!navigator.mediaDevices?.getUserMedia) {
      const isSecure = window.isSecureContext
      if (!isSecure) {
        toast.error('摄像头需要安全上下文（HTTPS 或 localhost），请使用 HTTPS 访问')
      } else {
        toast.error('当前浏览器不支持摄像头访问')
      }
      return
    }

    setOpening(true)
    try {
      const stream = await openStream(facingRef.current)

      streamRef.current = stream

      setActive(true)
      detectMultipleCameras()

      if (videoRef.current) {
        videoRef.current.srcObject = stream
        await videoRef.current.play()
      }

      // Start WebSocket connection
      const wsUrl = buildWebSocketUrl(cameraId)
      const ws = new WebSocket(wsUrl)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        setStreaming(true)
        onStreamingChange?.(true)
      }

      ws.onerror = () => {
        toast.error('WebSocket 连接失败')
        setStreaming(false)
        onStreamingChange?.(false)
      }

      ws.onclose = () => {
        setStreaming(false)
        onStreamingChange?.(false)
      }

      // Start frame capture interval - every 2 seconds
      intervalRef.current = setInterval(() => {
        const video = videoRef.current
        const canvas = canvasRef.current
        if (!video || !canvas || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return

        if (video.videoWidth === 0 || video.videoHeight === 0) return

        canvas.width = video.videoWidth
        canvas.height = video.videoHeight
        const ctx = canvas.getContext('2d')
        if (!ctx) return

        ctx.drawImage(video, 0, 0)
        const dataUrl = canvas.toDataURL('image/jpeg', 0.7)
        const binary = dataUrlToBinary(dataUrl)

        try {
          wsRef.current?.send(binary)
        } catch {
          // Silently ignore send errors - WebSocket will handle reconnection via onclose
        }
      }, 2000)
    } catch (err: unknown) {
      const error = err as DOMException
      if (error.name === 'NotAllowedError' || error.name === 'PermissionDeniedError') {
        toast.error('摄像头权限被拒绝，请在浏览器设置中允许访问摄像头')
      } else if (error.name === 'NotFoundError') {
        toast.error('未检测到摄像头设备')
      } else {
        toast.error(`无法打开摄像头: ${error.message || '未知错误'}`)
      }
    } finally {
      setOpening(false)
    }
  }, [cameraId, onStreamingChange, detectMultipleCameras])

  // 切换前后摄像头：保持 WebSocket 与推流不中断，只替换视频源
  const handleSwitch = useCallback(async () => {
    if (!mediaSupported || switching) return
    const prev = facingRef.current
    const next: FacingMode = prev === 'environment' ? 'user' : 'environment'

    setSwitching(true)
    try {
      // 先释放当前设备，浏览器才会真正按新 facingMode 选择另一颗摄像头
      if (streamRef.current) {
        streamRef.current.getTracks().forEach((track) => track.stop())
        streamRef.current = null
      }

      const newStream = await openStream(next)
      streamRef.current = newStream
      setFacingMode(next)

      if (videoRef.current) {
        videoRef.current.srcObject = newStream
        await videoRef.current.play()
      }
    } catch (err: unknown) {
      const error = err as DOMException
      if (error.name === 'NotFoundError' || error.name === 'OverconstrainedError') {
        toast.error('未找到对应方向的摄像头')
      } else {
        toast.error('切换摄像头失败')
      }
      // 尝试恢复原来的摄像头，避免推流空帧
      try {
        const restored = await openStream(prev)
        streamRef.current = restored
        if (videoRef.current) {
          videoRef.current.srcObject = restored
          await videoRef.current.play()
        }
      } catch {
        // 恢复失败则整体关闭
        cleanup()
      }
    } finally {
      setSwitching(false)
      detectMultipleCameras()
    }
  }, [mediaSupported, switching, cleanup, detectMultipleCameras])

  const handleClose = useCallback(() => {
    cleanup()
  }, [cleanup])

  return (
    <div className="space-y-3">
      {/* Hidden canvas for frame capture */}
      <canvas ref={canvasRef} className="hidden" />

      {/* Video preview area */}
      <div className="relative rounded-lg overflow-hidden bg-muted/50 aspect-video flex items-center justify-center">
        <video
          ref={videoRef}
          className={`w-full h-full object-cover ${active ? '' : 'hidden'}`}
          playsInline
          muted
        />
        {!active && (
          <div className="flex flex-col items-center gap-2 text-muted-foreground">
            <Video className="h-8 w-8 text-muted-foreground/30" />
            <p className="text-sm">摄像头未开启</p>
          </div>
        )}

        {/* Streaming indicator overlay */}
        {active && (
          <div className="absolute top-3 left-3">
            <span className="inline-flex items-center gap-1.5 rounded-full bg-black/60 px-2.5 py-1 text-xs text-white">
              <span className={`h-2 w-2 rounded-full ${streaming ? 'bg-emerald-400' : 'bg-zinc-400'}`} />
              {streaming ? '推流中' : '未推流'}
            </span>
          </div>
        )}

        {/* Facing direction overlay */}
        {active && (
          <div className="absolute top-3 right-3">
            <span className="inline-flex items-center rounded-full bg-black/60 px-2.5 py-1 text-xs text-white">
              {facingMode === 'user' ? '前置' : '后置'}
            </span>
          </div>
        )}

        {/* Switching overlay */}
        {switching && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/40">
            <Loader2 className="h-6 w-6 animate-spin text-white" />
          </div>
        )}
      </div>

      {/* Controls */}
      <div className="space-y-1">
        {!active ? (
          <>
            <Button onClick={handleOpen} disabled={opening || !mediaSupported} className="gap-2">
              {opening ? <Loader2 className="h-4 w-4 animate-spin" /> : <Video className="h-4 w-4" />}
              {opening ? '正在开启...' : mediaSupported ? '开启摄像头' : '需要 HTTPS'}
            </Button>
            {!mediaSupported && (
              <p className="text-xs text-muted-foreground">使用 localhost 或 127.0.0.1 访问也可以开启摄像头</p>
            )}
          </>
        ) : (
          <div className="flex gap-2">
            <Button variant="outline" onClick={handleClose} className="gap-2">
              <VideoOff className="h-4 w-4" />
              关闭摄像头
            </Button>
            <Button
              variant="outline"
              onClick={handleSwitch}
              disabled={switching || !hasMultipleCameras}
              className="gap-2"
              title={hasMultipleCameras ? '切换前后摄像头' : '仅检测到一个摄像头'}
            >
              {switching ? <Loader2 className="h-4 w-4 animate-spin" /> : <SwitchCamera className="h-4 w-4" />}
              切换摄像头
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
