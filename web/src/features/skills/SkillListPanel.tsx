import { useMemo } from 'react'
import { Bot, Download, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ResourceDirectory } from '@/components/resource-directory/ResourceDirectory'
import type { ResourceDirectoryBadge, ResourceDirectorySection } from '@/components/resource-directory/types'
import type { SkillSnapshot } from '@/lib/api'
import type { SkillsMode } from './skill-utils'
import { keyOf, scopeLabel, skillScopes } from './skill-utils'

interface SkillListPanelProps {
  snapshot: SkillSnapshot
  selectedKey: string | null
  loading: boolean
  agentOpen: boolean
  mode: SkillsMode
  onToggleAgent: () => void
  onCreate: () => void
  onInstall: () => void
  onSelect: (key: string) => void
}

/** Skills 左侧栏：Agent/新建/导入入口 + ResourceDirectory 分组列表。 */
export function SkillListPanel({
  snapshot,
  selectedKey,
  loading,
  agentOpen,
  mode,
  onToggleAgent,
  onCreate,
  onInstall,
  onSelect,
}: SkillListPanelProps) {
  const { t } = useTranslation()
  const sections = useMemo<ResourceDirectorySection[]>(() => skillScopes.map((scope) => {
    const scopeInfo = snapshot.scopes.find((item) => item.scope === scope)
    return {
      id: scope,
      label: scopeLabel(scope, t),
      items: snapshot.skills
        .filter((skill) => skill.scope === scope)
        .map((skill) => {
          const badges: ResourceDirectoryBadge[] = []
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
  }), [snapshot.scopes, snapshot.skills, t])

  const showSkeleton = loading && snapshot.skills.length === 0

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      <div className="grid shrink-0 grid-cols-3 gap-2 p-3">
        <button
          type="button"
          onClick={onToggleAgent}
          className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded border border-[var(--nova-border)] px-2 ${agentOpen ? 'is-active' : 'bg-[var(--nova-surface)]'}`}
        >
          <Bot className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('skills.agent.button')}</span>
        </button>
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
      {showSkeleton ? (
        <div className="space-y-2 p-2">
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
        />
      )}
    </div>
  )
}
