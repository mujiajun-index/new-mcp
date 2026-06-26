import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createCamera } from '../api'
import { getVisionConfigs } from '@/features/vision/api'
import type { VisionConfigListItem } from '@/features/vision/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from 'sonner'
import { ArrowLeft, Loader2, AlertCircle } from 'lucide-react'

export function CameraCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [form, setForm] = useState({
    name: '',
    description: '',
    vision_config_id: '' as string,
  })

  const { data: visionData, isLoading: visionLoading } = useQuery({
    queryKey: ['vision'],
    queryFn: () => getVisionConfigs(),
  })

  const visionConfigs: VisionConfigListItem[] = (visionData?.data || []).filter(
    (v: VisionConfigListItem) => v.status === 1
  )

  const canSubmit =
    form.name.trim() !== '' &&
    form.vision_config_id !== ''

  const createMutation = useMutation({
    mutationFn: () =>
      createCamera({
        name: form.name,
        description: form.description || undefined,
        vision_config_id: Number(form.vision_config_id),
      }),
    onSuccess: () => {
      toast.success(t('cameras.create.success'))
      navigate({ to: '/cameras' })
    },
  })

  return (
    <div className="p-6 lg:p-8 max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/cameras' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-xl font-semibold">{t('cameras.create.title')}</h1>
      </div>

      <div className="space-y-4 rounded-xl border bg-card p-6">
        <div className="space-y-2">
          <Label htmlFor="name">{t('cameras.create.nameRequired')}</Label>
          <Input
            id="name"
            placeholder={t('cameras.create.namePlaceholder')}
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="description">{t('cameras.create.description')}</Label>
          <Input
            id="description"
            placeholder={t('cameras.create.descriptionPlaceholder')}
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />
        </div>

        <div className="space-y-2">
          <Label>{t('cameras.create.visionConfigRequired')}</Label>
          {visionLoading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground py-2">
              <Loader2 className="h-4 w-4 animate-spin" />{t('common.loading')}
            </div>
          ) : visionConfigs.length === 0 ? (
            <div className="flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{t('cameras.create.noVisionConfig')}</span>
            </div>
          ) : (
            <Select
              value={form.vision_config_id}
              onValueChange={(value) => setForm({ ...form, vision_config_id: value })}
            >
              <SelectTrigger>
                <SelectValue placeholder={t('cameras.create.selectVisionConfig')} />
              </SelectTrigger>
              <SelectContent>
                {visionConfigs.map((vc) => (
                  <SelectItem key={vc.id} value={String(vc.id)}>
                    {vc.name} ({vc.model_name})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </div>
      </div>

      <div className="flex justify-end">
        <Button
          onClick={() => createMutation.mutate()}
          disabled={!canSubmit || createMutation.isPending || visionConfigs.length === 0}
        >
          {createMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {t('cameras.create.submit')}
        </Button>
      </div>
    </div>
  )
}
