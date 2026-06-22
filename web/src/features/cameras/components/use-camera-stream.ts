import { useState, useRef, useCallback, useEffect } from 'react'
import { toast } from 'sonner'

export type FacingMode = 'user' | 'environment'

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

function resolveToken(tokenOverride?: string): string {
  return tokenOverride ?? localStorage.getItem('newmcp-token') ?? ''
}

function buildWebSocketUrl(cameraId: number, tokenOverride?: string): string {
  const loc = window.location
  const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:'
  const base: string = import.meta.env.BASE_URL ?? '/'
  const apiBase = base.endsWith('/') ? base.slice(0, -1) : base
  const token = resolveToken(tokenOverride)
  return `${protocol}//${loc.host}${apiBase}/api/v1/cameras/${cameraId}/stream?token=${encodeURIComponent(token)}`
}

/**
 * 摄像头推流核心逻辑：getUserMedia 取流 → WebSocket 推帧（每 2s）→ 支持前后切换。
 * 供「详情页预览」与「独立视频页」共用，UI 由各消费方自行渲染。
 *
 * @param cameraId 摄像头 ID
 * @param tokenOverride 视频页等无登录态场景从 URL 传入的会话 token；不传则回退 localStorage
 */
export function useCameraStream(cameraId: number, tokenOverride?: string) {
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
  }, [])

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

  const open = useCallback(async () => {
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
      const wsUrl = buildWebSocketUrl(cameraId, tokenOverride)
      const ws = new WebSocket(wsUrl)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        setStreaming(true)
      }

      ws.onerror = () => {
        // 推流连接失败：关闭摄像头并给出提示，避免摄像头亮着却无法推送
        toast.error('推流连接失败，已关闭摄像头（请检查网络或授权 token 是否有效）')
        cleanup()
      }

      ws.onclose = () => {
        setStreaming(false)
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
  }, [cameraId, tokenOverride, detectMultipleCameras, cleanup])

  // 切换前后摄像头：保持 WebSocket 与推流不中断，只替换视频源
  const switchCamera = useCallback(async () => {
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

  const close = useCallback(() => {
    cleanup()
  }, [cleanup])

  return {
    videoRef,
    canvasRef,
    active,
    streaming,
    opening,
    switching,
    facingMode,
    hasMultipleCameras,
    mediaSupported,
    open,
    close,
    switchCamera,
  }
}
