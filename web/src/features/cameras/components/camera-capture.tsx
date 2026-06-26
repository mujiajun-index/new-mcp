import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Video, VideoOff, Loader2, SwitchCamera } from 'lucide-react'
import { useCameraStream } from './use-camera-stream'

interface CameraCaptureProps {
  cameraId: number
  onStreamingChange?: (streaming: boolean) => void
}

export function CameraCapture({ cameraId, onStreamingChange }: CameraCaptureProps) {
  const { t } = useTranslation()
  const {
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
  } = useCameraStream(cameraId)

  // 同步推流状态给父组件
  useEffect(() => {
    onStreamingChange?.(streaming)
  }, [streaming, onStreamingChange])

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
            <p className="text-sm">{t('cameras.capture.notStarted')}</p>
          </div>
        )}

        {/* Streaming indicator overlay */}
        {active && (
          <div className="absolute top-3 left-3">
            <span className="inline-flex items-center gap-1.5 rounded-full bg-black/60 px-2.5 py-1 text-xs text-white">
              <span className={`h-2 w-2 rounded-full ${streaming ? 'bg-emerald-400' : 'bg-zinc-400'}`} />
              {streaming ? t('cameras.streamStreaming') : t('cameras.streamIdle')}
            </span>
          </div>
        )}

        {/* Facing direction overlay */}
        {active && (
          <div className="absolute top-3 right-3">
            <span className="inline-flex items-center rounded-full bg-black/60 px-2.5 py-1 text-xs text-white">
              {facingMode === 'user' ? t('cameras.capture.front') : t('cameras.capture.back')}
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
            <Button onClick={open} disabled={opening || !mediaSupported} className="gap-2">
              {opening ? <Loader2 className="h-4 w-4 animate-spin" /> : <Video className="h-4 w-4" />}
              {opening ? t('cameras.capture.starting') : mediaSupported ? t('cameras.capture.start') : t('cameras.capture.needsHttps')}
            </Button>
            {!mediaSupported && (
              <p className="text-xs text-muted-foreground">{t('cameras.capture.httpsHint')}</p>
            )}
          </>
        ) : (
          <div className="flex gap-2">
            <Button variant="outline" onClick={close} className="gap-2">
              <VideoOff className="h-4 w-4" />
              {t('cameras.capture.stop')}
            </Button>
            <Button
              variant="outline"
              onClick={switchCamera}
              disabled={switching || !hasMultipleCameras}
              className="gap-2"
              title={hasMultipleCameras ? t('cameras.capture.flipTitle') : t('cameras.capture.oneCamOnly')}
            >
              {switching ? <Loader2 className="h-4 w-4 animate-spin" /> : <SwitchCamera className="h-4 w-4" />}
              {t('cameras.capture.flip')}
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
