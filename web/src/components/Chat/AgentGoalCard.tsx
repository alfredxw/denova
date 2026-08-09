import { useEffect, useState } from 'react'
import { ChevronDown, ChevronUp, Loader2, Pause, PenLine, Target, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { ConversationGoal } from '@/features/agent-goal/types'

interface AgentGoalCardProps {
  goal: ConversationGoal
  disabled?: boolean
  pending?: boolean
  onEdit: () => void
  onPause: () => void | Promise<void>
  onClear: () => void | Promise<void>
}

export function AgentGoalCard({ goal, disabled = false, pending = false, onEdit, onPause, onClear }: AgentGoalCardProps) {
  const { t } = useTranslation()
  const [collapsed, setCollapsed] = useState(false)
  const [, setClock] = useState(0)
  useEffect(() => {
    if (goal.status !== 'active') return
    const timer = window.setInterval(() => setClock((value) => value + 1), 1000)
    return () => window.clearInterval(timer)
  }, [goal.status])

  const title = t(`chat.goal.status.${goal.status}`)
  const elapsed = formatElapsed(goal)
  const controlsDisabled = disabled || pending
  return (
    <section className="pointer-events-auto mb-2 rounded-[16px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 shadow-[0_10px_28px_-24px_rgba(0,0,0,0.78)]" aria-label={t('chat.goal.cardLabel')}>
      <div className="flex min-w-0 items-center gap-2">
        <Target className="h-4 w-4 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
        <span className="truncate text-sm font-medium text-[var(--nova-text)]">{title}</span>
        {elapsed ? <span className="shrink-0 text-sm text-[var(--nova-text-faint)]">{elapsed}</span> : null}
        <div className="ml-auto flex shrink-0 items-center gap-0.5">
          <Button type="button" variant="ghost" size="icon-sm" disabled={controlsDisabled} onClick={onEdit} className="h-8 w-8 rounded-[9px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" aria-label={t('chat.goal.edit')}>
            <PenLine />
          </Button>
          {goal.status === 'active' ? (
            <Button type="button" variant="ghost" size="icon-sm" disabled={controlsDisabled} onClick={() => void onPause()} className="h-8 w-8 rounded-[9px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" aria-label={t('chat.goal.pause')}>
              {pending ? <Loader2 className="animate-spin" /> : <Pause />}
            </Button>
          ) : null}
          <Button type="button" variant="ghost" size="icon-sm" disabled={controlsDisabled} onClick={() => void onClear()} className="h-8 w-8 rounded-[9px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]" aria-label={t('chat.goal.clear')}>
            <Trash2 />
          </Button>
          <Button type="button" variant="ghost" size="icon-sm" onClick={() => setCollapsed((value) => !value)} className="h-8 w-8 rounded-[9px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" aria-label={collapsed ? t('chat.goal.expand') : t('chat.goal.collapse')}>
            {collapsed ? <ChevronDown /> : <ChevronUp />}
          </Button>
        </div>
      </div>
      {!collapsed ? (
        <div className="mt-1 text-sm leading-5 text-[var(--nova-text-muted)]">
          <div className="line-clamp-3 whitespace-pre-wrap">{goal.objective}</div>
          {goal.report ? <div className="mt-1 line-clamp-2 text-xs text-[var(--nova-text-faint)]">{goal.report}</div> : null}
        </div>
      ) : null}
    </section>
  )
}

function formatElapsed(goal: ConversationGoal) {
  let milliseconds = goal.active_duration_millis || 0
  if (goal.status === 'active' && goal.active_since) milliseconds += Math.max(0, Date.now() - new Date(goal.active_since).getTime())
  const seconds = Math.floor(milliseconds / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}
