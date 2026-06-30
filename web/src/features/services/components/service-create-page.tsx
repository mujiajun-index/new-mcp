import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createService, testConnection } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ArrowLeft, ArrowRight, Check, Loader2, Zap } from 'lucide-react'
import type { TransportType, AuthType, TestResult } from '@/types'

const transportOptions: { value: TransportType; labelKey: string; descKey: string }[] = [
  { value: 'streamable-http', labelKey: 'services.transports.streamable-http', descKey: 'services.transportDescs.streamable-http' },
  { value: 'sse', labelKey: 'services.transports.sse', descKey: 'services.transportDescs.sse' },
  { value: 'stdio', labelKey: 'services.transports.stdio', descKey: 'services.transportDescs.stdio' },
  { value: 'websocket', labelKey: 'services.transports.websocket', descKey: 'services.transportDescs.websocket' },
  { value: 'passive-ws', labelKey: 'services.transports.passive-ws', descKey: 'services.transportDescs.passive-ws' },
]

const authOptions: { value: AuthType; labelKey: string }[] = [
  { value: 'none', labelKey: 'services.authNone' },
  { value: 'api_key', labelKey: 'services.authApiKey' },
  { value: 'bearer', labelKey: 'services.authBearer' },
  { value: 'custom', labelKey: 'services.authCustom' },
]

export function ServiceCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [step, setStep] = useState(0)
  const [testResult, setTestResult] = useState<TestResult | null>(null)
  const [form, setForm] = useState({
    name: '',
    display_name: '',
    description: '',
    transport_type: 'streamable-http' as TransportType,
    config: {} as Record<string, unknown>,
    auth_type: 'none' as AuthType,
    auth_config: {} as Record<string, unknown>,
    tags: [] as string[],
    // Stdio fields
    command: '',
    args: '',
    env: '',
    // HTTP/SSE/WS fields
    url: '',
    // Auth fields
    api_key: '',
    bearer_token: '',
    custom_header_key: '',
    custom_header_value: '',
  })

  const steps = [
    t('services.create.stepNameBasic'),
    t('services.create.stepNameTransport'),
    t('services.create.stepNameAuth'),
    t('services.create.stepNameTest'),
  ]

  const createMutation = useMutation({
    mutationFn: () => {
      const config = buildConfig()
      const authConfig = buildAuthConfig()
      return createService({
        name: form.name,
        display_name: form.display_name || undefined,
        description: form.description || undefined,
        transport_type: form.transport_type,
        config,
        auth_type: form.auth_type === 'none' ? undefined : form.auth_type,
        auth_config: Object.keys(authConfig).length > 0 ? authConfig : undefined,
        tags: form.tags.length > 0 ? form.tags : undefined,
      })
    },
    onSuccess: () => {
      toast.success(t('services.serviceCreated'))
      navigate({ to: '/services' })
    },
  })

  const testMutation = useMutation({
    mutationFn: async () => {
      const config = buildConfig()
      const testRes = await testConnection({ transport_type: form.transport_type, config })
      setTestResult(testRes.data as TestResult)
    },
  })

  function buildConfig(): Record<string, unknown> {
    const headers: Record<string, string> = {}
    if (form.auth_type === 'api_key' && form.api_key) {
      headers['X-API-Key'] = form.api_key
    } else if (form.auth_type === 'bearer' && form.bearer_token) {
      headers['Authorization'] = `Bearer ${form.bearer_token}`
    } else if (form.auth_type === 'custom' && form.custom_header_key) {
      headers[form.custom_header_key] = form.custom_header_value
    }

    switch (form.transport_type) {
      case 'stdio':
        return {
          command: form.command,
          args: form.args ? form.args.split(/\s+/) : [],
          env: form.env ? JSON.parse(form.env) : {},
        }
      case 'sse':
      case 'streamable-http':
        return { url: form.url, headers }
      case 'websocket':
      case 'passive-ws':
        return { url: form.url }
      default:
        return {}
    }
  }

  function buildAuthConfig(): Record<string, unknown> {
    switch (form.auth_type) {
      case 'api_key': return { key: form.api_key }
      case 'bearer': return { token: form.bearer_token }
      default: return {}
    }
  }

  const canNext = () => {
    if (step === 0) return form.name.trim().length > 0
    if (step === 1) {
      if (form.transport_type === 'stdio') return form.command.trim().length > 0
      return form.url.trim().length > 0
    }
    return true
  }

  return (
    <div className="p-6 lg:p-8 max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/services' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-xl font-semibold">{t('services.registerNew')}</h1>
          <p className="text-sm text-muted-foreground">{t('services.create.step', { current: step + 1, total: steps.length })} — {steps[step]}</p>
        </div>
      </div>

      {/* Step indicators */}
      <div className="flex gap-2">
        {steps.map((_, i) => (
          <div
            key={i}
            className={`flex h-1.5 flex-1 rounded-full transition-colors ${
              i <= step ? 'bg-primary' : 'bg-muted'
            }`}
          />
        ))}
      </div>

      {/* Step 0: Basic info */}
      {step === 0 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <div className="space-y-2">
            <Label htmlFor="name">{t('services.create.serviceIdentifierRequired')}</Label>
            <Input id="name" placeholder="my-service" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <p className="text-xs text-muted-foreground">{t('services.create.serviceIdentifierTip')}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="display_name">{t('services.displayName')}</Label>
            <Input id="display_name" placeholder={t('services.placeholderMyService')} value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="description">{t('services.description')}</Label>
            <Input id="description" placeholder={t('services.placeholderDesc')} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
        </div>
      )}

      {/* Step 1: Transport config */}
      {step === 1 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <div className="space-y-2">
            <Label>{t('services.transportType')}</Label>
            <div className="grid gap-2 sm:grid-cols-2">
              {transportOptions.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setForm({ ...form, transport_type: opt.value })}
                  className={`rounded-lg border p-3 text-left transition-all ${
                    form.transport_type === opt.value
                      ? 'border-primary bg-primary/5 ring-1 ring-primary'
                      : 'hover:border-primary/30'
                  }`}
                >
                  <p className="text-sm font-medium">{t(opt.labelKey)}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{t(opt.descKey)}</p>
                </button>
              ))}
            </div>
          </div>

          {form.transport_type === 'stdio' ? (
            <>
              <div className="space-y-2">
                <Label htmlFor="command">{t('services.commandRequired')}</Label>
                <Input id="command" placeholder="npx" value={form.command} onChange={(e) => setForm({ ...form, command: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="args">{t('services.args')}</Label>
                <Input id="args" placeholder="-y @modelcontextprotocol/server-memory" value={form.args} onChange={(e) => setForm({ ...form, args: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="env">{t('services.envVars')}</Label>
                <Input id="env" placeholder='{"API_KEY": "xxx"}' value={form.env} onChange={(e) => setForm({ ...form, env: e.target.value })} />
              </div>
            </>
          ) : (
            <>
              <div className="space-y-2">
                <Label htmlFor="url">{t('services.serviceUrlRequired')}</Label>
                <Input id="url" placeholder="https://example.com/mcp" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} />
              </div>
            </>
          )}
        </div>
      )}

      {/* Step 2: Auth config */}
      {step === 2 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <div className="space-y-2">
            <Label>{t('services.authMethod')}</Label>
            <div className="flex flex-wrap gap-2">
              {authOptions.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setForm({ ...form, auth_type: opt.value })}
                  className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                    form.auth_type === opt.value
                      ? 'border-primary bg-primary/5'
                      : 'hover:border-primary/30'
                  }`}
                >
                  {t(opt.labelKey)}
                </button>
              ))}
            </div>
          </div>

          {form.auth_type === 'api_key' && (
            <div className="space-y-2">
              <Label>API Key</Label>
              <Input placeholder="sk-xxx" value={form.api_key} onChange={(e) => setForm({ ...form, api_key: e.target.value })} />
              <p className="text-xs text-muted-foreground">{t('services.create.authXApiKey')}</p>
            </div>
          )}
          {form.auth_type === 'bearer' && (
            <div className="space-y-2">
              <Label>Token</Label>
              <Input placeholder="eyJhbGci..." value={form.bearer_token} onChange={(e) => setForm({ ...form, bearer_token: e.target.value })} />
              <p className="text-xs text-muted-foreground">{t('services.create.authBearer')}</p>
            </div>
          )}
          {form.auth_type === 'none' && (
            <p className="text-sm text-muted-foreground">{t('services.create.noAuthHint')}</p>
          )}
          {form.auth_type === 'custom' && (
            <div className="space-y-2">
              <Label>{t('services.customHeaders')}</Label>
              <div className="flex gap-2">
                <Input placeholder={t('services.headerPlaceholder')} value={form.custom_header_key} onChange={(e) => setForm({ ...form, custom_header_key: e.target.value })} />
                <Input placeholder="Value" value={form.custom_header_value} onChange={(e) => setForm({ ...form, custom_header_value: e.target.value })} />
              </div>
              <p className="text-xs text-muted-foreground">{t('services.authHeaderTip', { header: `{ "Key": "Value" }` })}</p>
            </div>
          )}
        </div>
      )}

      {/* Step 3: Test & confirm */}
      {step === 3 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <p className="text-sm text-muted-foreground">{t('services.create.testHint')}</p>

          {testResult && (
            <div className={`rounded-lg p-4 ${testResult.connected ? 'bg-emerald-500/10' : 'bg-red-500/10'}`}>
              {testResult.connected ? (
                <div className="space-y-1 text-sm">
                  <p className="font-medium text-emerald-600 dark:text-emerald-400">{t('services.create.testSuccess')}</p>
                  <p className="text-muted-foreground">{t('services.create.testInfo', { count: testResult.tools_count, ms: testResult.latency_ms })}</p>
                </div>
              ) : (
                <p className="text-sm text-red-600 dark:text-red-400">{t('services.create.testFailed', { error: testResult.error || t('common.unknownError') })}</p>
              )}
            </div>
          )}

          <div className="flex gap-3">
            <Button
              variant="outline"
              className="gap-2"
              onClick={() => testMutation.mutate()}
              disabled={testMutation.isPending}
            >
              {testMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              <Zap className="h-4 w-4" />
              {t('services.create.testConnection')}
            </Button>
            <Button
              className={`gap-2 ${testResult?.connected ? 'bg-emerald-600 hover:bg-emerald-700' : ''}`}
              onClick={() => createMutation.mutate()}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              <Check className="h-4 w-4" />
              {t('services.create.createService')}
            </Button>
          </div>
        </div>
      )}

      {/* Navigation */}
      <div className="flex justify-between">
        <Button variant="outline" onClick={() => setStep(Math.max(0, step - 1))} disabled={step === 0}>
          <ArrowLeft className="h-4 w-4 mr-2" />
          {t('services.create.prevStep')}
        </Button>
        {step < steps.length - 1 && (
          <Button onClick={() => setStep(step + 1)} disabled={!canNext()}>
            {t('services.create.nextStep')}
            <ArrowRight className="h-4 w-4 ml-2" />
          </Button>
        )}
      </div>
    </div>
  )
}
