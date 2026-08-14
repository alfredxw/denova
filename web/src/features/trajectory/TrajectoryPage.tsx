import { useCallback, useEffect, useState } from 'react'
import { Activity, AlertTriangle, Download, RefreshCw, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { downloadAgentRunTrace, exportAgentRunTrace, getAgentRunTrace, getAgentRunTraces } from '@/lib/api'
import type { AgentRunTrace, AgentRunTraceSummary, ResourceTarget } from '@/lib/api'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'
import { TrajectoryTraceWorkspace } from './TrajectoryTraceWorkspace'

interface TrajectoryPageProps {
  target: ResourceTarget
  onClose?: () => void
}

/** Project-scoped developer workspace for hierarchical Agent trace analysis. */
export function TrajectoryPage({ target, onClose }: TrajectoryPageProps) {
  const { t } = useTranslation()
  const projectID = target.kind === 'project' ? target.projectId : ''
  const [runs, setRuns] = useState<AgentRunTraceSummary[]>([])
  const [selectedRunID, setSelectedRunID] = useState('')
  const [trace, setTrace] = useState<AgentRunTrace | null>(null)
  const [loadingRuns, setLoadingRuns] = useState(false)
  const [loadingTrace, setLoadingTrace] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadRuns = useCallback(async (preferredRunID?: string) => {
    if (!projectID) return
    setLoadingRuns(true)
    setError(null)
    try {
      const nextRuns = await getAgentRunTraces(projectID, 100)
      setRuns(nextRuns)
      const nextRunID = preferredRunID && nextRuns.some((run) => run.id === preferredRunID)
        ? preferredRunID
        : nextRuns[0]?.id ?? ''
      setSelectedRunID(nextRunID)
      if (!nextRunID) setTrace(null)
    } catch (cause) {
      console.error('[TrajectoryPage.tsx] failed to load project Agent traces', { projectID, cause })
      setError(errorMessage(cause))
      setRuns([])
      setSelectedRunID('')
      setTrace(null)
    } finally {
      setLoadingRuns(false)
    }
  }, [projectID])

  useEffect(() => {
    setRuns([])
    setSelectedRunID('')
    setTrace(null)
    void loadRuns()
  }, [loadRuns])

  useEffect(() => {
    if (!projectID || !selectedRunID) {
      setTrace(null)
      return
    }
    let cancelled = false
    setLoadingTrace(true)
    setError(null)
    void getAgentRunTrace(projectID, selectedRunID)
      .then((nextTrace) => {
        if (cancelled) return
        setTrace(nextTrace)
      })
      .catch((cause) => {
        if (cancelled) return
        console.error('[TrajectoryPage.tsx] failed to load Agent trace detail', { projectID, selectedRunID, cause })
        setError(errorMessage(cause))
        setTrace(null)
      })
      .finally(() => {
        if (!cancelled) setLoadingTrace(false)
      })
    return () => { cancelled = true }
  }, [projectID, selectedRunID])

  const exportTrace = async () => {
    if (!projectID || !selectedRunID) return
    try {
      const file = await exportAgentRunTrace(projectID, selectedRunID)
      downloadAgentRunTrace(file)
      toast.success(t('trajectory.export.success', { filename: file.filename }))
    } catch (cause) {
      console.error('[TrajectoryPage.tsx] failed to export Agent trace', { projectID, selectedRunID, cause })
      toast.error(t('trajectory.export.failed'), { description: errorMessage(cause) })
    }
  }

  return (
    <FeaturePageShell
      icon={Route}
      title={t('trajectory.title')}
      subtitle={t('trajectory.subtitle')}
      className="[&_button]:focus-visible:border-transparent [&_button]:focus-visible:bg-[var(--nova-hover)] [&_button]:focus-visible:outline-none [&_button]:focus-visible:ring-0"
      onClose={onClose}
      error={error}
      actions={(
        <>
          <Badge variant="outline" className="hidden h-5 px-1.5 text-[9px] uppercase tracking-[0.12em] sm:inline-flex">{t('trajectory.developerBadge')}</Badge>
          <Button
            type="button"
            size="icon-xs"
            variant="ghost"
            disabled={!projectID || loadingRuns || loadingTrace}
            onClick={() => void loadRuns(selectedRunID)}
            aria-label={t('trajectory.refresh')}
          >
            <RefreshCw className={cn((loadingRuns || loadingTrace) && 'animate-spin')} />
          </Button>
          <Button type="button" size="xs" variant="ghost" disabled={!selectedRunID} onClick={() => void exportTrace()}>
            <Download />{t('trajectory.export.action')}
          </Button>
        </>
      )}
    >
      {!projectID ? (
        <EmptyPage icon={AlertTriangle} title={t('trajectory.noProject')} description={t('trajectory.noProjectDescription')} />
      ) : loadingRuns && runs.length === 0 ? (
        <EmptyPage icon={Activity} title={t('common.loading')} description={t('trajectory.loadingDescription')} />
      ) : runs.length === 0 ? (
        <EmptyPage icon={Route} title={t('trajectory.empty')} description={t('trajectory.emptyDescription')} />
      ) : (
        <div className="grid min-h-0 flex-1 grid-rows-[132px_minmax(0,1fr)] bg-[var(--nova-bg)] md:grid-cols-[210px_minmax(0,1fr)] md:grid-rows-1">
          <RunList runs={runs} selectedRunID={selectedRunID} onSelect={setSelectedRunID} />
          <section className="relative flex min-h-0 min-w-0 flex-col overflow-hidden bg-[var(--nova-surface)]">
            {trace ? (
              <TrajectoryTraceWorkspace key={trace.summary.id} trace={trace} />
            ) : (
              <EmptyPage icon={Activity} title={loadingTrace ? t('common.loading') : t('trajectory.selectRun')} description={t('trajectory.selectRunDescription')} />
            )}
          </section>
        </div>
      )}
    </FeaturePageShell>
  )
}

function RunList({
  runs,
  selectedRunID,
  onSelect,
}: {
  runs: AgentRunTraceSummary[]
  selectedRunID: string
  onSelect: (runID: string) => void
}) {
  const { t } = useTranslation()
  return (
    <aside className="min-h-0 overflow-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] md:border-r md:border-b-0" aria-label={t('trajectory.runs.title')}>
      <div className="sticky top-0 z-10 flex h-9 items-center border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">
        {t('trajectory.runs.title')}<span className="ml-auto font-mono text-[var(--nova-text-faint)]">{runs.length}</span>
      </div>
      <div className="flex gap-1 overflow-x-auto p-2 md:block md:space-y-1 md:overflow-x-hidden">
        {runs.map((run) => (
          <button
            key={run.id}
            type="button"
            onClick={() => onSelect(run.id)}
            aria-current={selectedRunID === run.id ? 'true' : undefined}
            className={cn(
              'w-64 shrink-0 rounded-[var(--nova-radius)] border border-transparent px-2.5 py-2 text-left transition-colors focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none md:w-full',
              selectedRunID === run.id
                ? 'bg-[var(--nova-active)]'
                : 'hover:bg-[var(--nova-surface)]',
            )}
          >
            <span className="flex min-w-0 items-center gap-2">
              <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">{run.agent_kind || t('trajectory.runs.agent')}</span>
              <StatusDot status={run.status} />
              <span className="shrink-0 font-mono text-[9px] uppercase text-[var(--nova-text-faint)]">{run.status}</span>
            </span>
            <span className="mt-1 block truncate font-mono text-[9px] text-[var(--nova-text-muted)]">{shortRunID(run.id)}</span>
            <span className="mt-1 flex items-center gap-2 text-[9px] text-[var(--nova-text-faint)]">
              <span>{formatRunTime(run.created_at)}</span>
              <span>{formatTrajectoryDuration(run.duration_ms)}</span>
              <span>{t('trajectory.runs.calls', { models: run.llm_calls ?? 0, tools: run.tool_calls ?? 0 })}</span>
              <span className={run.content_captured ? 'text-[var(--nova-success)]' : ''}>{t(run.content_captured ? 'trajectory.runs.content' : 'trajectory.runs.metadata')}</span>
            </span>
          </button>
        ))}
      </div>
    </aside>
  )
}

function EmptyPage({ icon: Icon, title, description }: { icon: typeof Route; title: string; description: string }) {
  return (
    <div className="grid min-h-0 flex-1 place-items-center px-6 text-center">
      <div>
        <Icon className="mx-auto size-6 text-[var(--nova-text-faint)]" />
        <div className="mt-2 text-xs font-medium text-[var(--nova-text)]">{title}</div>
        <p className="mt-1 max-w-md text-[11px] leading-5 text-[var(--nova-text-faint)]">{description}</p>
      </div>
    </div>
  )
}

function StatusDot({ status }: { status: string }) {
  const normalized = status.toLowerCase()
  const tone = normalized === 'success' || normalized === 'completed'
    ? 'bg-[var(--nova-success)]'
    : normalized === 'running'
      ? 'bg-[var(--nova-warning)] animate-pulse'
      : 'bg-[var(--nova-danger)]'
  return <span className={cn('size-1.5 shrink-0 rounded-full', tone)} />
}

function shortRunID(runID: string) {
  return runID.length <= 34 ? runID : `${runID.slice(0, 22)}…${runID.slice(-8)}`
}

function formatRunTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function errorMessage(cause: unknown) {
  return cause instanceof Error ? cause.message : String(cause || 'Unknown error')
}
