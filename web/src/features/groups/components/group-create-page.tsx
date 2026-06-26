import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createGroup, checkGroupName } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ArrowLeft, Loader2 } from 'lucide-react'

export function GroupCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [form, setForm] = useState({
    name: '',
    display_name: '',
    description: '',
    expose_mode: 'direct' as 'direct' | 'smart',
  })
  const [nameExists, setNameExists] = useState(false)
  const [checking, setChecking] = useState(false)

  const createMutation = useMutation({
    mutationFn: () => createGroup({
      name: form.name,
      display_name: form.display_name || undefined,
      description: form.description || undefined,
      expose_mode: form.expose_mode,
    }),
    onSuccess: (res) => {
      toast.success(t('groups.createSuccess'))
      const id = res.data?.id
      navigate({ to: '/groups/$id', params: { id: String(id) } })
    },
    onError: () => {
      toast.error(t('groups.createFailed'))
    },
  })

  const handleNameBlur = async () => {
    const name = form.name.trim()
    if (!name) return
    setChecking(true)
    try {
      const res = await checkGroupName(name)
      setNameExists(res.data?.exists ?? false)
    } catch {
      setNameExists(false)
    } finally {
      setChecking(false)
    }
  }

  const handleCreate = () => {
    if (nameExists) {
      toast.error(t('groups.identifierExists'))
      return
    }
    createMutation.mutate()
  }

  return (
    <div className="p-6 lg:p-8 max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/groups' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-xl font-semibold">{t('groups.createGroup')}</h1>
      </div>

      <div className="space-y-4 rounded-xl border bg-card p-6">
        <div className="space-y-2">
          <Label htmlFor="name">{t('groups.identifierRequired')}</Label>
          <Input
            id="name"
            placeholder="my-group"
            value={form.name}
            onChange={(e) => { setForm({ ...form, name: e.target.value }); setNameExists(false) }}
            onBlur={handleNameBlur}
          />
          {checking && <p className="text-xs text-muted-foreground">{t('groups.checking')}</p>}
          {nameExists && <p className="text-xs text-destructive">{t('groups.exists')}</p>}
          <p className="text-xs text-muted-foreground">{t('groups.identifierTip')}</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="display_name">{t('groups.displayName')}</Label>
          <Input id="display_name" placeholder={t('groups.displayNamePlaceholder')} value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="description">{t('groups.description')}</Label>
          <Input id="description" placeholder={t('groups.descriptionPlaceholder')} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label>{t('groups.exposeMode')}</Label>
          <div className="grid gap-3 sm:grid-cols-2">
            <button
              type="button"
              onClick={() => setForm({ ...form, expose_mode: 'direct' })}
              className={`rounded-lg border p-4 text-left transition-all ${
                form.expose_mode === 'direct' ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
              }`}
            >
              <p className="text-sm font-semibold">{t('groups.modeDirect')}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t('groups.modeDirectDesc')}</p>
            </button>
            <button
              type="button"
              onClick={() => setForm({ ...form, expose_mode: 'smart' })}
              className={`rounded-lg border p-4 text-left transition-all ${
                form.expose_mode === 'smart' ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
              }`}
            >
              <p className="text-sm font-semibold">{t('groups.modeSmart')}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t('groups.modeSmartDesc')}</p>
            </button>
          </div>
        </div>
      </div>

      <div className="flex justify-end">
        <Button
          className="gap-2"
          onClick={handleCreate}
          disabled={!form.name.trim() || nameExists || checking || createMutation.isPending}
        >
          {createMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
          {t('groups.createGroup')}
        </Button>
      </div>
    </div>
  )
}
