import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface MobileCardMeta {
  label?: string
  value: ReactNode
}

interface MobileListCardProps {
  /** Primary identifier — left-aligned, medium weight. */
  title: ReactNode
  /** Optional content rendered at the right of the title row (badges, status). */
  badge?: ReactNode
  /** Two-column grid of label/value rows. */
  meta?: MobileCardMeta[]
  /** Optional full-width line rendered below the meta grid (e.g. an error message). */
  note?: ReactNode
  /** Right-aligned action row at the bottom of the card. */
  actions?: ReactNode
  className?: string
}

/**
 * Compact card used in place of a table row on mobile. Mirrors the reference
 * new-api MobileCardList layout: a title+badge row, a 2-up meta grid, and an
 * actions row. Pages wrap multiple cards in a
 * `<div className="divide-y rounded-lg border overflow-hidden">`.
 */
export function MobileListCard({
  title,
  badge,
  meta,
  note,
  actions,
  className,
}: MobileListCardProps) {
  return (
    <div className={cn('bg-card px-3 py-2.5', className)}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 font-medium">{title}</div>
        {badge && <div className="shrink-0">{badge}</div>}
      </div>

      {meta && meta.length > 0 && (
        <dl className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1.5">
          {meta.map((item, index) => (
            <div key={index} className="min-w-0">
              {item.label && (
                <dt className="text-[10px] text-muted-foreground">{item.label}</dt>
              )}
              <dd className="truncate text-xs">{item.value}</dd>
            </div>
          ))}
        </dl>
      )}

      {note && <div className="mt-2 text-xs">{note}</div>}

      {actions && <div className="mt-2 flex items-center justify-end gap-1">{actions}</div>}
    </div>
  )
}
