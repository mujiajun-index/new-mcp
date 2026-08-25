import { useEffect, useMemo, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Loader2, Play } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { callServiceTool } from '../api'
import { ContentBlock, TestResultPanel } from './test-result-panel'
import type { McpTool, ToolCallResult } from '@/types'

// 工具测试弹窗:服务详情页工具列表的「测试」入口。
// 参数区根据 inputSchema 动态生成表单,可切 JSON 模式直接编辑 arguments;
// 结果区渲染 MCP content 块(text/image/resource),支持原始 JSON 查看与复制。
// 表单值统一存 string(数字/数组/对象提交前转换),boolean 存 boolean。

interface ParamSchema {
  type?: string | string[]
  description?: string
  enum?: (string | number)[]
  default?: unknown
  items?: { type?: string }
}

function schemaOf(tool: McpTool | null) {
  const schema = (tool?.inputSchema || {}) as {
    properties?: Record<string, ParamSchema | null>
    required?: string[]
  }
  // 过滤值为 null/非对象的脏属性,避免下游 p.type/p.default 访问崩溃
  const properties = Object.fromEntries(
    Object.entries(schema.properties || {}).filter(([, p]) => !!p && typeof p === 'object'),
  ) as Record<string, ParamSchema>
  return { properties, required: new Set(Array.isArray(schema.required) ? schema.required : []) }
}

function firstType(p: ParamSchema | null): string {
  const t = p?.type
  return Array.isArray(t) ? (t[0] || 'string') : (t || 'string')
}

export function ToolTestDialog({
  serviceId,
  tool,
  open,
  onOpenChange,
}: {
  serviceId: number
  tool: McpTool | null
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const { t } = useTranslation()
  const { properties, required } = useMemo(() => schemaOf(tool), [tool])
  const names = Object.keys(properties)

  const [mode, setMode] = useState<'form' | 'json'>('form')
  // 表单值:string(标量/数组每行一个/对象为 JSON 文本)或 boolean
  const [values, setValues] = useState<Record<string, string | boolean>>({})
  // 用户是否改过该字段:boolean 未触碰且无默认值时不进 arguments
  const [touched, setTouched] = useState<Record<string, boolean>>({})
  const [jsonText, setJsonText] = useState('{}')
  const [jsonError, setJsonError] = useState('')
  const [result, setResult] = useState<ToolCallResult | null>(null)

  // 打开/切换工具时重置,并带入 schema 默认值
  useEffect(() => {
    if (!open || !tool) return
    setMode('form')
    setJsonError('')
    setResult(null)
    const init: Record<string, string | boolean> = {}
    for (const [name, p] of Object.entries(properties)) {
      if (p.default === undefined) continue
      if (typeof p.default === 'boolean') init[name] = p.default
      else if (Array.isArray(p.default) || typeof p.default === 'object') init[name] = JSON.stringify(p.default, null, 2)
      else init[name] = String(p.default)
    }
    setValues(init)
    setTouched({})
    setJsonText(JSON.stringify(Object.fromEntries(
      Object.entries(properties)
        .filter(([n, p]) => p.default !== undefined && init[n] !== undefined && init[n] !== '')
        .map(([n, p]) => {
          const type = firstType(p)
          const v = init[n]
          if (type === 'number' || type === 'integer') return [n, Number(v)]
          if (type === 'boolean') return [n, v]
          if (type === 'object') {
            try { return [n, JSON.parse(String(v))] } catch { return [n, v] }
          }
          return [n, v]
        }),
    ), null, 2))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, tool?.name])

  // 表单值 → arguments(空值策略:未填且无默认值的可选参数不放进 arguments)
  function buildFormArguments(): Record<string, unknown> | null {
    const args: Record<string, unknown> = {}
    for (const [name, p] of Object.entries(properties)) {
      const v = values[name]
      const type = firstType(p)
      if (p.enum) {
        if (typeof v === 'string' && v !== '') args[name] = v
        continue
      }
      if (type === 'number' || type === 'integer') {
        if (typeof v === 'string' && v.trim() !== '') {
          const n = Number(v)
          if (Number.isNaN(n)) {
            toast.error(`${name}: ${t('services.toolTest.invalidNumber')}`)
            return null
          }
          args[name] = n
        }
        continue
      }
      if (type === 'boolean') {
        if (touched[name] || p.default !== undefined) args[name] = v === true
        continue
      }
      if (type === 'array') {
        if (typeof v === 'string' && v.trim() !== '') {
          const lines = v.split('\n').map((s) => s.trim()).filter(Boolean)
          args[name] = p.items?.type === 'number' ? lines.map(Number) : lines
        }
        continue
      }
      if (type === 'object') {
        if (typeof v === 'string' && v.trim() !== '') {
          try {
            args[name] = JSON.parse(v)
          } catch {
            toast.error(`${name}: ${t('services.toolTest.invalidJson')}`)
            return null
          }
        }
        continue
      }
      // string 及未知类型按字符串处理
      if (typeof v === 'string' && v !== '') args[name] = v
    }
    return args
  }

  function parseJsonArguments(): Record<string, unknown> | null {
    try {
      const parsed = JSON.parse(jsonText)
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        setJsonError(t('services.toolTest.invalidJson'))
        return null
      }
      setJsonError('')
      return parsed as Record<string, unknown>
    } catch {
      setJsonError(t('services.toolTest.invalidJson'))
      return null
    }
  }

  // 必填未填校验(boolean 恒有值,跳过)
  const missingRequired = mode === 'form' && Object.entries(properties).some(([name, p]) => {
    if (!required.has(name) || firstType(p) === 'boolean') return false
    const v = values[name]
    return typeof v !== 'boolean' && String(v ?? '').trim() === ''
  })

  const runMutation = useMutation({
    // api 层返回 {success, message, data} 信封,这里解出真正的 ToolCallResult
    mutationFn: async (args: Record<string, unknown>): Promise<ToolCallResult> => {
      const res = await callServiceTool(serviceId, { name: tool!.name, arguments: args })
      return res.data as ToolCallResult
    },
    onSuccess: (data) => setResult(data),
    onError: () => setResult(null),
  })

  function handleRun() {
    const args = mode === 'json' ? parseJsonArguments() : buildFormArguments()
    if (args === null) return
    runMutation.mutate(args)
  }

  // 模式切换时双向同步:表单→JSON 序列化当前值;JSON→表单尽力回填(解析失败留在 JSON 模式)
  function handleModeChange(next: string) {
    if (next === mode) return
    if (next === 'json') {
      const args = buildFormArguments()
      if (args === null) return
      setJsonText(JSON.stringify(args, null, 2))
      setJsonError('')
      setMode('json')
      return
    }
    const parsed = parseJsonArguments()
    if (parsed === null) return
    const nextValues: Record<string, string | boolean> = {}
    for (const [name, raw] of Object.entries(parsed)) {
      const p = properties[name]
      if (!p) continue
      const type = firstType(p)
      if (typeof raw === 'boolean') nextValues[name] = raw
      else if (type === 'array' && Array.isArray(raw)) nextValues[name] = raw.map(String).join('\n')
      else if (typeof raw === 'object') nextValues[name] = JSON.stringify(raw, null, 2)
      else if (raw === null || raw === undefined) continue
      else nextValues[name] = String(raw)
      setTouched((prev) => ({ ...prev, [name]: true }))
    }
    setValues((prev) => ({ ...prev, ...nextValues }))
    setMode('form')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col gap-4 sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="font-mono">{tool?.name}</DialogTitle>
          {tool?.description && <DialogDescription className="line-clamp-2">{tool.description}</DialogDescription>}
        </DialogHeader>

        {/* 参数区(p-2 内边距 + 父级 gap,与标题/底部按钮留出间距) */}
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-2">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">{t('services.toolTest.params')}</span>
            {names.length > 0 && (
              <Tabs value={mode} onValueChange={handleModeChange}>
                <TabsList className="h-7">
                  <TabsTrigger value="form" className="h-5 px-2 text-xs">{t('services.toolTest.formMode')}</TabsTrigger>
                  <TabsTrigger value="json" className="h-5 px-2 text-xs">{t('services.toolTest.jsonMode')}</TabsTrigger>
                </TabsList>
              </Tabs>
            )}
          </div>

          {names.length === 0 ? (
            <p className="py-2 text-xs text-muted-foreground">{t('services.toolTest.noParams')}</p>
          ) : mode === 'form' ? (
            <div className="space-y-3">
              {Object.entries(properties).map(([name, p]) => {
                const type = firstType(p)
                const v = values[name]
                const isEnum = !!p.enum?.length
                return (
                  <div key={name} className="space-y-1">
                    <Label className="text-xs">
                      <span className="font-mono">{name}</span>
                      {required.has(name) && <span className="ml-0.5 text-red-500">*</span>}
                      {!isEnum && type !== 'boolean' && (
                        <span className="ml-1.5 text-[10px] font-normal text-muted-foreground">{type}</span>
                      )}
                    </Label>
                    {p.description && <p className="text-[10px] text-muted-foreground">{p.description}</p>}
                    {isEnum ? (
                      <Select
                        value={typeof v === 'string' ? v : undefined}
                        onValueChange={(val) => { setValues((s) => ({ ...s, [name]: val })); setTouched((s) => ({ ...s, [name]: true })) }}
                      >
                        <SelectTrigger className="h-8"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {p.enum!.map((opt) => (
                            <SelectItem key={String(opt)} value={String(opt)}>{String(opt)}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    ) : type === 'boolean' ? (
                      <div className="flex items-center gap-2 pt-1">
                        <Checkbox
                          id={`tt-${name}`}
                          checked={v === true}
                          onCheckedChange={(val) => { setValues((s) => ({ ...s, [name]: val === true })); setTouched((s) => ({ ...s, [name]: true })) }}
                        />
                        <Label htmlFor={`tt-${name}`} className="cursor-pointer text-xs text-muted-foreground">
                          {v === true ? 'true' : 'false'}
                        </Label>
                      </div>
                    ) : type === 'array' ? (
                      <Textarea
                        value={typeof v === 'string' ? v : ''}
                        onChange={(e) => { setValues((s) => ({ ...s, [name]: e.target.value })); setTouched((s) => ({ ...s, [name]: true })) }}
                        placeholder={t('services.toolTest.onePerLine')}
                        className="min-h-16 font-mono text-xs"
                      />
                    ) : type === 'object' ? (
                      <Textarea
                        value={typeof v === 'string' ? v : ''}
                        onChange={(e) => { setValues((s) => ({ ...s, [name]: e.target.value })); setTouched((s) => ({ ...s, [name]: true })) }}
                        placeholder='{ }'
                        className="min-h-16 font-mono text-xs"
                      />
                    ) : (
                      <Input
                        type={type === 'number' || type === 'integer' ? 'number' : 'text'}
                        value={typeof v === 'string' ? v : ''}
                        onChange={(e) => { setValues((s) => ({ ...s, [name]: e.target.value })); setTouched((s) => ({ ...s, [name]: true })) }}
                        className="h-8"
                      />
                    )}
                  </div>
                )
              })}
            </div>
          ) : (
            <div className="space-y-1">
              <Textarea
                value={jsonText}
                onChange={(e) => setJsonText(e.target.value)}
                placeholder='{ "param": "value" }'
                className="min-h-32 font-mono text-xs"
              />
              {jsonError && <p className="text-xs text-red-500">{jsonError}</p>}
            </div>
          )}

          {/* 结果区 */}
          {result && (
            <TestResultPanel result={result}>
              {Array.isArray(result.result?.content) && result.result.content.length > 0 ? (
                <div className="max-h-72 space-y-2 overflow-y-auto rounded-md bg-muted/50 p-2.5">
                  {result.result.content.map((block, i) => <ContentBlock key={i} block={block} index={i} />)}
                </div>
              ) : undefined}
            </TestResultPanel>
          )}
        </div>

        <DialogFooter>
          {missingRequired && <p className="mr-auto text-xs text-red-500">{t('services.toolTest.requiredMissing')}</p>}
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button onClick={handleRun} disabled={runMutation.isPending || missingRequired || !tool}>
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
