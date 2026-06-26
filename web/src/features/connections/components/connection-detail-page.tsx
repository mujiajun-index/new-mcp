import { useState, useEffect } from 'react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getConnection, deleteConnection, connectConnection, disconnectConnection, updateConnection } from '../api'
import { getApiKeys } from '@/features/api-keys/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ArrowLeft, Trash2, Wifi, WifiOff, Loader2, Pencil, X, Check } from 'lucide-react'

const statusColors: Record<string, string> = {
  connected: 'text-emerald-600 dark:text-emerald-400',
  disconnected: 'text-zinc-500',
  connecting: 'text-amber-600 dark:text-amber-400',
  error: 'text-red-600 dark:text-red-400',
}

export function ConnectionDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()
  const connId = Number(id)

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ name: '', wss_url: '', api_key_id: 0, expose_mode: 'smart' as 'smart' | 'direct' })

  const { data, isLoading } = useQuery({
    queryKey: ['connection', id],
    queryFn: () => getConnection(connId),
  })

  const { data: keysData } = useQuery({
    queryKey: ['api-keys'],
    queryFn: () => getApiKeys(),
  })

  const conn = data?.data
  const apiKeys = (keysData?.data || []).filter((k: { status: number }) => k.status === 1)
  const isDisabled = conn ? conn.status !== 1 : false

  const statusLabels: Record<string, string> = {
    connected: t('connections.status_connected'),
    disconnected: t('connections.status_disconnected'),
    connecting: t('connections.status_connecting'),
    error: t('connections.status_error'),
  }

  const cloudTypeLabels: Record<string, string> = {
    xiaozhi: t('connections.platform_xiaozhi'),
    custom: t('connections.platform_custom_wss'),
    ssh: t('connections.platform_ssh'),
  }

  useEffect(() => {
    if (conn && !editing) {
      setForm({
        name: conn.name || '',
        wss_url: conn.wss_url || '',
        api_key_id: conn.api_key_id || 0,
        expose_mode: conn.expose_mode || 'smart',
      })
    }
  }, [conn, editing])

  const deleteMutation = useMutation({
    mutationFn: () => deleteConnection(connId),
    onSuccess: () => { toast.success(t('connections.deleteSuccess')); navigate({ to: '/connections' }) },
  })

  const toggleMutation = useMutation({
    mutationFn: (action: 'connect' | 'disconnect') =>
      action === 'connect' ? connectConnection(connId) : disconnectConnection(connId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['connection', id] }),
    onError: (err) => toast.error(err.message || t('connections.operationFailed')),
  })

  const statusMutation = useMutation({
    mutationFn: (status: number) => updateConnection(connId, { status }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['connection', id] }),
  })

  const updateMutation = useMutation({
    mutationFn: () => updateConnection(connId, {
      name: form.name,
      wss_url: form.wss_url,
      api_key_id: form.api_key_id,
      expose_mode: form.expose_mode,
    }),
    onSuccess: () => {
      toast.success(t('connections.detail.updateSuccess'))
      setEditing(false)
      queryClient.invalidateQueries({ queryKey: ['connection', id] })
    },
    onError: () => {
      toast.error(t('connections.detail.updateFailed'))
    },
  })

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('common.loading')}</div>
  if (!conn) return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('connections.detail.notFound')}</div>

  const boundKey = apiKeys.find((k: { id: number }) => k.id === (editing ? form.api_key_id : conn.api_key_id))

  return (
    <div className={`p-6 lg:p-8 space-y-6${isDisabled ? ' opacity-60' : ''}`}>
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/connections' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{conn.name}</h1>
            <p className={`mt-0.5 text-sm font-medium ${statusColors[conn.connection_status] || ''}`}>
              {statusLabels[conn.connection_status] || conn.connection_status}
              {isDisabled && <span className="ml-2 text-zinc-500">{t('connections.detail.disabledHint')}</span>}
            </p>
          </div>
        </div>
        <div className="flex gap-2">
          {/* Enable/Disable toggle */}
          <Button
            variant="outline"
            size="sm"
            onClick={() => statusMutation.mutate(isDisabled ? 1 : 2)}
          >
            {isDisabled ? t('common.enabled') : t('common.disabled')}
          </Button>
          {!editing && (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              <Pencil className="h-4 w-4 mr-1.5" />{t('common.edit')}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => toggleMutation.mutate(conn.connection_status === 'connected' ? 'disconnect' : 'connect')}
            disabled={isDisabled || toggleMutation.isPending}
          >
            {toggleMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin mr-1.5" /> :
              conn.connection_status === 'connected' ? <><WifiOff className="h-4 w-4 mr-1.5" />{t('connections.disconnect')}</> : <><Wifi className="h-4 w-4 mr-1.5" />{t('connections.connect')}</>
            }
          </Button>
          <Button variant="outline" size="sm" className="text-destructive" onClick={() => { if (confirm(t('connections.detail.deleteConfirm'))) deleteMutation.mutate() }}>
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Basic config */}
      <div className="rounded-xl border bg-card p-5 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-muted-foreground">{t('connections.detail.basicConfig')}</h2>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          {/* Name */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{t('connections.detail.name')}</Label>
            {editing ? (
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            ) : (
              <p className="text-sm">{conn.name}</p>
            )}
          </div>

          {/* Cloud type (read-only) */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{t('connections.detail.platformType')}</Label>
            <p className="text-sm">{cloudTypeLabels[conn.cloud_type] || conn.cloud_type}</p>
          </div>

          {/* WSS URL */}
          {conn.cloud_type !== 'ssh' && (
            <div className="space-y-1.5 sm:col-span-2">
              <Label className="text-xs text-muted-foreground">{t('connections.detail.wssUrl')}</Label>
              {editing ? (
                <Input value={form.wss_url} onChange={(e) => setForm({ ...form, wss_url: e.target.value })} placeholder="wss://example.com/ws" />
              ) : (
                <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{conn.wss_url || '-'}</code>
              )}
            </div>
          )}

          {/* Bound API Key */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{t('connections.detail.bindApiKey')}</Label>
            {editing ? (
              apiKeys.length === 0 ? (
                <p className="text-xs text-muted-foreground py-2">{t('connections.detail.createApiKeyFirst')}</p>
              ) : (
                <div className="flex flex-wrap gap-2 pt-1">
                  {apiKeys.map((k: { id: number; name: string; key_prefix: string }) => (
                    <button
                      key={k.id}
                      type="button"
                      onClick={() => setForm({ ...form, api_key_id: k.id })}
                      className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                        form.api_key_id === k.id ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
                      }`}
                    >
                      {k.name} <span className="text-muted-foreground">({k.key_prefix}...)</span>
                    </button>
                  ))}
                </div>
              )
            ) : (
              <p className="text-sm">{boundKey ? `${boundKey.name} (${boundKey.key_prefix}...)` : '-'}</p>
            )}
          </div>

          {/* Expose Mode */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{t('connections.detail.exposeModeHint')}</Label>
            {editing ? (
              <div className="flex gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => setForm({ ...form, expose_mode: 'smart' })}
                  className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                    form.expose_mode === 'smart' ? 'border-purple-200 bg-purple-100 text-purple-700 dark:border-purple-800 dark:bg-purple-900/30 dark:text-purple-300' : 'hover:border-primary/30'
                  }`}
                >
                  {t('connections.detail.smartMode')}
                </button>
                <button
                  type="button"
                  onClick={() => setForm({ ...form, expose_mode: 'direct' })}
                  className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                    form.expose_mode === 'direct' ? 'border-blue-200 bg-blue-100 text-blue-700 dark:border-blue-800 dark:bg-blue-900/30 dark:text-blue-300' : 'hover:border-primary/30'
                  }`}
                >
                  {t('connections.detail.directMode')}
                </button>
              </div>
            ) : (
              <p className="text-sm">{conn.expose_mode === 'direct' ? t('connections.detail.directMode') : t('connections.detail.smartMode')}</p>
            )}
          </div>

          {/* Auto connect (read-only) */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{t('connections.detail.autoConnect')}</Label>
            <p className="text-sm">{conn.auto_connect ? t('connections.detail.yes') : t('connections.detail.no')}</p>
          </div>
        </div>

        {/* Edit actions */}
        {editing && (
          <div className="flex justify-end gap-2 pt-2 border-t">
            <Button variant="outline" size="sm" onClick={() => setEditing(false)}>
              <X className="h-4 w-4 mr-1.5" />{t('common.cancel')}
            </Button>
            <Button
              size="sm"
              onClick={() => updateMutation.mutate()}
              disabled={!form.name.trim() || !form.wss_url.trim() || form.api_key_id === 0 || updateMutation.isPending}
            >
              {updateMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin mr-1.5" /> : <Check className="h-4 w-4 mr-1.5" />}
              {t('common.save')}
            </Button>
          </div>
        )}
      </div>

      {/* Connection status */}
      <div className="rounded-xl border bg-card p-5 space-y-3">
        <h2 className="text-sm font-medium text-muted-foreground">{t('connections.detail.connectStatus')}</h2>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <p className="text-xs text-muted-foreground">{t('common.status')}</p>
            <p className={`mt-0.5 text-sm font-medium ${statusColors[conn.connection_status] || ''}`}>
              {statusLabels[conn.connection_status] || conn.connection_status}
            </p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t('connections.detail.remoteId')}</p>
            <p className="mt-0.5 text-sm font-mono">{conn.remote_id || '-'}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t('connections.detail.lastConnect')}</p>
            <p className="mt-0.5 text-sm">{conn.last_connected_at || '-'}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t('connections.detail.lastError')}</p>
            <p className="mt-0.5 text-sm">{conn.last_error || '-'}</p>
          </div>
        </div>
      </div>

      {/* SSH config */}
      {conn.cloud_config && Object.keys(conn.cloud_config).length > 0 && (
        <div className="rounded-xl border bg-card p-5 space-y-2">
          <h2 className="text-sm font-medium text-muted-foreground">{t('connections.detail.config')}</h2>
          <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto">
            {JSON.stringify(conn.cloud_config, null, 2)}
          </pre>
        </div>
      )}
    </div>
  )
}
