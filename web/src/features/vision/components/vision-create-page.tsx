import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createVisionConfig, testVisionConfig, listVisionModels } from '../api'
import type { ModelInfo } from '../api'
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
import { ArrowLeft, Check, Loader2, Zap, List } from 'lucide-react'
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

const providerPlaceholders: Record<string, string> = {
  openai: 'https://api.openai.com/v1',
  anthropic: 'https://api.anthropic.com/v1',
  gemini: 'https://generativelanguage.googleapis.com',
}

export function VisionCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
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

  const endpointPreview = form.endpoint_url.trim()
    ? form.endpoint_url.trim().replace(/\/+$/, '') + (providerSuffixes[form.provider] || '')
    : providerPlaceholders[form.provider] + (providerSuffixes[form.provider] || '')

  const createMutation = useMutation({
    mutationFn: () =>
      createVisionConfig({
        name: form.name,
        description: form.description || undefined,
        provider: form.provider,
        model_name: form.model_name,
        endpoint_url: form.endpoint_url,
        api_key: form.api_key,
        system_prompt: form.system_prompt || undefined,
        max_tokens: form.max_tokens || undefined,
      }),
    onSuccess: () => {
      toast.success(t('vision.create.success'))
      navigate({ to: '/vision' })
    },
    onError: () => {
      toast.error(t('vision.create.failed'))
    },
  })

  const testMutation = useMutation({
    mutationFn: () =>
      testVisionConfig({
        provider: form.provider,
        endpoint_url: form.endpoint_url,
        api_key: form.api_key,
        model_name: form.model_name,
      }),
    onSuccess: (res) => {
      if (res.data?.success) {
        toast.success(t('vision.create.testSuccess'))
      } else {
        toast.error(t('vision.create.testFailed', { error: res.data?.error || t('common.unknownError') }))
      }
    },
    onError: () => {
      toast.error(t('vision.create.testRequestFailed'))
    },
  })

  const handleFetchModels = async () => {
    setModelListLoading(true)
    setModelSearch('')
    try {
      const res = await listVisionModels({
        provider: form.provider,
        endpoint_url: form.endpoint_url,
        api_key: form.api_key,
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

  const canSubmit =
    form.name.trim().length > 0 &&
    form.endpoint_url.trim().length > 0 &&
    form.api_key.trim().length > 0 &&
    form.model_name.trim().length > 0

  return (
    <div className="p-6 lg:p-8 max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/vision' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-xl font-semibold">{t('vision.create.title')}</h1>
          <p className="text-sm text-muted-foreground">
            {t('vision.create.subtitle')}
          </p>
        </div>
      </div>

      <div className="space-y-5 rounded-xl border bg-card p-6">
        {/* Name */}
        <div className="space-y-2">
          <Label htmlFor="name">{t('vision.nameRequired')}</Label>
          <Input
            id="name"
            placeholder={t('vision.placeholderName')}
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <p className="text-xs text-muted-foreground">{t('vision.create.uniqueTip')}</p>
        </div>

        {/* Description */}
        <div className="space-y-2">
          <Label htmlFor="description">{t('vision.description')}</Label>
          <Input
            id="description"
            placeholder={t('vision.placeholderDesc')}
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />
        </div>

        {/* Provider */}
        <div className="space-y-2">
          <Label>Provider *</Label>
          <Select
            value={form.provider}
            onValueChange={(v) => {
              setForm({ ...form, provider: v })
              setModelList([])
            }}
          >
            <SelectTrigger>
              <SelectValue placeholder={t('vision.placeholderProvider')} />
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

        {/* Endpoint URL */}
        <div className="space-y-2">
          <Label htmlFor="endpoint_url">Endpoint URL *</Label>
          <Input
            id="endpoint_url"
            placeholder={providerPlaceholders[form.provider]}
            value={form.endpoint_url}
            onChange={(e) => {
              setForm({ ...form, endpoint_url: e.target.value })
              setModelList([])
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('vision.endpointPreview')}：<code className="text-primary">{endpointPreview}</code>
          </p>
        </div>

        {/* API Key */}
        <div className="space-y-2">
          <Label htmlFor="api_key">API Key *</Label>
          <Input
            id="api_key"
            type="password"
            placeholder="sk-..."
            value={form.api_key}
            onChange={(e) => setForm({ ...form, api_key: e.target.value })}
          />
        </div>

        {/* Model Name */}
        <div className="space-y-2">
          <Label htmlFor="model_name">{t('vision.modelNameRequired')}</Label>
          <div className="flex gap-2">
            <Input
              id="model_name"
              placeholder="gpt-4o"
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
                !form.endpoint_url.trim() ||
                !form.api_key.trim()
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

        {/* System Prompt */}
        <div className="space-y-2">
          <Label htmlFor="system_prompt">{t('vision.systemPrompt')}</Label>
          <textarea
            id="system_prompt"
            rows={4}
            placeholder={t('vision.placeholderPrompt')}
            value={form.system_prompt}
            onChange={(e) => setForm({ ...form, system_prompt: e.target.value })}
            className="flex w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 resize-y"
          />
        </div>

        {/* Max Tokens */}
        <div className="space-y-2">
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
      </div>

      {/* Actions */}
      <div className="flex items-center gap-3">
        <Button
          variant="outline"
          className="gap-2"
          disabled={
            testMutation.isPending ||
            !form.endpoint_url.trim() ||
            !form.api_key.trim() ||
            !form.model_name.trim()
          }
          onClick={() => testMutation.mutate()}
        >
          {testMutation.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Zap className="h-4 w-4" />
          )}
          {t('vision.create.testConnection')}
        </Button>
        <Button
          className="gap-2"
          disabled={!canSubmit || createMutation.isPending}
          onClick={() => createMutation.mutate()}
        >
          {createMutation.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Check className="h-4 w-4" />
          )}
          {t('common.save')}
        </Button>
        <Button variant="ghost" onClick={() => navigate({ to: '/vision' })}>
          {t('common.cancel')}
        </Button>
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
