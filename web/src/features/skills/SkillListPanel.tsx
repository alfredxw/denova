import { useEffect, useMemo, useState } from 'react'
import { ChevronDown, Download, FileText, Plus, Search, Tags } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { EmbeddedSidebar } from '@/components/navigation/embedded-sidebar'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInput,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarSeparator,
} from '@/components/ui/sidebar'
import type { SkillScope, SkillSnapshot } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { SkillsMode } from './skill-utils'
import { keyOf, scopeLabel, skillCategory, skillCategoryLabel, skillCategoryOptions, skillScopes } from './skill-utils'

interface SkillListPanelProps {
  snapshot: SkillSnapshot
  selectedKey: string | null
  loading: boolean
  mode: SkillsMode
  onCreate: () => void
  onInstall: () => void
  onSelect: (key: string) => void
}

/** Skill library navigation composed from the shadcn Sidebar primitives. */
export function SkillListPanel({
  snapshot,
  selectedKey,
  loading,
  mode,
  onCreate,
  onInstall,
  onSelect,
}: SkillListPanelProps) {
  const { t } = useTranslation()
  const [categoryFilter, setCategoryFilter] = useState('all')
  const [query, setQuery] = useState('')
  const [openScopes, setOpenScopes] = useState<Partial<Record<SkillScope, boolean>>>({})

  const categories = useMemo(() => {
    const discovered = Array.from(new Set(snapshot.skills.map(skillCategory)))
    const standard = skillCategoryOptions.filter((category) => discovered.includes(category))
    const custom = discovered
      .filter((category) => !skillCategoryOptions.includes(category as typeof skillCategoryOptions[number]))
      .sort()
    return [...standard, ...custom]
  }, [snapshot.skills])

  useEffect(() => {
    if (categoryFilter !== 'all' && !categories.includes(categoryFilter)) setCategoryFilter('all')
  }, [categories, categoryFilter])

  const searchWords = useMemo(() => query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean), [query])
  const visibleSkills = useMemo(() => snapshot.skills.filter((skill) => {
    if (categoryFilter !== 'all' && skillCategory(skill) !== categoryFilter) return false
    if (searchWords.length === 0) return true
    const searchable = `${skill.name}\n${skill.description}\n${skillCategoryLabel(skillCategory(skill), t)}`.toLocaleLowerCase()
    return searchWords.every((word) => searchable.includes(word))
  }), [categoryFilter, searchWords, snapshot.skills, t])

  const sections = useMemo(() => skillScopes.map((scope) => ({
    scope,
    scopeInfo: snapshot.scopes.find((item) => item.scope === scope),
    skills: visibleSkills.filter((skill) => skill.scope === scope),
  })).filter((section) => (
    searchWords.length === 0 && categoryFilter === 'all'
      ? true
      : section.skills.length > 0
  )), [categoryFilter, searchWords.length, snapshot.scopes, visibleSkills])

  const showSkeleton = loading && snapshot.skills.length === 0
  const showEmpty = !showSkeleton && visibleSkills.length === 0
  const emptyTitle = searchWords.length > 0
    ? t('common.searchNoResults')
    : categoryFilter === 'all'
      ? t('skills.empty')
      : t('skills.category.empty')

  return (
    <EmbeddedSidebar>
        <SidebarHeader className="gap-3 p-3">
          <div className="grid grid-cols-2 gap-2">
            <Button
              type="button"
              size="sm"
              variant={mode === 'create' ? 'secondary' : 'default'}
              aria-pressed={mode === 'create'}
              onClick={onCreate}
            >
              <Plus data-icon="inline-start" />
              {t('skills.create.newButton')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant={mode === 'install' ? 'secondary' : 'outline'}
              aria-pressed={mode === 'install'}
              onClick={onInstall}
            >
              <Download data-icon="inline-start" />
              {t('skills.install.action')}
            </Button>
          </div>

          <div className="relative">
            <SidebarInput
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t('skills.searchPlaceholder')}
              aria-label={t('skills.searchPlaceholder')}
              className="pl-8"
            />
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          </div>

          <Select value={categoryFilter} onValueChange={setCategoryFilter}>
            <SelectTrigger size="sm" aria-label={t('skills.category.filter')} className="w-full">
              <Tags aria-hidden="true" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">{t('skills.category.all')}</SelectItem>
                {categories.map((category) => (
                  <SelectItem key={category} value={category}>{skillCategoryLabel(category, t)}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </SidebarHeader>

        <SidebarSeparator />
        <SidebarContent>
          {showSkeleton ? (
            <SidebarGroup>
              <SidebarGroupLabel>{t('skills.loading')}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {Array.from({ length: 6 }).map((_, index) => (
                    <SidebarMenuItem key={index}>
                      <SidebarMenuSkeleton showIcon />
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          ) : showEmpty ? (
            <EmptyState variant="compact" title={emptyTitle} className="p-4 text-muted-foreground" />
          ) : sections.map(({ scope, scopeInfo, skills }) => {
            const open = openScopes[scope] ?? skills.length > 0
            const label = scopeLabel(scope, t)
            return (
              <Collapsible
                key={scope}
                open={open}
                onOpenChange={(nextOpen) => setOpenScopes((current) => ({ ...current, [scope]: nextOpen }))}
              >
                <SidebarGroup className="py-1">
                  <SidebarGroupLabel asChild title={scopeInfo?.path} className="gap-2">
                    <CollapsibleTrigger
                      aria-label={`${open ? t('common.collapse') : t('common.expand')}${label}`}
                      aria-expanded={open}
                    >
                      <ChevronDown className={cn('transition-transform', !open && '-rotate-90')} aria-hidden="true" />
                      <span className="min-w-0 flex-1 truncate">{label}</span>
                      <span className="text-sidebar-foreground/50">{skills.length}</span>
                      <span className="text-sidebar-foreground/50">
                        {scopeInfo?.writable ? t('skills.scope.editable') : t('skills.scope.readonly')}
                      </span>
                    </CollapsibleTrigger>
                  </SidebarGroupLabel>
                  <CollapsibleContent>
                    <SidebarGroupContent>
                      <SidebarMenu>
                        {skills.map((skill) => (
                          <SidebarMenuItem key={keyOf(skill)}>
                            <SidebarMenuButton
                              type="button"
                              size="lg"
                              isActive={selectedKey === keyOf(skill)}
                              className={cn(!skill.active && 'pr-20')}
                              onClick={() => onSelect(keyOf(skill))}
                            >
                              <FileText aria-hidden="true" />
                              <div className="grid min-w-0 flex-1 gap-0.5">
                                <span className="truncate font-mono text-xs text-sidebar-foreground">/{skill.name}</span>
                                <span className="truncate text-[11px] text-sidebar-foreground/60">
                                  {skill.description || skillCategoryLabel(skillCategory(skill), t)}
                                </span>
                              </div>
                            </SidebarMenuButton>
                            {!skill.active && <SidebarMenuBadge>{t('skills.shadowed')}</SidebarMenuBadge>}
                          </SidebarMenuItem>
                        ))}
                      </SidebarMenu>
                    </SidebarGroupContent>
                  </CollapsibleContent>
                </SidebarGroup>
              </Collapsible>
            )
          })}
        </SidebarContent>

    </EmbeddedSidebar>
  )
}
