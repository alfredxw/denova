import { ArrowLeft, Bot, Clock3, Image, PackageOpen, RefreshCcw, Sparkles } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { formatDateTime } from '@/i18n'
import type { TaskCenterResult, TaskCenterStatus, TaskCenterTask, TaskCenterTaskType } from '@/lib/api'

interface MobileTaskCenterProps {
  result: TaskCenterResult
  loadState: 'loading' | 'ready' | 'error'
  onBack: () => void
  onOpenTask: (task: TaskCenterTask) => void
}

const STATUS_STYLE: Record<TaskCenterStatus, string> = {
  running: 'text-[var(--nova-accent)]',
  waiting_user: 'text-[var(--nova-warning)]',
  failed: 'text-[var(--nova-danger)]',
  completed: 'text-[var(--nova-success)]',
  stopped: 'text-[var(--nova-text-faint)]',
}

const TYPE_ICON: Record<TaskCenterTaskType, typeof Bot> = {
  agent: Bot,
  automation: Clock3,
  interactive_story: Sparkles,
  image_generation: Image,
  import_export: PackageOpen,
}

export function MobileTaskCenter({ result, loadState, onBack, onOpenTask }: MobileTaskCenterProps) {
  const { t } = useTranslation()

  useEffect(() => {
    document.getElementById('mobile-task-center-back')?.focus()
  }, [])

  return (
    <section className="flex h-full min-h-0 flex-col" aria-labelledby="mobile-task-center-title">
      <header className="flex min-h-13 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
        <button
          type="button"
          id="mobile-task-center-back"
          onClick={onBack}
          className="nova-icon-button flex min-h-11 w-11! shrink-0 items-center justify-center text-[var(--nova-text-muted)]"
          aria-label={t('workbench.mobile.taskCenter.back')}
        >
          <ArrowLeft className="size-4" />
        </button>
        <div className="min-w-0 flex-1">
          <h2 id="mobile-task-center-title" className="truncate text-sm font-semibold text-[var(--nova-text)]">
            {t('workbench.mobile.taskCenter.title')}
          </h2>
          <p className="truncate text-xs text-[var(--nova-text-faint)]">
            {result.action_required_count > 0
              ? t('workbench.mobile.taskCenter.actionRequired', { count: result.action_required_count })
              : t('workbench.mobile.taskCenter.upToDate')}
          </p>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {loadState === 'loading' && result.tasks.length === 0 ? (
          <TaskCenterState icon={<RefreshCcw className="size-5 animate-spin" />} text={t('workbench.mobile.taskCenter.loading')} />
        ) : loadState === 'error' && result.tasks.length === 0 ? (
          <TaskCenterState icon={<RefreshCcw className="size-5" />} text={t('workbench.mobile.taskCenter.loadError')} />
        ) : result.tasks.length === 0 ? (
          <TaskCenterState icon={<PackageOpen className="size-5" />} text={t('workbench.mobile.taskCenter.empty')} />
        ) : (
          <div className="divide-y divide-[var(--nova-border)]">
            {result.tasks.map((task) => {
              const TypeIcon = TYPE_ICON[task.type]
              return (
                <button
                  key={task.id}
                  type="button"
                  onClick={() => onOpenTask(task)}
                  className="flex min-h-20 w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-[var(--nova-hover)]"
                  aria-label={t('workbench.mobile.taskCenter.openTask', { title: task.title })}
                >
                  <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]" aria-hidden="true">
                    <TypeIcon className="size-4" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block break-words text-sm font-medium leading-5 text-[var(--nova-text)]">{task.title}</span>
                    <span className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--nova-text-faint)]">
                      <span>{task.project.name || t('workbench.mobile.taskCenter.unknownProject')}</span>
                      <span>{t(`workbench.mobile.taskCenter.type.${task.type}`)}</span>
                      <span>{formatDateTime(task.updated_at)}</span>
                    </span>
                    <span className={`mt-1 block text-xs font-medium ${STATUS_STYLE[task.status]}`}>
                      {t(`workbench.mobile.taskCenter.status.${task.status}`)}
                    </span>
                  </span>
                </button>
              )
            })}
          </div>
        )}
      </div>
    </section>
  )
}

function TaskCenterState({ icon, text }: { icon: ReactNode; text: string }) {
  return (
    <div className="flex h-full min-h-48 flex-col items-center justify-center gap-2 px-6 text-center text-sm text-[var(--nova-text-faint)]">
      <span aria-hidden="true">{icon}</span>
      <span>{text}</span>
    </div>
  )
}
