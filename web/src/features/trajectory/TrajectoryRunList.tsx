import { useEffect, useMemo, useState } from 'react'
import { ChevronRight, MessagesSquare } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { GlobalAgentRunTraceSummary } from '@/lib/api'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'

interface TrajectoryRunListProps {
  runs: GlobalAgentRunTraceSummary[]
  selectedRunURI: string
  onSelect: (trajectoryURI: string) => void
}

/** Global Run directory with lossless flat and Project-scoped Session views. */
export function TrajectoryRunList({ runs, selectedRunURI, onSelect }: TrajectoryRunListProps) {
  const { t } = useTranslation()
  const [organization, setOrganization] = useState<'session' | 'run'>('session')
  const sessionGroups = useMemo(() => groupRunsBySession(runs), [runs])
  const selectedRun = useMemo(
    () => runs.find((run) => run.trajectory_uri === selectedRunURI),
    [runs, selectedRunURI],
  )
  const selectedSessionKey = selectedRun ? sessionGroupKey(selectedRun) : ''
  const [expandedSessions, setExpandedSessions] = useState<Set<string>>(new Set())

  useEffect(() => {
    if (!selectedSessionKey) return
    setExpandedSessions((current) => {
      if (current.has(selectedSessionKey)) return current
      const next = new Set(current)
      next.add(selectedSessionKey)
      return next
    })
  }, [selectedSessionKey])

  return (
    <aside className="h-full min-h-0 overflow-auto bg-[var(--nova-surface-2)]" aria-label={t('trajectory.runs.title')}>
      <div className="sticky top-0 z-10 flex h-9 items-center border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">
        {t('trajectory.runs.title')}<span className="ml-1 font-mono text-[var(--nova-text-faint)]">{runs.length}</span>
        <div className="ml-auto flex items-center gap-0.5 rounded-md border border-[var(--nova-border)] p-0.5" role="group" aria-label={t('trajectory.runs.organization')}>
          <Button
            type="button"
            size="xs"
            variant={organization === 'run' ? 'secondary' : 'ghost'}
            className="h-5 px-1.5 text-[9px] font-medium normal-case tracking-normal"
            aria-pressed={organization === 'run'}
            onClick={() => setOrganization('run')}
          >
            {t('trajectory.runs.byRun')}
          </Button>
          <Button
            type="button"
            size="xs"
            variant={organization === 'session' ? 'secondary' : 'ghost'}
            className="h-5 px-1.5 text-[9px] font-medium normal-case tracking-normal"
            aria-pressed={organization === 'session'}
            onClick={() => setOrganization('session')}
          >
            {t('trajectory.runs.bySession')}
          </Button>
        </div>
      </div>
      <div className="space-y-1 overflow-x-hidden p-2">
        {organization === 'run'
          ? runs.map((run) => (
              <RunListItem key={run.trajectory_uri} run={run} selected={selectedRunURI === run.trajectory_uri} onSelect={onSelect} />
            ))
          : sessionGroups.map((group) => {
              const expanded = expandedSessions.has(group.key)
              const containsSelected = group.runs.some((run) => run.trajectory_uri === selectedRunURI)
              const sessionLabel = group.sessionID || t('trajectory.runs.noSession')
              const sessionTitle = group.sessionTitle || sessionLabel
              return (
                <div key={group.key} className="w-full">
                  <button
                    type="button"
                    className={cn(
                      'w-full rounded-[var(--nova-radius)] px-2 py-1.5 text-left transition-colors hover:bg-[var(--nova-surface)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none',
                      containsSelected && !expanded && 'bg-[var(--nova-active)]',
                    )}
                    aria-expanded={expanded}
                    aria-label={t('trajectory.runs.sessionToggle', {
                      project: group.projectName,
                      session: group.sessionTitle ? `${sessionTitle} · ${sessionLabel}` : sessionLabel,
                      count: group.runs.length,
                    })}
                    onClick={() => setExpandedSessions((current) => {
                      const next = new Set(current)
                      if (expanded) next.delete(group.key)
                      else next.add(group.key)
                      return next
                    })}
                  >
                    <span className="flex min-w-0 items-center gap-1.5">
                      <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-text-faint)] transition-transform', expanded && 'rotate-90')} />
                      <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">{group.projectName}</span>
                      <span className="shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{t('trajectory.runs.sessionCount', { count: group.runs.length })}</span>
                    </span>
                    <span className="mt-0.5 flex min-w-0 items-center gap-1.5 pl-[18px]">
                      <MessagesSquare className="size-3 shrink-0 text-[var(--nova-text-faint)]" />
                      <span className="min-w-0 flex-1 truncate text-[10px] font-medium text-[var(--nova-text-muted)]" title={sessionTitle}>{sessionTitle}</span>
                    </span>
                    {group.sessionTitle && group.sessionID ? (
                      <span className="mt-0.5 block truncate pl-[34px] font-mono text-[8px] text-[var(--nova-text-faint)]" title={group.sessionID}>{group.sessionID}</span>
                    ) : null}
                  </button>
                  {expanded ? (
                    <div className="ml-3 mt-1 space-y-1 border-l border-[var(--nova-border)] pl-1.5">
                      {group.runs.map((run) => (
                        <RunListItem key={run.trajectory_uri} run={run} selected={selectedRunURI === run.trajectory_uri} onSelect={onSelect} grouped />
                      ))}
                    </div>
                  ) : null}
                </div>
              )
            })}
      </div>
    </aside>
  )
}

function RunListItem({
  run,
  selected,
  grouped = false,
  onSelect,
}: {
  run: GlobalAgentRunTraceSummary
  selected: boolean
  grouped?: boolean
  onSelect: (trajectoryURI: string) => void
}) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      onClick={() => onSelect(run.trajectory_uri)}
      aria-current={selected ? 'true' : undefined}
      className={cn(
        'w-full rounded-[var(--nova-radius)] border border-transparent px-2.5 py-2 text-left transition-colors focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none',
        selected ? 'bg-[var(--nova-active)]' : 'hover:bg-[var(--nova-surface)]',
      )}
    >
      <span className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">
          {grouped ? run.agent_kind || t('trajectory.runs.agent') : run.project_name}
        </span>
        <StatusDot status={run.status} />
        <span className="shrink-0 font-mono text-[9px] uppercase text-[var(--nova-text-faint)]">{run.status}</span>
      </span>
      <span className="mt-1 flex min-w-0 items-center gap-2">
        <span className="min-w-0 flex-1 truncate text-[10px] text-[var(--nova-text-muted)]">
          {grouped ? shortRunID(run.id) : run.agent_kind || t('trajectory.runs.agent')}
        </span>
        {grouped ? null : <span className="shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{shortRunID(run.id)}</span>}
      </span>
      <span className="mt-1 flex items-center gap-2 text-[9px] text-[var(--nova-text-faint)]">
        <span>{formatRunTime(run.created_at)}</span>
        <span>{formatTrajectoryDuration(run.duration_ms)}</span>
        <span>{t('trajectory.runs.calls', { models: run.llm_calls ?? 0, tools: run.tool_calls ?? 0 })}</span>
        <span className={run.content_captured ? 'text-[var(--nova-success)]' : ''}>{t(run.content_captured ? 'trajectory.runs.content' : 'trajectory.runs.metadata')}</span>
      </span>
    </button>
  )
}

interface SessionRunGroup {
  key: string
  projectName: string
  sessionID: string
  sessionTitle: string
  runs: GlobalAgentRunTraceSummary[]
}

function groupRunsBySession(runs: GlobalAgentRunTraceSummary[]): SessionRunGroup[] {
  const groups = new Map<string, SessionRunGroup>()
  const sortedRuns = [...runs].sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at))
  for (const run of sortedRuns) {
    const key = sessionGroupKey(run)
    const existing = groups.get(key)
    if (existing) {
      if (!existing.sessionTitle && run.session_title?.trim()) {
        existing.sessionTitle = run.session_title.trim()
      }
      existing.runs.push(run)
      continue
    }
    groups.set(key, {
      key,
      projectName: run.project_name,
      sessionID: run.session_id?.trim() ?? '',
      sessionTitle: run.session_title?.trim() ?? '',
      runs: [run],
    })
  }
  return [...groups.values()]
}

function sessionGroupKey(run: GlobalAgentRunTraceSummary) {
  const sessionID = run.session_id?.trim()
  return sessionID
    ? `${run.project_id}\u0000${sessionID}`
    : `${run.project_id}\u0000run\u0000${run.trajectory_uri}`
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
  return runID.length <= 28 ? runID : `${runID.slice(0, 18)}…${runID.slice(-7)}`
}

function formatRunTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}
