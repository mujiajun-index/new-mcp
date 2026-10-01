import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { SectionCard } from '@/components/section-card'
import { copyText } from '@/lib/copy'
import type { ServiceDetail } from '@/types'

export function PassiveEndpointCard({ service, actions }: { service: ServiceDetail; actions?: ReactNode }) {
  const { t } = useTranslation()
  const connected = service.status === 1 && service.passive_connected

  return (
    <SectionCard
      title={t('services.passive.endpoint')}
      actions={(
        <Badge variant={connected ? 'success' : 'secondary'}>
          {service.status !== 1
            ? t('services.statusBadgeDisabled')
            : connected ? t('services.passive.connected') : t('services.passive.waiting')}
        </Badge>
      )}
    >
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('services.passive.endpointHint')}</p>
        <div className="flex gap-2">
          <Input aria-label={t('services.passive.endpoint')} value={service.passive_url || ''} readOnly className="min-w-0 font-mono text-xs" />
          <Button variant="outline" className="shrink-0 gap-1.5" disabled={!service.passive_url} onClick={() => copyText(service.passive_url)}>
            <Copy className="h-3.5 w-3.5" />
            {t('common.copy')}
          </Button>
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </div>
    </SectionCard>
  )
}
