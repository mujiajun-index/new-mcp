import { useEffect, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft,
  CheckCircle2,
  ImagePlus,
  List,
  Lock,
  Loader2,
  Power,
  PowerOff,
  RotateCcw,
  Save,
  Upload,
  Wrench,
} from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  ContentBlock,
  TestResultPanel,
} from '@/features/services/components/test-result-panel'
import type { ToolCallResult } from '@/types'
import {
  createImageGenConfig,
  getImageGenConfig,
  updateImageGenConfig,
  enableImageGenConfig,
  disableImageGenConfig,
  listImageGenModels,
  testImageGenConfig,
  imageEndpoint,
} from '../api'
import type { ConfigInput, EditRequestFormat } from '../api'

const initial = {
  name: '',
  description: '',
  provider: 'openai' as 'openai' | 'gemini',
  model_name: '',
  endpoint_url: '',
  api_key: '',
  operation: 'both' as 'generate' | 'edit' | 'both',
  edit_request_format: 'generations_json' as EditRequestFormat,
  timeout_seconds: 180,
}

export function ImageGenConfigPage({ id }: { id?: number }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [form, setForm] = useState(initial)
  const [tools, setTools] = useState({
    generate_image_desc: '',
    edit_image_desc: '',
  })
  const [defaults, setDefaults] = useState('{}')
  const [models, setModels] = useState<Array<{ id: string; name: string }>>([])
  const [modelsOpen, setModelsOpen] = useState(false)
  const [modelSearch, setModelSearch] = useState('image')
  const [testOperation, setTestOperation] = useState<'generate' | 'edit'>(
    'generate'
  )
  const [prompt, setPrompt] = useState('')
  const [references, setReferences] = useState('')
  const [mask, setMask] = useState('')
  const [uploading, setUploading] = useState(false)
  const [result, setResult] = useState<ToolCallResult | null>(null)
  const { data, isLoading, isError } = useQuery({
    queryKey: ['imagegen-config', id],
    queryFn: () => getImageGenConfig(id!),
    enabled: !!id,
  })
  const config = data?.data
  useEffect(() => {
    if (!config) return
    setForm({
      name: config.name,
      description: config.description,
      provider: config.provider,
      model_name: config.model_name,
      endpoint_url: config.endpoint_url,
      api_key: '',
      operation: config.operation,
      edit_request_format: config.edit_request_format || 'multipart',
      timeout_seconds: config.timeout_seconds,
    })
    setTools({
      generate_image_desc: config.generate_image_desc,
      edit_image_desc: config.edit_image_desc,
    })
    setDefaults(JSON.stringify(config.default_params, null, 2))
    setTestOperation(config.operation === 'edit' ? 'edit' : 'generate')
  }, [config])

  function payload(): ConfigInput {
    const parsed: unknown = JSON.parse(defaults)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))
      throw new Error(t('imagegen.invalidDefaults'))
    return {
      ...form,
      api_key: form.api_key.trim() || undefined,
      model_name: form.model_name || undefined,
      default_params: parsed as Record<string, unknown>,
    }
  }
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['imagegen-config', id] })
    queryClient.invalidateQueries({ queryKey: ['imagegen'] })
    queryClient.invalidateQueries({ queryKey: ['services'] })
  }
  const save = useMutation({
    mutationFn: () =>
      id
        ? updateImageGenConfig(id, payload())
        : createImageGenConfig(payload()),
    onSuccess: () => {
      toast.success(t('imagegen.saved'))
      if (id) invalidate()
      else navigate({ to: '/imagegen' })
    },
    onError: (err) => toast.error(err.message),
  })
  const toggle = useMutation({
    mutationFn: () =>
      config?.auto_register
        ? disableImageGenConfig(id!)
        : enableImageGenConfig(id!),
    onSuccess: invalidate,
    onError: (err) => toast.error(err.message),
  })
  const saveTool = useMutation({
    mutationFn: (data: ConfigInput) => updateImageGenConfig(id!, data),
    onSuccess: () => {
      toast.success(t('imagegen.toolSaved'))
      invalidate()
    },
    onError: (err) => toast.error(err.message),
  })
  const fetchModels = useMutation({
    mutationFn: () =>
      listImageGenModels({
        ...payload(),
        name: form.name || undefined,
        config_id: id,
      }),
    onSuccess: (res) => {
      setModels(res.data || [])
      setModelSearch('image')
      setModelsOpen(true)
    },
    onError: (err) => toast.error(err.message),
  })
  const test = useMutation({
    mutationFn: () =>
      testImageGenConfig({
        ...payload(),
        name: form.name || undefined,
        config_id: id,
        tool: testOperation === 'generate' ? 'generate_image' : 'edit_image',
        arguments: {
          prompt,
          ...(testOperation === 'edit'
            ? {
                images: references
                  .split('\n')
                  .map((s) => s.trim())
                  .filter(Boolean),
                ...(supportsMask && mask.trim() ? { mask: mask.trim() } : {}),
              }
            : {}),
        },
      }),
    onMutate: () => setResult(null),
    onSuccess: (res) => setResult(res.data),
    onError: (err) => toast.error(err.message),
  })
  async function upload(file: File, asMask: boolean) {
    setUploading(true)
    try {
      const body = new FormData()
      body.append('image', file)
      const res = await api.post('/vision/upload', body)
      const url: string = res.data.data.url
      if (asMask) setMask(url)
      else setReferences((old) => [old, url].filter(Boolean).join('\n'))
    } catch {
      toast.error(t('imagegen.uploadFailed'))
    } finally {
      setUploading(false)
    }
  }
  const canConnect =
    !!form.endpoint_url.trim() &&
    (!!form.api_key.trim() || !!config?.has_api_key)
  const supportsMask =
    form.provider === 'openai' && form.edit_request_format === 'multipart'
  const canSave =
    canConnect &&
    !!form.model_name.trim() &&
    !!form.name.trim() &&
    Number.isInteger(form.timeout_seconds) &&
    form.timeout_seconds >= 0 &&
    form.timeout_seconds <= 86400
  const endpoint = imageEndpoint(
    form.endpoint_url ||
      (form.provider === 'openai'
        ? 'https://api.openai.com'
        : 'https://generativelanguage.googleapis.com'),
    form.provider,
    form.model_name,
    testOperation,
    form.edit_request_format
  )
  if (id && isLoading)
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        <Loader2 className="mr-2 h-5 w-5 animate-spin" />
        {t('common.loading')}
      </div>
    )
  if (id && isError)
    return (
      <div className="p-8 text-destructive">{t('imagegen.loadFailed')}</div>
    )

  return (
    <div
      className={
        id
          ? 'space-y-6 p-6 lg:p-8'
          : 'mx-auto max-w-4xl space-y-6 p-4 sm:p-6 lg:p-8'
      }
    >
      <div className="flex flex-col items-start gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3 min-w-0">
          <Link to="/imagegen">
            <Button variant="ghost" size="icon" aria-label={t('common.cancel')}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div className="min-w-0">
            <h1 className="text-xl font-semibold">
              {id ? config?.name : t('imagegen.create.title')}
            </h1>
            <p className="mt-0.5 break-words text-sm text-muted-foreground">
              {id
                ? `${config?.provider} / ${config?.model_name}`
                : t('imagegen.configSubtitle')}
            </p>
          </div>
        </div>
        {id && (
          <Button
            variant={config?.auto_register ? 'outline' : 'default'}
            size="sm"
            disabled={toggle.isPending}
            onClick={() => toggle.mutate()}
            className="gap-1.5"
          >
            {toggle.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : config?.auto_register ? (
              <PowerOff className="h-3.5 w-3.5" />
            ) : (
              <Power className="h-3.5 w-3.5" />
            )}
            {t(config?.auto_register ? 'imagegen.disable' : 'imagegen.enable')}
          </Button>
        )}
      </div>
      {config?.auto_register && config.registered_service_id && (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-4 py-3">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
          <span className="text-sm text-emerald-700 dark:text-emerald-300">
            {t('imagegen.registered', { id: config.registered_service_id })}
          </span>
          <Link
            to="/services/$id"
            params={{ id: String(config.registered_service_id) }}
            className="ml-auto text-sm text-primary hover:underline"
          >
            {t('imagegen.openService')}
          </Link>
        </div>
      )}
      <div className="space-y-5 rounded-xl border bg-card p-5">
        <h2 className="text-sm font-semibold">{t('imagegen.basicConfig')}</h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="imagegen-name">{t('imagegen.name')} *</Label>
            <Input
              id="imagegen-name"
              maxLength={128}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="imagegen-description">
              {t('imagegen.description')}
            </Label>
            <Input
              id="imagegen-description"
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
            />
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="imagegen-provider">{t('imagegen.provider')}</Label>
            <Select
              value={form.provider}
              onValueChange={(v: 'openai' | 'gemini') => {
                setForm({
                  ...form,
                  provider: v,
                  edit_request_format:
                    v === 'gemini' ? 'multipart' : 'generations_json',
                })
                setMask('')
                setModels([])
              }}
            >
              <SelectTrigger id="imagegen-provider">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="openai">
                  {t('imagegen.providers.openai')}
                </SelectItem>
                <SelectItem value="gemini">Gemini</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="imagegen-model">{t('imagegen.model')} *</Label>
            <div className="flex gap-2">
              <Input
                id="imagegen-model"
                className="min-w-0 flex-1"
                placeholder="gpt-image-1 / qwen-image-plus"
                value={form.model_name}
                onChange={(e) =>
                  setForm({ ...form, model_name: e.target.value })
                }
              />
              <Button
                variant="outline"
                size="sm"
                className="shrink-0 gap-1.5"
                disabled={!canConnect || fetchModels.isPending}
                onClick={() => fetchModels.mutate()}
              >
                {fetchModels.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <List className="h-4 w-4" />
                )}
                {t('imagegen.fetchModels')}
              </Button>
            </div>
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="imagegen-endpoint">
            {t('imagegen.endpointUrl')} *
          </Label>
          <Input
            id="imagegen-endpoint"
            placeholder="https://gateway.example.com/v1"
            value={form.endpoint_url}
            onChange={(e) => {
              setForm({ ...form, endpoint_url: e.target.value })
              setModels([])
            }}
          />
          {form.endpoint_url.trim() && (
            <p className="break-all text-xs text-muted-foreground">
              {t('imagegen.endpointPreview')}:{' '}
              <code className="text-primary">{endpoint}</code>
            </p>
          )}
        </div>
        <div className="space-y-2">
          <Label htmlFor="imagegen-key">API Key{id ? '' : ' *'}</Label>
          <Input
            id="imagegen-key"
            type="password"
            autoComplete="new-password"
            value={form.api_key}
            placeholder={config?.has_api_key ? t('imagegen.keepKey') : 'sk-...'}
            onChange={(e) => setForm({ ...form, api_key: e.target.value })}
          />
        </div>
        <div className="space-y-2 max-w-xs">
          <Label htmlFor="imagegen-operation">{t('imagegen.operation')}</Label>
          <Select
            value={form.operation}
            onValueChange={(v: 'generate' | 'edit' | 'both') => {
              setForm({ ...form, operation: v })
              setTestOperation(v === 'edit' ? 'edit' : 'generate')
            }}
          >
            <SelectTrigger id="imagegen-operation">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(['both', 'generate', 'edit'] as const).map((v) => (
                <SelectItem key={v} value={v}>
                  {t(`imagegen.operations.${v}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {form.provider === 'openai' && form.operation !== 'generate' && (
          <div className="space-y-2">
            <Label htmlFor="imagegen-edit-format">
              {t('imagegen.editRequestFormat')}
            </Label>
            <Select
              value={form.edit_request_format}
              onValueChange={(v: EditRequestFormat) => {
                setForm({ ...form, edit_request_format: v })
                setMask('')
              }}
            >
              <SelectTrigger id="imagegen-edit-format">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(
                  [
                    'multipart',
                    'generations_json',
                    'generations_json_extra_body',
                  ] as const
                ).map((v) => (
                  <SelectItem key={v} value={v}>
                    {t(`imagegen.editRequestFormats.${v}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t('imagegen.editRequestFormatHint')}
            </p>
            {form.endpoint_url.trim() && (
              <p className="break-all text-xs text-muted-foreground">
                {t('imagegen.editEndpointPreview')}:{' '}
                <code className="text-primary">
                  {imageEndpoint(
                    form.endpoint_url,
                    form.provider,
                    form.model_name,
                    'edit',
                    form.edit_request_format
                  )}
                </code>
              </p>
            )}
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="imagegen-defaults">
            {t('imagegen.defaultParams')}
          </Label>
          <Textarea
            id="imagegen-defaults"
            className="min-h-28 font-mono text-xs"
            value={defaults}
            onChange={(e) => setDefaults(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {t('imagegen.defaultsHint')}
          </p>
        </div>
        <div className="space-y-2 max-w-xs">
          <Label htmlFor="imagegen-timeout">{t('imagegen.timeout')}</Label>
          <Input
            id="imagegen-timeout"
            type="number"
            min={0}
            max={86400}
            value={form.timeout_seconds}
            onChange={(e) =>
              setForm({ ...form, timeout_seconds: Number(e.target.value) })
            }
          />
          <p className="text-xs text-muted-foreground">
            {t('imagegen.timeoutHint')}
          </p>
        </div>
        <div className="flex justify-end gap-3 pt-2">
          {!id && (
            <Link to="/imagegen">
              <Button variant="ghost">{t('common.cancel')}</Button>
            </Link>
          )}
          <Button
            disabled={!canSave || save.isPending}
            onClick={() => save.mutate()}
            className="gap-2"
          >
            {save.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            {t('imagegen.saveConfig')}
          </Button>
        </div>
      </div>
      {id && config && (
        <section className="space-y-4 rounded-xl border bg-card p-5">
          <div className="flex items-center gap-2">
            <Wrench className="h-4 w-4 text-muted-foreground" />
            <h2 className="text-sm font-semibold">
              {t('imagegen.registeredTools')}
            </h2>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('imagegen.registeredToolsHint')}
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            {(['generate', 'edit'] as const)
              .filter(
                (v) => config.operation === 'both' || config.operation === v
              )
              .map((v) => {
                const field =
                  v === 'generate' ? 'generate_image_desc' : 'edit_image_desc'
                const defaultDescription =
                  v === 'generate'
                    ? config.generate_image_desc_default
                    : config.edit_image_desc_default
                const saving =
                  saveTool.isPending &&
                  saveTool.variables?.[field] !== undefined
                return (
                  <div key={v} className="space-y-3 rounded-lg border p-4">
                    <div className="flex items-center gap-2">
                      <span className="inline-flex items-center justify-center rounded-md bg-primary/10 p-1.5">
                        <Wrench className="h-3.5 w-3.5 text-primary" />
                      </span>
                      <span className="text-sm font-semibold">{v}_image</span>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor={`imagegen-${field}`} className="text-xs">
                        {t('imagegen.toolDescription')}
                      </Label>
                      <Textarea
                        id={`imagegen-${field}`}
                        className="min-h-[80px] text-xs"
                        value={tools[field]}
                        onChange={(e) =>
                          setTools({ ...tools, [field]: e.target.value })
                        }
                      />
                    </div>
                    <div className="flex items-center justify-end gap-2 pt-1">
                      {tools[field] !== defaultDescription && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 gap-1.5 text-xs"
                          disabled={saveTool.isPending}
                          title={t('vision.restoreDefaultHint')}
                          onClick={() => {
                            setTools({ ...tools, [field]: defaultDescription })
                            toast.info(t('vision.restoreDefaultHint'))
                          }}
                        >
                          <RotateCcw className="h-3 w-3" />
                          {t('imagegen.restoreDefault')}
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-7 gap-1.5 text-xs"
                        disabled={saveTool.isPending}
                        onClick={() =>
                          saveTool.mutate({ [field]: tools[field] })
                        }
                      >
                        {saving ? (
                          <Loader2 className="h-3 w-3 animate-spin" />
                        ) : (
                          <Save className="h-3 w-3" />
                        )}
                        {t('common.save')}
                      </Button>
                    </div>
                  </div>
                )
              })}
            {config.operation !== 'generate' && (
              <div className="space-y-3 rounded-lg border border-dashed bg-muted/30 p-4">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <span className="inline-flex items-center justify-center rounded-md bg-primary/10 p-1.5">
                      <Wrench className="h-3.5 w-3.5 text-primary" />
                    </span>
                    <span className="text-sm font-semibold">upload_image</span>
                  </div>
                  <Badge variant="secondary" className="gap-1 font-normal">
                    <Lock className="h-3 w-3" />
                    {t('vision.uploadImageBuiltinBadge')}
                  </Badge>
                </div>
                <div className="space-y-2">
                  <Label className="text-xs">
                    {t('imagegen.toolDescription')}
                  </Label>
                  <p className="rounded-md border bg-background px-2.5 py-2 text-xs leading-relaxed text-muted-foreground">
                    {config.upload_image_desc}
                  </p>
                </div>
              </div>
            )}
          </div>
        </section>
      )}
      <section className="space-y-4 rounded-xl border bg-card p-5">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            <ImagePlus className="h-4 w-4 text-muted-foreground" />
            {t('imagegen.testTitle')}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('imagegen.testHint')}
          </p>
        </div>
        {form.operation === 'both' && (
          <Select
            value={testOperation}
            onValueChange={(v: 'generate' | 'edit') => setTestOperation(v)}
          >
            <SelectTrigger className="w-52">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="generate">
                {t('imagegen.operations.generate')}
              </SelectItem>
              <SelectItem value="edit">
                {t('imagegen.operations.edit')}
              </SelectItem>
            </SelectContent>
          </Select>
        )}
        <div className="space-y-2">
          <Label htmlFor="imagegen-prompt">{t('imagegen.prompt')}</Label>
          <Textarea
            id="imagegen-prompt"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={t('imagegen.promptPlaceholder')}
          />
        </div>
        {testOperation === 'edit' && (
          <>
            <div className="space-y-2">
              <Label htmlFor="imagegen-references">
                {t('imagegen.references')}
              </Label>
              <Textarea
                id="imagegen-references"
                value={references}
                onChange={(e) => setReferences(e.target.value)}
                placeholder="https://..."
              />
              <label className="inline-flex cursor-pointer items-center gap-2 text-xs text-primary">
                <Upload className="h-3.5 w-3.5" />
                {uploading
                  ? t('common.loading')
                  : t('imagegen.uploadReference')}
                <input
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  className="sr-only"
                  disabled={uploading}
                  onChange={(e) => {
                    const file = e.target.files?.[0]
                    if (file) upload(file, false)
                    e.target.value = ''
                  }}
                />
              </label>
            </div>
            {supportsMask && (
              <div className="space-y-2">
                <Label htmlFor="imagegen-mask">{t('imagegen.mask')}</Label>
                <Input
                  id="imagegen-mask"
                  value={mask}
                  onChange={(e) => setMask(e.target.value)}
                />
                <label className="inline-flex cursor-pointer items-center gap-2 text-xs text-primary">
                  <Upload className="h-3.5 w-3.5" />
                  {t('imagegen.uploadMask')}
                  <input
                    type="file"
                    accept="image/png"
                    className="sr-only"
                    disabled={uploading}
                    onChange={(e) => {
                      const file = e.target.files?.[0]
                      if (file) upload(file, true)
                      e.target.value = ''
                    }}
                  />
                </label>
                <p className="text-xs text-muted-foreground">
                  {t('imagegen.maskHint')}
                </p>
              </div>
            )}
          </>
        )}
        <Button
          variant="outline"
          className="gap-2"
          disabled={
            !canConnect ||
            !form.model_name.trim() ||
            !prompt.trim() ||
            uploading ||
            test.isPending ||
            (testOperation === 'edit' && !references.trim())
          }
          onClick={() => test.mutate()}
        >
          {test.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <ImagePlus className="h-4 w-4" />
          )}
          {t('imagegen.testGenerate')}
        </Button>
        {result && (
          <TestResultPanel result={result}>
            {result.result?.content?.map((block, i) => (
              <ContentBlock key={i} block={block} index={i} />
            ))}
          </TestResultPanel>
        )}
      </section>
      <Dialog open={modelsOpen} onOpenChange={setModelsOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('imagegen.selectModel')}</DialogTitle>
            <DialogDescription>{t('imagegen.modelsHint')}</DialogDescription>
          </DialogHeader>
          <Input
            value={modelSearch}
            onChange={(e) => setModelSearch(e.target.value)}
            placeholder={t('imagegen.searchModelPlaceholder')}
          />
          <div className="max-h-72 overflow-y-auto">
            {models
              .filter((m) =>
                `${m.id} ${m.name}`
                  .toLowerCase()
                  .includes(modelSearch.toLowerCase())
              )
              .map((m) => (
                <button
                  key={m.id}
                  type="button"
                  className="block w-full rounded-md px-3 py-2 text-left text-sm hover:bg-muted"
                  onClick={() => {
                    setForm({ ...form, model_name: m.id })
                    setModelsOpen(false)
                  }}
                >
                  <span className="font-medium">{m.id}</span>
                  {m.name !== m.id && (
                    <span className="ml-2 text-xs text-muted-foreground">
                      {m.name}
                    </span>
                  )}
                </button>
              ))}
            {!models.length && (
              <p className="py-6 text-center text-sm text-muted-foreground">
                {t('imagegen.noModelsFound')}
              </p>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
