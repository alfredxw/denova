import { ArrowUpLeft, Bot, ChevronRight, Copy, Link, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import type { AgentRunTrace } from '@/lib/api'
import { formatTrajectoryDuration } from './trajectory-analysis'

interface TrajectoryRunHeaderProps {
  trace: AgentRunTrace
  trajectoryURI: string
  onOpenRun: (runID: string) => void
}

/** Run identity and navigation are independent of the selected detail view. */
export function TrajectoryRunHeader({ trace, trajectoryURI, onOpenRun }: TrajectoryRunHeaderProps) {
  const { t } = useTranslation()
  const { summary, children = [] } = trace
  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(t('trajectory.copy.success'))
    } catch (error) {
      console.warn('[TrajectoryRunHeader.tsx] failed to copy Run reference', { runID: summary.id, error })
      toast.error(t('trajectory.copy.failed'))
    }
  }

  return (
    <div className="shrink-0 border-b border-border bg-background">
      <div className="flex min-w-0 flex-wrap items-center gap-1 px-2 py-1">
        {summary.parent_run_id && (
          <Button size="xs" variant="ghost" onClick={() => onOpenRun(summary.parent_run_id!)}>
            <ArrowUpLeft data-icon="inline-start" />{t('trajectory.parentRun')}
          </Button>
        )}
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground select-text" title={summary.id}>{summary.id}</span>
        <Button size="xs" variant="ghost" onClick={() => void copy(summary.id)} aria-label={t('trajectory.copy.runId')} title={t('trajectory.copy.runId')}>
          <Copy data-icon="inline-start" /><span className="hidden sm:inline">{t('trajectory.copy.runId')}</span>
        </Button>
        <Button size="xs" variant="ghost" onClick={() => void copy(trajectoryURI)} aria-label={t('trajectory.copy.reference')} title={t('trajectory.copy.reference')}>
          <Link data-icon="inline-start" /><span className="hidden sm:inline">{t('trajectory.copy.reference')}</span>
        </Button>
      </div>
      {children.length > 0 && (
        <Collapsible>
          <CollapsibleTrigger asChild>
            <Button size="xs" variant="ghost" className="w-full justify-start px-2">
              <ChevronRight data-icon="inline-start" className="transition-transform group-aria-expanded/button:rotate-90" />
              {t('trajectory.children.title', { count: children.length })}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="max-h-48 overflow-auto px-2 pb-2">
            <div className="flex flex-col gap-1">
              {children.map((child) => (
                <div key={child.id} className="flex min-w-0 items-center gap-1">
                  <Button
                    size="xs" variant="ghost" className="h-auto min-w-0 flex-1 justify-start py-1.5"
                    disabled={child.status === 'unavailable'} onClick={() => onOpenRun(child.id)}
                    aria-label={t('trajectory.children.open', { name: child.agent_name || child.id })}
                  >
                    <Bot data-icon="inline-start" />
                    <span className="flex min-w-0 flex-1 flex-col items-start gap-1 text-left">
                      <span className="max-w-full truncate">{child.agent_name || t('trajectory.runs.agent')}</span>
                      <span className="max-w-full truncate font-mono text-muted-foreground">{child.id}</span>
                      {child.status === 'unavailable' && <span className="whitespace-normal text-muted-foreground">{t('trajectory.children.unavailable')}</span>}
                    </span>
                    <span className="flex shrink-0 flex-col items-end gap-1">
                      <Badge variant="outline">{t(`trajectory.runStatus.${child.status || 'running'}`)}</Badge>
                      {child.status !== 'unavailable' && (
                        <span className="flex items-center gap-1 text-muted-foreground">
                          {formatTrajectoryDuration(child.duration_ms)}
                          <span className="hidden items-center gap-1 sm:flex" aria-label={t('trajectory.runs.calls', { models: child.llm_calls ?? 0, tools: child.tool_calls ?? 0 })}>
                            <Bot />{child.llm_calls ?? 0}<Wrench />{child.tool_calls ?? 0}
                          </span>
                        </span>
                      )}
                    </span>
                  </Button>
                  <Button size="icon-xs" variant="ghost" onClick={() => void copy(child.id)} aria-label={t('trajectory.children.copy', { name: child.agent_name || child.id })}>
                    <Copy />
                  </Button>
                </div>
              ))}
            </div>
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  )
}
