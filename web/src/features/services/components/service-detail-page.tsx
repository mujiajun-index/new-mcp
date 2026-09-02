import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { getService, updateService, deleteService, testService, refreshTools, getServiceResources, getServicePrompts, getServiceProcessStat, serviceKeysApi } from '../api'
import { StdioProcessControl } from './stdio-process-control'
import { ServiceKeysCard } from '@/components/service-keys-card'
import { ToolTestDialog } from './tool-test-dialog'
import { ResourceTestDialog, type ResourceTarget } from './resource-test-dialog'
import { PromptTestDialog } from './prompt-test-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ToolItem } from '@/components/tool-params'
import { SectionCard } from '@/components/section-card'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'
import {
  ArrowLeft, Trash2, Zap, RefreshCw, Server, Activity,
  Pencil, X, Check, Loader2, ChevronDown, ChevronRight, FlaskConical, Ban, Play, ExternalLink,
} from 'lucide-react'
import type { McpTool, McpResource, McpResourceTemplate, McpPrompt, AuthType, UpdateServiceReq, ServiceProcessStat } from '@/types'

// parseEnv / envToString: 与注册页一致，环境变量用「每行 KEY=value」文本表示，
// 而非 JSON。编辑 stdio 服务时仅环境变量可编辑，命令/参数只读。
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

function envToString(env: Record<string, unknown> | undefined | null): string {
  if (!env || typeof env !== 'object') return ''
  return Object.entries(env)
    .map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join('\n')
}

// formatBytes/formatUptime: 进程资源指标的展示格式化(B/KB/MB/GB、d/h/m/s)
function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}

function formatUptime(seconds?: number): string {
  if (!seconds || seconds <= 0) return '-'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

export function ServiceDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const router = useRouter()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()
  const serviceId = Number(id)

  const transportLabels: Record<string, string> = {
    'stdio': t('services.transports.stdio'),
    'sse': t('services.transports.sse'),
    'streamable-http': t('services.transports.streamable-http'),
    'websocket': t('services.transports.websocket'),
    'passive-ws': t('services.transports.passive-ws'),
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
  // 资源/模板/提示的展开项(单开):点击名称切换描述显隐,与工具条目同一交互
  const [openItem, setOpenItem] = useState<string | null>(null)
  // 工具测试:当前正在测试的工具(弹窗打开时非空)
  const [testingTool, setTestingTool] = useState<McpTool | null>(null)
  const [testingResource, setTestingResource] = useState<ResourceTarget | null>(null)
  const [testingPrompt, setTestingPrompt] = useState<McpPrompt | null>(null)
  const [form, setForm] = useState<EditForm>({
    display_name: '',
    description: '',
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

  const { data: resourcesData } = useQuery({
    queryKey: ['service-resources', id],
    queryFn: () => getServiceResources(serviceId),
  })

  const { data: promptsData } = useQuery({
    queryKey: ['service-prompts', id],
    queryFn: () => getServicePrompts(serviceId),
  })

  // stdio 服务进程资源占用:仅 stdio 启用,5s 自动轮询,离开详情页(组件卸载)即停止
  const isStdioService = data?.data?.transport_type === 'stdio'
  const { data: processData } = useQuery({
    queryKey: ['service-process', id],
    queryFn: () => getServiceProcessStat(serviceId),
    enabled: isStdioService,
    refetchInterval: 5000,
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteService(serviceId),
    onSuccess: () => {
      toast.success(t('services.deleteSuccess'))
      navigate({ to: '/services' })
    },
  })

  // 启用/禁用:与列表页同口径。禁用会从全部分组移除服务并终止 stdio 进程,需二次确认;
  // 成功后一并失效进程/总览轮询键,禁用即停的状态立即回读。
  const toggleStatusMutation = useMutation({
    mutationFn: (status: number) => updateService(serviceId, { status }),
    onSuccess: (_data, status) => {
      queryClient.invalidateQueries({ queryKey: ['service', id] })
      queryClient.invalidateQueries({ queryKey: ['services'] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['service-process'] })
      queryClient.invalidateQueries({ queryKey: ['services-overview'] })
      toast.success(status === 0 ? t('services.disabledToast') : t('services.enabledToast'))
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
      // 刷新会同步更新资源/提示缓存
      queryClient.invalidateQueries({ queryKey: ['service-resources', id] })
      queryClient.invalidateQueries({ queryKey: ['service-prompts', id] })
    },
  })

  const updateMutation = useMutation({
    mutationFn: () => {
      const payload: UpdateServiceReq = {
        display_name: form.display_name,
        description: form.description,
        config: buildConfig(),
      }
      const authConfig = buildAuthConfig()
      // 多秘钥服务:认证由秘钥池承载,不提交认证字段(避免覆盖 AuthConfig 里的
      // key_mode/header_name;切换认证类型须先在秘钥管理切回单秘钥)
      const multiKey = service?.key_mode === 'random' || service?.key_mode === 'polling'
      if (!multiKey && form.auth_type === 'none') {
        // 切换为无需认证时，显式清空已保存的认证类型与凭据
        payload.auth_type = 'none'
        payload.auth_config = {}
      } else if (!multiKey && Object.keys(authConfig).length > 0) {
        // 仅在填写了认证凭据时才更新认证，避免清空已有配置
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
      // 多秘钥:认证头不在 headers 里(由秘钥池注入),直接用服务行存的认证类型,
      // 凭据字段留空(编辑态隐藏凭据输入、锁定认证类型)。
      const isMulti = service.key_mode === 'random' || service.key_mode === 'polling'

      let authType: AuthType = 'none'
      let apiKey = ''
      let bearerToken = ''
      let customKey = ''
      let customValue = ''
      if (isMulti) {
        authType = service.auth_type
      } else if (headers['X-API-Key']) {
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
        env: envToString(cfg.env as Record<string, unknown> | undefined),
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
    // 多秘钥:认证头由秘钥池注入,不做认证重建(凭据输入在多秘钥下隐藏)
    const isMultiSvc = service?.key_mode === 'random' || service?.key_mode === 'polling'

    // 判断本次是否提交了新的认证凭据
    const hasNewAuth =
      !isMultiSvc &&
      ((form.auth_type === 'api_key' && !!form.api_key) ||
      (form.auth_type === 'bearer' && !!form.bearer_token) ||
      (form.auth_type === 'custom' && !!form.custom_header_key))

    let headers: Record<string, string>
    if (isMultiSvc) {
      headers = { ...originalHeaders }
    } else if (form.auth_type === 'none') {
      // 切换为无需认证时，清除已保存的认证 headers
      headers = {}
    } else if (hasNewAuth) {
      headers = {}
      if (form.auth_type === 'api_key') headers['X-API-Key'] = form.api_key
      else if (form.auth_type === 'bearer') headers['Authorization'] = `Bearer ${form.bearer_token}`
      else if (form.auth_type === 'custom' && form.custom_header_key) headers[form.custom_header_key] = form.custom_header_value
    } else {
      // 未改动认证：保留原 headers
      headers = { ...originalHeaders }
    }

    switch (service?.transport_type) {
      case 'stdio':
        // 命令/参数不可编辑，沿用原配置；仅环境变量可编辑（保存后由后端自动重连生效）。
        return {
          command: cfg.command,
          args: Array.isArray(cfg.args) ? cfg.args : [],
          env: parseEnv(form.env),
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
      case 'custom':
        // header_name 必须带上:auth_type 只有在 authConfig 非空时才随保存提交,
        // custom 返回空对象会导致改成自定义后库里残留旧 auth_type(单→多秘钥时
        // 按旧类型推导注入头而报"未找到认证头");同时作为之后切换多秘钥的默认注入头。
        return form.custom_header_key ? { header_name: form.custom_header_key } : {}
      default: return {}
    }
  }

  const canSave = (() => {
    if (!service) return false
    if (service.transport_type === 'stdio') return !!(service.config as Record<string, unknown>)?.command
    return form.url.trim().length > 0
  })()

  if (isLoading) {
    return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('common.loading')}</div>
  }

  if (!service) {
    return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('services.detailNotFound')}</div>
  }

  const tools: McpTool[] = service.tools_cache || []
  const resources: McpResource[] = resourcesData?.data?.resources || []
  const templates: McpResourceTemplate[] = resourcesData?.data?.templates || []
  const prompts: McpPrompt[] = promptsData?.data || []
  const isVirtual = service.transport_type === 'virtual'
  const isStdio = service.transport_type === 'stdio'
  const isMultiKeyService = service.key_mode === 'random' || service.key_mode === 'polling'
  // 秘钥管理卡片:自有 HTTP 类(sse/streamable-http)且有认证的服务
  const keysCardVisible =
    service.source !== 'marketplace' &&
    !isVirtual &&
    (service.transport_type === 'sse' || service.transport_type === 'streamable-http') &&
    service.auth_type !== 'none'
  const stdioConfig = ((service.config as Record<string, unknown>) || {})
  const virtualSource = isVirtual ? sourceLabels[service.source] : null

  return (
    <div className="p-6 lg:p-8 space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => {
              // 真回退:从总览/列表/仪表盘任一入口进详情都回到来路;
              // 新标签直开详情时无历史可退,兜底回服务列表。
              if (router.history.canGoBack()) router.history.back()
              else navigate({ to: '/services' })
            }}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{service.display_name || service.name}</h1>
            <div className="mt-0.5 flex items-center gap-2">
              <p className="text-sm text-muted-foreground">{service.name}</p>
              {service.source === 'marketplace' && (
                <span className="inline-flex items-center rounded-md bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                  {t('marketplace.platformHosted')}
                </span>
              )}
              {isMultiKeyService && (
                <span className="inline-flex items-center rounded-md bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
                  {t('services.keys.multiKeyBadge', {
                    mode: service.key_mode === 'random' ? t('services.keys.modeRandom') : t('services.keys.modePolling'),
                    count: service.key_count ?? 0,
                  })}
                </span>
              )}
              {virtualSource && (
                <span className={`inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-medium ${virtualSource.color}`}>
                  {virtualSource.label}
                </span>
              )}
            </div>
            {service.description && <p className="mt-2 text-sm text-muted-foreground">{service.description}</p>}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => testMutation.mutate()} disabled={editing || testMutation.isPending || isVirtual} title={isVirtual ? t('services.virtualNotTestable') : undefined}>
            <Zap className="h-3.5 w-3.5" />
            {testMutation.isPending ? t('services.testPending') : t('services.test')}
          </Button>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => refreshMutation.mutate()} disabled={editing || refreshMutation.isPending || isVirtual} title={isVirtual ? t('services.virtualNotRefreshable') : undefined}>
            <RefreshCw className={`h-3.5 w-3.5 ${refreshMutation.isPending ? 'animate-spin' : ''}`} />
            {t('services.refreshTools')}
          </Button>
          {!isVirtual && !editing && service.source !== 'marketplace' && (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              <Pencil className="h-3.5 w-3.5 mr-1.5" />{t('common.edit')}
            </Button>
          )}
          {!editing && (
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5"
              disabled={toggleStatusMutation.isPending}
              onClick={() => {
                if (service.status === 1) {
                  if (confirm(t('services.disableConfirm', { name: service.display_name || service.name }))) {
                    toggleStatusMutation.mutate(0)
                  }
                } else {
                  toggleStatusMutation.mutate(1)
                }
              }}
            >
              {service.status === 1
                ? <><Ban className="h-3.5 w-3.5" />{t('services.statusBadgeDisabled')}</>
                : <><Play className="h-3.5 w-3.5" />{t('services.statusBadgeEnabled')}</>}
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

      {service.source === 'marketplace' && (
        <div className="flex flex-wrap items-center gap-2 rounded-xl border border-primary/20 bg-primary/5 p-4 text-xs text-primary">
          <span className="font-medium">{t('marketplace.platformHosted')}</span>
          <span className="text-primary/80">{t('marketplace.platformHostedDesc')}</span>
          {/* 跳转市场详情(价格/快照等以市场页为准);条目缺 ID 时不渲染 */}
          {service.marketplace_item_id != null && (
            <Link
              to="/marketplace/$id"
              params={{ id: String(service.marketplace_item_id) }}
              className="ml-auto inline-flex items-center gap-1 font-medium underline-offset-2 hover:underline"
            >
              {t('marketplace.viewMarketDetail')}
              <ExternalLink className="h-3 w-3" />
            </Link>
          )}
        </div>
      )}

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

            {/* Stdio fields: 命令/参数只读，仅环境变量可编辑（每行 KEY=value，保存后立即重连生效） */}
            {isStdio ? (
              <>
                <div className="space-y-1.5 sm:col-span-2">
                  <Label className="text-xs text-muted-foreground">{t('services.commandRequired')}</Label>
                  <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{(stdioConfig.command as string) || '-'}</code>
                </div>
                <div className="space-y-1.5 sm:col-span-2">
                  <Label className="text-xs text-muted-foreground">{t('services.args')}</Label>
                  <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{Array.isArray(stdioConfig.args) ? (stdioConfig.args as unknown[]).join(' ') : '-'}</code>
                </div>
                <div className="space-y-1.5 sm:col-span-2">
                  <Label className="text-xs text-muted-foreground">{t('services.create.envLabel')}</Label>
                  {editing ? (
                    <>
                      <Textarea rows={4} value={form.env} onChange={(e) => setForm({ ...form, env: e.target.value })} placeholder={'API_KEY=xxx\nNODE_ENV=production'} />
                      <p className="text-xs text-muted-foreground">{t('services.create.envHint')}</p>
                      <p className="text-xs text-muted-foreground">{t('services.envEditHint')}</p>
                    </>
                  ) : (
                    <code className="block whitespace-pre-wrap text-sm break-all rounded-md bg-muted/50 px-3 py-2">{form.env || '-'}</code>
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
                isMultiKeyService ? (
                  /* 多秘钥:认证类型由秘钥池承载,锁定切换(改类型请先切回单秘钥) */
                  <div className="flex flex-wrap items-center gap-2 pt-1">
                    {authOptions.map((opt) => (
                      <button
                        key={opt.value}
                        type="button"
                        disabled={opt.value !== form.auth_type}
                        className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                          form.auth_type === opt.value
                            ? 'border-primary bg-primary/5'
                            : 'cursor-not-allowed text-muted-foreground/40'
                        }`}
                      >
                        {opt.label}
                      </button>
                    ))}
                    <span className="text-xs text-muted-foreground">{t('services.keys.authLockedInMulti')}</span>
                  </div>
                ) : (
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
                )
              ) : (
                <p className="text-sm">
                  {authOptions.find((o) => o.value === form.auth_type)?.label || t('services.noAuth')}
                  {isMultiKeyService && (
                    <span className="ml-2 text-xs text-muted-foreground">
                      {t('services.keys.providedByPool', { count: service.key_count ?? 0 })}
                    </span>
                  )}
                </p>
              )}
            </div>

            {editing && isMultiKeyService ? (
              /* 多秘钥:凭据由秘钥池提供,不在此编辑 */
              <p className="text-xs text-muted-foreground sm:col-span-2">{t('services.keys.manageInCardBelow')}</p>
            ) : (
              <>
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
              </>
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
                disabled={!canSave || updateMutation.isPending}
              >
                {updateMutation.isPending ? <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" /> : <Check className="h-3.5 w-3.5 mr-1.5" />}
                {t('common.save')}
              </Button>
            </div>
          )}
        </div>
      )}

      {/* 秘钥管理(多秘钥池) */}
      {!editing && keysCardVisible && (
        <ServiceKeysCard
          id={serviceId}
          api={serviceKeysApi(serviceId)}
          onModeChanged={() => queryClient.invalidateQueries({ queryKey: ['service', id] })}
        />
      )}

      {/* Process stats (stdio only): 整棵进程树的内存/CPU/运行时长,5s 轮询 */}
      {isStdio && (() => {
        const proc: ServiceProcessStat | undefined = processData?.data
        return (
          <SectionCard
            title={t('services.processInfo')}
            actions={
              <div className="flex items-center gap-1.5">
                {service.status === 1 && (
                  <StdioProcessControl serviceId={serviceId} running={!!proc?.running} />
                )}
                <Badge variant={proc?.running ? 'success' : 'secondary'}>
                  {proc?.running ? t('services.processRunning') : t('services.processStopped')}
                </Badge>
              </div>
            }
          >
            {proc?.running ? (
              <div className="space-y-3">
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  {[
                    { label: t('services.memoryUsage'), value: formatBytes(proc.memory_rss_bytes), sub: t('services.processCount', { count: proc.process_count || 1 }) },
                    { label: t('services.cpuUsage'), value: `${(proc.cpu_percent ?? 0).toFixed(1)}%`, sub: null },
                    { label: t('services.uptime'), value: formatUptime(proc.uptime_seconds), sub: null },
                    { label: 'PID', value: String(proc.pid ?? '-'), sub: null },
                  ].map((item) => (
                    <div key={item.label} className="rounded-xl border bg-card p-4">
                      <p className="text-xs text-muted-foreground">{item.label}</p>
                      <p className="mt-1 text-lg font-semibold tabular-nums">{item.value}</p>
                      {item.sub && <p className="mt-0.5 text-xs text-muted-foreground">{item.sub}</p>}
                    </div>
                  ))}
                </div>
                {proc.command && (
                  <p className="truncate font-mono text-xs text-muted-foreground" title={proc.command}>
                    {proc.command}
                  </p>
                )}
              </div>
            ) : (
              <div className="flex flex-col items-center py-8 text-center">
                <Activity className="h-8 w-8 text-muted-foreground/30 mb-2" />
                <p className="text-sm text-muted-foreground">{t('services.processStopped')}</p>
                <p className="mt-1 text-xs text-muted-foreground/60">{t('services.processNotRunningHint')}</p>
              </div>
            )}
          </SectionCard>
        )
      })()}

      {/* Tools */}
      <SectionCard title={t('services.toolsList', { count: tools.length })}>
        {tools.length === 0 ? (
          <div className="flex flex-col items-center py-8 text-center">
            <Server className="h-8 w-8 text-muted-foreground/30 mb-2" />
            <p className="text-sm text-muted-foreground">{t('services.noTools')}</p>
            <p className="text-xs text-muted-foreground/60 mt-1">{t('services.clickRefresh')}</p>
          </div>
        ) : (
          <div className="space-y-2">
            {tools.map((tool) => (
              <ToolItem
                key={tool.name}
                name={tool.name}
                description={tool.description}
                schema={tool.inputSchema}
                action={(
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-6 gap-1 px-1.5 text-[11px]"
                    onClick={() => setTestingTool(tool)}
                  >
                    <FlaskConical className="h-3 w-3" />
                    {t('services.testTool')}
                  </Button>
                )}
              />
            ))}
          </div>
        )}
      </SectionCard>

      {/* Resources */}
      <SectionCard title={t('services.resourcesList', { count: resources.length + templates.length })}>
        {resources.length === 0 && templates.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noResources')}</p>
        ) : (
          <div className="space-y-2">
            {resources.map((r) => (
              <div key={r.uri} className="rounded-lg border p-3">
                <div className="flex items-start gap-1">
                  {r.description && (openItem === `res:${r.uri}`
                    ? <ChevronDown className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    : <ChevronRight className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />)}
                  <button
                    type="button"
                    disabled={!r.description}
                    onClick={() => setOpenItem((v) => (v === `res:${r.uri}` ? null : `res:${r.uri}`))}
                    className={`min-w-0 flex-1 text-left text-sm font-medium font-mono break-all ${r.description ? 'cursor-pointer' : 'cursor-default'}`}
                  >
                    {r.name || r.uri}
                  </button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-6 shrink-0 gap-1 px-1.5 text-[11px]"
                    onClick={() => setTestingResource({ kind: 'resource', data: r })}
                  >
                    <FlaskConical className="h-3 w-3" />
                    {t('services.testTool')}
                  </Button>
                </div>
                {r.name && <p className="mt-0.5 text-xs font-mono text-muted-foreground break-all">{r.uri}</p>}
                {r.description && openItem === `res:${r.uri}` && <p className="mt-0.5 text-xs text-muted-foreground">{r.description}</p>}
                <div className="mt-0.5 flex flex-wrap gap-1">
                  <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{t('services.resourceKind')}</span>
                  {r.mimeType && <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{r.mimeType}</span>}
                </div>
              </div>
            ))}
            {templates.map((tpl) => (
              <div key={tpl.uriTemplate} className="rounded-lg border border-dashed p-3">
                <div className="flex items-start gap-1">
                  {tpl.description && (openItem === `tpl:${tpl.uriTemplate}`
                    ? <ChevronDown className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    : <ChevronRight className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />)}
                  <button
                    type="button"
                    disabled={!tpl.description}
                    onClick={() => setOpenItem((v) => (v === `tpl:${tpl.uriTemplate}` ? null : `tpl:${tpl.uriTemplate}`))}
                    className={`min-w-0 flex-1 text-left text-sm font-medium font-mono break-all ${tpl.description ? 'cursor-pointer' : 'cursor-default'}`}
                  >
                    {tpl.name || tpl.uriTemplate}
                  </button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-6 shrink-0 gap-1 px-1.5 text-[11px]"
                    onClick={() => setTestingResource({ kind: 'template', data: tpl })}
                  >
                    <FlaskConical className="h-3 w-3" />
                    {t('services.testTool')}
                  </Button>
                </div>
                {tpl.name && <p className="mt-0.5 text-xs font-mono text-muted-foreground break-all">{tpl.uriTemplate}</p>}
                {tpl.description && openItem === `tpl:${tpl.uriTemplate}` && <p className="mt-0.5 text-xs text-muted-foreground">{tpl.description}</p>}
                <div className="mt-0.5 flex flex-wrap gap-1">
                  <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{t('services.resourceTemplate')}</span>
                  {tpl.mimeType && <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{tpl.mimeType}</span>}
                </div>
              </div>
            ))}
          </div>
        )}
      </SectionCard>

      {/* Prompts */}
      <SectionCard title={t('services.promptsList', { count: prompts.length })}>
        {prompts.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noPrompts')}</p>
        ) : (
          <div className="space-y-2">
            {prompts.map((p) => (
              <div key={p.name} className="rounded-lg border p-3">
                <div className="flex items-center gap-1">
                  {p.description && (openItem === `prompt:${p.name}`
                    ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />)}
                  <button
                    type="button"
                    disabled={!p.description}
                    onClick={() => setOpenItem((v) => (v === `prompt:${p.name}` ? null : `prompt:${p.name}`))}
                    className={`min-w-0 flex-1 text-left text-sm font-medium font-mono ${p.description ? 'cursor-pointer' : 'cursor-default'}`}
                  >
                    {p.name}
                  </button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-6 shrink-0 gap-1 px-1.5 text-[11px]"
                    onClick={() => setTestingPrompt(p)}
                  >
                    <FlaskConical className="h-3 w-3" />
                    {t('services.testTool')}
                  </Button>
                </div>
                {p.description && openItem === `prompt:${p.name}` && <p className="mt-0.5 text-xs text-muted-foreground">{p.description}</p>}
                {p.arguments && p.arguments.length > 0 && (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {p.arguments.map((a) => (
                      <span key={a.name} className={`rounded px-1.5 py-0.5 text-[10px] ${a.required ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'}`}>
                        {a.name}{a.required ? '*' : ''}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </SectionCard>

      {/* 工具测试弹窗:市场(平台托管)服务测试按网关价格计费,billable 让弹窗出提示 */}
      <ToolTestDialog
        serviceId={serviceId}
        tool={testingTool}
        billable={service.source === 'marketplace'}
        open={!!testingTool}
        onOpenChange={(v) => !v && setTestingTool(null)}
      />

      {/* 资源/提示测试弹窗 */}
      <ResourceTestDialog
        serviceId={serviceId}
        target={testingResource}
        open={!!testingResource}
        onOpenChange={(v) => !v && setTestingResource(null)}
      />
      <PromptTestDialog
        serviceId={serviceId}
        prompt={testingPrompt}
        open={!!testingPrompt}
        onOpenChange={(v) => !v && setTestingPrompt(null)}
      />
    </div>
  )
}

type EditForm = {
  display_name: string
  description: string
  env: string
  url: string
  auth_type: AuthType
  api_key: string
  bearer_token: string
  custom_header_key: string
  custom_header_value: string
}
