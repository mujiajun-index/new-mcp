import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ChevronLeft, ChevronRight } from 'lucide-react'

// LocalPager 客户端本地分页控件(数据全量已在前端):左总数右翻页,样式与调用日志/
// 市场管理页的接口分页一致。page 由调用方夹紧(不超 totalPages)后传入;数据轮询
// 刷新导致条数变化时,调用方用 min(page, totalPages) 保证落在有效页。
export function LocalPager({
  total, page, totalPages, onPage,
}: {
  total: number
  page: number
  totalPages: number
  onPage: (p: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between">
      <p className="text-sm text-muted-foreground">
        {t('common.total')} {total} {t('common.items')}
      </p>
      <div className="flex items-center gap-2">
        <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => onPage(page - 1)}>
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <span className="text-sm tabular-nums">{page} / {totalPages}</span>
        <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => onPage(page + 1)}>
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
