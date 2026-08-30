import { useEffect, useMemo, useState } from 'react'
import { Bot, ChevronRight, FileText, MessageCircle, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { formatDateTime } from '@/i18n'
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
  const projectGroups = useMemo(() => groupRunsByProject(runs), [runs])
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
      <div className="overflow-x-hidden px-2 pb-2">
        {organization === 'run'
          ? runs.map((run) => (
              <RunListItem key={run.trajectory_uri} run={run} selected={selectedRunURI === run.trajectory_uri} onSelect={onSelect} />
            ))
          : projectGroups.map((project, projectIndex) => (
              <section
                key={project.key}
                className={cn('pb-1 pt-2', projectIndex > 0 && 'border-t border-[var(--nova-border-soft)]')}
                aria-label={project.name}
              >
                <div className="flex min-w-0 items-center gap-2 px-2 pb-1">
                  <span className="min-w-0 flex-1 truncate text-[11px] font-semibold text-[var(--nova-text)]" title={project.name}>{project.name}</span>
                  <span className="shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{t('trajectory.runs.sessionCount', { count: project.runCount })}</span>
                </div>
                <div className="space-y-0.5">
                  {project.sessions.map((session) => (
                    <SessionRunListItem
                      key={session.key}
                      session={session}
                      projectName={project.name}
                      expanded={expandedSessions.has(session.key)}
                      containsSelected={session.runs.some((run) => run.trajectory_uri === selectedRunURI)}
                      selectedRunURI={selectedRunURI}
                      onToggle={() => setExpandedSessions((current) => {
                        const next = new Set(current)
                        if (next.has(session.key)) next.delete(session.key)
                        else next.add(session.key)
                        return next
                      })}
                      onSelect={onSelect}
                    />
                  ))}
                </div>
              </section>
            ))}
      </div>
    </aside>
  )
}

function SessionRunListItem({
  session,
  projectName,
  expanded,
  containsSelected,
  selectedRunURI,
  onToggle,
  onSelect,
}: {
  session: SessionRunGroup
  projectName: string
  expanded: boolean
  containsSelected: boolean
  selectedRunURI: string
  onToggle: () => void
  onSelect: (trajectoryURI: string) => void
}) {
  const { t } = useTranslation()
  const sessionLabel = session.sessionID || t('trajectory.runs.noSession')
  const sessionTitle = session.sessionTitle || sessionLabel
  const fullTitle = session.sessionTitle && session.sessionID ? `${sessionTitle} · ${sessionLabel}` : sessionTitle

  return (
    <div className="w-full">
      <button
        type="button"
        className={cn(
          'flex w-full min-w-0 items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1 text-left transition-colors hover:bg-[var(--nova-surface)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none',
          containsSelected && !expanded && 'bg-[var(--nova-active)]',
        )}
        aria-expanded={expanded}
        aria-label={t('trajectory.runs.sessionToggle', {
          project: projectName,
          session: fullTitle,
          count: session.runs.length,
        })}
        onClick={onToggle}
      >
        <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-text-faint)] transition-transform', expanded && 'rotate-90')} />
        <MessageCircle className="size-3 shrink-0 text-[var(--nova-text-faint)]" />
        <span className="min-w-0 flex-1 truncate text-[10px] font-medium text-[var(--nova-text-muted)]" title={fullTitle}>{sessionTitle}</span>
        <span className="shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{t('trajectory.runs.sessionCount', { count: session.runs.length })}</span>
      </button>
      {expanded ? (
        <div className="ml-3 mt-0.5 space-y-0.5 border-l border-[var(--nova-border)] pl-1.5">
          {session.runs.map((run) => (
            <RunListItem key={run.trajectory_uri} run={run} selected={selectedRunURI === run.trajectory_uri} onSelect={onSelect} grouped />
          ))}
        </div>
      ) : null}
    </div>
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
      aria-label={`${grouped ? run.agent_kind || t('trajectory.runs.agent') : run.project_name} · ${run.status} · ${formatDateTime(run.created_at)} · ${run.id}`}
      className={cn(
        'w-full rounded-[var(--nova-radius)] border border-transparent px-2 py-1.5 text-left transition-colors focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none',
        selected ? 'bg-[var(--nova-active)]' : 'hover:bg-[var(--nova-surface)]',
      )}
    >
      <span className="flex min-w-0 items-center gap-1.5">
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">
          {grouped ? run.agent_kind || t('trajectory.runs.agent') : run.project_name}
        </span>
        <StatusDot status={run.status} />
        <span className="shrink-0 font-mono text-[9px] uppercase text-[var(--nova-text-faint)]">{run.status}</span>
      </span>
      <span className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[9px] text-[var(--nova-text-faint)]">
        {!grouped ? <span className="min-w-0 truncate text-[var(--nova-text-muted)]">{run.agent_kind || t('trajectory.runs.agent')}</span> : null}
        <span className="shrink-0 whitespace-nowrap font-mono">{formatDateTime(run.created_at) || '—'}</span>
        <span className="shrink-0 whitespace-nowrap">{formatTrajectoryDuration(run.duration_ms)}</span>
        <span
          className="flex shrink-0 items-center gap-1 whitespace-nowrap"
          title={t('trajectory.runs.calls', { models: run.llm_calls ?? 0, tools: run.tool_calls ?? 0 })}
          aria-label={t('trajectory.runs.calls', { models: run.llm_calls ?? 0, tools: run.tool_calls ?? 0 })}
        >
          <Bot className="size-2.5" aria-hidden="true" /><span aria-hidden="true">{run.llm_calls ?? 0}</span>
          <Wrench className="ml-0.5 size-2.5" aria-hidden="true" /><span aria-hidden="true">{run.tool_calls ?? 0}</span>
        </span>
        <span
          className={cn('shrink-0', run.content_captured && 'text-[var(--nova-success)]')}
          role="img"
          aria-label={t(run.content_captured ? 'trajectory.runs.content' : 'trajectory.runs.metadata')}
          title={t(run.content_captured ? 'trajectory.runs.content' : 'trajectory.runs.metadata')}
        >
          <FileText className="size-2.5" aria-hidden="true" />
        </span>
      </span>
    </button>
  )
}

interface ProjectRunGroup {
  key: string
  name: string
  runCount: number
  sessions: SessionRunGroup[]
}

interface SessionRunGroup {
  key: string
  sessionID: string
  sessionTitle: string
  runs: GlobalAgentRunTraceSummary[]
}

function groupRunsByProject(runs: GlobalAgentRunTraceSummary[]): ProjectRunGroup[] {
  const projects = new Map<string, ProjectRunGroup>()
  const sortedRuns = [...runs].sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at))
  for (const run of sortedRuns) {
    let project = projects.get(run.project_id)
    if (!project) {
      project = {
        key: run.project_id,
        name: run.project_name,
        runCount: 0,
        sessions: [],
      }
      projects.set(run.project_id, project)
    }
    project.runCount += 1

    const key = sessionGroupKey(run)
    const session = project.sessions.find((candidate) => candidate.key === key)
    if (session) {
      if (!session.sessionTitle && run.session_title?.trim()) session.sessionTitle = run.session_title.trim()
      session.runs.push(run)
    } else {
      project.sessions.push({
        key,
        sessionID: run.session_id?.trim() ?? '',
        sessionTitle: run.session_title?.trim() ?? '',
        runs: [run],
      })
    }
  }
  return [...projects.values()]
}

function sessionGroupKey(run: GlobalAgentRunTraceSummary) {
  const sessionID = run.session_id?.trim()
  return sessionID
    ? `${run.project_id}\u0000${sessionID}`
    : `${run.project_id}\u0000no-session`
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
