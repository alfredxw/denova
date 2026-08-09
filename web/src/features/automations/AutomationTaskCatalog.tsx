import { useMemo } from 'react'
import { Bot, Clock3, FileText, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { ResourceDirectory } from '@/components/resource-directory/ResourceDirectory'
import type { ResourceDirectorySection } from '@/components/resource-directory/types'
import { Button } from '@/components/ui/button'
import type { AutomationActiveRun, AutomationTask } from '@/lib/api'
import { automationTaskKey, isAutomationTaskRunning } from './automation-catalog'

interface AutomationTaskCatalogProps {
  tasks: AutomationTask[]
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
  activeRuns,
  activeId,
  agentActive,
  onSelect,
  onCreate,
  onOpenAgent,
}: AutomationTaskCatalogProps) {
  const { t } = useTranslation()
  const taskByKey = useMemo(() => new Map(tasks.map((task) => [automationTaskKey(task), task])), [tasks])
  const sections = useMemo<ResourceDirectorySection[]>(() => {
    if (tasks.length === 0) return []
    const orderedTasks = [...tasks].sort((left, right) => (
      Number(isAutomationTaskRunning(right, activeRuns)) - Number(isAutomationTaskRunning(left, activeRuns))
    ))
    const runningCount = orderedTasks.filter((task) => isAutomationTaskRunning(task, activeRuns)).length
    return [{
      id: 'project',
      label: t('automations.group.currentProject'),
      icon: FileText,
      items: orderedTasks.map((task) => {
        const running = isAutomationTaskRunning(task, activeRuns)
        return {
          id: automationTaskKey(task),
          title: task.name,
          summary: running ? t('automations.running') : task.enabled ? t('automations.enabled') : t('automations.disabled'),
          icon: FileText,
          status: running ? { label: t('automations.running'), tone: 'success' as const } : undefined,
        }
      }),
      headerMeta: runningCount > 0 ? (
        <span className="shrink-0 text-[10px] text-[var(--nova-success)]">
          {t('automations.group.running', { count: runningCount })}
        </span>
      ) : undefined,
    }]
  }, [activeRuns, t, tasks])

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
