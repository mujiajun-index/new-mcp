import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Image as ImageIcon, Copy, Check } from 'lucide-react'
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
export function Thumb({ src, alt }: { src: string; alt: string }) {
  const [broken, setBroken] = useState(false)
  if (broken || !src) {
    return (
      <div className="flex h-11 w-11 items-center justify-center rounded-md border bg-muted">
        <ImageIcon className="h-5 w-5 text-muted-foreground/40" />
      </div>
    )
  }
  return (
    <img
      src={src}
      alt={alt}
      loading="lazy"
      onError={() => setBroken(true)}
      className="h-11 w-11 rounded-md border object-cover"
    />
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
