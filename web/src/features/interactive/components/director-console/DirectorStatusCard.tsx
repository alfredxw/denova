import { Activity, AlertCircle, CheckCircle2, Clock3, FileText, Loader2, RefreshCw, ScrollText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { DirectorPlanMetadata } from '../../types'
import type { DirectorStatusLike } from './types'
import { directorPlanTotals, directorStatusFallback, directorStatusLabel, formatBytes, formatShortDate } from './utils'

// 导演运行状态卡：合并原「规划」公开摘要与「后台」运行状态，导演 tab 的常驻概览。
export function DirectorStatusCard({ storyId, hasDirectorRun, status, metadata, loading, running, analyzing, canAnalyze, error, onRun, onAnalyze }: {
  storyId?: string
  hasDirectorRun: boolean
  status?: DirectorStatusLike
  metadata?: DirectorPlanMetadata
  loading: boolean
  running: boolean
  analyzing: boolean
  canAnalyze: boolean
  error?: string
  onRun: () => void
  onAnalyze: () => void
}) {
  const { t } = useTranslation()
  const currentStatus = loading && !status?.status ? 'loading' : status?.status || ''
  const statusRunning = currentStatus === 'running' || currentStatus === 'loading'
  const failed = currentStatus === 'failed'
  const totals = directorPlanTotals(status, metadata)
  const summary = error || status?.error || status?.summary || directorStatusFallback(currentStatus, t)
  const updatedAt = status?.updated_at || metadata?.updated_at || ''
  const statusIcon = failed ? <AlertCircle className="h-4 w-4" /> : statusRunning ? <Loader2 className="h-4 w-4 animate-spin" /> : <CheckCircle2 className="h-4 w-4" />

  return (
    <section data-testid="director-run-summary" className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
      <div className={`h-0.5 w-full ${statusRunning ? 'animate-pulse bg-[var(--director-brass)]' : failed ? 'bg-[var(--nova-danger)]' : 'bg-[var(--director-live)]'}`} />
      <div className="space-y-3 p-3">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border bg-[var(--nova-surface)] ${failed ? 'border-[var(--nova-danger-border)] text-[var(--nova-danger)]' : statusRunning ? 'border-[var(--nova-accent-blue)]/40 text-[var(--nova-accent-blue)]' : 'border-[var(--nova-border)] text-[var(--nova-accent-green)]'}`}>
              {statusIcon}
            </span>
            <div className="min-w-0">
              <h3 className="director-console__display truncate text-sm font-semibold text-[var(--nova-text)]">{t('directorPanel.run.statusTitle')}</h3>
              <p className="mt-0.5 truncate text-[9px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{directorStatusLabel(status, loading, t)}</p>
            </div>
          </div>
          {updatedAt ? (
            <span className="shrink-0 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {formatShortDate(updatedAt)}
            </span>
          ) : null}
        </div>

        <p className="break-words text-xs leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{summary}</p>
        {!hasDirectorRun ? <p className="text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('directorPanel.directorManualRunHint')}</p> : null}

        {status?.decision?.mode ? (
          <div className="flex flex-wrap items-center gap-1.5 text-[10px] text-[var(--nova-text-muted)]">
            <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 font-medium text-[var(--nova-text)]">
              {t(`directorPanel.planDecision.${status.decision.mode}`, { defaultValue: status.decision.mode })}
            </span>
            {status.decision.triggers?.slice(0, 3).map((trigger) => (
              <span key={trigger} className="rounded-full bg-[var(--nova-hover)] px-2 py-0.5">{trigger}</span>
            ))}
          </div>
        ) : null}

        <div className="grid grid-cols-3 gap-px overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-border)]">
          <StatusMetric icon={<FileText className="h-3.5 w-3.5" />} label={t('snapshot.director.docs')} value={`${totals.completed}/${totals.planned}`} />
          <StatusMetric icon={<Clock3 className="h-3.5 w-3.5" />} label={t('snapshot.director.branchPlanningTurns')} value={String(metadata?.branch_planning_turns || 5)} />
          <StatusMetric icon={<Activity className="h-3.5 w-3.5" />} label={t('directorPanel.run.visibleBytes')} value={`${formatBytes(totals.visibleBytes)} / ${formatBytes(totals.totalBytes)}`} />
        </div>

        <div className="grid grid-cols-2 gap-2">
          <Button type="button" variant="outline" size="sm" className="min-w-0 justify-start gap-2 rounded-[9px] border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45" disabled={!storyId || running} onClick={onRun}>
            {running ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            <span className="min-w-0 truncate">{running ? t('directorPanel.directorManualRunning') : t('directorPanel.directorManualRun')}</span>
          </Button>
          <Button type="button" variant="outline" size="sm" className="min-w-0 justify-start gap-2 rounded-[9px] border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:border-[var(--director-brass)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45" disabled={!canAnalyze || analyzing} onClick={onAnalyze}>
            {analyzing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <ScrollText className="h-3.5 w-3.5" />}
            <span className="min-w-0 truncate">{analyzing ? t('chat.contextAnalysis.loading') : t('directorPanel.directorContextAnalysis')}</span>
          </Button>
        </div>
      </div>
    </section>
  )
}

function StatusMetric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className="min-w-0 bg-[var(--nova-surface)] px-2.5 py-2">
      <div className="flex min-w-0 items-center gap-1.5 text-[10px] text-[var(--nova-text-faint)]">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <div className="mt-1 truncate text-xs font-medium text-[var(--nova-text)]" title={value}>{value}</div>
    </div>
  )
}
