import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { getService, updateService, deleteService, testService, refreshTools } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import {
  ArrowLeft, Trash2, Zap, RefreshCw, Server,
  Pencil, X, Check, Loader2,
} from 'lucide-react'
import type { McpTool, AuthType, UpdateServiceReq } from '@/types'

export function ServiceDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()
  const serviceId = Number(id)

  const transportLabels: Record<string, string> = {
    'stdio': t('services.transports.stdio'),
    'sse': t('services.transports.sse'),
    'streamable-http': t('services.transports["streamable-http"]'),
    'websocket': t('services.transports.websocket'),
    'passive-ws': t('services.transports["passive-ws"]'),
    'virtual': t('services.transport_virtual'),
  }

  const sourceLabels: Record<string, { label: string; color: string }> = {
    'vision': { label: t('services.sourceVision'), color: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300' },
    'camera': { label: t('services.sourceCamera'), color: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' },
  }

  const authOptions: { value: AuthType; label: string }[] = [
    { value: 'none', label: t('services.authNone') },
    { value: 'api_key', label: t('services.authApiKey') },
    { value: 'bearer', label: t('services.authBearer') },
    { value: 'custom', label: t('services.authCustom') },
  ]

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<EditForm>({
    display_name: '',
    description: '',
    command: '',
    args: '',
    env: '',
    url: '',
    auth_type: 'none',
    api_key: '',
    bearer_token: '',
    custom_header_key: '',
    custom_header_value: '',
  })

  const { data, isLoading } = useQuery({
    queryKey: ['service', id],
    queryFn: () => getService(serviceId),
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteService(serviceId),
    onSuccess: () => {
      toast.success(t('services.deleteSuccess'))
      navigate({ to: '/services' })
    },
  })

  const testMutation = useMutation({
    mutationFn: () => testService(serviceId),
    onSuccess: (res) => {
      if (res.data?.connected) {
        toast.success(t('services.connectSuccessDetail', { count: res.data.tools_count, ms: res.data.latency_ms }))
      } else {
        toast.error(t('services.connectFailedDetail', { error: res.data?.error || t('common.unknownError') }))
      }
    },
  })

  const refreshMutation = useMutation({
    mutationFn: () => refreshTools(serviceId),
    onSuccess: (res) => {
      toast.success(t('services.refreshed', { count: res.data?.tools_count || 0 }))
      queryClient.invalidateQueries({ queryKey: ['service', id] })
    },
  })

  const updateMutation = useMutation({
    mutationFn: () => {
      const payload: UpdateServiceReq = {
        display_name: form.display_name,
        description: form.description,
        config: buildConfig(),
      }
      // 仅在填写了认证凭据时才更新认证，避免清空已有配置
      const authConfig = buildAuthConfig()
      if (form.auth_type !== 'none' && Object.keys(authConfig).length > 0) {
        payload.auth_type = form.auth_type
        payload.auth_config = authConfig
      }
      return updateService(serviceId, payload)
    },
    onSuccess: () => {
      toast.success(t('services.updateSuccess'))
      setEditing(false)
      queryClient.invalidateQueries({ queryKey: ['service', id] })
    },
    onError: () => {
      toast.error(t('services.updateFailed'))
    },
  })

  const service = data?.data

  // 非编辑态下，把当前服务配置反向解析回表单字段，便于编辑预填
  useEffect(() => {
    if (service && !editing) {
      const cfg = (service.config || {}) as Record<string, unknown>
      const headers = ((cfg.headers as Record<string, string>) || {})

      let authType: AuthType = 'none'
      let apiKey = ''
      let bearerToken = ''
      let customKey = ''
      let customValue = ''
      if (headers['X-API-Key']) {
        authType = 'api_key'
        apiKey = headers['X-API-Key']
      } else if (headers['Authorization']?.startsWith('Bearer ')) {
        authType = 'bearer'
        bearerToken = headers['Authorization']!.slice(7)
      } else {
        const k = Object.keys(headers)[0]
        if (k) {
          authType = 'custom'
          customKey = k
          customValue = headers[k] || ''
        }
      }

      setForm({
        display_name: service.display_name || '',
        description: service.description || '',
        command: typeof cfg.command === 'string' ? cfg.command : '',
        args: Array.isArray(cfg.args) ? (cfg.args as unknown[]).join(' ') : '',
        env: cfg.env ? JSON.stringify(cfg.env) : '',
        url: typeof cfg.url === 'string' ? cfg.url : '',
        auth_type: authType,
        api_key: apiKey,
        bearer_token: bearerToken,
        custom_header_key: customKey,
        custom_header_value: customValue,
      })
    }
  }, [service, editing])

  function buildConfig(): Record<string, unknown> {
    const cfg = (service?.config || {}) as Record<string, unknown>
    const originalHeaders = ((cfg.headers as Record<string, string>) || {})

    // 判断本次是否提交了新的认证凭据
    const hasNewAuth =
      (form.auth_type === 'api_key' && !!form.api_key) ||
      (form.auth_type === 'bearer' && !!form.bearer_token) ||
      (form.auth_type === 'custom' && !!form.custom_header_key)

    const headers: Record<string, string> = hasNewAuth ? {} : { ...originalHeaders }
    if (hasNewAuth) {
      if (form.auth_type === 'api_key') headers['X-API-Key'] = form.api_key
      else if (form.auth_type === 'bearer') headers['Authorization'] = `Bearer ${form.bearer_token}`
      else if (form.auth_type === 'custom' && form.custom_header_key) headers[form.custom_header_key] = form.custom_header_value
    }

    switch (service?.transport_type) {
      case 'stdio':
        let envObj: Record<string, unknown> = {}
        if (form.env) {
          try { envObj = JSON.parse(form.env) } catch { /* 忽略非法 JSON */ }
        }
        return {
          command: form.command,
          args: form.args ? form.args.split(/\s+/).filter(Boolean) : [],
          env: envObj,
        }
      case 'sse':
      case 'streamable-http':
        return { url: form.url, headers }
      case 'websocket':
      case 'passive-ws':
        return { url: form.url }
      default:
        return cfg
    }
  }

  function buildAuthConfig(): Record<string, unknown> {
    switch (form.auth_type) {
      case 'api_key': return { key: form.api_key }
      case 'bearer': return { token: form.bearer_token }
      default: return {}
    }
  }

  const canSave = (() => {
    if (!service) return false
    if (service.transport_type === 'stdio') return form.command.trim().length > 0
    return form.url.trim().length > 0
  })()

  if (isLoading) {
    return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('common.loading')}</div>
  }

  if (!service) {
    return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('services.detailNotFound')}</div>
  }

  const tools: McpTool[] = service.tools_cache || []
  const isVirtual = service.transport_type === 'virtual'
  const isStdio = service.transport_type === 'stdio'
  const virtualSource = isVirtual ? sourceLabels[service.source] : null
  const envValid = !form.env || (() => { try { JSON.parse(form.env); return true } catch { return false } })()

  return (
    <div className="p-6 lg:p-8 space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/services' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{service.display_name || service.name}</h1>
            <div className="mt-0.5 flex items-center gap-2">
              <p className="text-sm text-muted-foreground">{service.name}</p>
              {virtualSource && (
                <span className={`inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-medium ${virtualSource.color}`}>
                  {virtualSource.label}
                </span>
              )}
            </div>
            {service.description && <p className="mt-2 text-sm text-muted-foreground">{service.description}</p>}
          </div>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => testMutation.mutate()} disabled={editing || testMutation.isPending || isVirtual} title={isVirtual ? t('services.virtualNotTestable') : undefined}>
            <Zap className="h-3.5 w-3.5" />
            {testMutation.isPending ? t('services.testPending') : t('services.test')}
          </Button>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => refreshMutation.mutate()} disabled={editing || refreshMutation.isPending || isVirtual} title={isVirtual ? t('services.virtualNotRefreshable') : undefined}>
            <RefreshCw className={`h-3.5 w-3.5 ${refreshMutation.isPending ? 'animate-spin' : ''}`} />
            {t('services.refreshTools')}
          </Button>
          {!isVirtual && !editing && (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              <Pencil className="h-3.5 w-3.5 mr-1.5" />{t('common.edit')}
            </Button>
          )}
          {!isVirtual && (
          <Button variant="outline" size="sm" className="text-destructive hover:text-destructive" disabled={editing} onClick={() => {
            if (confirm(t('services.deletePrompt'))) deleteMutation.mutate()
          }}>
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
          )}
        </div>
      </div>

      {/* Info cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {[
          { label: t('services.transportType'), value: transportLabels[service.transport_type] || service.transport_type },
          { label: t('services.healthStatus'), value: service.health_status || t('common.unknown') },
          { label: t('services.toolsCount'), value: String(tools.length) },
          { label: t('services.protocolVersion'), value: service.protocol_version || '-' },
        ].map((item) => (
          <div key={item.label} className="rounded-xl border bg-card p-4">
            <p className="text-xs text-muted-foreground">{item.label}</p>
            <p className="mt-1 text-sm font-medium">{item.value}</p>
          </div>
        ))}
      </div>

      {/* Server Info */}
      {service.server_info && Object.keys(service.server_info).length > 0 && (
        <div className="rounded-xl border bg-card p-5">
          <h2 className="mb-3 text-sm font-semibold">{t('services.serverInfo')}</h2>
          <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto">
            {JSON.stringify(service.server_info, null, 2)}
          </pre>
        </div>
      )}

      {/* Basic config / editable */}
      {!isVirtual && (
        <div className="rounded-xl border bg-card p-5 space-y-4">
          <h2 className="text-sm font-semibold">{t('services.basicConfig')}</h2>

          <div className="grid gap-4 sm:grid-cols-2">
            {/* Display name */}
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">{t('services.displayName')}</Label>
              {editing ? (
                <Input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} placeholder={t('services.placeholderMyService')} />
              ) : (
                <p className="text-sm">{service.display_name || '-'}</p>
              )}
            </div>

            {/* Name (read-only) */}
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">{t('services.serviceIdentifier')}</Label>
              <p className="text-sm font-mono">{service.name}</p>
            </div>

            {/* Description */}
            <div className="space-y-1.5 sm:col-span-2">
              <Label className="text-xs text-muted-foreground">{t('services.description')}</Label>
              {editing ? (
                <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} placeholder={t('services.placeholderDesc')} />
              ) : (
                <p className="text-sm">{service.description || '-'}</p>
              )}
            </div>

            {/* Transport type (read-only) */}
            <div className="space-y-1.5 sm:col-span-2">
              <Label className="text-xs text-muted-foreground">{t('services.transportType')}</Label>
              <p className="text-sm">{transportLabels[service.transport_type] || service.transport_type}</p>
            </div>

            {/* Stdio fields */}
            {isStdio ? (
              <>
                <div className="space-y-1.5 sm:col-span-2">
                  <Label className="text-xs text-muted-foreground">{t('services.commandRequired')}</Label>
                  {editing ? (
                    <Input value={form.command} onChange={(e) => setForm({ ...form, command: e.target.value })} placeholder="npx" />
                  ) : (
                    <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{(service.config as Record<string, unknown>)?.command as string || '-'}</code>
                  )}
                </div>
                <div className="space-y-1.5 sm:col-span-2">
                  <Label className="text-xs text-muted-foreground">{t('services.args')}</Label>
                  {editing ? (
                    <Input value={form.args} onChange={(e) => setForm({ ...form, args: e.target.value })} placeholder="-y @modelcontextprotocol/server-memory" />
                  ) : (
                    <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{form.args || '-'}</code>
                  )}
                </div>
                <div className="space-y-1.5 sm:col-span-2">
                  <Label className="text-xs text-muted-foreground">{t('services.envVars')}</Label>
                  {editing ? (
                    <>
                      <Input value={form.env} onChange={(e) => setForm({ ...form, env: e.target.value })} placeholder='{"API_KEY": "xxx"}' />
                      {!envValid && <p className="text-xs text-red-500">{t('services.envInvalidJson')}</p>}
                    </>
                  ) : (
                    <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{form.env || '-'}</code>
                  )}
                </div>
              </>
            ) : (
              /* HTTP/SSE/WS url */
              <div className="space-y-1.5 sm:col-span-2">
                <Label className="text-xs text-muted-foreground">{t('services.serviceUrlRequired')}</Label>
                {editing ? (
                  <Input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://example.com/mcp" />
                ) : (
                  <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{form.url || '-'}</code>
                )}
              </div>
            )}

            {/* Auth config */}
            <div className="space-y-1.5 sm:col-span-2">
              <Label className="text-xs text-muted-foreground">{t('services.authMethod')}</Label>
              {editing ? (
                <div className="flex flex-wrap gap-2 pt-1">
                  {authOptions.map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => setForm({ ...form, auth_type: opt.value })}
                      className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                        form.auth_type === opt.value ? 'border-primary bg-primary/5' : 'hover:border-primary/30'
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              ) : (
                <p className="text-sm">{authOptions.find((o) => o.value === form.auth_type)?.label || t('services.noAuth')}</p>
              )}
            </div>

            {editing && form.auth_type === 'api_key' && (
              <div className="space-y-1.5 sm:col-span-2">
                <Label className="text-xs text-muted-foreground">API Key</Label>
                <Input value={form.api_key} onChange={(e) => setForm({ ...form, api_key: e.target.value })} placeholder={t('services.placeholderKeepUnchanged')} />
              </div>
            )}
            {editing && form.auth_type === 'bearer' && (
              <div className="space-y-1.5 sm:col-span-2">
                <Label className="text-xs text-muted-foreground">Token</Label>
                <Input value={form.bearer_token} onChange={(e) => setForm({ ...form, bearer_token: e.target.value })} placeholder={t('services.placeholderKeepUnchanged')} />
              </div>
            )}
            {editing && form.auth_type === 'custom' && (
              <div className="space-y-1.5 sm:col-span-2">
                <Label className="text-xs text-muted-foreground">{t('services.customHeaders')}</Label>
                <div className="flex gap-2">
                  <Input value={form.custom_header_key} onChange={(e) => setForm({ ...form, custom_header_key: e.target.value })} placeholder="Header Key" />
                  <Input value={form.custom_header_value} onChange={(e) => setForm({ ...form, custom_header_value: e.target.value })} placeholder="Value" />
                </div>
              </div>
            )}
            {editing && form.auth_type !== 'none' && (
              <p className="text-xs text-muted-foreground sm:col-span-2">{t('services.headerKeepUnchanged')}</p>
            )}
          </div>

          {/* Edit actions */}
          {editing && (
            <div className="flex justify-end gap-2 pt-2 border-t">
              <Button variant="outline" size="sm" onClick={() => setEditing(false)}>
                <X className="h-3.5 w-3.5 mr-1.5" />{t('common.cancel')}
              </Button>
              <Button
                size="sm"
                onClick={() => updateMutation.mutate()}
                disabled={!canSave || !envValid || updateMutation.isPending}
              >
                {updateMutation.isPending ? <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" /> : <Check className="h-3.5 w-3.5 mr-1.5" />}
                {t('common.save')}
              </Button>
            </div>
          )}
        </div>
      )}

      {/* Raw config (read-only) */}
      {!editing && service.config && Object.keys(service.config).length > 0 && (
        <div className="rounded-xl border bg-card p-5">
          <h2 className="mb-3 text-sm font-semibold">{t('services.connectionConfig')}</h2>
          <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto">
            {JSON.stringify(service.config, null, 2)}
          </pre>
        </div>
      )}

      {/* Tools */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="mb-3 text-sm font-semibold">{t('services.toolsList', { count: tools.length })}</h2>
        {tools.length === 0 ? (
          <div className="flex flex-col items-center py-8 text-center">
            <Server className="h-8 w-8 text-muted-foreground/30 mb-2" />
            <p className="text-sm text-muted-foreground">{t('services.noTools')}</p>
            <p className="text-xs text-muted-foreground/60 mt-1">{t('services.clickRefresh')}</p>
          </div>
        ) : (
          <div className="space-y-2">
            {tools.map((tool) => (
              <div key={tool.name} className="rounded-lg border p-3">
                <p className="text-sm font-medium font-mono">{tool.name}</p>
                {tool.description && (
                  <p className="mt-0.5 text-xs text-muted-foreground">{tool.description}</p>
                )}
                {tool.inputSchema?.properties != null && (
                  <div className="mt-2 flex flex-wrap gap-1">
                    {Object.keys(tool.inputSchema.properties as Record<string, unknown>).map((param) => (
                      <span key={param} className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
                        {param}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

type EditForm = {
  display_name: string
  description: string
  command: string
  args: string
  env: string
  url: string
  auth_type: AuthType
  api_key: string
  bearer_token: string
  custom_header_key: string
  custom_header_value: string
}
