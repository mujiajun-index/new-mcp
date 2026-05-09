import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { createService, testConnection } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ArrowLeft, ArrowRight, Check, Loader2, Zap } from 'lucide-react'
import type { TransportType, AuthType, TestResult } from '@/types'

const transportOptions: { value: TransportType; label: string; desc: string }[] = [
  { value: 'streamable-http', label: 'Streamable HTTP', desc: 'MCP 推荐的 HTTP 传输方式' },
  { value: 'sse', label: 'SSE', desc: 'Server-Sent Events，适合远程 HTTP 服务' },
  { value: 'stdio', label: 'Stdio', desc: '本地命令行启动，适合自建部署' },
  { value: 'websocket', label: 'WebSocket', desc: '全双工通信，适合实时场景' },
  { value: 'passive-ws', label: 'Passive WS', desc: '被动 WebSocket 接入' },
]

const authOptions: { value: AuthType; label: string }[] = [
  { value: 'none', label: '无需认证' },
  { value: 'api_key', label: 'API Key' },
  { value: 'bearer', label: 'Bearer Token' },
  { value: 'basic', label: 'Basic Auth' },
  { value: 'oauth', label: 'OAuth' },
]

const steps = ['基本信息', '传输配置', '认证配置', '连接测试']

export function ServiceCreatePage() {
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
    headers: '',
    // Auth fields
    api_key: '',
    bearer_token: '',
    basic_user: '',
    basic_pass: '',
  })

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
      toast.success('服务创建成功')
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
    switch (form.transport_type) {
      case 'stdio':
        return {
          command: form.command,
          args: form.args ? form.args.split(/\s+/) : [],
          env: form.env ? JSON.parse(form.env) : {},
        }
      case 'sse':
      case 'streamable-http':
        return {
          url: form.url,
          headers: form.headers ? JSON.parse(form.headers) : {},
        }
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
      case 'basic': return { username: form.basic_user, password: form.basic_pass }
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
          <h1 className="text-xl font-semibold">注册新服务</h1>
          <p className="text-sm text-muted-foreground">步骤 {step + 1} / {steps.length} — {steps[step]}</p>
        </div>
      </div>

      {/* Step indicators */}
      <div className="flex gap-2">
        {steps.map((s, i) => (
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
            <Label htmlFor="name">服务标识 *</Label>
            <Input id="name" placeholder="my-service" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <p className="text-xs text-muted-foreground">唯一标识，用于分组中引用，创建后不可修改</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="display_name">显示名称</Label>
            <Input id="display_name" placeholder="我的服务" value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="description">描述</Label>
            <Input id="description" placeholder="服务用途说明" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
        </div>
      )}

      {/* Step 1: Transport config */}
      {step === 1 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <div className="space-y-2">
            <Label>传输类型</Label>
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
                  <p className="text-sm font-medium">{opt.label}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{opt.desc}</p>
                </button>
              ))}
            </div>
          </div>

          {form.transport_type === 'stdio' ? (
            <>
              <div className="space-y-2">
                <Label htmlFor="command">命令 *</Label>
                <Input id="command" placeholder="npx" value={form.command} onChange={(e) => setForm({ ...form, command: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="args">参数</Label>
                <Input id="args" placeholder="-y @modelcontextprotocol/server-memory" value={form.args} onChange={(e) => setForm({ ...form, args: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="env">环境变量 (JSON)</Label>
                <Input id="env" placeholder='{"API_KEY": "xxx"}' value={form.env} onChange={(e) => setForm({ ...form, env: e.target.value })} />
              </div>
            </>
          ) : (
            <>
              <div className="space-y-2">
                <Label htmlFor="url">服务 URL *</Label>
                <Input id="url" placeholder="https://example.com/mcp" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} />
              </div>
              {(form.transport_type === 'sse' || form.transport_type === 'streamable-http') && (
                <div className="space-y-2">
                  <Label htmlFor="headers">请求头 (JSON)</Label>
                  <Input id="headers" placeholder='{"Authorization": "Bearer xxx"}' value={form.headers} onChange={(e) => setForm({ ...form, headers: e.target.value })} />
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* Step 2: Auth config */}
      {step === 2 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <div className="space-y-2">
            <Label>认证方式</Label>
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
                  {opt.label}
                </button>
              ))}
            </div>
          </div>

          {form.auth_type === 'api_key' && (
            <div className="space-y-2">
              <Label>API Key</Label>
              <Input placeholder="sk-xxx" value={form.api_key} onChange={(e) => setForm({ ...form, api_key: e.target.value })} />
            </div>
          )}
          {form.auth_type === 'bearer' && (
            <div className="space-y-2">
              <Label>Token</Label>
              <Input placeholder="eyJhbGci..." value={form.bearer_token} onChange={(e) => setForm({ ...form, bearer_token: e.target.value })} />
            </div>
          )}
          {form.auth_type === 'basic' && (
            <>
              <div className="space-y-2">
                <Label>用户名</Label>
                <Input value={form.basic_user} onChange={(e) => setForm({ ...form, basic_user: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label>密码</Label>
                <Input type="password" value={form.basic_pass} onChange={(e) => setForm({ ...form, basic_pass: e.target.value })} />
              </div>
            </>
          )}
          {form.auth_type === 'none' && (
            <p className="text-sm text-muted-foreground">该服务无需认证</p>
          )}
        </div>
      )}

      {/* Step 3: Test & confirm */}
      {step === 3 && (
        <div className="space-y-4 rounded-xl border bg-card p-6">
          <p className="text-sm text-muted-foreground">可选：先测试连接是否正常，再点击创建提交。</p>

          {testResult && (
            <div className={`rounded-lg p-4 ${testResult.connected ? 'bg-emerald-500/10' : 'bg-red-500/10'}`}>
              {testResult.connected ? (
                <div className="space-y-1 text-sm">
                  <p className="font-medium text-emerald-600 dark:text-emerald-400">连接成功</p>
                  <p className="text-muted-foreground">工具数: {testResult.tools_count} · 延迟: {testResult.latency_ms}ms</p>
                </div>
              ) : (
                <p className="text-sm text-red-600 dark:text-red-400">连接失败: {testResult.error || '未知错误'}</p>
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
              测试连接
            </Button>
            <Button
              className={`gap-2 ${testResult?.connected ? 'bg-emerald-600 hover:bg-emerald-700' : ''}`}
              onClick={() => createMutation.mutate()}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              <Check className="h-4 w-4" />
              创建服务
            </Button>
          </div>
        </div>
      )}

      {/* Navigation */}
      <div className="flex justify-between">
        <Button variant="outline" onClick={() => setStep(Math.max(0, step - 1))} disabled={step === 0}>
          <ArrowLeft className="h-4 w-4 mr-2" />
          上一步
        </Button>
        {step < steps.length - 1 && (
          <Button onClick={() => setStep(step + 1)} disabled={!canNext()}>
            下一步
            <ArrowRight className="h-4 w-4 ml-2" />
          </Button>
        )}
      </div>
    </div>
  )
}
