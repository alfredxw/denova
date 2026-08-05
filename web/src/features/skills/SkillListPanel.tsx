import { useEffect, useMemo, useState } from 'react'
import { Bot, Download, Plus, Tags } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { ResourceDirectory } from '@/components/resource-directory/ResourceDirectory'
import type { ResourceDirectoryBadge, ResourceDirectorySection } from '@/components/resource-directory/types'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { SkillSnapshot } from '@/lib/api'
import type { SkillsMode } from './skill-utils'
import { keyOf, scopeLabel, skillCategory, skillCategoryLabel, skillCategoryOptions, skillScopes } from './skill-utils'

interface SkillListPanelProps {
  snapshot: SkillSnapshot
  selectedKey: string | null
  loading: boolean
  agentAvailable: boolean
  agentOpen: boolean
  mode: SkillsMode
  onToggleAgent: () => void
  onCreate: () => void
  onInstall: () => void
  onSelect: (key: string) => void
}

/** Skill navigation, creation/import actions, and the grouped resource directory. */
export function SkillListPanel({
  snapshot,
  selectedKey,
  loading,
  agentAvailable,
  agentOpen,
  mode,
  onToggleAgent,
  onCreate,
  onInstall,
  onSelect,
}: SkillListPanelProps) {
  const { t } = useTranslation()
  const [categoryFilter, setCategoryFilter] = useState('all')
  const categories = useMemo(() => {
    const discovered = Array.from(new Set(snapshot.skills.map(skillCategory)))
    const standard = skillCategoryOptions.filter((category) => discovered.includes(category))
    const custom = discovered.filter((category) => !skillCategoryOptions.includes(category as typeof skillCategoryOptions[number])).sort()
    return [...standard, ...custom]
  }, [snapshot.skills])
  useEffect(() => {
    if (categoryFilter !== 'all' && !categories.includes(categoryFilter)) setCategoryFilter('all')
  }, [categories, categoryFilter])
  const visibleSkills = useMemo(() => (
    categoryFilter === 'all'
      ? snapshot.skills
      : snapshot.skills.filter((skill) => skillCategory(skill) === categoryFilter)
  ), [categoryFilter, snapshot.skills])
  const sections = useMemo<ResourceDirectorySection[]>(() => skillScopes.map((scope) => {
    const scopeInfo = snapshot.scopes.find((item) => item.scope === scope)
    return {
      id: scope,
      label: scopeLabel(scope, t),
      items: visibleSkills
        .filter((skill) => skill.scope === scope)
        .map((skill) => {
          const badges: ResourceDirectoryBadge[] = []
          badges.push({ label: skillCategoryLabel(skillCategory(skill), t), tone: 'muted' })
          if (skill.active) {
            badges.push({ label: '✓', title: t('skills.active'), tone: 'default' })
          } else {
            badges.push({ label: t('skills.shadowed'), tone: 'warning' })
          }
          if (!skill.editable) badges.push({ label: t('skills.scope.readonly'), tone: 'muted' })
          return {
            id: keyOf(skill),
            title: `/${skill.name}`,
            summary: skill.description || undefined,
            badges,
          }
        }),
      headerMeta: (
        <span className="flex min-w-0 items-center gap-1.5">
          <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">
            {scopeInfo?.writable ? t('skills.scope.editable') : t('skills.scope.readonly')}
          </span>
          {scopeInfo?.path && (
            <span className="max-w-28 truncate font-mono text-[10px] text-[var(--nova-text-faint)]" title={scopeInfo.path}>
              {scopeInfo.path}
            </span>
          )}
        </span>
      ),
    }
  }).filter((section) => categoryFilter === 'all' || section.items.length > 0), [categoryFilter, snapshot.scopes, t, visibleSkills])

  const showSkeleton = loading && snapshot.skills.length === 0

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      <div className={`grid shrink-0 gap-2 p-3 ${agentAvailable ? 'grid-cols-3' : 'grid-cols-2'}`}>
        {agentAvailable && (
          <button
            type="button"
            onClick={onToggleAgent}
            className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded border border-[var(--nova-border)] px-2 ${agentOpen ? 'is-active' : 'bg-[var(--nova-surface)]'}`}
          >
            <Bot className="h-3.5 w-3.5" />
            <span className="min-w-0 truncate">{t('skills.agent.button')}</span>
          </button>
        )}
        <button
          type="button"
          onClick={onCreate}
          className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded border border-[var(--nova-border)] px-2 ${mode === 'create' ? 'is-active' : 'bg-[var(--nova-surface)]'}`}
        >
          <Plus className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('skills.create.newButton')}</span>
        </button>
        <button
          type="button"
          onClick={onInstall}
          className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded border border-[var(--nova-border)] px-2 ${mode === 'install' ? 'is-active' : 'bg-[var(--nova-surface)]'}`}
        >
          <Download className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('skills.install.action')}</span>
        </button>
      </div>
      <div className="shrink-0 border-y border-[var(--nova-border)] px-3 py-2">
        <Select value={categoryFilter} onValueChange={setCategoryFilter}>
          <SelectTrigger aria-label={t('skills.category.filter')} className="h-8 w-full border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs">
            <Tags className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent className="border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]">
            <SelectItem value="all" className="text-xs">{t('skills.category.all')}</SelectItem>
            {categories.map((category) => (
              <SelectItem key={category} value={category} className="text-xs">{skillCategoryLabel(category, t)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {showSkeleton ? (
        <div className="flex flex-col gap-2 p-2">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="h-10 animate-pulse rounded bg-[var(--nova-surface)]" />
          ))}
        </div>
      ) : (
        <ResourceDirectory
          sections={sections}
          activeId={selectedKey}
          onSelect={onSelect}
          searchPlaceholder={t('skills.searchPlaceholder')}
          emptyContent={(
            <EmptyState
              variant="compact"
              title={categoryFilter === 'all' ? t('skills.empty') : t('skills.category.empty')}
              className="text-[var(--nova-text-faint)]"
            />
          )}
        />
      )}
    </div>
  )
}
