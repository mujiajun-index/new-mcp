import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import {
  Image as ImageIcon,
  Copy,
  Check,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react'
import type { UploadListItem } from '../api'
import { copyText } from '../utils'
import { toast } from 'sonner'

export function BackendBadge({ backend }: { backend: string }) {
  const isS3 = backend === 's3'
  return (
    <Badge variant="outline" className="font-mono text-[10px]">
      {isS3 ? 'S3' : 'local'}
    </Badge>
  )
}

// A small thumbnail that loads the image straight from its signed GET URL.
// Broken/expired links degrade to a placeholder icon instead of throwing.
// `onClick` is fired on click — the parent owns the preview dialog so it can
// page through the whole list, not just this one image.
export function Thumb({
  src,
  alt,
  onClick,
}: {
  src: string
  alt: string
  onClick?: () => void
}) {
  const { t } = useTranslation()
  const [broken, setBroken] = useState(false)

  if (broken || !src) {
    return (
      <div className="flex h-11 w-11 items-center justify-center rounded-md border bg-muted">
        <ImageIcon className="h-5 w-5 text-muted-foreground/40" />
      </div>
    )
  }

  return (
    <button
      type="button"
      aria-label={t('uploads.preview')}
      onClick={onClick}
      className="flex h-11 w-11 cursor-zoom-in items-center justify-center overflow-hidden rounded-md border bg-muted transition hover:ring-2 hover:ring-ring/50"
    >
      <img
        src={src}
        alt={alt}
        loading="lazy"
        onError={() => setBroken(true)}
        className="h-full w-full object-cover"
      />
    </button>
  )
}

// Fixed-size preview dialog that pages through a list of uploads. The stage is
// viewport-sized so the dialog keeps its shape regardless of the image aspect
// ratio. Keyboard: ← / → to move, Esc / overlay click / X to close (Radix).
export function ImagePreviewDialog({
  items,
  index,
  open,
  onIndexChange,
  onOpenChange,
}: {
  items: UploadListItem[]
  index: number
  open: boolean
  onIndexChange: (index: number) => void
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const total = items.length
  const current = items[index]
  const hasPrev = index > 0
  const hasNext = index < total - 1

  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft' && index > 0) onIndexChange(index - 1)
      else if (e.key === 'ArrowRight' && index < total - 1) onIndexChange(index + 1)
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [open, index, total, onIndexChange])

  if (!current) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex w-auto max-w-none flex-col items-center justify-center gap-0 overflow-hidden p-0">
        <DialogTitle className="sr-only">{t('uploads.preview')}</DialogTitle>
        <div className="flex h-[80vh] w-[82vw] max-w-[1200px] items-center justify-center p-2">
          <img
            key={current.id}
            src={current.url}
            alt={current.key}
            className="max-h-full max-w-full animate-fade-in object-contain"
          />
        </div>

        {total > 1 && (
          <>
            <button
              type="button"
              aria-label={t('uploads.prevImage')}
              disabled={!hasPrev}
              onClick={() => onIndexChange(index - 1)}
              className="absolute left-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full border border-border/50 bg-background/80 text-foreground backdrop-blur transition hover:bg-background disabled:pointer-events-none disabled:opacity-0"
            >
              <ChevronLeft className="h-6 w-6" />
            </button>
            <button
              type="button"
              aria-label={t('uploads.nextImage')}
              disabled={!hasNext}
              onClick={() => onIndexChange(index + 1)}
              className="absolute right-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full border border-border/50 bg-background/80 text-foreground backdrop-blur transition hover:bg-background disabled:pointer-events-none disabled:opacity-0"
            >
              <ChevronRight className="h-6 w-6" />
            </button>
            <div className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full bg-foreground/70 px-3 py-1 text-xs font-medium tabular-nums text-background backdrop-blur">
              {index + 1} / {total}
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

export function CopyUrlButton({ url, t }: { url: string; t: (k: string) => string }) {
  const [copied, setCopied] = useState(false)
  return (
    <Button
      variant="ghost"
      size="sm"
      className="gap-1"
      onClick={async () => {
        await copyText(url)
        setCopied(true)
        toast.success(t('common.copied'))
        setTimeout(() => setCopied(false), 2000)
      }}
    >
      {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
      {copied ? t('common.copied') : t('uploads.copyUrl')}
    </Button>
  )
}
