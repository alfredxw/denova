import { useEffect, useState, type ReactNode } from 'react'
import { ChevronRight, ListTodo } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TaskCenterResult, TaskCenterTask } from '@/lib/api'
import { MobileTaskCenter } from './MobileTaskCenter'

export interface MobileMoreMenuItem {
  id: string
  label: string
  icon: ReactNode
  onSelect: () => void
}

const MOBILE_TASK_CENTER_HISTORY_KEY = 'novaMobileWorkbenchLayer'

interface MobileMoreMenuProps {
  mode: 'ide' | 'interactive'
  modeSwitchLabel: string
  writingModeLabel: string
  gameModeLabel: string
  items: MobileMoreMenuItem[]
  taskCenter: TaskCenterResult
  taskCenterLoadState: 'loading' | 'ready' | 'error'
  notice?: ReactNode
  onSelectMode: (mode: 'ide' | 'interactive') => void
  onOpenTask: (task: TaskCenterTask) => void
}

export function MobileMoreMenu({
  mode,
  modeSwitchLabel,
  writingModeLabel,
  gameModeLabel,
  items,
  taskCenter,
  taskCenterLoadState,
  notice,
  onSelectMode,
  onOpenTask,
}: MobileMoreMenuProps) {
  const { t } = useTranslation()
  const [taskCenterOpen, setTaskCenterOpen] = useState(false)

  useEffect(() => {
    if (!taskCenterOpen) return
    const handlePopState = () => setTaskCenterOpen(false)
    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [taskCenterOpen])

  useEffect(() => {
    if (!taskCenterOpen) return
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      setTaskCenterOpen(false)
      restoreFocusToMore()
      if (history.state?.[MOBILE_TASK_CENTER_HISTORY_KEY] === 'task-center') history.back()
    }
    window.addEventListener('keydown', handleEscape)
    return () => window.removeEventListener('keydown', handleEscape)
  }, [taskCenterOpen])

  const openTaskCenter = () => {
    const currentState = history.state && typeof history.state === 'object' ? history.state : {}
    history.pushState({ ...currentState, [MOBILE_TASK_CENTER_HISTORY_KEY]: 'task-center' }, '')
    setTaskCenterOpen(true)
  }

  const returnFromTaskCenter = () => {
    setTaskCenterOpen(false)
    restoreFocusToMore()
    if (history.state?.[MOBILE_TASK_CENTER_HISTORY_KEY] === 'task-center') history.back()
  }

  const handleOpenTask = (task: TaskCenterTask) => {
    returnFromTaskCenter()
    onOpenTask(task)
  }

  if (taskCenterOpen) {
    return (
      <MobileTaskCenter
        result={taskCenter}
        loadState={taskCenterLoadState}
        onBack={returnFromTaskCenter}
        onOpenTask={handleOpenTask}
      />
    )
  }

  const actionRequired = taskCenter.action_required_count
  const taskCenterLabel = actionRequired > 0
    ? t('workbench.mobile.taskCenter.entryActionable', { count: actionRequired })
    : t('workbench.mobile.taskCenter.title')

  return (
    <section className="h-full overflow-y-auto px-4 py-4">
      <div
        role="group"
        className="grid grid-cols-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1"
        aria-label={modeSwitchLabel}
      >
        <ModeButton active={mode === 'ide'} label={writingModeLabel} onClick={() => onSelectMode('ide')} />
        <ModeButton active={mode === 'interactive'} label={gameModeLabel} onClick={() => onSelectMode('interactive')} />
      </div>

      <div className="mt-4 border-y border-[var(--nova-border)]">
        <button
          type="button"
          className="flex min-h-14 w-full items-center gap-3 px-1 text-left text-sm text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={openTaskCenter}
          aria-label={taskCenterLabel}
        >
          <span className="flex size-5 shrink-0 items-center justify-center text-[var(--nova-text-faint)]" aria-hidden="true">
            <ListTodo className="size-4" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate">{t('workbench.mobile.taskCenter.title')}</span>
            <span className="mt-0.5 block truncate text-xs text-[var(--nova-text-faint)]">
              {actionRequired > 0
                ? t('workbench.mobile.taskCenter.actionRequired', { count: actionRequired })
                : t('workbench.mobile.taskCenter.upToDate')}
            </span>
          </span>
          {actionRequired > 0 && (
            <span className="min-w-5 rounded-full bg-[var(--nova-danger)] px-1.5 py-0.5 text-center text-[10px] font-semibold leading-4 text-white" aria-hidden="true">
              {actionRequired > 99 ? '99+' : actionRequired}
            </span>
          )}
          <ChevronRight className="size-4 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
        </button>
      </div>

      <div className="mt-4 divide-y divide-[var(--nova-border)] border-y border-[var(--nova-border)]">
        {items.map((item) => (
          <button
            key={item.id}
            type="button"
            className="flex min-h-12 w-full items-center gap-3 px-1 text-left text-sm text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            onClick={item.onSelect}
          >
            <span className="flex size-5 shrink-0 items-center justify-center text-[var(--nova-text-faint)]" aria-hidden="true">
              {item.icon}
            </span>
            <span className="min-w-0 flex-1 truncate">{item.label}</span>
          </button>
        ))}
      </div>

      {notice && <div className="mt-4">{notice}</div>}
    </section>
  )
}

function ModeButton({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={`min-h-11 rounded-[6px] px-3 text-xs ${active ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`}
    >
      {label}
    </button>
  )
}

function restoreFocusToMore() {
  requestAnimationFrame(() => {
    document.querySelector<HTMLElement>('[data-mobile-more-button]')?.focus()
  })
}
