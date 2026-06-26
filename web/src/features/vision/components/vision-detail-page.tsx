import { useState, useEffect } from 'react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  getVisionConfig,
  updateVisionConfig,
  enableVisionConfig,
  disableVisionConfig,
  listVisionModels,
} from '../api'
import type { UpdateVisionConfigReq, ModelInfo } from '../api'
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
import {
  ArrowLeft,
  Loader2,
  Save,
  Power,
  PowerOff,
  Wrench,
  CheckCircle2,
  List,
} from 'lucide-react'
import { toast } from 'sonner'

const providerOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
]

const providerSuffixes: Record<string, string> = {
  openai: '/v1/chat/completions',
  anthropic: '/v1/messages',
  gemini: '/v1beta/models',
}

export function VisionDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['vision', id],
    queryFn: () => getVisionConfig(Number(id)),
  })

  const config = data?.data

  // Config form state
  const [form, setForm] = useState({
    name: '',
    description: '',
    provider: 'openai',
    endpoint_url: '',
    api_key: '',
    model_name: '',
    system_prompt: '',
    max_tokens: 4096,
  })

  const [modelList, setModelList] = useState<ModelInfo[]>([])
  const [modelListLoading, setModelListLoading] = useState(false)
  const [modelModalOpen, setModelModalOpen] = useState(false)
  const [modelSearch, setModelSearch] = useState('')

  // Tool form state
  const [tools, setTools] = useState({
    analyze_image_name: '',
    analyze_image_desc: '',
    describe_scene_name: '',
    describe_scene_desc: '',
  })

  useEffect(() => {
    if (config) {
      setForm({
        name: config.name || '',
        description: config.description || '',
        provider: config.provider || 'openai',
        endpoint_url: config.endpoint_url || '',
        api_key: '',
        model_name: config.model_name || '',
        system_prompt: config.system_prompt || '',
        max_tokens: config.max_tokens || 4096,
      })
      setTools({
        analyze_image_name: config.analyze_image_name || 'analyze_image',
        analyze_image_desc: config.analyze_image_desc || '',
        describe_scene_name: config.describe_scene_name || 'describe_scene',
        describe_scene_desc: config.describe_scene_desc || '',
      })
    }
  }, [config])

  const endpointPreview = form.endpoint_url.trim()
    ? form.endpoint_url.trim().replace(/\/+$/, '') + (providerSuffixes[form.provider] || '')
    : ''

  const updateMutation = useMutation({
    mutationFn: (data: UpdateVisionConfigReq) => updateVisionConfig(Number(id), data),
    onSuccess: () => {
      toast.success(t('vision.updateSuccess'))
      queryClient.invalidateQueries({ queryKey: ['vision', id] })
      queryClient.invalidateQueries({ queryKey: ['vision'] })
    },
    onError: () => {
      toast.error(t('vision.updateFailed'))
    },
  })

  const enableMutation = useMutation({
    mutationFn: () => enableVisionConfig(Number(id)),
    onSuccess: () => {
      toast.success(t('vision.enableSuccess'))
      queryClient.invalidateQueries({ queryKey: ['vision', id] })
      queryClient.invalidateQueries({ queryKey: ['vision'] })
    },
    onError: () => {
      toast.error(t('vision.enableFailed'))
    },
  })

  const disableMutation = useMutation({
    mutationFn: () => disableVisionConfig(Number(id)),
    onSuccess: () => {
      toast.success(t('vision.disableSuccess'))
      queryClient.invalidateQueries({ queryKey: ['vision', id] })
      queryClient.invalidateQueries({ queryKey: ['vision'] })
    },
    onError: () => {
      toast.error(t('vision.disableFailed'))
    },
  })

  const toolMutation = useMutation({
    mutationFn: (data: UpdateVisionConfigReq) => updateVisionConfig(Number(id), data),
    onSuccess: () => {
      toast.success(t('vision.toolUpdateSuccess'))
      queryClient.invalidateQueries({ queryKey: ['vision', id] })
    },
    onError: () => {
      toast.error(t('vision.toolUpdateFailed'))
    },
  })

  const handleFetchModels = async () => {
    setModelListLoading(true)
    setModelSearch('')
    try {
      const res = await listVisionModels({
        provider: form.provider,
        endpoint_url: form.endpoint_url,
        api_key: form.api_key || undefined,
        config_id: Number(id),
      })
      setModelList(res.data || [])
      setModelModalOpen(true)
      if (!res.data?.length) {
        toast.info(t('vision.noModelsFound'))
      }
    } catch {
      toast.error(t('vision.fetchModelsFailed'))
    } finally {
      setModelListLoading(false)
    }
  }

  const filteredModels = modelList.filter((m) => {
    if (!modelSearch.trim()) return true
    const q = modelSearch.toLowerCase()
    return (m.id + m.name).toLowerCase().includes(q)
  })

  const handleSaveConfig = () => {
    const payload: UpdateVisionConfigReq = {
      name: form.name,
      description: form.description,
      provider: form.provider,
      model_name: form.model_name,
      endpoint_url: form.endpoint_url,
      system_prompt: form.system_prompt,
      max_tokens: form.max_tokens,
    }
    // Only send api_key if user typed something
    if (form.api_key.trim()) {
      payload.api_key = form.api_key
    }
    updateMutation.mutate(payload)
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        <Loader2 className="h-5 w-5 animate-spin mr-2" />
        {t('common.loading')}
      </div>
    )
  }

  if (!config) {
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        {t('vision.notFound')}
      </div>
    )
  }

  const isEnabled = config.auto_register

  return (
    <div className="p-6 lg:p-8 space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => navigate({ to: '/vision' })}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{config.name}</h1>
            <p className="mt-0.5 text-sm text-muted-foreground">
              {config.provider} / {config.model_name}
            </p>
          </div>
        </div>
        <Button
          variant={isEnabled ? 'outline' : 'default'}
          size="sm"
          className="gap-1.5"
          disabled={enableMutation.isPending || disableMutation.isPending}
          onClick={() => {
            if (isEnabled) {
              disableMutation.mutate()
            } else {
              enableMutation.mutate()
            }
          }}
        >
          {enableMutation.isPending || disableMutation.isPending ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : isEnabled ? (
            <PowerOff className="h-3.5 w-3.5" />
          ) : (
            <Power className="h-3.5 w-3.5" />
          )}
          {isEnabled ? t('vision.disable') : t('vision.enable')}
        </Button>
      </div>

      {/* Status info */}
      {config.auto_register && config.registered_service_id && (
        <div className="flex items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-4 py-3">
          <CheckCircle2 className="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
          <span className="text-sm text-emerald-700 dark:text-emerald-300">
            {t('vision.registered', { id: config.registered_service_id })}
          </span>
        </div>
      )}

      {/* Config editing section */}
      <div className="rounded-xl border bg-card p-5 space-y-5">
        <h2 className="text-sm font-semibold">{t('vision.basicConfig')}</h2>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="name">{t('vision.nameRequired')}</Label>
            <Input
              id="name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="description">{t('vision.description')}</Label>
            <Input
              id="description"
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
            />
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Provider</Label>
            <Select
              value={form.provider}
              onValueChange={(v) => {
                setForm({ ...form, provider: v })
                setModelList([])
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {providerOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="model_name">{t('vision.modelNameRequired')}</Label>
            <div className="flex gap-2">
              <Input
                id="model_name"
                value={form.model_name}
                onChange={(e) => setForm({ ...form, model_name: e.target.value })}
                className="flex-1"
              />
              <Button
                variant="outline"
                size="sm"
                className="gap-1.5 shrink-0"
                disabled={
                  modelListLoading ||
                  !form.endpoint_url.trim()
                }
                onClick={modelList.length > 0 ? () => { setModelSearch(''); setModelModalOpen(true) } : handleFetchModels}
              >
                {modelListLoading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <List className="h-4 w-4" />
                )}
                {t('vision.fetchModels')}
              </Button>
            </div>
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="endpoint_url">Endpoint URL *</Label>
          <Input
            id="endpoint_url"
            value={form.endpoint_url}
            onChange={(e) => {
              setForm({ ...form, endpoint_url: e.target.value })
              setModelList([])
            }}
          />
          {endpointPreview && (
            <p className="text-xs text-muted-foreground">
              {t('vision.endpointPreview')}：<code className="text-primary">{endpointPreview}</code>
            </p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="api_key">API Key</Label>
          <Input
            id="api_key"
            type="password"
            placeholder={t('vision.placeholderKeepUnchanged')}
            value={form.api_key}
            onChange={(e) => setForm({ ...form, api_key: e.target.value })}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="system_prompt">{t('vision.systemPrompt')}</Label>
          <textarea
            id="system_prompt"
            aria-label={t('vision.systemPrompt')}
            rows={4}
            value={form.system_prompt}
            onChange={(e) => setForm({ ...form, system_prompt: e.target.value })}
            className="flex w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 resize-y"
          />
        </div>

        <div className="space-y-2 max-w-xs">
          <Label htmlFor="max_tokens">Max Tokens</Label>
          <Input
            id="max_tokens"
            type="number"
            min={1}
            max={128000}
            value={form.max_tokens}
            onChange={(e) =>
              setForm({ ...form, max_tokens: parseInt(e.target.value, 10) || 0 })
            }
          />
        </div>

        <div className="flex justify-end pt-2">
          <Button
            className="gap-2"
            disabled={updateMutation.isPending}
            onClick={handleSaveConfig}
          >
            {updateMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            {t('vision.saveConfig')}
          </Button>
        </div>
      </div>

      {/* Tool display section */}
      <div className="rounded-xl border bg-card p-5 space-y-4">
        <div className="flex items-center gap-2">
          <Wrench className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">{t('vision.registeredTools')}</h2>
        </div>
        <p className="text-xs text-muted-foreground">
          {t('vision.registeredToolsHint')}
        </p>

        <div className="grid gap-4 sm:grid-cols-2">
          {/* analyze_image tool card */}
          <div className="rounded-lg border p-4 space-y-3">
            <div className="flex items-center gap-2">
              <span className="inline-flex items-center justify-center rounded-md bg-primary/10 p-1.5">
                <Wrench className="h-3.5 w-3.5 text-primary" />
              </span>
              <span className="text-sm font-semibold">analyze_image</span>
            </div>

            <div className="space-y-2">
              <Label className="text-xs">{t('vision.toolName')}</Label>
              <Input
                value={tools.analyze_image_name}
                onChange={(e) =>
                  setTools({ ...tools, analyze_image_name: e.target.value })
                }
                className="text-xs h-8"
              />
            </div>

            <div className="space-y-2">
              <Label className="text-xs">{t('vision.toolDesc')}</Label>
              <Input
                value={tools.analyze_image_desc}
                onChange={(e) =>
                  setTools({ ...tools, analyze_image_desc: e.target.value })
                }
                className="text-xs h-8"
                placeholder={t('vision.toolDescPlaceholderAnalyze')}
              />
            </div>

            <div className="flex justify-end pt-1">
              <Button
                size="sm"
                variant="outline"
                className="gap-1.5 text-xs h-7"
                disabled={toolMutation.isPending}
                onClick={() =>
                  toolMutation.mutate({
                    analyze_image_name: tools.analyze_image_name,
                    analyze_image_desc: tools.analyze_image_desc,
                  })
                }
              >
                {toolMutation.isPending ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <Save className="h-3 w-3" />
                )}
                {t('vision.save')}
              </Button>
            </div>
          </div>

          {/* describe_scene tool card */}
          <div className="rounded-lg border p-4 space-y-3">
            <div className="flex items-center gap-2">
              <span className="inline-flex items-center justify-center rounded-md bg-primary/10 p-1.5">
                <Wrench className="h-3.5 w-3.5 text-primary" />
              </span>
              <span className="text-sm font-semibold">describe_scene</span>
            </div>

            <div className="space-y-2">
              <Label className="text-xs">{t('vision.toolName')}</Label>
              <Input
                value={tools.describe_scene_name}
                onChange={(e) =>
                  setTools({ ...tools, describe_scene_name: e.target.value })
                }
                className="text-xs h-8"
              />
            </div>

            <div className="space-y-2">
              <Label className="text-xs">{t('vision.toolDesc')}</Label>
              <Input
                value={tools.describe_scene_desc}
                onChange={(e) =>
                  setTools({ ...tools, describe_scene_desc: e.target.value })
                }
                className="text-xs h-8"
                placeholder={t('vision.toolDescPlaceholderScene')}
              />
            </div>

            <div className="flex justify-end pt-1">
              <Button
                size="sm"
                variant="outline"
                className="gap-1.5 text-xs h-7"
                disabled={toolMutation.isPending}
                onClick={() =>
                  toolMutation.mutate({
                    describe_scene_name: tools.describe_scene_name,
                    describe_scene_desc: tools.describe_scene_desc,
                  })
                }
              >
                {toolMutation.isPending ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <Save className="h-3 w-3" />
                )}
                {t('vision.save')}
              </Button>
            </div>
          </div>
        </div>
      </div>

      {/* Model List Modal */}
      {modelModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setModelModalOpen(false)}>
          <div className="w-full max-w-md rounded-xl border bg-card p-0 shadow-xl" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between border-b px-4 py-3">
              <h3 className="text-sm font-semibold">{t('vision.selectModel')}</h3>
              <button className="text-muted-foreground hover:text-foreground text-lg leading-none" onClick={() => setModelModalOpen(false)}>&times;</button>
            </div>
            <div className="px-4 pt-3">
              <Input
                placeholder={t('vision.searchModelPlaceholder')}
                value={modelSearch}
                onChange={(e) => setModelSearch(e.target.value)}
                autoFocus
              />
            </div>
            <div className="max-h-72 overflow-y-auto p-2">
              {filteredModels.length === 0 ? (
                <p className="py-8 text-center text-xs text-muted-foreground">
                  {modelList.length === 0 ? t('vision.noModels') : t('vision.noMatches')}
                </p>
              ) : (
                filteredModels.map((m) => (
                  <button
                    key={m.id}
                    type="button"
                    className={`w-full text-left px-3 py-2 text-sm rounded-md hover:bg-muted transition-colors ${
                      form.model_name === m.id ? 'bg-primary/10 text-primary font-medium' : ''
                    }`}
                    onClick={() => {
                      setForm({ ...form, model_name: m.id })
                      setModelModalOpen(false)
                    }}
                  >
                    <span className="font-medium">{m.id}</span>
                    {m.name && m.name !== m.id && (
                      <span className="ml-2 text-xs text-muted-foreground">{m.name}</span>
                    )}
                  </button>
                ))
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
