import { useQuery, useMutation } from '@tanstack/react-query'
import { useNavigate, useParams } from '@tanstack/react-router'
import { getMarketplaceItem, installFromMarketplace } from '../api'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import { ArrowLeft, Download, Star, Zap, Code2, ExternalLink } from 'lucide-react'

export function MarketplaceDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }

  const { data, isLoading } = useQuery({
    queryKey: ['marketplace-item', id],
    queryFn: () => getMarketplaceItem(Number(id)),
  })

  const installMutation = useMutation({
    mutationFn: () => installFromMarketplace({ item_id: Number(id) }),
    onSuccess: (res) => {
      toast.success(`已安装为服务: ${res.data?.name}`)
      navigate({ to: '/services' })
    },
  })

  const item = data?.data

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">加载中...</div>
  if (!item) return <div className="flex items-center justify-center py-20 text-muted-foreground">服务不存在</div>

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-4xl mx-auto">
      <Button variant="ghost" size="sm" className="gap-1.5" onClick={() => navigate({ to: '/marketplace' })}>
        <ArrowLeft className="h-4 w-4" />返回广场
      </Button>

      {/* Header */}
      <div className="flex items-start gap-4">
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 text-primary shrink-0">
          {item.icon_url ? (
            <img src={item.icon_url} alt="" className="h-8 w-8" />
          ) : (
            <span className="text-2xl font-bold">{(item.display_name || item.name).charAt(0)}</span>
          )}
        </div>
        <div className="flex-1">
          <h1 className="text-xl font-semibold">{item.display_name || item.name}</h1>
          <div className="mt-1 flex items-center gap-3">
            <span className={`rounded px-2 py-0.5 text-xs font-medium ${
              item.category === 'instant' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-amber-500/10 text-amber-600'
            }`}>
              {item.category === 'instant' ? '即用型' : '源码型'}
            </span>
            {item.version && <span className="text-xs text-muted-foreground">v{item.version}</span>}
            <span className="flex items-center gap-1 text-xs text-muted-foreground">
              <Download className="h-3 w-3" />{item.install_count} 次安装
            </span>
            {item.rating_count > 0 && (
              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                <Star className="h-3 w-3" />{item.rating_avg.toFixed(1)} ({item.rating_count})
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Description */}
      {item.description && (
        <div className="rounded-xl border bg-card p-5">
          <p className="text-sm leading-relaxed">{item.description}</p>
        </div>
      )}

      {/* Install action */}
      <div className="rounded-xl border bg-card p-5">
        {item.category === 'instant' ? (
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">一键安装</p>
              <p className="text-xs text-muted-foreground">自动创建 MCP 服务配置</p>
            </div>
            <Button className="gap-2" onClick={() => installMutation.mutate()} disabled={installMutation.isPending}>
              <Zap className="h-4 w-4" />
              {installMutation.isPending ? '安装中...' : '添加到我的服务'}
            </Button>
          </div>
        ) : (
          <div className="space-y-4">
            <div>
              <p className="text-sm font-medium">部署指南</p>
              <p className="text-xs text-muted-foreground">需要先自行部署服务，然后在 NewMCP 中注册</p>
            </div>
            {item.repo_url && (
              <a href={item.repo_url} target="_blank" rel="noopener noreferrer">
                <Button variant="outline" className="gap-2">
                  <ExternalLink className="h-4 w-4" />查看源码仓库
                </Button>
              </a>
            )}
            {item.install_guide && (
              <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto whitespace-pre-wrap">{item.install_guide}</pre>
            )}
            {item.required_env && item.required_env.length > 0 && (
              <div>
                <p className="text-xs font-medium mb-1.5">需要的环境变量:</p>
                <div className="flex flex-wrap gap-1.5">
                  {item.required_env.map((env) => (
                    <span key={env} className="rounded bg-muted px-2 py-0.5 text-xs font-mono">{env}</span>
                  ))}
                </div>
              </div>
            )}
            <Button variant="outline" onClick={() => navigate({ to: '/services/create' })}>
              我已部署，去注册
            </Button>
          </div>
        )}
      </div>

      {/* Tools snapshot */}
      {item.tools_snapshot && item.tools_snapshot.length > 0 && (
        <div className="rounded-xl border bg-card p-5">
          <h2 className="mb-3 text-sm font-semibold">提供的工具 ({item.tools_snapshot.length})</h2>
          <div className="space-y-2">
            {item.tools_snapshot.map((tool) => (
              <div key={tool.name} className="rounded-lg border p-3">
                <p className="text-sm font-medium font-mono">{tool.name}</p>
                {tool.description && <p className="mt-0.5 text-xs text-muted-foreground">{tool.description}</p>}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Tags */}
      {item.tags && item.tags.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {item.tags.map((tag) => (
            <span key={tag} className="rounded-full bg-muted px-3 py-1 text-xs">{tag}</span>
          ))}
        </div>
      )}
    </div>
  )
}
