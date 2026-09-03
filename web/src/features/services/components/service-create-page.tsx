import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { createService, testConnection, prepareStdio } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'sonner'
import { ArrowLeft, ArrowRight, Check, Loader2, Zap, RefreshCw } from 'lucide-react'
import type { TransportType, AuthType, TestResult, PrepareStdioResult } from '@/types'
import { useAuthStore } from '@/stores/auth-store'
import { isAdminRole } from '@/lib/roles'
import { maskSecret } from '../lib/mask-secret'

type CommandChoice = 'npx' | 'uvx' | 'custom'
type InstallStatus = 'idle' | 'ready' | 'failed'
// 秘钥模式:single=单秘钥;random/polling=多秘钥(随机/轮询)
type SecretMode = 'single' | 'random' | 'polling'

// 服务标识规则(与后端 validateServiceName 同口径):英文字母开头,
// 仅含字母/数字/下划线/连字符,最长 64
const SERVICE_NAME_RE = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/

// 单把秘钥的连通性测试结果(多秘钥创建时逐把测试)
interface KeyTestResult {
  index: number
  masked: string
  connected: boolean
  error?: string
  tools_count: number
  latency_ms: number
}

interface RegistryOption { key: string; label: string; url: string }

const NPM_REGISTRIES: RegistryOption[] = [
  { key: 'npmmirror', label: '淘宝', url: 'https://registry.npmmirror.com' },
]
const UV_REGISTRIES: RegistryOption[] = [
  { key: 'tsinghua', label: '清华', url: 'https://pypi.tuna.tsinghua.edu.cn/simple' },
  { key: 'aliyun', label: '阿里云', url: 'http://mirrors.aliyun.com/pypi/simple/' },
  { key: 'ustc', label: 'USTC', url: 'https://mirrors.ustc.edu.cn/pypi/simple/' },
  { key: 'huaweicloud', label: '华为云', url: 'https://repo.huaweicloud.com/repository/pypi/simple/' },
  { key: 'tencent', label: '腾讯云', url: 'https://mirrors.cloud.tencent.com/pypi/simple/' },
]

// parseArgs: one argument per line (no shell splitting, paths with spaces are safe).
function parseArgs(text: string): string[] {
  return text.split('\n').map((s) => s.trim()).filter(Boolean)
}

// parseEnv: KEY=value per line; blank lines and # comments ignored; never throws.
function parseEnv(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) continue
    const i = line.indexOf('=')
    if (i <= 0) continue
    out[line.slice(0, i).trim()] = line.slice(i + 1)
  }
  return out
}

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
  // stdio 服务在服务器本地执行命令行子进程，属特权操作：仅管理员可创建，普通用户隐藏该传输选项。
  const { auth } = useAuthStore()
  const isAdmin = isAdminRole(auth.user?.role)
  const visibleTransportOptions = isAdmin
    ? transportOptions
    : transportOptions.filter((opt) => opt.value !== 'stdio')
  const [step, setStep] = useState(0)
  const [testResult, setTestResult] = useState<TestResult | null>(null)
  const [keyTestResults, setKeyTestResults] = useState<KeyTestResult[]>([])
  const [form, setForm] = useState({
    name: '',
    display_name: '',
    description: '',
    transport_type: 'streamable-http' as TransportType,
    config: {} as Record<string, unknown>,
    auth_type: 'none' as AuthType,
    auth_config: {} as Record<string, unknown>,
    tags: [] as string[],
    // Secret pool (multi-secret)
    key_mode: 'single' as SecretMode,
    auth_keys: [] as string[],
    auth_keys_input: '',
    // Stdio fields
    command: 'npx',
    args: '',
    env: '',
    commandChoice: 'npx' as CommandChoice,
    registry: '',
    registryPreset: 'default',
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
      const multi = form.key_mode !== 'single' && form.auth_type !== 'none'
      return createService({
        name: form.name,
        display_name: form.display_name || undefined,
        description: form.description || undefined,
        transport_type: form.transport_type,
        config,
        auth_type: form.auth_type === 'none' ? undefined : form.auth_type,
        auth_config: Object.keys(authConfig).length > 0 ? authConfig : undefined,
        key_mode: multi && form.key_mode !== 'single' ? form.key_mode : undefined,
        auth_keys: multi ? form.auth_keys : undefined,
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
      // 多秘钥:逐把换认证头测试(复用现有 test-connection 接口,不改后端)
      if (form.key_mode !== 'single' && form.auth_type !== 'none' && form.auth_keys.length > 0) {
        const results: KeyTestResult[] = []
        for (let i = 0; i < form.auth_keys.length; i++) {
          const secret = form.auth_keys[i] || ''
          const config = buildConfig(secret)
          let r: { connected: boolean; error?: string; tools_count: number; latency_ms: number }
          try {
            const testRes = await testConnection({ transport_type: form.transport_type, config })
            r = testRes.data as TestResult
          } catch (e) {
            const err = e as { response?: { data?: { message?: string } }; message?: string }
            r = { connected: false, error: err?.response?.data?.message || err?.message || '', tools_count: 0, latency_ms: 0 }
          }
          results.push({ index: i + 1, masked: maskSecret(secret), connected: r.connected, error: r.error, tools_count: r.tools_count, latency_ms: r.latency_ms })
          setKeyTestResults([...results])
        }
        return
      }
      const config = buildConfig()
      const testRes = await testConnection({ transport_type: form.transport_type, config })
      setTestResult(testRes.data as TestResult)
    },
  })

  // --- stdio detect/install state ---
  const [installStatus, setInstallStatus] = useState<InstallStatus>('idle')
  const [installMessage, setInstallMessage] = useState('')
  const [installResult, setInstallResult] = useState<PrepareStdioResult | null>(null)
  const [preparedSig, setPreparedSig] = useState('')

  const registryOptions = form.commandChoice === 'npx' ? NPM_REGISTRIES : form.commandChoice === 'uvx' ? UV_REGISTRIES : []

  // Signature of the inputs covered by the last successful prepare; editing
  // command/args/registry afterwards flips readyForCurrentInputs back to false.
  const commandSig = `${form.command}|${parseArgs(form.args).join('\n')}|${form.registry}`
  const readyForCurrentInputs = installStatus === 'ready' && preparedSig === commandSig

  const prepareMutation = useMutation({
    mutationFn: () => prepareStdio({
      command: form.command,
      args: parseArgs(form.args),
      env: parseEnv(form.env),
      registry: form.registry,
    }),
    onSuccess: (res) => {
      const data = res.data as PrepareStdioResult
      setInstallResult(data)
      setInstallMessage(data.message)
      if (data.installed) {
        setInstallStatus('ready')
        setPreparedSig(commandSig)
      } else {
        setInstallStatus('failed')
      }
    },
    onError: (e: unknown) => {
      const err = e as { response?: { data?: { message?: string } }; message?: string }
      setInstallStatus('failed')
      setInstallMessage(err?.response?.data?.message || err?.message || '')
      setInstallResult(null)
    },
  })

  function onCommandChoice(choice: CommandChoice) {
    if (choice === 'npx' || choice === 'uvx') {
      setForm({ ...form, commandChoice: choice, command: choice, registryPreset: 'default', registry: '' })
    } else {
      setForm({ ...form, commandChoice: 'custom' })
    }
  }

  function onRegistryPresetChange(key: string) {
    if (key === 'default') {
      setForm({ ...form, registryPreset: 'default', registry: '' })
    } else if (key === 'custom') {
      setForm({ ...form, registryPreset: 'custom' })
    } else {
      const opt = registryOptions.find((r) => r.key === key)
      if (opt) setForm({ ...form, registryPreset: key, registry: opt.url })
    }
  }

  // 多秘钥仅这两类 HTTP 传输支持(stdio 的 env 无法按请求轮换)
  const multiKeySupported = form.transport_type === 'streamable-http' || form.transport_type === 'sse'
  const isMultiKey = form.key_mode !== 'single' && form.auth_type !== 'none' && multiKeySupported

  // 多秘钥模式下注入头名:api_key/bearer 固定,custom 用用户填的头名
  function multiKeyHeaderName(): string {
    if (form.auth_type === 'api_key') return 'X-API-Key'
    if (form.auth_type === 'bearer') return 'Authorization'
    return form.custom_header_key.trim()
  }

  // buildConfig 组装传输配置;multiKeyKey 非空时(单秘钥/逐把测试)把认证头烘进 headers,
  // 多秘钥正式创建时不放认证头(值存秘钥池,由网关按策略注入)。
  function buildConfig(multiKeyKey?: string): Record<string, unknown> {
    const headers: Record<string, string> = {}
    const withAuth = multiKeyKey !== undefined || !isMultiKey
    if (withAuth && form.auth_type !== 'none') {
      const key = multiKeyKey ?? (form.auth_type === 'api_key' ? form.api_key : form.auth_type === 'bearer' ? form.bearer_token : form.custom_header_value)
      if (form.auth_type === 'api_key' && key) {
        headers['X-API-Key'] = key
      } else if (form.auth_type === 'bearer' && key) {
        headers['Authorization'] = `Bearer ${key}`
      } else if (form.auth_type === 'custom' && form.custom_header_key) {
        headers[form.custom_header_key] = key
      }
    }

    switch (form.transport_type) {
      case 'stdio':
        return {
          command: form.command,
          args: parseArgs(form.args),
          env: parseEnv(form.env),
          registry: form.registry,
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
    if (isMultiKey) {
      // 多秘钥:key_mode 由请求字段传,这里只带注入头名(custom 必填,其余后端也能兜底)
      const headerName = multiKeyHeaderName()
      return headerName ? { header_name: headerName } : {}
    }
    switch (form.auth_type) {
      case 'api_key': return { key: form.api_key }
      case 'bearer': return { token: form.bearer_token }
      case 'custom':
        // 同详情页:记录注入头名,供之后切换多秘钥时作默认值
        return form.custom_header_key ? { header_name: form.custom_header_key } : {}
      default: return {}
    }
  }

  // 多秘钥:textarea 批量添加(每行一把,去空行/去重)
  function addAuthKeys() {
    const lines = form.auth_keys_input.split('\n').map((s) => s.trim()).filter(Boolean)
    if (lines.length === 0) return
    const merged = [...form.auth_keys]
    let added = 0
    for (const line of lines) {
      if (!merged.includes(line)) {
        merged.push(line)
        added++
      }
    }
    setForm({ ...form, auth_keys: merged, auth_keys_input: '' })
    if (added < lines.length) toast.info(t('services.keys.duplicateSkipped', { count: lines.length - added }))
  }

  function removeAuthKey(index: number) {
    setForm({ ...form, auth_keys: form.auth_keys.filter((_, i) => i !== index) })
  }

  const canNext = () => {
    if (step === 0) return form.name.trim().length > 0 && SERVICE_NAME_RE.test(form.name)
    if (step === 1) {
      if (form.transport_type === 'stdio') return readyForCurrentInputs
      return form.url.trim().length > 0
    }
    if (step === 2 && isMultiKey) {
      if (form.auth_type === 'custom' && !form.custom_header_key.trim()) return false
      return form.auth_keys.length > 0
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
            <Input id="name" placeholder="my-service" maxLength={64} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <p className="text-xs text-muted-foreground">{t('services.create.serviceIdentifierTip')}</p>
            {/* 填了但格式不符:即时行内报错(下一步按钮同时禁用,原因可见) */}
            {form.name.trim().length > 0 && !SERVICE_NAME_RE.test(form.name) && (
              <p className="text-xs text-destructive">{t('services.create.serviceIdentifierInvalid')}</p>
            )}
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
              {visibleTransportOptions.map((opt) => (
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
              {/* Command type */}
              <div className="space-y-2">
                <Label>{t('services.create.commandChoice')}</Label>
                <div className="grid gap-2 sm:grid-cols-3">
                  {([
                    { v: 'npx', label: t('services.create.commandNpx') },
                    { v: 'uvx', label: t('services.create.commandUvx') },
                    { v: 'custom', label: t('services.create.commandCustom') },
                  ] as { v: CommandChoice; label: string }[]).map((opt) => (
                    <button
                      key={opt.v}
                      type="button"
                      onClick={() => onCommandChoice(opt.v)}
                      className={`rounded-lg border p-3 text-left text-sm transition-all ${
                        form.commandChoice === opt.v ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              </div>

              {/* Custom command input */}
              {form.commandChoice === 'custom' && (
                <div className="space-y-2">
                  <Label htmlFor="command">{t('services.commandRequired')}</Label>
                  <Input id="command" placeholder={t('services.create.commandCustomPlaceholder')} value={form.command} onChange={(e) => setForm({ ...form, command: e.target.value })} />
                </div>
              )}

              {/* Package registry / mirror (npx / uvx only) */}
              {form.commandChoice !== 'custom' && (
                <div className="space-y-2">
                  <Label>{t('services.create.registryLabel')}</Label>
                  <Select value={form.registryPreset} onValueChange={onRegistryPresetChange}>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="default">{t('services.create.registryDefault')}</SelectItem>
                      {registryOptions.map((r) => (
                        <SelectItem key={r.key} value={r.key}>{r.label}</SelectItem>
                      ))}
                      <SelectItem value="custom">{t('services.create.registryCustom')}</SelectItem>
                    </SelectContent>
                  </Select>
                  {form.registryPreset === 'custom' && (
                    <Input placeholder={t('services.create.registryCustomPlaceholder')} value={form.registry} onChange={(e) => setForm({ ...form, registry: e.target.value })} />
                  )}
                  <p className="text-xs text-muted-foreground">
                    {form.commandChoice === 'npx' ? t('services.create.registryHintNpx') : t('services.create.registryHintUvx')}
                  </p>
                </div>
              )}

              {/* Arguments (one per line) */}
              <div className="space-y-2">
                <Label htmlFor="args">{t('services.create.argsLabel')}</Label>
                <Textarea id="args" rows={3} placeholder={'-y\n@modelcontextprotocol/server-memory'} value={form.args} onChange={(e) => setForm({ ...form, args: e.target.value })} />
                <p className="text-xs text-muted-foreground">{t('services.create.argsHint')}</p>
              </div>

              {/* Environment variables (KEY=value, one per line) */}
              <div className="space-y-2">
                <Label htmlFor="env">{t('services.create.envLabel')}</Label>
                <Textarea id="env" rows={3} placeholder={'API_KEY=xxx\nNODE_ENV=production'} value={form.env} onChange={(e) => setForm({ ...form, env: e.target.value })} />
                <p className="text-xs text-muted-foreground">{t('services.create.envHint')}</p>
              </div>

              {/* Detect & install */}
              <div className="space-y-2 rounded-lg bg-muted/40 p-3">
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="gap-1.5"
                    onClick={() => prepareMutation.mutate()}
                    disabled={prepareMutation.isPending || !form.command.trim()}
                  >
                    {prepareMutation.isPending ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : installStatus === 'ready' ? (
                      <RefreshCw className="h-3.5 w-3.5" />
                    ) : (
                      <Zap className="h-3.5 w-3.5" />
                    )}
                    {prepareMutation.isPending
                      ? t('services.create.installing')
                      : installStatus === 'ready'
                        ? t('services.create.reinstall')
                        : t('services.create.installButton')}
                  </Button>
                </div>

                {prepareMutation.isPending && (
                  <p className="text-sm text-muted-foreground">{t('services.create.installing')}</p>
                )}
                {installStatus === 'ready' && readyForCurrentInputs && (
                  <p className="text-sm text-emerald-600 dark:text-emerald-400">{t('services.create.installReady', { msg: installMessage })}</p>
                )}
                {installStatus === 'ready' && !readyForCurrentInputs && (
                  <p className="text-sm text-muted-foreground">{t('services.create.installStaleHint')}</p>
                )}
                {installStatus === 'failed' && (
                  <p className="text-sm text-red-600 dark:text-red-400">{t('services.create.installFailed', { msg: installMessage })}</p>
                )}
                {installStatus === 'idle' && (
                  <p className="text-sm text-muted-foreground">{t('services.create.installStatusIdle')}</p>
                )}
                {!readyForCurrentInputs && (
                  <p className="text-xs text-muted-foreground">{t('services.create.installRequiredNext')}</p>
                )}
                {installResult?.stderr && installStatus === 'failed' && (
                  <pre className="mt-1 max-h-32 overflow-auto rounded bg-muted p-2 text-xs">{installResult.stderr}</pre>
                )}
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

          {form.auth_type === 'api_key' && !isMultiKey && (
            <div className="space-y-2">
              <Label>API Key</Label>
              <Input placeholder="sk-xxx" value={form.api_key} onChange={(e) => setForm({ ...form, api_key: e.target.value })} />
              <p className="text-xs text-muted-foreground">{t('services.create.authXApiKey')}</p>
            </div>
          )}
          {form.auth_type === 'bearer' && !isMultiKey && (
            <div className="space-y-2">
              <Label>Token</Label>
              <Input placeholder="eyJhbGci..." value={form.bearer_token} onChange={(e) => setForm({ ...form, bearer_token: e.target.value })} />
              <p className="text-xs text-muted-foreground">{t('services.create.authBearer')}</p>
            </div>
          )}
          {form.auth_type === 'none' && (
            <p className="text-sm text-muted-foreground">{t('services.create.noAuthHint')}</p>
          )}
          {form.auth_type === 'custom' && !isMultiKey && (
            <div className="space-y-2">
              <Label>{t('services.customHeaders')}</Label>
              <div className="flex gap-2">
                <Input placeholder={t('services.headerPlaceholder')} value={form.custom_header_key} onChange={(e) => setForm({ ...form, custom_header_key: e.target.value })} />
                <Input placeholder="Value" value={form.custom_header_value} onChange={(e) => setForm({ ...form, custom_header_value: e.target.value })} />
              </div>
              <p className="text-xs text-muted-foreground">{t('services.authHeaderTip', { header: `{ "Key": "Value" }` })}</p>
            </div>
          )}

          {/* 秘钥模式:单秘钥 / 多秘钥(随机/轮询),仅 streamable-http & sse */}
          {form.auth_type !== 'none' && multiKeySupported && (
            <div className="space-y-3 border-t pt-4">
              <div className="space-y-2">
                <Label>{t('services.keys.modeLabel')}</Label>
                <div className="flex flex-wrap gap-2">
                  {([
                    { v: 'single' as SecretMode, label: t('services.keys.modeSingle') },
                    { v: 'random' as SecretMode, label: t('services.keys.modeRandom') },
                    { v: 'polling' as SecretMode, label: t('services.keys.modePolling') },
                  ]).map((opt) => (
                    <button
                      key={opt.v}
                      type="button"
                      onClick={() => setForm({ ...form, key_mode: opt.v })}
                      className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                        form.key_mode === opt.v
                          ? 'border-primary bg-primary/5'
                          : 'hover:border-primary/30'
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">
                  {form.key_mode === 'single'
                    ? t('services.keys.modeSingleHint')
                    : form.key_mode === 'random'
                      ? t('services.keys.modeRandomHint')
                      : t('services.keys.modePollingHint')}
                </p>
              </div>

              {isMultiKey && (
                <div className="space-y-3">
                  {form.auth_type === 'custom' && (
                    <div className="space-y-2">
                      <Label>{t('services.keys.headerNameLabel')}</Label>
                      <Input placeholder="X-Custom-Auth" value={form.custom_header_key} onChange={(e) => setForm({ ...form, custom_header_key: e.target.value })} />
                      <p className="text-xs text-muted-foreground">{t('services.keys.headerNameHint')}</p>
                    </div>
                  )}
                  <div className="space-y-2">
                    <Label>{t('services.keys.addLabel')}</Label>
                    <Textarea
                      rows={4}
                      placeholder={t('services.keys.textareaPlaceholder')}
                      value={form.auth_keys_input}
                      onChange={(e) => setForm({ ...form, auth_keys_input: e.target.value })}
                    />
                    <Button type="button" variant="outline" size="sm" onClick={addAuthKeys}>
                      {t('services.keys.addBtn')}
                    </Button>
                  </div>
                  {form.auth_keys.length > 0 && (
                    <div className="space-y-2">
                      <p className="text-sm font-medium">{t('services.keys.poolCount', { count: form.auth_keys.length })}</p>
                      <div className="space-y-1.5">
                        {form.auth_keys.map((k, i) => (
                          <div key={i} className="flex items-center justify-between rounded-lg border px-3 py-1.5 text-sm">
                            <span className="font-mono text-xs text-muted-foreground">
                              <span className="mr-2 inline-block w-6 text-right">{i + 1}</span>
                              {maskSecret(k)}
                            </span>
                            <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-destructive" onClick={() => removeAuthKey(i)}>
                              {t('common.delete')}
                            </Button>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* Step 3: Test & confirm */}
      {step === 3 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <p className="text-sm text-muted-foreground">
            {form.transport_type === 'stdio' && readyForCurrentInputs
              ? t('services.create.testHintInstalled')
              : t('services.create.testHint')}
          </p>

          {testResult && !keyTestResults.length && (
            <div className={`rounded-lg p-4 ${testResult.connected ? 'bg-emerald-500/10' : 'bg-red-500/10'}`}>
              {testResult.connected ? (
                <div className="space-y-1 text-sm">
                  <p className="font-medium text-emerald-600 dark:text-emerald-400">{t('services.create.testSuccess')}</p>
                  <p className="text-muted-foreground">{t('services.create.testInfo', { count: testResult.tools_count, ms: testResult.latency_ms })}</p>
                  {testResult.protocol_version && (
                    <p className="text-muted-foreground">{t('services.create.testProtocol', { version: testResult.protocol_version })}</p>
                  )}
                </div>
              ) : (
                <p className="text-sm text-red-600 dark:text-red-400">{t('services.create.testFailed', { error: testResult.error || t('common.unknownError') })}</p>
              )}
            </div>
          )}

          {/* 多秘钥:逐把测试结果 */}
          {keyTestResults.length > 0 && (
            <div className="space-y-1.5">
              {keyTestResults.map((r) => (
                <div
                  key={r.index}
                  className={`flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm ${
                    r.connected ? 'border-emerald-500/40 bg-emerald-500/5' : 'border-red-500/40 bg-red-500/5'
                  }`}
                >
                  <span className="font-mono text-xs text-muted-foreground">
                    <span className="mr-2 inline-block w-6 text-right">#{r.index}</span>
                    {r.masked}
                  </span>
                  {r.connected ? (
                    <span className="text-xs text-emerald-600 dark:text-emerald-400">
                      {t('services.keys.testOk', { count: r.tools_count, ms: r.latency_ms })}
                    </span>
                  ) : (
                    <span className="truncate text-xs text-red-600 dark:text-red-400" title={r.error}>
                      {t('services.keys.testFailed', { error: r.error || t('common.unknownError') })}
                    </span>
                  )}
                </div>
              ))}
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
              disabled={createMutation.isPending || (form.transport_type === 'stdio' && !readyForCurrentInputs)}
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
