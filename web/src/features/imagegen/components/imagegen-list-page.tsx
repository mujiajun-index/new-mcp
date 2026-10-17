import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  getImageGenConfigs,
  deleteImageGenConfig,
  enableImageGenConfig,
  disableImageGenConfig,
} from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import {
  ImagePlus,
  Plus,
  Trash2,
  Loader2,
  Power,
  PowerOff,
  MoreHorizontal,
} from 'lucide-react'
import { toast } from 'sonner'
import type { ImageGenConfigListItem } from '../api'

function EnabledBadge({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation()
  if (enabled) {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
        {t('imagegen.statusEnabled')}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-zinc-500/10 px-2 py-0.5 text-xs font-medium text-zinc-500">
      {t('imagegen.statusDisabled')}
    </span>
  )
}

const providerLabels: Record<string, string> = {
  openai: 'OpenAI',
  openrouter: 'OpenRouter',
  gemini: 'Gemini',
}

export function ImageGenListPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['imagegen'],
    queryFn: getImageGenConfigs,
  })

  const deleteMutation = useMutation({
    mutationFn: deleteImageGenConfig,
    onSuccess: () => {
      toast.success(t('imagegen.deleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['imagegen'] })
      queryClient.invalidateQueries({ queryKey: ['services'] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
    },
    onError: (err) => toast.error(err.message),
  })

  const enableMutation = useMutation({
    mutationFn: enableImageGenConfig,
    onSuccess: () => {
      toast.success(t('imagegen.enableSuccess'))
      queryClient.invalidateQueries({ queryKey: ['imagegen'] })
      queryClient.invalidateQueries({ queryKey: ['services'] })
    },
    onError: () => {
      toast.error(t('imagegen.enableFailed'))
    },
  })

  const disableMutation = useMutation({
    mutationFn: disableImageGenConfig,
    onSuccess: () => {
      toast.success(t('imagegen.disableSuccess'))
      queryClient.invalidateQueries({ queryKey: ['imagegen'] })
      queryClient.invalidateQueries({ queryKey: ['services'] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
    },
    onError: () => {
      toast.error(t('imagegen.disableFailed'))
    },
  })

  const configs: ImageGenConfigListItem[] = data?.data || []

  const filtered = keyword.trim()
    ? configs.filter(
        (c) =>
          c.name.toLowerCase().includes(keyword.toLowerCase()) ||
          c.provider.toLowerCase().includes(keyword.toLowerCase()) ||
          c.model_name.toLowerCase().includes(keyword.toLowerCase())
      )
    : configs

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setKeyword(searchInput)
  }

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {t('imagegen.title')}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('imagegen.subtitle')}
          </p>
        </div>
        <Link to="/imagegen/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            {t('imagegen.create.title')}
          </Button>
        </Link>
      </div>

      {/* Search */}
      <form onSubmit={handleSearch} className="relative max-w-sm">
        <ImagePlus className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder={t('imagegen.searchPlaceholder')}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          className="pl-9"
        />
      </form>

      {/* List */}
      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t('common.loading')}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <ImagePlus className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">
              {t('imagegen.noConfigs')}
            </p>
            <p className="mt-1 text-xs text-muted-foreground/60">
              {t('imagegen.noConfigsHint')}
            </p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {filtered.map((c) => {
              const isEnabled = c.auto_register
              return (
                <MobileListCard
                  key={c.id}
                  title={
                    <Link
                      to="/imagegen/$id"
                      params={{ id: String(c.id) }}
                      className="font-medium transition-colors hover:text-primary"
                    >
                      {c.name}
                    </Link>
                  }
                  badge={<EnabledBadge enabled={isEnabled} />}
                  meta={[
                    {
                      label: t('imagegen.provider'),
                      value: providerLabels[c.provider] || c.provider,
                    },
                    {
                      label: t('imagegen.model'),
                      value: <span className="font-mono">{c.model_name}</span>,
                    },
                  ]}
                  actions={
                    <>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="gap-1"
                        disabled={
                          enableMutation.isPending || disableMutation.isPending
                        }
                        onClick={() => {
                          if (isEnabled) disableMutation.mutate(c.id)
                          else enableMutation.mutate(c.id)
                        }}
                      >
                        {isEnabled ? (
                          <PowerOff className="h-3.5 w-3.5" />
                        ) : (
                          <Power className="h-3.5 w-3.5" />
                        )}
                        {isEnabled
                          ? t('imagegen.disable')
                          : t('imagegen.enable')}
                      </Button>
                      <Link to="/imagegen/$id" params={{ id: String(c.id) }}>
                        <Button variant="ghost" size="sm">
                          {t('imagegen.detail')}
                        </Button>
                      </Link>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8"
                          >
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onClick={() => {
                              if (
                                confirm(
                                  t('imagegen.deleteConfirm', { name: c.name })
                                )
                              ) {
                                deleteMutation.mutate(c.id)
                              }
                            }}
                          >
                            <Trash2 className="mr-2 h-4 w-4" />
                            {t('common.delete')}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </>
                  }
                />
              )
            })}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t('imagegen.name')}
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t('imagegen.provider')}
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t('imagegen.model')}
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t('common.status')}
                  </th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">
                    {t('imagegen.actions')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((c) => {
                  const isEnabled = c.auto_register
                  return (
                    <tr
                      key={c.id}
                      className="border-b last:border-0 hover:bg-muted/30 transition-colors"
                    >
                      <td className="px-4 py-3">
                        <Link
                          to="/imagegen/$id"
                          params={{ id: String(c.id) }}
                          className="font-medium hover:text-primary transition-colors"
                        >
                          {c.name}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-xs text-muted-foreground">
                          {providerLabels[c.provider] || c.provider}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className="font-mono text-xs">
                          {c.model_name}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <EnabledBadge enabled={isEnabled} />
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="gap-1"
                            disabled={
                              enableMutation.isPending ||
                              disableMutation.isPending
                            }
                            onClick={() => {
                              if (isEnabled) {
                                disableMutation.mutate(c.id)
                              } else {
                                enableMutation.mutate(c.id)
                              }
                            }}
                            title={
                              isEnabled
                                ? t('imagegen.disable')
                                : t('imagegen.enable')
                            }
                          >
                            {isEnabled ? (
                              <PowerOff className="h-3.5 w-3.5" />
                            ) : (
                              <Power className="h-3.5 w-3.5" />
                            )}
                            {isEnabled
                              ? t('imagegen.disable')
                              : t('imagegen.enable')}
                          </Button>
                          <Link
                            to="/imagegen/$id"
                            params={{ id: String(c.id) }}
                          >
                            <Button variant="ghost" size="sm">
                              {t('imagegen.detail')}
                            </Button>
                          </Link>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => {
                              if (
                                confirm(
                                  t('imagegen.deleteConfirm', { name: c.name })
                                )
                              ) {
                                deleteMutation.mutate(c.id)
                              }
                            }}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
