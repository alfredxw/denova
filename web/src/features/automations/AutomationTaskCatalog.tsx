import { useMemo } from 'react'
import { Bot, Clock3, FileText, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { ResourceDirectory } from '@/components/resource-directory/ResourceDirectory'
import type { ResourceDirectorySection } from '@/components/resource-directory/types'
import { Button } from '@/components/ui/button'
import type { AutomationActiveRun, AutomationTask, BookRecord } from '@/lib/api'
import { automationTaskKey, groupAutomationTasks, isAutomationTaskRunning } from './automation-catalog'

interface AutomationTaskCatalogProps {
  tasks: AutomationTask[]
  books: BookRecord[]
  activeRuns: AutomationActiveRun[]
  activeId: string
  agentActive: boolean
  onSelect: (task: AutomationTask) => void
  onCreate: () => void
  onOpenAgent: () => void
}

/** Domain adapter that maps automation targets and run state onto the shared resource directory. */
export function AutomationTaskCatalog({
  tasks,
  books,
  activeRuns,
  activeId,
  agentActive,
  onSelect,
  onCreate,
  onOpenAgent,
}: AutomationTaskCatalogProps) {
  const { t } = useTranslation()
  const taskByKey = useMemo(() => new Map(tasks.map((task) => [automationTaskKey(task), task])), [tasks])
  const sections = useMemo<ResourceDirectorySection[]>(() => (
    groupAutomationTasks(tasks, books, activeRuns).map((group) => ({
      id: group.kind === 'user' ? 'user' : `workspace:${group.workspace}`,
      label: group.kind === 'user' ? t('automations.group.global') : group.label,
      description: group.kind === 'workspace' ? group.workspace : undefined,
      icon: group.kind === 'user' ? Clock3 : FileText,
      items: group.tasks.map((task) => {
        const running = isAutomationTaskRunning(task, activeRuns)
        return {
          id: automationTaskKey(task),
          title: task.name,
          summary: running ? t('automations.running') : task.enabled ? t('automations.enabled') : t('automations.disabled'),
          icon: FileText,
          status: running ? { label: t('automations.running'), tone: 'success' as const } : undefined,
        }
      }),
      headerMeta: group.runningCount > 0 ? (
        <span className="shrink-0 text-[10px] text-[var(--nova-success)]">
          {t('automations.group.running', { count: group.runningCount })}
        </span>
      ) : undefined,
    }))
  ), [activeRuns, books, t, tasks])

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      <ResourceDirectory
        sections={sections}
        activeId={activeId}
        onSelect={(id) => {
          const task = taskByKey.get(id)
          if (task) onSelect(task)
        }}
        showSearch={false}
        headerContent={(
          <div className="grid grid-cols-2 gap-2">
            <Button
              type="button"
              size="sm"
              variant={agentActive ? 'secondary' : 'outline'}
              onClick={onOpenAgent}
              className="nova-nav-item border-[var(--nova-border)]"
            >
              <Bot data-icon="inline-start" />
              <span className="min-w-0 truncate">{t('automations.view.agent')}</span>
            </Button>
            <Button
              type="button"
              size="sm"
              variant="secondary"
              onClick={onCreate}
              className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-active)]"
            >
              <Plus data-icon="inline-start" />
              <span className="min-w-0 truncate">{t('automations.newTask')}</span>
            </Button>
          </div>
        )}
        emptyContent={(
          <EmptyState
            variant="compact"
            icon={Clock3}
            title={t('automations.empty')}
            className="text-[var(--nova-text-faint)]"
          />
        )}
      />
    </div>
  )
}
