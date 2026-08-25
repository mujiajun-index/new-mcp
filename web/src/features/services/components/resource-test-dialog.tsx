import { useEffect, useMemo, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Loader2, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { readServiceResource } from '../api'
import { TestResultPanel, TEXT_TRUNCATE } from './test-result-panel'
import type { McpResource, McpResourceTemplate, ToolCallResult } from '@/types'

// 资源测试弹窗:服务详情页资源/资源模板列表的「测试」入口,执行 resources/read。
// 具体资源:URI 预填后可直接修改;模板:解析 uriTemplate 的 {param} 占位符生成
// 独立输入并实时预览替换后的 URI(简单字符串替换,不实现 RFC 6570 完整语义)。

export type ResourceTarget =
  | { kind: 'resource'; data: McpResource }
  | { kind: 'template'; data: McpResourceTemplate }

// 解析 uriTemplate 中的 {param} 占位符(保序去重)
function templateParams(tpl: string): string[] {
  const out: string[] = []
  for (const m of tpl.matchAll(/\{([^{}]+)\}/g)) {
    const name = m[1]
    if (name !== undefined && !out.includes(name)) out.push(name)
  }
  return out
}

// resources/read 的 contents 渲染:text 等宽展示、图片 blob 内联、其余 blob 展示 base64(截断)。
function ResourceContents({ contents }: { contents: NonNullable<ToolCallResult['result']['contents']> }) {
  const { t } = useTranslation()
  return (
    <div className="max-h-72 space-y-3 overflow-y-auto rounded-md bg-muted/50 p-2.5">
      {contents.map((c, i) => {
        const text = typeof c.text === 'string' ? c.text : undefined
        const blob = typeof c.blob === 'string' ? c.blob : undefined
        const body = text ?? blob
        const truncated = typeof body === 'string' && body.length > TEXT_TRUNCATE
        return (
          <div key={i} className="space-y-1">
            <div className="flex items-center gap-1.5">
              {c.uri && <p className="min-w-0 flex-1 truncate font-mono text-[10px] text-muted-foreground">{c.uri}</p>}
              {c.mimeType && (
                <span className="shrink-0 rounded bg-muted px-1 py-px font-mono text-[10px] text-muted-foreground">{c.mimeType}</span>
              )}
            </div>
            {typeof body === 'string' && c.mimeType?.startsWith('image/') && !text ? (
              <img src={`data:${c.mimeType};base64,${body}`} alt="" className="max-h-96 rounded-md border" />
            ) : typeof body === 'string' ? (
              <>
                <pre className="whitespace-pre-wrap break-all font-mono text-xs">
                  {truncated ? body.slice(0, TEXT_TRUNCATE) : body}
                </pre>
                {truncated && <p className="text-[10px] text-muted-foreground">{t('services.toolTest.truncated')}</p>}
              </>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

export function ResourceTestDialog({
  serviceId,
  target,
  open,
  onOpenChange,
}: {
  serviceId: number
  target: ResourceTarget | null
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const { t } = useTranslation()
  // 模板模式:占位符参数与取值;具体资源模式:params 为空,直接编辑 URI
  const params = useMemo(
    () => (target?.kind === 'template' ? templateParams(target.data.uriTemplate) : []),
    [target],
  )
  const [uri, setUri] = useState('')
  const [values, setValues] = useState<Record<string, string>>({})
  const [result, setResult] = useState<ToolCallResult | null>(null)

  // 打开/切换条目时重置;无占位符的模板按可编辑 URI 处理(预填模板串)
  const targetKind = target?.kind
  const targetKey = target ? (target.kind === 'resource' ? target.data.uri : target.data.uriTemplate) : ''
  useEffect(() => {
    if (!open || !target) return
    if (target.kind === 'resource') {
      setUri(target.data.uri)
    } else {
      setUri(templateParams(target.data.uriTemplate).length === 0 ? target.data.uriTemplate : '')
    }
    setValues({})
    setResult(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, targetKind, targetKey])

  // 占位符全部填充后替换出的最终 URI
  const resolvedUri = target?.kind === 'template'
    ? target.data.uriTemplate.replace(/\{([^{}]+)\}/g, (_, name: string) => values[name] || `{${name}}`)
    : uri.trim()
  const missing = target?.kind === 'template' && params.some((p) => !(values[p] || '').trim())

  const runMutation = useMutation({
    // api 层返回 {success, message, data} 信封,这里解出真正的 ToolCallResult
    mutationFn: async (readUri: string): Promise<ToolCallResult> => {
      const res = await readServiceResource(serviceId, { uri: readUri })
      return res.data as ToolCallResult
    },
    onSuccess: (data) => setResult(data),
    onError: () => setResult(null),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col gap-4 sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="font-mono">
            {target?.kind === 'template' ? target.data.name || target.data.uriTemplate : target?.data.name || target?.data.uri}
          </DialogTitle>
          {target?.data.description && <DialogDescription className="line-clamp-2">{target.data.description}</DialogDescription>}
        </DialogHeader>

        {/* 参数区(p-2 内边距 + 父级 gap,与标题/底部按钮留出间距) */}
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-2">
          <span className="text-sm font-medium">{t('services.toolTest.params')}</span>
          {target?.kind === 'template' && params.length > 0 ? (
            <div className="space-y-3">
              {params.map((name) => (
                <div key={name} className="space-y-1">
                  <Label className="text-xs">
                    <span className="font-mono">{name}</span>
                    <span className="ml-0.5 text-red-500">*</span>
                  </Label>
                  <Input
                    value={values[name] || ''}
                    onChange={(e) => setValues((s) => ({ ...s, [name]: e.target.value }))}
                    className="h-8 font-mono text-xs"
                  />
                </div>
              ))}
              <div className="space-y-1">
                <Label className="text-xs">{t('services.resourceTest.resolvedUri')}</Label>
                <p className="break-all rounded-md bg-muted/40 px-2 py-1.5 font-mono text-xs">{resolvedUri}</p>
              </div>
            </div>
          ) : (
            <div className="space-y-1">
              <Label className="text-xs">URI</Label>
              <Input
                value={uri}
                onChange={(e) => setUri(e.target.value)}
                className="h-8 font-mono text-xs"
              />
            </div>
          )}

          {/* 结果区 */}
          {result && (
            <TestResultPanel result={result}>
              {Array.isArray(result.result?.contents) && result.result.contents.length > 0 ? (
                <ResourceContents contents={result.result.contents} />
              ) : undefined}
            </TestResultPanel>
          )}
        </div>

        <DialogFooter>
          {missing && <p className="mr-auto text-xs text-red-500">{t('services.toolTest.requiredMissing')}</p>}
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button
            onClick={() => runMutation.mutate(resolvedUri)}
            disabled={runMutation.isPending || missing || resolvedUri === ''}
          >
            {runMutation.isPending
              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
              : <Play className="h-3.5 w-3.5" />}
            {runMutation.isPending ? t('services.toolTest.running') : t('services.toolTest.run')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
