import { useState, useEffect } from 'react'
import { useParams, useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { adminGetMarketplace, adminUpdateMarketplace, adminRefreshMarketplace } from '../api'
import { adminListMarketplaceGroups, adminListMarketplaceTags } from '@/features/admin/marketplace-categories/api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel } from '@/lib/billing'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SectionCard } from '@/components/section-card'
import { ToolItem } from '@/components/tool-params'
import { ResourceItemCard, PromptItemCard } from '@/components/mcp-items'
import { toast } from 'sonner'
import {
  ArrowLeft, Save, RefreshCw, Pencil, X, Check, Loader2, ChevronDown, ChevronRight,
} from 'lucide-react'
import type { MarketplaceDetail, AuthType } from '@/types'

// AdminMarketplaceDetailPage 市场项详情 + 编辑(§11)。上半只读概览,下半编辑表单(调 adminUpdateMarketplace)。
export function AdminMarketplaceDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams({ from: '/_authenticated/admin/marketplace/$id' })
  const queryClient = useQueryClient()
  const { config } = useSystemConfigStore()
  const notSelfUse = !config.selfUseModeEnabled

  const { data, isLoading } = useQuery({
    queryKey: ['admin-marketplace-detail', id],
    queryFn: () => adminGetMarketplace(Number(id)),
  })
  const item: MarketplaceDetail | undefined = data?.data

  const { data: groupsData } = useQuery({
    queryKey: ['admin-marketplace-groups'],
    queryFn: () => adminListMarketplaceGroups(),
  })
  const groups: any[] = groupsData?.data ?? []
  const { data: tagsData } = useQuery({
    queryKey: ['admin-marketplace-tags'],
    queryFn: () => adminListMarketplaceTags({ page: 1, page_size: 200 }),
  })
  const tagsLib: any[] = tagsData?.data ?? []

  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Record<string, unknown> }) => adminUpdateMarketplace(id, body),
    onSuccess: () => {
      toast.success(t('common.success'))
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-detail', id] })
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  // 手动刷新快照(仅平台托管项):用市场项的平台上游配置直连上游,拉取 tools/resources/prompts
  const refreshMutation = useMutation({
    mutationFn: () => adminRefreshMarketplace(Number(id)),
    onSuccess: (res) => {
      const d = res?.data
      // 工具恒显;资源/模板/提示计数为 0 时不进提示文案
      const segs = [t('marketplace.refreshSegTools', { num: d?.tools_count ?? 0 })]
      const add = (key: string, num: number | undefined) => {
        if ((num ?? 0) > 0) segs.push(t(key, { num }))
      }
      add('marketplace.refreshSegResources', d?.resources_count)
      add('marketplace.refreshSegTemplates', d?.templates_count)
      add('marketplace.refreshSegPrompts', d?.prompts_count)
      toast.success(t('marketplace.refreshedPrefix') + segs.join(t('marketplace.refreshSeparator')))
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-detail', id] })
    },
  })

  if (isLoading || !item) {
    return <div className="p-8 text-sm text-muted-foreground">{t('common.loading')}</div>
  }

  // 快照数据(三个列表区块用;旧市场项无快照时为空)
  const tools = item.tools_snapshot || []
  const resources = item.resources_snapshot?.resources || []
  const templates = item.resources_snapshot?.templates || []
  const prompts = item.prompts_snapshot || []
  // 上游握手拿到的真实服务版本(优先于手填的上架版本展示)
  const serverName = typeof item.server_info?.name === 'string' ? item.server_info.name : ''
  const serverVersion = typeof item.server_info?.version === 'string' ? item.server_info.version : ''

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <Button variant="ghost" size="sm" className="gap-1.5 w-fit" onClick={() => navigate({ to: '/admin/marketplace' })}>
        <ArrowLeft className="h-4 w-4" />{t('marketplace.backToMarketplace')}
      </Button>

      {/* 概览(只读);右上角为快照手动刷新(仅平台托管项) */}
      <div className="rounded-xl border bg-card p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-xl font-semibold">{item.display_name || item.name}</h2>
            <p className="mt-1 break-words text-sm text-muted-foreground">
              {item.name} · {serverVersion
                ? <>{serverName && <span>{serverName} · </span>}v{serverVersion}</>
                : <>v{item.version}</>}
            </p>
          </div>
          {item.category === 'instant' && (
            <Button variant="outline" size="sm" className="shrink-0 gap-1.5" disabled={refreshMutation.isPending} onClick={() => refreshMutation.mutate()}>
              <RefreshCw className={`h-3.5 w-3.5 ${refreshMutation.isPending ? 'animate-spin' : ''}`} />
              {t('marketplace.refreshSnapshots')}
            </Button>
          )}
        </div>
        <div className="mt-3 flex flex-wrap gap-2 text-xs">
          <Badge variant="outline">{item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}</Badge>
          {item.group_name && <Badge variant="secondary">{item.group_name}</Badge>}
          {item.tags?.map((tag) => <Badge key={tag} variant="outline" className="font-normal">{tag}</Badge>)}
          <Badge variant={item.billing_type === 'free' ? 'secondary' : 'outline'}>
            {priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}
          </Badge>
          <Badge variant={item.status === 1 ? 'outline' : 'secondary'}
            className={item.status === 1 ? 'text-emerald-600 border-emerald-300' : ''}>
            {item.status === 1 ? t('common.enabled') : t('common.disabled')}
          </Badge>
        </div>
        <div className="mt-3 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div><span className="text-muted-foreground">{t('marketplace.installCount')}</span>: {item.install_count}</div>
          <div><span className="text-muted-foreground">{t('marketplace.rating')}</span>: {item.rating_count > 0 ? item.rating_avg.toFixed(1) : '-'}</div>
          <div><span className="text-muted-foreground">{t('services.transportType')}</span>: {item.transport_type}</div>
          <div><span className="text-muted-foreground">{t('common.createdAt')}</span>: {item.created_at.slice(0, 10)}</div>
          <div><span className="text-muted-foreground">{t('services.protocolVersion')}</span>: {item.protocol_version || '-'}</div>
          <div><span className="text-muted-foreground">{t('marketplace.listingVersion')}</span>: v{item.version}</div>
        </div>
        {item.description && <p className="mt-3 text-sm text-muted-foreground">{item.description}</p>}
      </div>

      {/* 平台上游配置编辑(仅平台托管项):折叠区,展开后可改 URL/鉴权等,与服务详情同款 */}
      {item.category === 'instant' && <UpstreamConfigCard item={item} queryId={id} />}

      {/* 编辑表单 */}
      <EditForm item={item} groups={groups} tagsLib={tagsLib} notSelfUse={notSelfUse}
        onSave={(body) => updateMutation.mutate({ id: Number(id), body })} pending={updateMutation.isPending} />

      {/* 快照展示(与前台市场/服务详情同款),编辑表单下方 */}
      <SectionCard title={t('marketplace.toolsProvided', { count: tools.length })}>
        {tools.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noTools')}</p>
        ) : (
          <div className="space-y-2">
            {tools.map((tool) => (
              <ToolItem key={tool.name} name={tool.name} description={tool.description} schema={tool.inputSchema} />
            ))}
          </div>
        )}
      </SectionCard>

      <SectionCard title={t('marketplace.resourcesProvided', { count: resources.length + templates.length })}>
        {resources.length === 0 && templates.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noResources')}</p>
        ) : (
          <div className="space-y-2">
            {resources.map((r) => (
              <ResourceItemCard key={r.uri} name={r.name} uri={r.uri} description={r.description} mimeType={r.mimeType} />
            ))}
            {templates.map((tpl) => (
              <ResourceItemCard key={tpl.uriTemplate} name={tpl.name} uri={tpl.uriTemplate} description={tpl.description} mimeType={tpl.mimeType} isTemplate />
            ))}
          </div>
        )}
      </SectionCard>

      <SectionCard title={t('marketplace.promptsProvided', { count: prompts.length })}>
        {prompts.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noPrompts')}</p>
        ) : (
          <div className="space-y-2">
            {prompts.map((p) => (
              <PromptItemCard key={p.name} name={p.name} description={p.description} args={p.arguments} />
            ))}
          </div>
        )}
      </SectionCard>
    </div>
  )
}

function EditForm({ item, groups, tagsLib, notSelfUse, onSave, pending }: {
  item: MarketplaceDetail
  groups: any[]
  tagsLib: any[]
  notSelfUse: boolean
  onSave: (body: Record<string, unknown>) => void
  pending: boolean
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState({
    display_name: item.display_name,
    description: item.description,
    icon_url: item.icon_url,
    category: item.category,
    version: item.version,
    group_id: item.group_id ? String(item.group_id) : '',
    repo_url: item.repo_url,
    install_guide: item.install_guide,
    billing_type: item.billing_type,
    price_per_call: String(item.price_per_call),
    status: String(item.status),
  })
  const [selectedTags, setSelectedTags] = useState<string[]>(item.tags ?? [])

  const toggleTag = (name: string) =>
    setSelectedTags((prev) => (prev.includes(name) ? prev.filter((x) => x !== name) : [...prev, name]))

  const submit = () => {
    const billingType = form.billing_type
    const price = parseFloat(form.price_per_call) || 0
    if (price < 0) { toast.error(t('marketplace.priceNegative')); return }
    if (notSelfUse && billingType !== 'free' && price <= 0) { toast.error(t('pricing.commercialNote')); return }
    onSave({
      display_name: form.display_name,
      description: form.description,
      icon_url: form.icon_url,
      category: form.category,
      version: form.version,
      group_id: form.group_id ? Number(form.group_id) : null,
      tags: selectedTags,
      repo_url: form.repo_url,
      install_guide: form.install_guide,
      billing_type: billingType,
      price_per_call: billingType === 'free' ? 0 : price,
      status: Number(form.status),
    })
  }

  return (
    <div className="space-y-4 rounded-xl border bg-card p-5">
      <h3 className="font-semibold">{t('common.edit')}</h3>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>{t('services.displayName')}</Label>
          <Input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label>{t('services.iconUrl')}</Label>
          <Input value={form.icon_url} onChange={(e) => setForm({ ...form, icon_url: e.target.value })} />
        </div>
      </div>
      <div className="space-y-2">
        <Label>{t('services.description')}</Label>
        <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>{t('marketplace.category')}</Label>
          <Select value={form.category} onValueChange={(v) => setForm({ ...form, category: v as 'instant' | 'source' })}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="instant">{t('marketplace.ready')}</SelectItem>
              <SelectItem value="source">{t('marketplace.source')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{t('marketplace.version')}</Label>
          <Input value={form.version} onChange={(e) => setForm({ ...form, version: e.target.value })} />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>{t('categories.groups')}</Label>
          <Select value={form.group_id || '__none__'} onValueChange={(v) => setForm({ ...form, group_id: v === '__none__' ? '' : v })}>
            <SelectTrigger><SelectValue placeholder={t('marketplace.noGroup')} /></SelectTrigger>
            <SelectContent>
              <SelectItem value="__none__">{t('marketplace.noGroup')}</SelectItem>
              {groups.map((g) => <SelectItem key={g.id} value={String(g.id)}>{g.name}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{t('common.status')}</Label>
          <Select value={form.status} onValueChange={(v) => setForm({ ...form, status: v })}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="1">{t('common.enabled')}</SelectItem>
              <SelectItem value="2">{t('common.disabled')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      {form.category === 'source' && (
        <div className="space-y-4 rounded-lg border border-dashed bg-muted/20 p-4">
          <p className="text-xs text-muted-foreground">{t('marketplace.sourceFieldsHint')}</p>
          <div className="space-y-2">
            <Label>{t('marketplace.repoUrl')}</Label>
            <Input value={form.repo_url} onChange={(e) => setForm({ ...form, repo_url: e.target.value })} placeholder="https://github.com/..." />
          </div>
          <div className="space-y-2">
            <Label>{t('marketplace.installGuide')}</Label>
            <Textarea rows={4} value={form.install_guide} onChange={(e) => setForm({ ...form, install_guide: e.target.value })} />
          </div>
        </div>
      )}
      <div className="space-y-2">
        <Label>{t('marketplace.tags')}</Label>
        <div className="flex flex-wrap gap-2">
          {tagsLib.length === 0 && <p className="text-xs text-muted-foreground">{t('marketplace.noTagsHint')}</p>}
          {tagsLib.map((tag) => {
            const selected = selectedTags.includes(tag.name)
            return (
              <button key={tag.id} type="button" onClick={() => toggleTag(tag.name)}
                className={`rounded-full border px-3 py-1 text-xs transition-colors ${selected ? 'border-primary bg-primary text-primary-foreground' : 'bg-muted/40 hover:bg-muted'}`}>
                {tag.name}
              </button>
            )
          })}
        </div>
      </div>
      <div className="grid grid-cols-2 gap-4 border-t pt-4">
        <div className="space-y-2">
          <Label>{t('marketplace.billingType')}</Label>
          <Select value={form.billing_type} onValueChange={(v) => setForm({ ...form, billing_type: v, price_per_call: v === 'free' ? '0' : form.price_per_call })}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="per_call">{t('marketplace.billingPerCall')}</SelectItem>
              <SelectItem value="free">{t('marketplace.billingFree')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>{t('marketplace.pricePerCall')}</Label>
          <Input type="number" min="0" step="0.0001" disabled={form.billing_type === 'free'}
            value={form.price_per_call} onChange={(e) => setForm({ ...form, price_per_call: e.target.value })} />
        </div>
      </div>
      <div className="flex justify-end">
        <Button className="gap-2" disabled={pending} onClick={submit}><Save className="h-4 w-4" />{t('common.save')}</Button>
      </div>
    </div>
  )
}

// --- 平台上游配置(instant 项)折叠编辑区 ---

// 与服务详情一致:环境变量用「每行 KEY=value」文本表示,而非 JSON
function parseEnvText(text: string): Record<string, string> {
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

function envToText(env: Record<string, unknown> | undefined | null): string {
  if (!env || typeof env !== 'object') return ''
  return Object.entries(env)
    .map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join('\n')
}

type UpstreamForm = {
  url: string
  env: string
  auth_type: AuthType
  api_key: string
  bearer_token: string
  custom_header_key: string
  custom_header_value: string
}

// UpstreamConfigCard 平台托管项的上游连接配置:默认折叠,展开可查看并以服务详情同款交互编辑
// URL/鉴权(stdio 为环境变量)。保存调 adminUpdateMarketplace({config_template}),
// 后端踢掉该市场项全部引用服务的旧会话,新调用按新配置重连。
function UpstreamConfigCard({ item, queryId }: { item: MarketplaceDetail; queryId: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<UpstreamForm>({
    url: '', env: '', auth_type: 'none',
    api_key: '', bearer_token: '', custom_header_key: '', custom_header_value: '',
  })

  const cfg = (item.config_template || {}) as Record<string, unknown>
  const isStdio = item.transport_type === 'stdio'

  const updateMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => adminUpdateMarketplace(item.id, body),
    onSuccess: () => {
      toast.success(t('common.success'))
      setEditing(false)
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-detail', queryId] })
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  // 非编辑态下把上游配置反向解析回表单(认证方式从 headers 推断),便于编辑预填
  useEffect(() => {
    if (editing) return
    const headers = (cfg.headers as Record<string, string>) || {}
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
      url: typeof cfg.url === 'string' ? cfg.url : '',
      env: envToText(cfg.env as Record<string, unknown> | undefined),
      auth_type: authType,
      api_key: apiKey,
      bearer_token: bearerToken,
      custom_header_key: customKey,
      custom_header_value: customValue,
    })
  }, [item, editing])

  // 与服务详情 buildConfig 同款:未填新凭据时保留原 headers,切回 none 才清空;
  // stdio 的命令/参数不可编辑,沿用原配置仅改环境变量
  function buildConfig(): Record<string, unknown> {
    const originalHeaders = (cfg.headers as Record<string, string>) || {}
    const hasNewAuth =
      (form.auth_type === 'api_key' && !!form.api_key) ||
      (form.auth_type === 'bearer' && !!form.bearer_token) ||
      (form.auth_type === 'custom' && !!form.custom_header_key)

    let headers: Record<string, string>
    if (form.auth_type === 'none') {
      headers = {}
    } else if (hasNewAuth) {
      headers = {}
      if (form.auth_type === 'api_key') headers['X-API-Key'] = form.api_key
      else if (form.auth_type === 'bearer') headers['Authorization'] = `Bearer ${form.bearer_token}`
      else if (form.auth_type === 'custom' && form.custom_header_key) headers[form.custom_header_key] = form.custom_header_value
    } else {
      headers = { ...originalHeaders }
    }

    switch (item.transport_type) {
      case 'stdio':
        return { command: cfg.command, args: Array.isArray(cfg.args) ? cfg.args : [], env: parseEnvText(form.env) }
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

  const authOptions: { value: AuthType; label: string }[] = [
    { value: 'none', label: t('services.authNone') },
    { value: 'api_key', label: t('services.authApiKey') },
    { value: 'bearer', label: t('services.authBearer') },
    { value: 'custom', label: t('services.authCustom') },
  ]

  const canSave = isStdio ? !!cfg.command : form.url.trim().length > 0

  return (
    <div className="rounded-xl border bg-card">
      <button type="button" className="flex w-full items-center gap-2 p-5 text-left"
        onClick={() => setOpen((v) => !v)}>
        {open
          ? <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
          : <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />}
        <h3 className="font-semibold">{t('marketplace.upstreamConfig')}</h3>
        <span className="truncate text-xs text-muted-foreground">
          {isStdio ? String(cfg.command || '-') : form.url || t('services.serviceUrlRequired')}
        </span>
      </button>
      {open && (
        <div className="space-y-4 border-t p-5">
          <p className="text-xs text-muted-foreground">{t('marketplace.upstreamConfigHint')}</p>
          <p className="text-xs text-muted-foreground">{t('marketplace.upstreamMaskHint')}</p>
          <div className="grid gap-4 sm:grid-cols-2">
            {isStdio ? (
              <>
                <div className="space-y-1.5">
                  <Label className="text-xs text-muted-foreground">{t('services.commandRequired')}</Label>
                  <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{String(cfg.command || '-')}</code>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs text-muted-foreground">{t('services.args')}</Label>
                  <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">
                    {Array.isArray(cfg.args) ? (cfg.args as unknown[]).join(' ') : '-'}
                  </code>
                </div>
                <div className="space-y-1.5 sm:col-span-2">
                  <Label className="text-xs text-muted-foreground">{t('services.create.envLabel')}</Label>
                  {editing ? (
                    <>
                      <Textarea rows={4} value={form.env} onChange={(e) => setForm({ ...form, env: e.target.value })}
                        placeholder={'API_KEY=xxx\nNODE_ENV=production'} />
                      <p className="text-xs text-muted-foreground">{t('services.create.envHint')}</p>
                    </>
                  ) : (
                    <code className="block whitespace-pre-wrap text-sm break-all rounded-md bg-muted/50 px-3 py-2">{form.env || '-'}</code>
                  )}
                </div>
              </>
            ) : (
              <div className="space-y-1.5 sm:col-span-2">
                <Label className="text-xs text-muted-foreground">{t('services.serviceUrlRequired')}</Label>
                {editing ? (
                  <Input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://example.com/mcp" />
                ) : (
                  <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{form.url || '-'}</code>
                )}
              </div>
            )}

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
                <p className="text-sm">{authOptions.find((o) => o.value === form.auth_type)?.label || t('services.authNone')}</p>
              )}
            </div>

            {editing && form.auth_type === 'api_key' && (
              <div className="space-y-1.5 sm:col-span-2">
                <Label className="text-xs text-muted-foreground">API Key</Label>
                <Input value={form.api_key} onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                  placeholder={t('services.placeholderKeepUnchanged')} autoComplete="off" />
              </div>
            )}
            {editing && form.auth_type === 'bearer' && (
              <div className="space-y-1.5 sm:col-span-2">
                <Label className="text-xs text-muted-foreground">Token</Label>
                <Input value={form.bearer_token} onChange={(e) => setForm({ ...form, bearer_token: e.target.value })}
                  placeholder={t('services.placeholderKeepUnchanged')} autoComplete="off" />
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
          </div>

          {editing ? (
            <div className="flex justify-end gap-2 border-t pt-4">
              <Button variant="outline" size="sm" onClick={() => setEditing(false)}>
                <X className="h-3.5 w-3.5 mr-1.5" />{t('common.cancel')}
              </Button>
              <Button size="sm" onClick={() => updateMutation.mutate({ config_template: buildConfig() })}
                disabled={!canSave || updateMutation.isPending}>
                {updateMutation.isPending
                  ? <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                  : <Check className="h-3.5 w-3.5 mr-1.5" />}
                {t('common.save')}
              </Button>
            </div>
          ) : (
            <div className="flex justify-end border-t pt-4">
              <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
                <Pencil className="h-3.5 w-3.5 mr-1.5" />{t('common.edit')}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
