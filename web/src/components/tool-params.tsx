import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight } from 'lucide-react'

// 工具参数展示:参数名 chips(必填带 *),点击 chip 展开该参数的类型与描述,再点收起。
// ToolItem:工具条目卡片,点击工具名展开/收起工具描述(默认收起,与参数 chips 同一交互)。
// 分组详情(聚合工具列表)与服务详情(工具列表)共用。
export function ToolParams({ schema }: { schema?: Record<string, unknown> }) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<string | null>(null)
  const properties = (schema?.properties || {}) as Record<string, { type?: string | string[]; description?: string }>
  const required = new Set<string>(Array.isArray(schema?.required) ? (schema.required as string[]) : [])
  const names = Object.keys(properties)
  if (names.length === 0) return null

  const meta = selected ? properties[selected] || {} : null
  return (
    <div className="mt-2">
      <div className="flex flex-wrap gap-1">
        {names.map((name) => (
          <button
            key={name}
            type="button"
            onClick={(e) => { e.stopPropagation(); setSelected(selected === name ? null : name) }}
            className={`inline-flex max-w-full items-baseline rounded px-1.5 py-0.5 font-mono text-[10px] transition-colors ${
              selected === name
                ? 'bg-primary text-primary-foreground'
                : 'bg-muted text-muted-foreground hover:text-foreground'
            }`}
          >
            <span className="truncate">{name}</span>{required.has(name) && <span className="ml-0.5 shrink-0 text-red-500">*</span>}
          </button>
        ))}
      </div>
      {selected && meta && (
        <div className="mt-1.5 flex flex-wrap items-baseline gap-x-2 gap-y-0.5 rounded-md bg-muted/40 px-2.5 py-1.5 text-xs">
          <code className="font-mono text-foreground/90">{selected}</code>
          {meta.type && (
            <span className="rounded bg-muted px-1 py-px text-[10px] text-muted-foreground">
              {Array.isArray(meta.type) ? meta.type.join(' | ') : meta.type}
            </span>
          )}
          {required.has(selected) && (
            <span className="text-[10px] font-medium text-red-500">{t('common.paramRequired')}</span>
          )}
          {meta.description && <span className="min-w-0 text-muted-foreground">{meta.description}</span>}
        </div>
      )}
    </div>
  )
}

export function ToolItem({
  name,
  description,
  schema,
  leading,
  subtitle,
  action,
}: {
  name: string
  description?: string
  schema?: Record<string, unknown>
  leading?: ReactNode
  subtitle?: ReactNode
  action?: ReactNode
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="flex items-start gap-3 rounded-lg border p-3">
      {leading}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1">
          <button
            type="button"
            disabled={!description}
            onClick={() => setOpen((v) => !v)}
            title={name}
            className={`flex min-w-0 flex-1 items-center gap-1 text-left text-sm font-medium font-mono ${description ? 'cursor-pointer' : 'cursor-default'}`}
          >
            {/* 展开箭头放名称前,避免与右侧 action 按钮挤在一起;固定宽占位保持无描述条目名称对齐 */}
            <span className="flex h-3.5 w-3.5 shrink-0 items-center justify-center text-muted-foreground">
              {description && (open
                ? <ChevronDown className="h-3.5 w-3.5" />
                : <ChevronRight className="h-3.5 w-3.5" />)}
            </span>
            <span className="truncate">{name}</span>
          </button>
          {action && <div className="shrink-0">{action}</div>}
        </div>
        {subtitle}
        {open && description && (
          <p className="mt-1 break-words text-xs text-muted-foreground">{description}</p>
        )}
        <ToolParams schema={schema} />
      </div>
    </div>
  )
}
