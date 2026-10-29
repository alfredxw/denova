import { useEffect, useState } from 'react'
import { ChevronRight, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleTrigger } from '@/components/ui/collapsible'
import { agentSubAgentSessionKey, agentViewContent, isAgentSubAgentTimelineView, type AgentExecutionTiming, type AgentMessageView } from '@/lib/agent-message-view'

/** The disclosure header is one virtual row; its children are sibling virtual rows. */
export function AgentExecutionProcess({ views, running, expanded, onExpandedChange, timing }: {
  views: AgentMessageView[]
  running: boolean
  expanded: boolean
  onExpandedChange: (expanded: boolean) => void | Promise<void>
  timing?: AgentExecutionTiming
}) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [failed, setFailed] = useState(false)
  const changeExpanded = async (next: boolean) => {
    setLoading(true)
    setFailed(false)
    try {
      await onExpandedChange(next)
    } catch (error) {
      console.error('[agent-execution] load execution details failed', error)
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }
  const progressCount = views.filter(view => !view.metadata.subagent && view.kind === 'assistant' && agentViewContent(view).trim()).length
  const toolCount = views.filter(view => view.kind === 'tool').length
  const subAgentCount = new Set(views.filter(isAgentSubAgentTimelineView).map(agentSubAgentSessionKey)).size
  const duration = useExecutionDuration(timing, running)
  const label = [
    running ? t('chat.trace.executing') : t('chat.trace.execution'),
    progressCount > 0 ? t('chat.trace.progressUpdates', { count: progressCount }) : '',
    toolCount > 0 ? t('chat.trace.toolCalls', { count: toolCount }) : '',
    subAgentCount > 0 ? t('chat.subagent.label') : '',
  ].filter(Boolean).join(' · ')
  return (
    <div className="flex justify-start" data-agent-execution-process>
      <Collapsible open={expanded} onOpenChange={next => void changeExpanded(next)} className="w-full">
        <CollapsibleTrigger type="button" aria-controls={undefined} disabled={loading} aria-busy={loading}
          className="group flex w-full min-w-0 flex-wrap items-center gap-1 py-1 text-left text-xs text-[var(--nova-text-muted)] transition-colors hover:text-[var(--nova-text)]">
          {running ? <span aria-hidden="true" className="size-1.5 animate-pulse rounded-full bg-[var(--nova-text-muted)]" /> : null}
          <span>{label}</span>
          {loading ? <LoaderCircle aria-label={t('common.loading')} className="size-3 animate-spin" /> : null}
          {failed ? <span role="alert">{t('chat.history.loadExecutionFailed')}</span> : null}
          {duration ? <><span aria-hidden="true">·</span><span className="font-mono tabular-nums">{duration}</span></> : null}
          <ChevronRight aria-hidden="true" data-agent-execution-toggle-icon
            className={`size-3 opacity-60 transition-[transform,opacity] duration-[var(--nova-motion-fast)] ease-[var(--nova-panel-motion-ease)] group-hover:opacity-100 ${expanded ? 'rotate-90' : ''}`} />
        </CollapsibleTrigger>
      </Collapsible>
    </div>
  )
}

function useExecutionDuration(timing: AgentExecutionTiming | undefined, running: boolean) {
  const [, refresh] = useState(0)
  const hasFinalDuration = timing?.durationMS !== undefined && Number.isFinite(timing.durationMS) && timing.durationMS >= 0
  const hasLiveStart = running && timing?.startedAtMS !== undefined && Number.isFinite(timing.startedAtMS)

  useEffect(() => {
    if (hasFinalDuration || !hasLiveStart) return undefined
    const interval = window.setInterval(() => refresh(value => value + 1), 1_000)
    return () => window.clearInterval(interval)
  }, [hasFinalDuration, hasLiveStart, timing?.startedAtMS])

  if (hasFinalDuration) return formatExecutionDuration(timing.durationMS as number)
  if (!hasLiveStart) return ''
  return formatExecutionDuration(Math.max(0, Date.now() - (timing?.startedAtMS as number)))
}

export function formatExecutionDuration(milliseconds: number) {
  const totalSeconds = Math.max(0, Math.floor(milliseconds / 1_000))
  const seconds = totalSeconds % 60
  const totalMinutes = Math.floor(totalSeconds / 60)
  if (totalMinutes === 0) return `${seconds}s`
  const minutes = totalMinutes % 60
  const hours = Math.floor(totalMinutes / 60)
  if (hours === 0) return `${minutes}m${seconds}s`
  return `${hours}h${minutes}m${seconds}s`
}
