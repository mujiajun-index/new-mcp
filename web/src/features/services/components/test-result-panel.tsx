import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { McpContentBlock, ToolCallResult } from '@/types'

// 测试调用(工具/资源/提示)共用的结果展示面板:
// 头部为耗时/错误标记/原始 JSON/复制,主体由调用方以 children 传入专属渲染
// (工具→content 块、资源→contents、提示→messages),未传或为空时回退原始 JSON。

// 文本类内容单块最大展示长度(字符),超出截断并提示
export const TEXT_TRUNCATE = 100_000
const RAW_TRUNCATE = 200_000

// 安全上下文(HTTPS)用 navigator.clipboard,否则回退 execCommand(与 user-logs-page 一致)。
export async function copyText(text: string) {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text)
    return
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.top = '0'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.focus()
  ta.select()
  const ok = document.execCommand('copy')
  document.body.removeChild(ta)
  if (!ok) throw new Error('copy failed')
}

// 单个 content 块渲染:text 等宽展示、image 内联图片、resource 展示 uri+文本,未知类型回退原始 JSON。
export function ContentBlock({ block, index }: { block: McpContentBlock; index: number }) {
  const { t } = useTranslation()
  if (block.type === 'text' && typeof block.text === 'string') {
    const truncated = block.text.length > TEXT_TRUNCATE
    const text = truncated ? block.text.slice(0, TEXT_TRUNCATE) : block.text
    return (
      <>
        <pre key={index} className="whitespace-pre-wrap break-words font-mono text-xs">{text}</pre>
        {truncated && <p className="mt-1 text-[10px] text-muted-foreground">{t('services.toolTest.truncated')}</p>}
      </>
    )
  }
  if (block.type === 'image') {
    const src = block.data ? `data:${block.mimeType || 'image/png'};base64,${block.data}` : block.url
    if (src) {
      return <img key={index} src={src} alt="" className="max-h-96 rounded-md border" />
    }
  }
  if (block.type === 'resource' && block.resource) {
    const r = block.resource
    const text = r.text ?? r.blob
    return (
      <div key={index} className="rounded-md bg-muted/40 p-2">
        {r.uri && <p className="break-all font-mono text-[10px] text-muted-foreground">{r.uri}</p>}
        {typeof text === 'string' && <pre className="mt-1 whitespace-pre-wrap break-words font-mono text-xs">{text}</pre>}
      </div>
    )
  }
  return <pre key={index} className="whitespace-pre-wrap break-words font-mono text-xs">{JSON.stringify(block, null, 2)}</pre>
}

// TestResultPanel 结果区外壳:头部(耗时/错误/原始 JSON/复制)+ 主体(children 为空回退原始 JSON)。
// 面板随 result 置空而卸载,新结果到来时重置展开/复制状态。
export function TestResultPanel({ result, children }: { result: ToolCallResult; children?: ReactNode }) {
  const { t } = useTranslation()
  const [showRaw, setShowRaw] = useState(false)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    setShowRaw(false)
    setCopied(false)
  }, [result])

  async function handleCopy() {
    try {
      // ?? null:JSON.stringify(undefined) 返回 undefined,会导致下游 .length 崩溃
      await copyText(JSON.stringify(result.result ?? null, null, 2))
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const rawJson = JSON.stringify(result.result ?? null, null, 2)
  const rawTruncated = rawJson.length > RAW_TRUNCATE

  return (
    <div className="space-y-2 border-t pt-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{t('services.toolTest.result')}</span>
        <span className="text-xs text-muted-foreground">{t('services.toolTest.duration', { ms: result.duration_ms })}</span>
        {result.is_error && <Badge variant="destructive">{t('services.toolTest.error')}</Badge>}
        <div className="ml-auto flex gap-1">
          <Button variant="outline" size="sm" className="h-7" onClick={() => setShowRaw((v) => !v)}>
            {t('services.toolTest.rawJson')}
          </Button>
          <Button variant="outline" size="sm" className="h-7" onClick={handleCopy}>
            {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
            {copied ? t('services.toolTest.copied') : t('services.toolTest.copy')}
          </Button>
        </div>
      </div>
      {result.error ? (
        <div className="rounded-md border border-destructive/30 bg-destructive/10 p-2.5 text-xs text-destructive break-words">
          {result.error}
        </div>
      ) : showRaw ? (
        <>
          <pre className="max-h-72 overflow-auto rounded-md bg-muted/50 p-2.5 font-mono text-xs whitespace-pre-wrap break-all">
            {rawTruncated ? rawJson.slice(0, RAW_TRUNCATE) : rawJson}
          </pre>
          {rawTruncated && <p className="text-[10px] text-muted-foreground">{t('services.toolTest.truncated')}</p>}
        </>
      ) : children ?? (
        <pre className="max-h-72 overflow-auto rounded-md bg-muted/50 p-2.5 font-mono text-xs whitespace-pre-wrap break-all">
          {rawJson}
        </pre>
      )}
    </div>
  )
}
