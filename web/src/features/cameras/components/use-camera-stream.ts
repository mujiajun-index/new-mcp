import { useState, useRef, useCallback, useEffect } from 'react'
import { toast } from 'sonner'
import i18n from '@/i18n/config'

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

/**
 * 手持设备（手机/平板）判定：桌面 Chrome 会把请求的 facingMode 约束原样回显到
 * getSettings()，笔记本红外摄像头还会让设备数虚增，这些信号在桌面端都不可信，
 * 因此只有手持设备才信任 track 上报值。请求桌面站点的 iPad UA 形如 Macintosh，
 * 需按触屏硬件区分，但 maxTouchPoints 在新版桌面 Chrome/Firefox 会因触控板虚报
 * 为 >0（真 Mac 被误判成手持设备、摄像头错标为后置），故改用 (any-pointer: coarse)：
 * 它描述真实触屏硬件，UA 伪装与触控板都影响不到（Windows 触屏本 UA 含 Windows NT，
 * 不会误入该分支）。
 */
function isHandheldDevice(): boolean {
  const ua = navigator.userAgent
  if (/Android|iPhone|iPad|iPod|Mobile/i.test(ua)) return true
  if (/Macintosh/i.test(ua)) return window.matchMedia('(any-pointer: coarse)').matches
  return false
}

/**
 * 识别流的真实朝向：手持设备以 track 上报的 facingMode 为准（未上报时用请求值兜底）；
 * 桌面/笔记本（含触屏本、外接摄像头）一律按前置处理——摄像头朝向用户，镜像预览才符合直觉。
 */
function resolveFacingMode(stream: MediaStream, requested: FacingMode): FacingMode {
  if (isHandheldDevice()) {
    const reported = stream.getVideoTracks()[0]?.getSettings?.().facingMode
    if (reported === 'user' || reported === 'environment') return reported
    return requested
  }
  return 'user'
}

function buildWebSocketUrl(cameraId: number, streamKey: string): string {
  const loc = window.location
  const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:'
  const base: string = import.meta.env.BASE_URL ?? '/'
  const apiBase = base.endsWith('/') ? base.slice(0, -1) : base
  return `${protocol}//${loc.host}${apiBase}/api/v1/cameras/${cameraId}/stream?k=${encodeURIComponent(streamKey)}`
}

/**
 * 推流预检：浏览器 WS API 不暴露握手失败的状态码/响应体，先用普通 GET（无 Upgrade 头）
 * 请求推流端点，拿到服务器的 JSON 错误（密钥无效/已过期/已撤销/摄像头已禁用/正在推流中）。
 * 返回 400（"not using the websocket protocol"）说明校验全部通过，可继续真实 WS 连接。
 */
async function preflightStream(cameraId: number, streamKey: string): Promise<{ ok: boolean; message?: string }> {
  const loc = window.location
  const base: string = import.meta.env.BASE_URL ?? '/'
  const apiBase = base.endsWith('/') ? base.slice(0, -1) : base
  try {
    const res = await fetch(`${loc.origin}${apiBase}/api/v1/cameras/${cameraId}/stream?k=${encodeURIComponent(streamKey)}`)
    if (res.status === 400) return { ok: true }
    const json = await res.json().catch(() => null)
    return { ok: false, message: json?.error }
  } catch {
    // 预检本身的网络错误不阻塞，交给后续 WS 连接报错
    return { ok: true }
  }
}

/**
 * 摄像头推流核心逻辑：getUserMedia 取流 → WebSocket 推帧（每 2s）→ 支持前后切换。
 * 供「详情页预览」与「独立视频页」共用，UI 由各消费方自行渲染。
 *
 * @param cameraId 摄像头 ID
 * @param streamKey 推流密钥（管理页生成，随 /camera-live/:id?k= 链接分发）；WS 握手唯一凭证
 */
export function useCameraStream(cameraId: number, streamKey?: string) {
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
        toast.error(i18n.t('cameras.capture.httpsRequiredHook'))
      } else {
        toast.error(i18n.t('cameras.capture.notSupportedHook'))
      }
      return
    }

    if (!streamKey) {
      toast.error(i18n.t('cameras.capture.noStreamKey'))
      return
    }

    // 预检拿到精确的服务端错误提示（无需登录态，手机端同样生效），通过后再申请摄像头
    const probe = await preflightStream(cameraId, streamKey)
    if (!probe.ok) {
      toast.error(probe.message || i18n.t('cameras.capture.streamLoadFailed'))
      return
    }

    setOpening(true)
    try {
      const stream = await openStream(facingRef.current)

      streamRef.current = stream

      // 识别实际朝向（笔记本/桌面按前置），驱动镜像预览与朝向标签
      const actual = resolveFacingMode(stream, facingRef.current)
      facingRef.current = actual
      setFacingMode(actual)

      setActive(true)
      detectMultipleCameras()

      if (videoRef.current) {
        videoRef.current.srcObject = stream
        await videoRef.current.play()
      }

      // Start WebSocket connection
      const wsUrl = buildWebSocketUrl(cameraId, streamKey)
      const ws = new WebSocket(wsUrl)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        setStreaming(true)
      }

      ws.onerror = () => {
        // 推流连接失败：关闭摄像头并给出提示，避免摄像头亮着却无法推送
        toast.error(i18n.t('cameras.capture.streamLoadFailed'))
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
        toast.error(i18n.t('cameras.capture.permissionDeniedHook'))
      } else if (error.name === 'NotFoundError') {
        toast.error(i18n.t('cameras.capture.noDeviceHook'))
      } else {
        toast.error(i18n.t('cameras.capture.openFailedHook', { error: error.message || i18n.t('common.unknownError') }))
      }
    } finally {
      setOpening(false)
    }
  }, [cameraId, streamKey, detectMultipleCameras, cleanup])

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
        toast.error(i18n.t('cameras.capture.noCameraFacing'))
      } else {
        toast.error(i18n.t('cameras.capture.switchFailedHook'))
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
