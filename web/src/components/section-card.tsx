import { useState, type ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

// 列表卡片:点击标题行整体收起/展开(默认展开)。收起时只留标题行,
// 内容与右侧操作按钮一并隐藏;展开时布局与原「标题 + 操作按钮」卡片一致。
export function SectionCard({
  title,
  actions,
  children,
}: {
  title: ReactNode
  actions?: ReactNode
  children: ReactNode
}) {
  const [open, setOpen] = useState(true)
  return (
    <div className="rounded-xl border bg-card p-5">
      <div className={`flex items-center justify-between gap-2 ${open ? 'mb-3' : ''}`}>
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-1 text-left"
        >
          {open
            ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
          <span className="truncate text-sm font-semibold">{title}</span>
        </button>
        {open && actions}
      </div>
      {open && children}
    </div>
  )
}
