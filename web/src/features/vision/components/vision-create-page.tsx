import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { createVisionConfig, testVisionConfig } from '../api'
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
import { ArrowLeft, Check, Loader2, Zap } from 'lucide-react'
import { toast } from 'sonner'

const providerOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'custom', label: 'Custom' },
  { value: 'glm', label: 'GLM' },
  { value: 'qwen', label: 'Qwen' },
  { value: 'ollama', label: 'Ollama' },
]

export function VisionCreatePage() {
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
      toast.success('视觉配置创建成功')
      navigate({ to: '/vision' })
    },
    onError: () => {
      toast.error('创建失败，请检查表单信息')
    },
  })

  const testMutation = useMutation({
    mutationFn: () =>
      testVisionConfig({
        endpoint_url: form.endpoint_url,
        api_key: form.api_key,
        model_name: form.model_name,
      }),
    onSuccess: (res) => {
      if (res.data?.success) {
        toast.success('连接测试成功')
      } else {
        toast.error(`连接测试失败: ${res.data?.error || '未知错误'}`)
      }
    },
    onError: () => {
      toast.error('测试请求失败')
    },
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
          <h1 className="text-xl font-semibold">新建视觉配置</h1>
          <p className="text-sm text-muted-foreground">
            配置视觉模型以提供图像理解能力
          </p>
        </div>
      </div>

      <div className="space-y-5 rounded-xl border bg-card p-6">
        {/* Name */}
        <div className="space-y-2">
          <Label htmlFor="name">配置名称 *</Label>
          <Input
            id="name"
            placeholder="例如: gpt-4o-vision"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <p className="text-xs text-muted-foreground">唯一标识，便于后续管理</p>
        </div>

        {/* Description */}
        <div className="space-y-2">
          <Label htmlFor="description">描述</Label>
          <Input
            id="description"
            placeholder="配置用途说明"
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />
        </div>

        {/* Provider */}
        <div className="space-y-2">
          <Label>Provider *</Label>
          <Select
            value={form.provider}
            onValueChange={(v) => setForm({ ...form, provider: v })}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择 Provider" />
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
            placeholder="https://api.openai.com/v1/chat/completions"
            value={form.endpoint_url}
            onChange={(e) => setForm({ ...form, endpoint_url: e.target.value })}
          />
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
          <Label htmlFor="model_name">模型名称 *</Label>
          <Input
            id="model_name"
            placeholder="gpt-4o"
            value={form.model_name}
            onChange={(e) => setForm({ ...form, model_name: e.target.value })}
          />
        </div>

        {/* System Prompt */}
        <div className="space-y-2">
          <Label htmlFor="system_prompt">系统提示词</Label>
          <textarea
            id="system_prompt"
            rows={4}
            placeholder="你是一个图像分析助手..."
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
          测试连接
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
          保存
        </Button>
        <Button variant="ghost" onClick={() => navigate({ to: '/vision' })}>
          取消
        </Button>
      </div>
    </div>
  )
}
