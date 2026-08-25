import { useEffect, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Loader2, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { callServicePrompt } from '../api'
import { ContentBlock, TestResultPanel } from './test-result-panel'
import type { McpPrompt, ToolCallResult } from '@/types'

// 提示测试弹窗:服务详情页提示列表的「测试」入口,执行 prompts/get。
// 参数区按 prompt.arguments 生成字符串输入(MCP 提示参数均为字符串);
// 结果区渲染 messages(角色 chip + 内容块,内容块与工具测试共用渲染)。

export function PromptTestDialog({
  serviceId,
  prompt,
  open,
  onOpenChange,
}: {
  serviceId: number
  prompt: McpPrompt | null
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const { t } = useTranslation()
  const args = prompt?.arguments || []
  const [values, setValues] = useState<Record<string, string>>({})
  const [result, setResult] = useState<ToolCallResult | null>(null)

  // 打开/切换提示时重置
  useEffect(() => {
    if (!open || !prompt) return
    setValues({})
    setResult(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, prompt?.name])

  const missingRequired = args.some((a) => a.required && !(values[a.name] || '').trim())

  const runMutation = useMutation({
    // api 层返回 {success, message, data} 信封,这里解出真正的 ToolCallResult
    mutationFn: async (): Promise<ToolCallResult> => {
      const arguments_: Record<string, string> = {}
      for (const a of args) {
        const v = (values[a.name] || '').trim()
        if (v !== '') arguments_[a.name] = v
      }
      const res = await callServicePrompt(serviceId, { name: prompt!.name, arguments: arguments_ })
      return res.data as ToolCallResult
    },
    onSuccess: (data) => setResult(data),
    onError: () => setResult(null),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col gap-4 sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="font-mono">{prompt?.name}</DialogTitle>
          {prompt?.description && <DialogDescription className="line-clamp-2">{prompt.description}</DialogDescription>}
        </DialogHeader>

        {/* 参数区(p-2 内边距 + 父级 gap,与标题/底部按钮留出间距) */}
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-2">
          <span className="text-sm font-medium">{t('services.toolTest.params')}</span>
          {args.length === 0 ? (
            <p className="py-2 text-xs text-muted-foreground">{t('services.promptTest.noParams')}</p>
          ) : (
            <div className="space-y-3">
              {args.map((a) => (
                <div key={a.name} className="space-y-1">
                  <Label className="text-xs">
                    <span className="font-mono">{a.name}</span>
                    {a.required && <span className="ml-0.5 text-red-500">*</span>}
                  </Label>
                  {a.description && <p className="text-[10px] text-muted-foreground">{a.description}</p>}
                  <Input
                    value={values[a.name] || ''}
                    onChange={(e) => setValues((s) => ({ ...s, [a.name]: e.target.value }))}
                    className="h-8"
                  />
                </div>
              ))}
            </div>
          )}

          {/* 结果区 */}
          {result && (
            <TestResultPanel result={result}>
              {Array.isArray(result.result?.messages) && result.result.messages.length > 0 ? (
                <div className="max-h-72 space-y-2 overflow-y-auto rounded-md bg-muted/50 p-2.5">
                  {typeof result.result.description === 'string' && (
                    <p className="text-xs text-muted-foreground">{result.result.description}</p>
                  )}
                  {result.result.messages.map((m, i) => (
                    <div key={i} className="space-y-1 rounded-md bg-muted/40 p-2">
                      <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                        {m.role || 'user'}
                      </span>
                      {m.content && <ContentBlock block={m.content} index={i} />}
                    </div>
                  ))}
                </div>
              ) : undefined}
            </TestResultPanel>
          )}
        </div>

        <DialogFooter>
          {missingRequired && <p className="mr-auto text-xs text-red-500">{t('services.toolTest.requiredMissing')}</p>}
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button onClick={() => runMutation.mutate()} disabled={runMutation.isPending || missingRequired || !prompt}>
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
