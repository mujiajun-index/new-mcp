import { useState } from 'react'
import { useTranslation } from 'react-i18next'

// 工具参数展示:参数名 chips(必填带 *),点击 chip 展开该参数的类型与描述,再点收起。
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
            className={`rounded px-1.5 py-0.5 font-mono text-[10px] transition-colors ${
              selected === name
                ? 'bg-primary text-primary-foreground'
                : 'bg-muted text-muted-foreground hover:text-foreground'
            }`}
          >
            {name}{required.has(name) && <span className="ml-0.5 text-red-500">*</span>}
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
