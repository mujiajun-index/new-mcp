import { useMemo, useState, type ElementType, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Mail, Link2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { EmailBindDialog } from './email-bind-dialog'

// A single bindable account row. Adding a new account type (e.g. GitHub, OIDC)
// later means appending an entry to the `bindings` array below — the grid and
// row rendering are generic.
interface BindingItem {
  id: string
  label: string
  icon: ElementType
  value?: string
  isBound: boolean
  isEnabled: boolean
  onBind: () => void
}

interface AccountBindingsCardProps {
  // Minimal profile shape this card needs; the full profile object is compatible.
  profile?: { email?: string; [key: string]: unknown }
  onUpdate: () => void
}

export function AccountBindingsCard({ profile, onUpdate }: AccountBindingsCardProps) {
  const { t } = useTranslation()
  const { config } = useSystemConfigStore()
  const requireEmailVerify = !!config.smtpConfigured
  const [emailDialogOpen, setEmailDialogOpen] = useState(false)

  const bindings = useMemo<BindingItem[]>(() => {
    return [
      {
        id: 'email',
        label: t('settings.bindingEmail'),
        icon: Mail,
        value: profile?.email,
        isBound: !!profile?.email,
        isEnabled: true,
        onBind: () => setEmailDialogOpen(true),
      },
      // Future account types (GitHub, Google, OIDC, ...) slot in here.
    ].filter((b) => b.isEnabled)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profile, t])

  return (
    <>
      <div className="rounded-xl border bg-card p-5 space-y-4">
        <div className="flex items-center gap-2">
          <Link2 className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">{t('settings.accountBindings')}</h2>
        </div>
        <p className="-mt-2 text-xs text-muted-foreground">{t('settings.accountBindingsDesc')}</p>

        <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 sm:gap-3">
          {bindings.map((binding) => (
            <BindingRow key={binding.id} binding={binding} />
          ))}
        </div>
      </div>

      <EmailBindDialog
        open={emailDialogOpen}
        onOpenChange={setEmailDialogOpen}
        currentEmail={profile?.email}
        requireVerify={requireEmailVerify}
        onSuccess={onUpdate}
      />
    </>
  )
}

function BindingRow({ binding }: { binding: BindingItem }): ReactNode {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-between gap-2.5 rounded-lg border p-2.5 sm:gap-3 sm:p-3">
      <div className="flex min-w-0 items-center gap-2.5 sm:gap-3">
        <div className="bg-muted shrink-0 rounded-md p-1.5 sm:p-2">
          <binding.icon className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-1.5">
            <p className="text-sm font-medium">{binding.label}</p>
            {binding.isBound && <Badge variant="success">{t('settings.bound')}</Badge>}
          </div>
          <p className="truncate text-xs text-muted-foreground">{binding.value || t('settings.unbound')}</p>
        </div>
      </div>
      <Button
        variant="outline"
        size="sm"
        className="h-7 shrink-0 px-2.5 text-xs"
        onClick={binding.onBind}
      >
        {binding.isBound ? t('settings.change') : t('settings.bind')}
      </Button>
    </div>
  )
}
