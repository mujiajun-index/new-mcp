import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  adminListMarketplaceGroups, adminCreateMarketplaceGroup, adminUpdateMarketplaceGroup, adminDeleteMarketplaceGroup,
  adminListMarketplaceTags, adminCreateMarketplaceTag, adminUpdateMarketplaceTag, adminDeleteMarketplaceTag,
} from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, FolderTree, Tags } from 'lucide-react'

// AdminCategoriesPage 市场分类管理:分组(业务分类) + 标签(字典)两个 Tab(§11)。
export function AdminCategoriesPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'groups' | 'tags'>('groups')
  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('nav.adminCategories')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('categories.subtitle')}</p>
      </div>
      <div className="flex gap-2">
        <Button variant={tab === 'groups' ? 'default' : 'outline'} size="sm" className="gap-1.5" onClick={() => setTab('groups')}>
          <FolderTree className="h-3.5 w-3.5" />{t('categories.groups')}
        </Button>
        <Button variant={tab === 'tags' ? 'default' : 'outline'} size="sm" className="gap-1.5" onClick={() => setTab('tags')}>
          <Tags className="h-3.5 w-3.5" />{t('categories.tags')}
        </Button>
      </div>
      {tab === 'groups' ? <GroupsTab /> : <TagsTab />}
    </div>
  )
}

// --- Groups tab ---

function GroupsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<any | null>(null)

  const { data } = useQuery({
    queryKey: ['admin-marketplace-groups'],
    queryFn: () => adminListMarketplaceGroups(),
  })
  const groups: any[] = data?.data ?? []

  const createMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => adminCreateMarketplaceGroup(body),
    onSuccess: () => { toast.success(t('common.success')); setCreateOpen(false); queryClient.invalidateQueries({ queryKey: ['admin-marketplace-groups'] }) },
  })
  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Record<string, unknown> }) => adminUpdateMarketplaceGroup(id, body),
    onSuccess: () => { toast.success(t('common.success')); setEditing(null); queryClient.invalidateQueries({ queryKey: ['admin-marketplace-groups'] }) },
  })
  const deleteMutation = useMutation({
    mutationFn: adminDeleteMarketplaceGroup,
    onSuccess: () => { toast.success(t('common.delete')); queryClient.invalidateQueries({ queryKey: ['admin-marketplace-groups'] }) },
  })

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button className="gap-2" onClick={() => setCreateOpen(true)}><Plus className="h-4 w-4" />{t('common.create')}</Button>
      </div>
      <div className="rounded-xl border bg-card overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/50">
              <TableHead className="px-4">{t('categories.colName')}</TableHead>
              <TableHead className="px-4">{t('categories.colSortOrder')}</TableHead>
              <TableHead className="px-4">{t('common.status')}</TableHead>
              <TableHead className="px-4 text-right">{t('common.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {groups.map((g) => (
              <TableRow key={g.id}>
                <TableCell className="px-4 font-medium">{g.name}</TableCell>
                <TableCell className="px-4 text-muted-foreground">{g.sort_order}</TableCell>
                <TableCell className="px-4">
                  <Badge variant={g.status === 1 ? 'outline' : 'secondary'}
                    className={g.status === 1 ? 'text-emerald-600 border-emerald-300' : ''}>
                    {g.status === 1 ? t('common.enabled') : t('common.disabled')}
                  </Badge>
                </TableCell>
                <TableCell className="px-4 text-right">
                  <Button variant="ghost" size="sm" onClick={() => setEditing(g)}><Pencil className="h-3.5 w-3.5" /></Button>
                  <Button variant="ghost" size="sm" className="text-destructive"
                    onClick={() => { if (confirm(t('services.deleteConfirm', { name: g.name }))) deleteMutation.mutate(g.id) }}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <GroupDialog open={createOpen} onOpenChange={setCreateOpen}
        onConfirm={(body) => createMutation.mutate(body)} pending={createMutation.isPending} />
      {editing && (
        <GroupDialog open onOpenChange={(v) => !v && setEditing(null)} initial={editing}
          onConfirm={(body) => updateMutation.mutate({ id: editing.id, body })} pending={updateMutation.isPending} />
      )}
    </div>
  )
}

function GroupDialog({ open, onOpenChange, onConfirm, pending, initial }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onConfirm: (body: Record<string, unknown>) => void
  pending: boolean
  initial?: any
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState({
    name: initial?.name ?? '',
    description: initial?.description ?? '',
    sort_order: String(initial?.sort_order ?? 0),
    status: initial ? String(initial.status) : '1',
  })
  const isEdit = !!initial

  const submit = () => {
    if (!form.name.trim()) return
    onConfirm({
      name: form.name.trim(),
      description: form.description || undefined,
      sort_order: parseInt(form.sort_order) || 0,
      status: parseInt(form.status),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? t('common.edit') : t('common.create')}</DialogTitle>
          <DialogDescription>{t('categories.groupDesc')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t('categories.colName')} <span className="text-destructive">*</span></Label>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="space-y-2">
            <Label>{t('common.description')}</Label>
            <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t('categories.colSortOrder')}</Label>
              <Input type="number" value={form.sort_order} onChange={(e) => setForm({ ...form, sort_order: e.target.value })} />
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
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending || !form.name.trim()} onClick={submit}>{isEdit ? t('common.save') : t('common.create')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// --- Tags tab ---

function TagsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<any | null>(null)

  const { data } = useQuery({
    queryKey: ['admin-marketplace-tags'],
    queryFn: () => adminListMarketplaceTags({ page: 1, page_size: 200 }),
  })
  const tags: any[] = data?.data ?? []

  const createMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => adminCreateMarketplaceTag(body),
    onSuccess: () => { toast.success(t('common.success')); setCreateOpen(false); queryClient.invalidateQueries({ queryKey: ['admin-marketplace-tags'] }) },
  })
  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Record<string, unknown> }) => adminUpdateMarketplaceTag(id, body),
    onSuccess: () => { toast.success(t('common.success')); setEditing(null); queryClient.invalidateQueries({ queryKey: ['admin-marketplace-tags'] }) },
  })
  const deleteMutation = useMutation({
    mutationFn: adminDeleteMarketplaceTag,
    onSuccess: () => { toast.success(t('common.delete')); queryClient.invalidateQueries({ queryKey: ['admin-marketplace-tags'] }) },
  })

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button className="gap-2" onClick={() => setCreateOpen(true)}><Plus className="h-4 w-4" />{t('common.create')}</Button>
      </div>
      <div className="rounded-xl border bg-card overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/50">
              <TableHead className="px-4">{t('categories.colName')}</TableHead>
              <TableHead className="px-4">{t('common.description')}</TableHead>
              <TableHead className="px-4">{t('categories.colSortOrder')}</TableHead>
              <TableHead className="px-4">{t('common.status')}</TableHead>
              <TableHead className="px-4 text-right">{t('common.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tags.map((tag) => (
              <TableRow key={tag.id}>
                <TableCell className="px-4 font-medium">{tag.name}</TableCell>
                <TableCell className="px-4 text-muted-foreground">{tag.description || '-'}</TableCell>
                <TableCell className="px-4 text-muted-foreground">{tag.sort_order}</TableCell>
                <TableCell className="px-4">
                  <Badge variant={tag.status === 1 ? 'outline' : 'secondary'}
                    className={tag.status === 1 ? 'text-emerald-600 border-emerald-300' : ''}>
                    {tag.status === 1 ? t('common.enabled') : t('common.disabled')}
                  </Badge>
                </TableCell>
                <TableCell className="px-4 text-right">
                  <Button variant="ghost" size="sm" onClick={() => setEditing(tag)}><Pencil className="h-3.5 w-3.5" /></Button>
                  <Button variant="ghost" size="sm" className="text-destructive"
                    onClick={() => { if (confirm(t('services.deleteConfirm', { name: tag.name }))) deleteMutation.mutate(tag.id) }}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <TagDialog open={createOpen} onOpenChange={setCreateOpen}
        onConfirm={(body) => createMutation.mutate(body)} pending={createMutation.isPending} />
      {editing && (
        <TagDialog open onOpenChange={(v) => !v && setEditing(null)} initial={editing}
          onConfirm={(body) => updateMutation.mutate({ id: editing.id, body })} pending={updateMutation.isPending} />
      )}
    </div>
  )
}

function TagDialog({ open, onOpenChange, onConfirm, pending, initial }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onConfirm: (body: Record<string, unknown>) => void
  pending: boolean
  initial?: any
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState({
    name: initial?.name ?? '',
    description: initial?.description ?? '',
    sort_order: String(initial?.sort_order ?? 0),
    status: initial ? String(initial.status) : '1',
  })
  const isEdit = !!initial

  const submit = () => {
    if (!form.name.trim()) return
    onConfirm({
      name: form.name.trim(),
      description: form.description || undefined,
      sort_order: parseInt(form.sort_order) || 0,
      status: parseInt(form.status),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? t('common.edit') : t('common.create')}</DialogTitle>
          <DialogDescription>{t('categories.tagDesc')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t('categories.colName')} <span className="text-destructive">*</span></Label>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="space-y-2">
            <Label>{t('common.description')}</Label>
            <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t('categories.colSortOrder')}</Label>
              <Input type="number" value={form.sort_order} onChange={(e) => setForm({ ...form, sort_order: e.target.value })} />
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
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending || !form.name.trim()} onClick={submit}>{isEdit ? t('common.save') : t('common.create')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
