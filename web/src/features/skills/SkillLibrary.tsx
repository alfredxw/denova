import { useEffect, useMemo, useState } from 'react'
import { Folder, Globe, LayoutGrid, List, Search, Sparkles, Store, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { setSkillPreference } from '@/lib/api'
import type { SkillCatalogTarget, SkillPreferenceChange, SkillSnapshot } from '@/lib/api'
import { cn } from '@/lib/utils'
import { exchange, type Installation, type LocalRef } from '@/features/market/api'
import { ExportDialog } from '@/features/market/ExportDialog'
import { useWorkspaceStore } from '@/stores/workspace-store'
import { keyOf, scopeLabel, skillCategory, skillCategoryLabel } from './skill-utils'

interface SkillLibraryProps {
  target: SkillCatalogTarget
  snapshot: SkillSnapshot
  loading: boolean
  onSelect: (key: string) => void
  onChanged: () => Promise<void>
}

/** The library owns browsing and library preferences; document editing keeps
 * its existing revision-aware lifecycle in SkillsView. */
export function SkillLibrary({ target, snapshot, loading, onSelect, onChanged }: SkillLibraryProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [source, setSource] = useState('all')
  const [category, setCategory] = useState('all')
  const [view, setView] = useState('grid')
  const [pending, setPending] = useState<string | null>(null)
  const hasFilters = query.trim() !== '' || status !== 'all' || source !== 'all' || category !== 'all'
  const categories = useMemo(() => Array.from(new Set(snapshot.skills.map(skillCategory))).sort(), [snapshot.skills])
  const [installations, setInstallations] = useState<Installation[]>([])
  const [exporting, setExporting] = useState<LocalRef>()
  useEffect(() => {
    let active = true
    void exchange<Installation[]>('/installations').then((items) => { if (active) setInstallations(items) }).catch(() => { if (active) toast.error(t('market.errors.operationFailed')) })
    return () => { active = false }
  }, [snapshot, t])
  const sources = useMemo(() => {
    const result = new Map<string, Installation>()
    for (const item of installations) {
      if (item.tracking !== 'tracked') continue
      for (const binding of item.bindings) {
        if (binding.local.kind !== 'skill') continue
        if (binding.local.project_id && (target.kind !== 'project' || binding.local.project_id !== target.projectId)) continue
        result.set(`${binding.local.scope}:${binding.local.id}`, item)
      }
    }
    return result
  }, [installations, target])
  const filtered = useMemo(() => snapshot.skills.filter((skill) => {
    const enabled = skill.active && skill.enabled !== false
    if (status === 'enabled' && !enabled || status === 'disabled' && enabled) return false
    if (source === 'remote' && !sources.has(keyOf(skill)) || source === 'denova' && skill.scope === 'shared' || source === 'shared' && skill.scope !== 'shared') return false
    if (category !== 'all' && skillCategory(skill) !== category) return false
    return `${skill.name} ${skill.description}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())
  }), [category, query, snapshot.skills, source, sources, status])

  const preference = async (id: string, change: SkillPreferenceChange) => {
    setPending(id)
    try {
      await setSkillPreference(target, change)
      await onChanged()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('skills.library.preferenceFailed'))
    } finally { setPending(null) }
  }

  const sections = [
    { id: 'denova', label: t('skills.library.denova'), skills: filtered.filter((skill) => skill.scope !== 'shared') },
    { id: 'shared', label: t('skills.library.shared'), skills: filtered.filter((skill) => skill.scope === 'shared') },
  ]

  return (
    <div className="h-full min-h-0 overflow-y-auto" data-testid="skill-library">
      <div className="mx-auto flex w-full min-w-0 max-w-[1600px] flex-col gap-6 p-4 sm:p-6">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">{t('skills.library.title')}</h1>
          <Badge variant="outline">{snapshot.skills.length}</Badge>
        </div>

        <div className="@container flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <InputGroup className="min-w-0 flex-1 basis-64">
              <InputGroupAddon><Search /></InputGroupAddon>
              <InputGroupInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('skills.library.search')} aria-label={t('skills.searchPlaceholder')} />
            </InputGroup>
            <ToggleGroup type="single" value={status} onValueChange={(value) => value && setStatus(value)} variant="outline" size="sm" spacing={0} aria-label={t('skills.library.statusFilter')}>
              {['all', 'enabled', 'disabled'].map((item) => <ToggleGroupItem key={item} value={item}>{t(`skills.library.${item}`)}</ToggleGroupItem>)}
            </ToggleGroup>
            <div className="ml-auto flex flex-wrap items-center gap-2">
              <Button variant="outline" size="sm" onClick={() => useWorkspaceStore.getState().openMarketInstallation()}>
                <Store data-icon="inline-start" />{t('market.title')}
              </Button>
              <ToggleGroup type="single" value={view} onValueChange={(value) => value && setView(value)} variant="outline" size="sm" spacing={0} aria-label={t('skills.library.layout')}>
                <ToggleGroupItem value="grid" aria-label={t('skills.library.grid')}><LayoutGrid /></ToggleGroupItem>
                <ToggleGroupItem value="list" aria-label={t('skills.library.list')}><List /></ToggleGroupItem>
              </ToggleGroup>
            </div>
          </div>
          <div className="flex flex-col gap-3 @3xl:flex-row @3xl:items-center @3xl:gap-4">
            <ToggleGroup type="single" value={source} onValueChange={(value) => value && setSource(value)} size="sm" aria-label={t('skills.library.sourceFilter')} className="max-w-full flex-wrap @3xl:shrink-0">
              {['all', 'denova', 'shared', 'remote'].map((item) => <ToggleGroupItem key={item} value={item}>{t(`skills.library.${item}`)}</ToggleGroupItem>)}
            </ToggleGroup>
            {categories.length > 1 && (
              <>
                <Separator className="@3xl:hidden" />
                <Separator orientation="vertical" className="my-1 hidden @3xl:block" />
                <ToggleGroup type="single" value={category} onValueChange={(value) => setCategory(value || 'all')} size="sm" aria-label={t('skills.category.filter')} className="min-w-0 max-w-full flex-wrap">
                  {categories.map((item) => <ToggleGroupItem key={item} value={item}>{skillCategoryLabel(item, t)}</ToggleGroupItem>)}
                </ToggleGroup>
              </>
            )}
          </div>
        </div>

        {!loading && hasFilters && filtered.length === 0 && <p role="status" className="text-sm text-muted-foreground">{t('skills.library.noMatches')}</p>}
        {loading && snapshot.skills.length === 0 ? (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,280px),1fr))] gap-4" aria-label={t('skills.loading')}>
            {[0, 1, 2, 3, 4, 5].map((item) => <Skeleton key={item} className="h-44 rounded-xl" />)}
          </div>
        ) : sections.map((section) => {
          const shared = section.id === 'shared'
          if (hasFilters && section.skills.length === 0) return null
          if (source === 'denova' && shared || source === 'shared' && !shared || source === 'remote' && shared) return null
          return (
            <section key={section.id} className="flex min-w-0 flex-col gap-3" aria-label={section.label}>
              <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3">
                <div className="flex min-w-0 flex-col gap-1">
                  <div className="flex items-center gap-2">
                    {shared ? <Globe className="size-4 text-muted-foreground" /> : <Sparkles className="size-4 text-muted-foreground" />}
                    <h2 className="text-sm font-medium">{section.label}</h2>
                    <span className="text-xs text-muted-foreground">{section.skills.length}</span>
                  </div>
                  {shared && <p className="text-xs leading-relaxed text-muted-foreground">{t('skills.library.sharedHint')}</p>}
                </div>
                {shared && (
                  <label className="flex cursor-pointer items-center gap-3 text-xs">
                    {t('skills.library.useShared')}
                    <Switch checked={snapshot.shared_enabled ?? false} disabled={Boolean(pending)} onCheckedChange={(checked) => void preference('shared', { shared_enabled: checked })} aria-label={t('skills.library.useShared')} />
                  </label>
                )}
              </div>
              {section.skills.length === 0 ? (
                <Empty className="py-8">
                  <EmptyHeader>
                    <EmptyMedia variant="icon">{shared ? <Globe /> : <Search />}</EmptyMedia>
                    <EmptyTitle>{t('skills.library.noMatches')}</EmptyTitle>
                    <EmptyDescription>{t(shared ? 'skills.library.sharedEmpty' : 'skills.library.emptyHint')}</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : (
                <div className={cn('grid gap-3', view === 'grid' ? 'grid-cols-[repeat(auto-fill,minmax(min(100%,280px),1fr))]' : 'grid-cols-1')}>
                  {section.skills.map((skill) => {
                    const enabled = skill.active && skill.enabled !== false
                    const remote = sources.get(keyOf(skill))
                    return (
                      <Card key={keyOf(skill)} size="sm" data-testid={`skill-card-${keyOf(skill)}`} className="min-w-0 cursor-pointer transition-shadow hover:ring-foreground/25 focus-within:ring-ring" onClick={() => onSelect(keyOf(skill))}>
                        <CardHeader>
                          <CardTitle className="min-w-0">
                            <button type="button" className="flex w-full min-w-0 items-center gap-2 text-left outline-none" onClick={(event) => { event.stopPropagation(); onSelect(keyOf(skill)) }}>
                              <span aria-hidden className={cn('size-2 shrink-0 rounded-full', enabled ? 'bg-[var(--nova-success)]' : 'bg-muted-foreground/30')} />
                              <span className="truncate" title={skill.name}>{skill.name}</span>
                            </button>
                          </CardTitle>
                          <CardAction onClick={(event) => event.stopPropagation()}>
                            <Switch size="sm" checked={enabled} disabled={Boolean(pending) || !skill.active || shared && !snapshot.shared_enabled} onCheckedChange={(checked) => void preference(keyOf(skill), { scope: skill.scope, name: skill.name, enabled: checked })} aria-label={t('skills.library.enableSkill', { name: skill.name })} />
                          </CardAction>
                          <CardDescription className="line-clamp-2 min-h-10 break-words" title={skill.description}>{skill.description}</CardDescription>
                        </CardHeader>
                        <CardContent className="flex flex-1 flex-wrap items-start gap-1.5">
                          <Badge variant="secondary">{skillCategoryLabel(skillCategory(skill), t)}</Badge>
                          {!skill.active && (!shared || snapshot.shared_enabled) && <Badge variant="outline">{t('skills.shadowed')}</Badge>}
                          {remote && <Badge variant="outline">{t(`market.states.${remote.local_state || 'unchanged'}`)}</Badge>}
                        </CardContent>
                        <CardFooter className="flex flex-wrap justify-between gap-x-3 gap-y-2" onClick={(event) => event.stopPropagation()}>
                          <span className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground" title={remote?.source.url || skill.path}>
                            {remote ? <Globe className="size-3.5 shrink-0" /> : <Folder className="size-3.5 shrink-0" />}
                            {scopeLabel(skill.scope, t)}{remote && <> · {t('skills.library.remote')}</>}
                          </span>
                          <div className="flex flex-wrap gap-1">
                            <Button size="sm" variant="ghost" onClick={() => setExporting({ kind: 'skill', scope: skill.scope, id: skill.name, project_id: skill.scope === 'workspace' && target.kind === 'project' ? target.projectId : undefined })}><Upload data-icon="inline-start" />{t('market.export.title')}</Button>
                            {remote && <Button size="sm" variant="ghost" onClick={() => useWorkspaceStore.getState().openMarketInstallation(remote.installation_id)}>{t('market.acquired.sourceActions')}</Button>}
                          </div>
                        </CardFooter>
                      </Card>
                    )
                  })}
                </div>
              )}
            </section>
          )
        })}
        {exporting && <ExportDialog projectID={exporting.project_id} initialResources={[exporting]} onClose={() => setExporting(undefined)} />}
      </div>
    </div>
  )
}
