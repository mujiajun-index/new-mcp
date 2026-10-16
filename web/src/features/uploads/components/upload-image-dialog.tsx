import { useEffect, useId, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Check, Loader2, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { uploadImage } from '../api'
import { formatBytes } from '../utils'
import { CopyUrlButton } from './shared'

export function UploadImageDialog({
  onClose,
  onUploaded,
}: {
  onClose: () => void
  onUploaded: () => void
}) {
  const { t } = useTranslation()
  const fileInputId = useId()
  const urlInputId = useId()
  const fileInput = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [previewUrl, setPreviewUrl] = useState('')
  const [dragging, setDragging] = useState(false)
  const upload = useMutation({
    mutationFn: uploadImage,
    onSuccess: () => {
      toast.success(t('uploads.uploadSuccess'))
      onUploaded()
    },
  })

  useEffect(() => {
    if (!file) return
    const url = URL.createObjectURL(file)
    setPreviewUrl(url)
    return () => URL.revokeObjectURL(url)
  }, [file])

  function selectFile(selectedFile?: File) {
    if (!selectedFile || upload.isPending) return
    if (
      !['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(
        selectedFile.type,
      ) &&
      !/\.(png|jpe?g|webp|gif)$/i.test(selectedFile.name)
    ) {
      toast.error(t('uploads.imageFormatsError'))
      return
    }
    upload.reset()
    setFile(selectedFile)
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !upload.isPending) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('uploads.uploadImage')}</DialogTitle>
          <DialogDescription>
            {t('uploads.uploadDescription')}
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (file && !upload.isPending && !upload.isSuccess)
              upload.mutate(file)
          }}
          className="space-y-4"
        >
          <div>
            <input
              ref={fileInput}
              id={fileInputId}
              type="file"
              className="hidden"
              accept="image/png,image/jpeg,image/webp,image/gif,.png,.jpg,.jpeg,.webp,.gif"
              disabled={upload.isPending}
              onChange={(event) => {
                selectFile(event.target.files?.[0])
                event.target.value = ''
              }}
            />
            <button
              type="button"
              disabled={upload.isPending}
              aria-label={t('uploads.chooseImage')}
              aria-controls={fileInputId}
              onClick={() => fileInput.current?.click()}
              onDragOver={(event) => {
                event.preventDefault()
                if (upload.isPending) return
                event.dataTransfer.dropEffect = 'copy'
                setDragging(true)
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={(event) => {
                event.preventDefault()
                setDragging(false)
                selectFile(event.dataTransfer.files[0])
              }}
              className={cn(
                'group flex min-h-40 w-full cursor-pointer items-center justify-center rounded-xl border-2 border-dashed border-border bg-muted/20 px-5 py-7 text-center transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 enabled:hover:border-primary/50 enabled:hover:bg-primary/5 disabled:cursor-not-allowed disabled:opacity-50',
                dragging && 'border-primary bg-primary/10',
              )}
            >
              <div className="pointer-events-none flex flex-col items-center gap-3">
                <Upload
                  className={cn(
                    'h-7 w-7 text-muted-foreground transition-colors group-hover:text-primary',
                    dragging && 'text-primary',
                  )}
                  strokeWidth={1.5}
                />
                <div className="space-y-1.5">
                  <p className="text-sm text-muted-foreground">
                    {dragging ? (
                      t('uploads.releaseImage')
                    ) : (
                      <>
                        {t('uploads.dropImageHere')}{' '}
                        <span className="font-medium text-primary">
                          {t('uploads.chooseImage')}
                        </span>
                      </>
                    )}
                  </p>
                  <p className="text-xs leading-relaxed text-muted-foreground/75">
                    {t('uploads.uploadFormats')}
                  </p>
                </div>
              </div>
            </button>
          </div>

          {file && previewUrl && (
            <div className="overflow-hidden rounded-lg border bg-muted/30">
              <div className="flex h-48 items-center justify-center p-3">
                <img
                  src={upload.data?.url || previewUrl}
                  alt={file.name}
                  className="max-h-full max-w-full object-contain"
                  referrerPolicy="no-referrer"
                />
              </div>
              <p
                className="truncate border-t px-3 py-2 text-xs text-muted-foreground"
                title={file.name}
              >
                {file.name} · {formatBytes(upload.data?.size ?? file.size)}
              </p>
            </div>
          )}

          {upload.data && (
            <div className="space-y-2 rounded-lg border p-3">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor={urlInputId}>{t('uploads.imageUrl')}</Label>
                <CopyUrlButton url={upload.data.url} t={t} />
              </div>
              <Input
                id={urlInputId}
                value={upload.data.url}
                readOnly
                className="font-mono text-xs"
                onFocus={(event) => event.target.select()}
              />
              <p className="text-xs text-muted-foreground">
                {t('uploads.uploadOwnership')}
              </p>
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={upload.isPending}
              onClick={onClose}
            >
              {t('common.close')}
            </Button>
            <Button
              type="submit"
              disabled={!file || upload.isPending || upload.isSuccess}
              className="gap-2"
            >
              {upload.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : upload.isSuccess ? (
                <Check className="h-4 w-4" />
              ) : (
                <Upload className="h-4 w-4" />
              )}
              {t(
                upload.isPending
                  ? 'uploads.uploading'
                  : upload.isSuccess
                    ? 'uploads.uploadSuccess'
                    : 'uploads.uploadImage',
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
