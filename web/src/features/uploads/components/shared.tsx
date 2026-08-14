import { useEffect, useMemo, useState } from 'react'
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
  Loader2,
  ExternalLink,
} from 'lucide-react'
import type { UploadListItem, UploadPreviewResponse } from '../api'
import { copyText, formatBytes } from '../utils'
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
        referrerPolicy="no-referrer"
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
            referrerPolicy="no-referrer"
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

export function CopyUrlButton({
  url,
  t,
  iconOnly = false,
}: {
  url: string
  t: (k: string) => string
  iconOnly?: boolean
}) {
  const [copied, setCopied] = useState(false)
  return (
    <Button
      variant="ghost"
      size="sm"
      className="gap-1"
      title={t('uploads.copyUrl')}
      onClick={async () => {
        await copyText(url)
        setCopied(true)
        toast.success(t('common.copied'))
        setTimeout(() => setCopied(false), 2000)
      }}
    >
      {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
      {!iconOnly && (copied ? t('common.copied') : t('uploads.copyUrl'))}
    </Button>
  )
}

// dataUrlToBlobUrl decodes a `data:<mime>;base64,...` URL into a blob: URL.
// Browsers BLOCK top-level navigation to data: URLs (a target=_blank / link click
// yields a blank tab), but blob: URLs are navigable — so converting lets the
// "open in new tab" affordance actually render the optimized image. The caller is
// responsible for URL.revokeObjectURL when done.
function dataUrlToBlobUrl(dataUrl: string): string {
  const comma = dataUrl.indexOf(',')
  const meta = dataUrl.slice(5, comma) // "image/png;base64"
  const mime = meta.split(';')[0] // "image/png"
  const bin = atob(dataUrl.slice(comma + 1))
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return URL.createObjectURL(new Blob([bytes], { type: mime }))
}

// PreviewImage is a clickable preview tile. Wrapping the <img> in a real
// <a target="_blank"> makes BOTH left-click and right-click → "open in new tab"
// work reliably: the browser's native "open image in new tab" is flaky for data:
// URLs (which the optimized image is), and the old wrapping <div> ate the click
// entirely. Opening at full size lets the admin compare original vs optimized in
// detail in separate tabs.
function PreviewImage({ src, alt, openLabel }: { src: string; alt?: string; openLabel: string }) {
  return (
    <a
      href={src}
      target="_blank"
      rel="noopener noreferrer"
      title={openLabel}
      className="group relative flex h-48 cursor-zoom-in items-center justify-center overflow-hidden rounded-md border bg-muted"
    >
      <img
        src={src}
        alt={alt}
        referrerPolicy="no-referrer"
        className="max-h-full max-w-full object-contain"
      />
      <span className="pointer-events-none absolute bottom-1 right-1 flex items-center gap-1 rounded bg-foreground/70 px-1.5 py-0.5 text-[10px] text-background opacity-0 transition group-hover:opacity-100">
        <ExternalLink className="h-3 w-3" />
        {openLabel}
      </span>
    </a>
  )
}

// OptimizedPreviewDialog shows what actually gets sent upstream after the CURRENT
// read-path transform settings (resize / re-encode) are applied to one stored
// image. It is a read-only dry-run (the backend computes on a byte copy and never
// writes back). The original is always shown alongside so the admin can eyeball
// the difference; when the transform is a no-op (`unchanged`), the optimized side
// renders the original URL and a note explains why nothing changed.
export function OptimizedPreviewDialog({
  item,
  result,
  loading,
  error,
  open,
  onOpenChange,
}: {
  item: UploadListItem | null
  result: UploadPreviewResponse | null
  loading: boolean
  error: unknown
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const orig = result?.original
  const opt = result?.optimized
  const settings = result?.settings
  const unchanged = result?.unchanged ?? false
  const resized = !!orig && !!opt && (orig.width !== opt.width || orig.height !== opt.height)
  const formatChanged = !!orig && !!opt && orig.mime !== opt.mime
  const saved =
    orig && opt && orig.size > 0 ? Math.max(0, Math.round((1 - opt.size / orig.size) * 100)) : 0

  // The optimized image arrives as a data: URL. Browsers BLOCK top-level navigation
  // to data: URLs (opening one yields a blank tab), so decode it once into a blob:
  // URL — which IS navigable — and use that for both display and "open in new tab".
  // When there's no transform (unchanged), fall back to the original's signed URL,
  // which opens fine on its own. The blob is revoked on change/unmount.
  const optimizedSrc = useMemo(() => {
    if (!result || unchanged || !result.data_url) return item?.url ?? ''
    return dataUrlToBlobUrl(result.data_url)
  }, [result, unchanged, item?.url])
  useEffect(() => {
    if (!optimizedSrc.startsWith('blob:')) return
    return () => URL.revokeObjectURL(optimizedSrc)
  }, [optimizedSrc])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl gap-4">
        <DialogTitle>{t('uploads.optimizedPreview')}</DialogTitle>

        {settings && (
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <Badge variant={settings.resize_enabled ? 'default' : 'outline'}>
              {t('uploads.previewResize')}: {settings.resize_enabled ? `${settings.resize_max_edge}px` : '—'}
            </Badge>
            <Badge variant={settings.compress_enabled ? 'default' : 'outline'}>
              {t('uploads.previewCompress')}: {settings.compress_enabled ? `q${settings.jpeg_quality}` : '—'}
            </Badge>
          </div>
        )}

        {loading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t('common.loading')}
          </div>
        ) : error ? (
          <div className="py-10 text-center text-sm text-destructive">
            {t('uploads.optimizedPreviewError')}
          </div>
        ) : result ? (
          <div className="space-y-3">
            {unchanged && (
              <p className="rounded-md border border-dashed bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                {t('uploads.optimizedUnchanged')}
              </p>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <figure className="space-y-2">
                <PreviewImage src={item?.url ?? ''} alt={item?.key} openLabel={t('uploads.previewOpenFull')} />
                <figcaption className="text-xs text-muted-foreground">
                  <div className="font-medium text-foreground">{t('uploads.previewOriginal')}</div>
                  {orig && (
                    <div className="tabular-nums">
                      {orig.width}×{orig.height} · {formatBytes(orig.size)} · {orig.mime}
                    </div>
                  )}
                </figcaption>
              </figure>

              <figure className="space-y-2">
                <PreviewImage
                  src={optimizedSrc}
                  alt={item?.key}
                  openLabel={t('uploads.previewOpenFull')}
                />
                <figcaption className="space-y-0.5 text-xs text-muted-foreground">
                  <div className="flex flex-wrap items-center gap-1.5 font-medium text-foreground">
                    {t('uploads.previewOptimized')}
                    {!unchanged && resized && (
                      <Badge variant="secondary" className="text-[10px]">
                        {t('uploads.previewResized')}
                      </Badge>
                    )}
                    {!unchanged && formatChanged && (
                      <Badge variant="secondary" className="text-[10px]">
                        {t('uploads.previewFormatChanged')}
                      </Badge>
                    )}
                  </div>
                  {opt && (
                    <div className="tabular-nums">
                      {opt.width}×{opt.height} · {formatBytes(opt.size)} · {opt.mime}
                    </div>
                  )}
                  {!unchanged && saved > 0 && (
                    <div className="font-medium text-emerald-600 dark:text-emerald-400">
                      {t('uploads.previewSaved', { pct: saved })}
                    </div>
                  )}
                </figcaption>
              </figure>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
