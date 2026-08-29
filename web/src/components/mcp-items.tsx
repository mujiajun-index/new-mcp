import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight } from 'lucide-react'

// 资源/资源模板/提示条目卡片:与服务详情页同一交互——
// 名称点击展开/收起描述(默认收起),标签 chips 常显在下方。
// 市场详情页(快照数据)使用;服务详情页为同结构的内联实现。
// action 渲染在名称行最右(与 ToolItem 同款,市场详情页的条目价格徽标/定价控件用)。

export function ResourceItemCard({
  name,
  uri,
  description,
  mimeType,
  isTemplate,
  action,
}: {
  name?: string
  uri: string
  description?: string
  mimeType?: string
  isTemplate?: boolean
  action?: ReactNode
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <div className={`rounded-lg border p-3${isTemplate ? ' border-dashed' : ''}`}>
      <div className="flex items-start gap-1">
        <button
          type="button"
          disabled={!description}
          onClick={() => setOpen((v) => !v)}
          className={`min-w-0 flex-1 text-left text-sm font-medium font-mono break-all ${description ? 'cursor-pointer' : 'cursor-default'}`}
        >
          {name || uri}
        </button>
        {action && <div className="shrink-0">{action}</div>}
        {description && (open
          ? <ChevronDown className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          : <ChevronRight className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />)}
      </div>
      {name && <p className="mt-0.5 text-xs font-mono text-muted-foreground break-all">{uri}</p>}
      {open && description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
      <div className="mt-0.5 flex flex-wrap gap-1">
        <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
          {isTemplate ? t('services.resourceTemplate') : t('services.resourceKind')}
        </span>
        {mimeType && <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{mimeType}</span>}
      </div>
    </div>
  )
}

export function PromptItemCard({
  name,
  description,
  args,
  action,
}: {
  name: string
  description?: string
  args?: Array<{ name: string; required?: boolean }>
  action?: ReactNode
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="rounded-lg border p-3">
      <div className="flex items-center gap-1">
        <button
          type="button"
          disabled={!description}
          onClick={() => setOpen((v) => !v)}
          className={`min-w-0 flex-1 text-left text-sm font-medium font-mono ${description ? 'cursor-pointer' : 'cursor-default'}`}
        >
          {name}
        </button>
        {action && <div className="shrink-0">{action}</div>}
        {description && (open
          ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />)}
      </div>
      {open && description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
      {args && args.length > 0 && (
        <div className="mt-1 flex flex-wrap gap-1">
          {args.map((a) => (
            <span key={a.name} className={`rounded px-1.5 py-0.5 text-[10px] ${a.required ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'}`}>
              {a.name}{a.required ? '*' : ''}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
