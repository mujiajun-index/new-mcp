import { useState, useEffect } from 'react'
import { useParams, useNavigate, useRouter } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { adminGetMarketplace, adminUpdateMarketplace, adminRefreshMarketplace, adminGetMarketplaceProcess, adminControlMarketplaceProcess, adminSetEntryPrices } from '../api'
import { EntryPriceControl, EntryPricingBar, useEntryPricingDraft } from './entry-pricing'
import { adminListMarketplaceGroups, adminListMarketplaceTags } from '@/features/admin/marketplace-categories/api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel } from '@/lib/billing'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Progress } from '@/components/ui/progress'
import { LocalPager } from '@/components/local-pager'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger,
} from '@/components/ui/dialog'
import { SectionCard } from '@/components/section-card'
import { ToolItem } from '@/components/tool-params'
import { ResourceItemCard, PromptItemCard } from '@/components/mcp-items'
import { toast } from 'sonner'
import {
  ArrowLeft, Save, RefreshCw, Pencil, X, Check, Loader2, ChevronDown, ChevronRight,
  Activity, Power, Play, Ban, Square, RotateCw, Layers, MemoryStick, Search, User, Users, Download,
} from 'lucide-react'
import type { MarketplaceDetail, MarketplaceEntryPrice, AuthType, MarketplaceItemProcess, MarketplaceItemProcessInstance, ProcessControlAction } from '@/types'

// AdminMarketplaceDetailPage 市场项详情 + 编辑(§11)。上半只读概览,下半编辑表单(调 adminUpdateMarketplace)。
export function AdminMarketplaceDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const router = useRouter()
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
  // 仅启用分组:禁用分组不在启用字典,选中后保存会被后端 cleanAndValidateGroupIDs 拒绝
  const allGroups: any[] = groupsData?.data ?? []
  const groups = allGroups.filter((g) => g.status === 1)
  const { data: tagsData } = useQuery({
    queryKey: ['admin-marketplace-tags'],
    // 仅启用标签:禁用标签不在启用字典,选中后保存会被后端 validateTags 拒绝
    queryFn: () => adminListMarketplaceTags({ page: 1, page_size: 200, status: 1 }),
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

  // 右上角启用/禁用(与列表页行内开关同口径):复用 adminUpdateMarketplace({status})。
  // 非自用模式下启用未定价项会被后端 requireExplicitPricingIfNotSelfUse 拒绝,错误经全局拦截器 toast。
  const statusMutation = useMutation({
    mutationFn: (status: number) => adminUpdateMarketplace(Number(id), { status }),
    onSuccess: () => {
      toast.success(t('common.success'))
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-detail', id] })
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  // 条目级定价草稿(工具/资源/提示逐条设价,三个列表共享);item 加载前为空态
  const entryPricing = useEntryPricingDraft(item)
  const entryMutation = useMutation({
    mutationFn: (prices: MarketplaceEntryPrice[]) => adminSetEntryPrices(Number(id), prices),
    onSuccess: () => {
      toast.success(t('common.success'))
      entryPricing.reset()
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-detail', id] })
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
  // 上游握手拿到的真实服务版本(名称下方标识行展示;名称行跟手填的上架版本)
  const serverName = typeof item.server_info?.name === 'string' ? item.server_info.name : ''
  const serverVersion = typeof item.server_info?.version === 'string' ? item.server_info.version : ''

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <Button variant="ghost" size="sm" className="gap-1.5 w-fit" onClick={() => {
        // 真回退:回到来路(市场管理列表/健康页等);新标签直开无历史时兜底回市场管理
        if (router.history.canGoBack()) router.history.back()
        else navigate({ to: '/admin/marketplace' })
      }}>
        <ArrowLeft className="h-4 w-4" />{t('common.back')}
      </Button>

      {/* 概览(只读);右上角为快照手动刷新(仅平台托管项) */}
      <div className="rounded-xl border bg-card p-5">
        <div className="flex items-start justify-between gap-3">
          {/* 头部与广场卡片/前台详情同布局:图标 + 名称行(部署形态标识、版本号紧跟名称) */}
          <div className="flex min-w-0 items-start gap-4">
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-primary/10 text-primary">
              {item.icon_url ? (
                <img src={item.icon_url} alt="" className="h-8 w-8" />
              ) : (
                <span className="text-2xl font-bold">{(item.display_name || item.name).charAt(0)}</span>
              )}
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="text-xl font-semibold">{item.display_name || item.name}</h2>
                <span className={`shrink-0 rounded px-1.5 py-0.5 text-xs font-medium ${
                  item.category === 'instant' ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
                }`}>
                  {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
                </span>
                {/* 版本号跟手填上架版本(与前台详情一致);上游真实版本在统计格展示 */}
                {item.version && <span className="shrink-0 text-xs text-muted-foreground">v{item.version}</span>}
              </div>
              {/* 标识行:英文标识(内部名)· 服务端名 · 真实版本(上游握手所得) */}
              <p className="mt-1 break-words text-sm text-muted-foreground">
                {item.name}{serverName && <> · {serverName}</>}{serverVersion && <> · v{serverVersion}</>}
              </p>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              variant="outline" size="sm" className="gap-1.5" disabled={statusMutation.isPending}
              onClick={() => {
                if (item.status === 1) {
                  if (confirm(t('marketplace.disableConfirm', { name: item.display_name || item.name }))) {
                    statusMutation.mutate(2)
                  }
                } else {
                  statusMutation.mutate(1)
                }
              }}
            >
              {item.status === 1
                ? <><Ban className="h-3.5 w-3.5" />{t('marketplace.disable')}</>
                : <><Play className="h-3.5 w-3.5" />{t('marketplace.enable')}</>}
            </Button>
            {item.category === 'instant' && (
              <Button variant="outline" size="sm" className="gap-1.5" disabled={refreshMutation.isPending} onClick={() => refreshMutation.mutate()}>
                <RefreshCw className={`h-3.5 w-3.5 ${refreshMutation.isPending ? 'animate-spin' : ''}`} />
                {t('marketplace.refreshSnapshots')}
              </Button>
            )}
          </div>
        </div>
        {/* 徽标行只剩分组/标签 + 安装数(图标形式,与前台详情头部一致);价格下沉统计格,
            启用/禁用状态右上角按钮已表达,不再重复出徽标 */}
        <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
          {item.group_names?.map((name) => <Badge key={name} variant="secondary">{name}</Badge>)}
          {item.tags?.map((tag) => {
            const color = tagsLib.find((t) => t.name === tag)?.color
            return <Badge key={tag} variant="outline" className="font-normal"
              style={color ? { color, backgroundColor: `${color}1A`, borderColor: `${color}55` } : undefined}>{tag}</Badge>
          })}
          <span className="flex items-center gap-1 text-muted-foreground">
            <Download className="h-3 w-3" />{item.install_count}
          </span>
        </div>
        <div className="mt-3 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div><span className="text-muted-foreground">{t('marketplace.price')}</span>: {priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}</div>
          <div><span className="text-muted-foreground">{t('services.transportType')}</span>: {item.transport_type}</div>
          <div><span className="text-muted-foreground">{t('common.createdAt')}</span>: {item.created_at.slice(0, 10)}</div>
          <div><span className="text-muted-foreground">{t('services.protocolVersion')}</span>: {item.protocol_version || '-'}</div>
        </div>
        {item.description && <p className="mt-3 text-sm text-muted-foreground">{item.description}</p>}
      </div>

      {/* 平台上游配置编辑(仅平台托管项):折叠区,展开后可改 URL/鉴权等,与服务详情同款 */}
      {item.category === 'instant' && <UpstreamConfigCard item={item} queryId={id} />}

      {/* stdio 平台托管项的进程视图与启停:共享=平台唯一进程;独占=按安装用户逐行 */}
      {item.category === 'instant' && item.transport_type === 'stdio' && (
        <MarketplaceProcessCard item={item} queryId={id} />
      )}

      {/* 编辑表单 */}
      <EditForm item={item} groups={groups} tagsLib={tagsLib} notSelfUse={notSelfUse}
        onSave={(body) => updateMutation.mutate({ id: Number(id), body })} pending={updateMutation.isPending} />

      {/* 快照展示(与前台市场/服务详情同款)+ 条目级定价(每条右侧),编辑表单下方。
          保存为全量替换:不在载荷中的条目按缺省回退(工具→服务统一价,资源/提示→免费);
          资源/提示显式继承服务价以 inherit 行入载荷。 */}
      <p className="text-xs text-muted-foreground">{t('marketplace.entryPricingHint')}</p>

      <SectionCard title={t('marketplace.toolsProvided', { count: tools.length })}>
        {tools.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noTools')}</p>
        ) : (
          <div className="space-y-2">
            {tools.map((tool) => (
              <ToolItem key={tool.name} name={tool.name} description={tool.description} schema={tool.inputSchema}
                action={
                  <EntryPriceControl value={entryPricing.getEntry('tool', tool.name)}
                    onChange={(v) => entryPricing.setEntry('tool', tool.name, v)} />
                } />
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
              <ResourceItemCard key={r.uri} name={r.name} uri={r.uri} description={r.description} mimeType={r.mimeType}
                action={
                  <EntryPriceControl value={entryPricing.getEntry('resource', r.uri)}
                    onChange={(v) => entryPricing.setEntry('resource', r.uri, v)} />
                } />
            ))}
            {/* 模板不可定价:读取按展开后的具体 URI 走资源条目价,模板价无意义 */}
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
              <PromptItemCard key={p.name} name={p.name} description={p.description} args={p.arguments}
                action={
                  <EntryPriceControl value={entryPricing.getEntry('prompt', p.name)}
                    onChange={(v) => entryPricing.setEntry('prompt', p.name, v)} />
                } />
            ))}
          </div>
        )}
      </SectionCard>

      <EntryPricingBar dirtyCount={entryPricing.dirtyCount} pending={entryMutation.isPending}
        onCancel={entryPricing.reset}
        onSave={() => {
          const payload = entryPricing.buildPayload()
          if (payload === null) { toast.error(t('marketplace.entryPriceRequired')); return }
          entryMutation.mutate(payload)
        }} />
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
    repo_url: item.repo_url,
    install_guide: item.install_guide,
    billing_type: item.billing_type,
    price_per_call: String(item.price_per_call),
  })
  const [selectedTags, setSelectedTags] = useState<string[]>(item.tags ?? [])
  const [selectedGroups, setSelectedGroups] = useState<number[]>(item.group_ids ?? [])

  const toggleTag = (name: string) =>
    setSelectedTags((prev) => (prev.includes(name) ? prev.filter((x) => x !== name) : [...prev, name]))

  const toggleGroup = (id: number) =>
    setSelectedGroups((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))

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
      // 只提交字典内(启用)的分组:历史遗留的失效绑定(分组曾被禁用/删除)静默丢弃,顺带修复存量行
      group_ids: selectedGroups.filter((id) => groups.some((g) => g.id === id)),
      // 只提交字典内(启用)的标签:历史遗留的失效名(标签曾被改名/删除)静默丢弃,顺带修复存量行
      tags: selectedTags.filter((name) => tagsLib.some((tag) => tag.name === name)),
      repo_url: form.repo_url,
      install_guide: form.install_guide,
      billing_type: billingType,
      price_per_call: billingType === 'free' ? 0 : price,
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
      <div className="space-y-2">
        <Label>{t('categories.groups')}</Label>
        <div className="flex flex-wrap gap-2">
          {groups.length === 0 && <p className="text-xs text-muted-foreground">{t('marketplace.noGroupsHint')}</p>}
          {groups.map((g) => {
            const selected = selectedGroups.includes(g.id)
            return (
              <button key={g.id} type="button" onClick={() => toggleGroup(g.id)}
                className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-all ${
                  selected ? 'border-primary bg-primary text-primary-foreground shadow-sm' : 'bg-muted/40 hover:bg-muted'}`}>
                {selected && <Check className="h-3 w-3 shrink-0" />}
                {g.name}
              </button>
            )
          })}
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
                className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-all ${
                  selected
                    ? tag.color ? 'shadow-sm' : 'border-primary bg-primary text-primary-foreground'
                    : tag.color ? 'hover:opacity-80' : 'bg-muted/40 hover:bg-muted'}`}
                style={selected && tag.color
                  ? { color: tag.color, backgroundColor: `${tag.color}33`, borderColor: tag.color }
                  : !selected && tag.color
                    ? { color: tag.color, backgroundColor: `${tag.color}1A`, borderColor: `${tag.color}55` }
                    : undefined}>
                {selected
                  ? <Check className="h-3 w-3 shrink-0" />
                  : tag.color && <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ backgroundColor: tag.color }} />}
                {tag.name}
              </button>
            )
          })}
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

// --- stdio 条目进程(共享=平台唯一进程 / 独占=按安装用户逐行) ---

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function formatUptime(seconds?: number): string {
  if (!seconds || seconds <= 0) return '0s'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

// 条目级进程启停:与自有服务 StdioProcessControl 同款电源弹框,但调市场条目端点。
// serviceId>0 = 独占模式操作该安装引用行;共享模式忽略 serviceId(操作平台唯一进程)。
// colored:独占实例卡传入,按钮按进程实际状态着色(绿=运行中/红=已停止),对齐总览卡。
function MarketplaceProcessControl({
  itemId, queryId, running, serviceId, colored,
}: {
  itemId: number
  queryId: string
  running: boolean
  serviceId?: number
  colored?: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)

  const controlMutation = useMutation({
    mutationFn: (action: ProcessControlAction) => adminControlMarketplaceProcess(itemId, action, serviceId),
    onSuccess: (_res, action) => {
      const successKey: Record<ProcessControlAction, string> = {
        start: 'services.processStartSuccess',
        stop: 'services.processStopSuccess',
        restart: 'services.processRestartSuccess',
      }
      toast.success(t(successKey[action]))
      setOpen(false)
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-process', queryId] })
    },
  })

  const actions: Array<{
    key: ProcessControlAction
    label: string
    icon: React.ComponentType<{ className?: string }>
    disabled: boolean
    danger?: boolean
    positive?: boolean
  }> = [
    { key: 'start', label: t('services.processStart'), icon: Play, disabled: running, positive: true },
    { key: 'stop', label: t('services.processStop'), icon: Square, disabled: !running, danger: true },
    { key: 'restart', label: t('services.processRestart'), icon: RotateCw, disabled: !running },
  ]

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!controlMutation.isPending) setOpen(v) }}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className={`h-7 w-7 shrink-0 ${
            !colored
              ? 'text-muted-foreground'
              : running
                ? 'text-emerald-500 hover:bg-emerald-500/10 hover:text-emerald-600'
                : 'text-destructive hover:bg-destructive/10 hover:text-destructive'
          }`}
          title={t('services.processControl')}
          aria-label={t('services.processControl')}
        >
          <Power className="h-4 w-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>{t('services.processControl')}</DialogTitle>
          <DialogDescription>{t('services.processControlHint')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          {actions.map(({ key, label, icon: Icon, disabled, danger, positive }) => (
            <Button
              key={key}
              variant="outline"
              className={`w-full justify-start gap-2 ${danger ? 'text-destructive hover:text-destructive' : ''} ${
                positive ? 'text-emerald-600 hover:text-emerald-600 dark:text-emerald-400 dark:hover:text-emerald-400' : ''
              }`}
              disabled={disabled || controlMutation.isPending}
              onClick={() => controlMutation.mutate(key)}
            >
              {controlMutation.isPending && controlMutation.variables === key
                ? <Loader2 className="h-4 w-4 animate-spin" />
                : <Icon className="h-4 w-4" />}
              {label}
            </Button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// 进程模式段选(共享↔独占):切换调 adminUpdateMarketplace({isolated_process}),
// 后端踢掉该条目全部池内会话按新模式重建(共享进程收敛为 1 或裂解为每用户 1)。
function ProcessModeSegment({ item, queryId }: { item: MarketplaceDetail; queryId: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const current = item.isolated_process ?? false

  const updateMutation = useMutation({
    mutationFn: (isolated: boolean) => adminUpdateMarketplace(item.id, { isolated_process: isolated }),
    onSuccess: () => {
      toast.success(t('common.success'))
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-detail', queryId] })
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-process', queryId] })
    },
  })

  const switchTo = (isolated: boolean) => {
    if (isolated === current) return
    if (!confirm(t('marketplace.processModeSwitchConfirm'))) return
    updateMutation.mutate(isolated)
  }

  return (
    <div className="space-y-2 border-b pb-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{t('marketplace.processMode')}</span>
        {([false, true] as const).map((isolated) => (
          <button
            key={String(isolated)}
            type="button"
            onClick={() => switchTo(isolated)}
            disabled={updateMutation.isPending}
            className={`rounded-lg border px-3 py-1 text-xs transition-all ${
              current === isolated ? 'border-primary bg-primary/5' : 'hover:border-primary/30'
            }`}
          >
            {isolated ? t('marketplace.modeIsolated') : t('marketplace.modeShared')}
          </button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {current ? t('marketplace.modeIsolatedHint') : t('marketplace.modeSharedHint')}
      </p>
    </div>
  )
}

// MarketplaceProcessCard stdio 平台托管项的进程卡片:共享模式=平台唯一进程(与服务
// 详情同款 4 格资源快照 + 启停);独占模式=按安装用户逐行枚举(参考总览 stdio 卡,
// 含未安装连接的行,未运行固定形态)。5s 轮询,只读现状不拉起进程。
function MarketplaceProcessCard({ item, queryId }: { item: MarketplaceDetail; queryId: string }) {
  const { t } = useTranslation()
  const { data: procData } = useQuery({
    queryKey: ['admin-marketplace-process', queryId],
    queryFn: () => adminGetMarketplaceProcess(item.id),
    refetchInterval: 5000,
  })
  const proc: MarketplaceItemProcess | undefined = procData?.data
  const isolated = proc?.isolated ?? item.isolated_process ?? false

  return (
    <SectionCard
      title={t('marketplace.processCardTitle')}
      actions={
        isolated ? (
          <Badge variant="secondary">{t('marketplace.modeIsolated')}</Badge>
        ) : (
          <div className="flex items-center gap-1.5">
            {item.status === 1 && (
              <MarketplaceProcessControl itemId={item.id} queryId={queryId} running={!!proc?.shared?.running} />
            )}
            <Badge variant={proc?.shared?.running ? 'success' : 'secondary'}>
              {proc?.shared?.running ? t('services.processRunning') : t('services.processStopped')}
            </Badge>
            <Badge variant="secondary">{t('marketplace.modeShared')}</Badge>
          </div>
        )
      }
    >
      <ProcessModeSegment item={item} queryId={queryId} />

      {!isolated ? (
        (() => {
          const shared = proc?.shared
          return shared?.running ? (
            <div className="space-y-3 pt-4">
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                {[
                  { label: t('services.memoryUsage'), value: formatBytes(shared.memory_rss_bytes), sub: t('services.processCount', { count: shared.process_count || 1 }) },
                  { label: t('services.cpuUsage'), value: `${(shared.cpu_percent ?? 0).toFixed(1)}%`, sub: null },
                  { label: t('services.uptime'), value: formatUptime(shared.uptime_seconds), sub: null },
                  { label: 'PID', value: String(shared.pid ?? '-'), sub: null },
                ].map((tile) => (
                  <div key={tile.label} className="rounded-xl border bg-card p-4">
                    <p className="text-xs text-muted-foreground">{tile.label}</p>
                    <p className="mt-1 text-lg font-semibold tabular-nums">{tile.value}</p>
                    {tile.sub && <p className="mt-0.5 text-xs text-muted-foreground">{tile.sub}</p>}
                  </div>
                ))}
              </div>
              {shared.command && (
                <p className="truncate font-mono text-xs text-muted-foreground" title={shared.command}>
                  {shared.command}
                </p>
              )}
            </div>
          ) : (
            <div className="flex flex-col items-center py-8 text-center">
              <Activity className="h-8 w-8 text-muted-foreground/30 mb-2" />
              <p className="text-sm text-muted-foreground">{t('services.processStopped')}</p>
              <p className="mt-1 text-xs text-muted-foreground/60">{t('services.processNotRunningHint')}</p>
            </div>
          )
        })()
      ) : (proc?.total ?? 0) === 0 ? (
        <p className="py-6 text-center text-sm text-muted-foreground">{t('marketplace.noInstalledInstances')}</p>
      ) : (
        <div className="flex flex-col gap-3 pt-4 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">
            {t('services.overview.runningSub', { count: proc?.running_instances ?? 0 })}
            {' · '}
            {t('common.total')} {proc?.total} {t('common.items')}
          </p>
          <InstancesDialog item={item} queryId={queryId} count={proc?.total ?? 0} />
        </div>
      )}
    </SectionCard>
  )
}

// InstancesDialog 独占条目的安装实例弹窗:总览风格——顶部 总进程/内存占用/CPU 三张
// 概述卡(服务端按全部运行实例合计,CPU 为各实例之和、多核可超 100%),下方总览
// 同款工具栏(左标题/右用户名筛选+条数)+ 实例卡网格。列表为**服务端分页**
// (每页 18 条,万级安装也只拉一页;username 筛选在服务端执行),弹窗开着时随
// 5s 轮询刷新;启停失效同键前缀,当前页/筛选保持。
function InstancesDialog({
  item, queryId, count,
}: {
  item: MarketplaceDetail
  queryId: string
  count: number
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const PAGE_SIZE = 18

  // 子键挂在父查询键下:启停/模式切换失效 ['admin-marketplace-process', queryId]
  // 前缀即同时刷新本弹窗;弹窗关闭时停轮询
  const { data: procData } = useQuery({
    queryKey: ['admin-marketplace-process', queryId, 'instances', page, search],
    queryFn: () => adminGetMarketplaceProcess(item.id, {
      page, page_size: PAGE_SIZE, username: search.trim() || undefined,
    }),
    refetchInterval: 5000,
    enabled: open,
  })
  const proc: MarketplaceItemProcess | undefined = procData?.data
  const instances = proc?.instances ?? []
  const total = proc?.total ?? 0
  const totalPages = proc?.total_pages ?? 1

  // 数据刷新后当前页越界(如筛选后总页数变少)时夹紧到最后一页
  useEffect(() => {
    if (proc && (proc.total ?? 0) > 0 && page > totalPages && totalPages >= 1) {
      setPage(totalPages)
    }
  }, [proc, page, totalPages])

  const tiles = [
    { label: t('services.overview.processes'), value: String(proc?.total_processes ?? 0), sub: t('services.overview.runningSub', { count: proc?.running_instances ?? 0 }), icon: Layers, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
    { label: t('services.overview.memoryUsage'), value: formatBytes(proc?.memory_bytes ?? 0), sub: null, icon: MemoryStick, color: 'text-amber-500', bg: 'bg-amber-500/10' },
    { label: t('services.overview.cpuUsage'), value: `${(proc?.cpu_percent_total ?? 0).toFixed(1)}%`, sub: t('services.overview.cpuHint'), icon: Activity, color: 'text-rose-500', bg: 'bg-rose-500/10' },
  ]

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <Users className="h-3.5 w-3.5" />
          {t('marketplace.viewInstances', { count })}
        </Button>
      </DialogTrigger>
      {/* 大屏弹窗:内容宽约 1472px,230px 最小卡宽 + 12px 间距正好容 6 列×3 行=18 卡 */}
      <DialogContent className="max-w-[95rem]">
        <DialogHeader>
          <DialogTitle>{t('marketplace.installedInstancesTitle')}</DialogTitle>
          <DialogDescription>{t('marketplace.installedInstancesDesc')}</DialogDescription>
        </DialogHeader>

        {/* 概述三卡:服务端按全部运行实例合计 */}
        <div className="grid gap-3 sm:grid-cols-3">
          {tiles.map((tile) => (
            <div key={tile.label} className="rounded-xl border bg-card p-4" title={tile.sub ?? undefined}>
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="text-xs text-muted-foreground">{tile.label}</p>
                  <p className="mt-1 text-2xl font-semibold tabular-nums">{tile.value}</p>
                </div>
                <div className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${tile.bg}`}>
                  <tile.icon className={`h-4 w-4 ${tile.color}`} />
                </div>
              </div>
            </div>
          ))}
        </div>

        {/* 实例工具栏:总览同款——左侧列表标题,右侧搜索框 + 条数 */}
        <div className="mt-2 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h3 className="text-sm font-semibold">{t('marketplace.instanceListTitle')}</h3>
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-full sm:w-56">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t('marketplace.filterByUsername')}
                value={search}
                onChange={(e) => { setSearch(e.target.value); setPage(1) }}
                className="h-9 pl-9"
              />
            </div>
            <span className="text-xs text-muted-foreground">
              {t('common.total')} {total} {t('common.items')}
            </span>
          </div>
        </div>

        {/* 实例卡网格(服务端分页,每页 18 条);未安装与筛选无结果区分提示 */}
        {total === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-xl border bg-card py-10 text-center">
            <Users className="mb-2 h-8 w-8 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">
              {search ? t('marketplace.noMatchInstances') : t('marketplace.noInstalledInstances')}
            </p>
          </div>
        ) : (
          <>
            <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(min(230px,100%),1fr))]">
              {instances.map((inst) => (
                <InstanceProcessCard
                  key={inst.service_id}
                  inst={inst}
                  item={item}
                  queryId={queryId}
                  totalMem={proc?.memory_bytes ?? 0}
                />
              ))}
            </div>
            {totalPages > 1 && (
              <LocalPager total={total} page={proc?.page ?? page} totalPages={totalPages} onPage={setPage} />
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

// InstanceProcessCard 独占条目下单个安装用户(引用行)的实例卡:总览 stdio 卡同款——
// 状态圆点(引用行启用/禁用)+ 名称 + 电源启停(colored)、用户名行 + 运行徽章、
// CPU/内存两行进度条(内存条按本条目运行实例合计占比,无全局内存分母)。
// 名称不做链接(他人引用行无管理详情页可跳)。用户自行禁用的行不可拉起。
function InstanceProcessCard({
  inst, item, queryId, totalMem,
}: {
  inst: MarketplaceItemProcessInstance
  item: MarketplaceDetail
  queryId: string
  totalMem: number
}) {
  const { t } = useTranslation()
  const running = inst.stat.running
  const cpu = running ? (inst.stat.cpu_percent ?? 0) : 0
  const mem = running ? (inst.stat.memory_rss_bytes ?? 0) : 0
  const memPct = totalMem > 0 ? (mem / totalMem) * 100 : 0

  return (
    <div className="space-y-3 rounded-xl border bg-card p-4 transition-all duration-200 hover:border-ring/20 hover:shadow-md hover:shadow-black/[0.03]">
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span
            className={`h-2 w-2 shrink-0 rounded-full ${inst.status === 1 ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-600'}`}
            title={inst.status === 1 ? t('common.enabled') : t('common.disabled')}
          />
          <span className="min-w-0 truncate text-sm font-medium" title={inst.name}>{inst.name}</span>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {item.status === 1 && inst.status === 1 && (
            <MarketplaceProcessControl itemId={item.id} queryId={queryId} running={running} serviceId={inst.service_id} colored />
          )}
        </div>
      </div>

      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="inline-flex min-w-0 items-center gap-1 truncate">
          <User className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{inst.username || `UID ${inst.user_id}`}</span>
        </span>
        <Badge variant={running ? 'success' : 'secondary'} className="shrink-0 text-[10px]">
          {running ? t('services.processRunning') : t('services.processStopped')}
        </Badge>
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">{t('services.overview.cpuRow')}</span>
          <span className="tabular-nums">{running && inst.stat.cpu_percent != null ? `${inst.stat.cpu_percent.toFixed(1)}%` : '—'}</span>
        </div>
        <Progress value={cpu} className="h-1 bg-sky-500/15 [&_[data-slot=progress-indicator]]:bg-sky-500" />
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">{t('services.overview.memRow')}</span>
          <span className="tabular-nums">
            {running && inst.stat.memory_rss_bytes != null ? formatBytes(inst.stat.memory_rss_bytes) : '—'}
          </span>
        </div>
        <Progress value={memPct} className="h-1 bg-emerald-500/15 [&_[data-slot=progress-indicator]]:bg-emerald-500" />
      </div>
    </div>
  )
}
