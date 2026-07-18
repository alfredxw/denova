import { Bot, ChevronRight, Clock3, FileText, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
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
  const groups = groupAutomationTasks(tasks, books, activeRuns)
  return (
    <div className="h-full min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] p-3">
      <div className="mb-3 grid grid-cols-2 gap-2">
        <button type="button" onClick={onOpenAgent} className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2 ${agentActive ? 'is-active' : 'bg-[var(--nova-surface)]'}`}>
          <Bot className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('automations.view.agent')}</span>
        </button>
        <button type="button" onClick={onCreate} className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2">
          <Plus className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('automations.newTask')}</span>
        </button>
      </div>
      {groups.length === 0 ? (
        <div className="px-2 py-8 text-center text-[var(--nova-text-faint)]">{t('automations.empty')}</div>
      ) : (
        <div className="space-y-4">
          {groups.map((group) => {
            const groupLabel = group.kind === 'user' ? t('automations.group.global') : group.label
            return (
              <section key={group.kind === 'user' ? 'user' : group.workspace}>
                <Collapsible defaultOpen>
                  <CollapsibleTrigger asChild>
                    <button
                      type="button"
                      className="group nova-nav-item mb-1.5 flex w-full items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-left text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]"
                      title={group.workspace || undefined}
                    >
                      <ChevronRight className="h-3 w-3 shrink-0 transition-transform group-data-[state=open]:rotate-90" />
                      {group.kind === 'user' ? <Clock3 className="h-3 w-3 shrink-0" /> : <FileText className="h-3 w-3 shrink-0" />}
                      <span className="min-w-0 flex-1 truncate">{groupLabel}</span>
                      {group.runningCount > 0 && <span className="shrink-0 normal-case tracking-normal text-[var(--nova-success)]">{t('automations.group.running', { count: group.runningCount })}</span>}
                      <span className="shrink-0">{group.tasks.length}</span>
                    </button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <div className="space-y-1">
                      {group.tasks.map((task) => {
                        const key = automationTaskKey(task)
                        const running = isAutomationTaskRunning(task, activeRuns)
                        return (
                          <button key={key} type="button" onClick={() => onSelect(task)} className={`nova-nav-item flex w-full items-start gap-2 rounded-[var(--nova-radius)] px-2.5 py-2 text-left ${activeId === key ? 'is-active' : ''}`}>
                            <span className="relative mt-0.5 shrink-0">
                              <FileText className="h-4 w-4 text-[var(--nova-text-muted)]" />
                              {running && <span className="absolute -right-1 -top-1 h-2 w-2 rounded-full bg-[var(--nova-success)] ring-2 ring-[var(--nova-surface-2)]" />}
                            </span>
                            <span className="min-w-0 flex-1">
                              <span className="block truncate font-medium text-[var(--nova-text)]">{task.name}</span>
                              <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">
                                {running ? t('automations.running') : task.enabled ? t('automations.enabled') : t('automations.disabled')}
                              </span>
                            </span>
                          </button>
                        )
                      })}
                    </div>
                  </CollapsibleContent>
                </Collapsible>
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}
